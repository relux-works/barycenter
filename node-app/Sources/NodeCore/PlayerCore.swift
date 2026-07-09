// PlayerCore: executes coordinator commands on this node (spec 6.2 item 5,
// mechanics 6.3): two-step load (play paused + seek), resume_at via T_local,
// pause with fade, voice inserts, wait, offset_test clicks, audible_position
// bookkeeping and the ended-after-ring-drain rule.

import AVFAudio
import Foundation

public final class PlayerCore {
    public enum Playback: String {
        case stopped, loading, paused, playing, voice, wait
    }

    private let engine: AudioEngine
    private let librespot: LibrespotClient
    private let supervisor: LibrespotSupervisor
    private let cache: VoiceCache
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.player-core")

    // Coordinator link is set after init (mutual wiring).
    public weak var coordinator: CoordinatorClient?

    /// MenuStatus is a thread-safe snapshot for the menu bar (R2).
    public struct MenuStatus {
        public let mode: String
        public let playback: String
        public let uri: String?
        public let volume: Int
    }

    public func menuStatus() -> MenuStatus {
        queue.sync {
            MenuStatus(mode: mode, playback: "\(playback)", uri: currentURI, volume: volume)
        }
    }

    public private(set) var mode = "shared"
    public private(set) var playback = Playback.stopped
    public private(set) var currentElementID: String?
    public private(set) var currentURI: String?
    public private(set) var volume = 80
    public var outputLatencyOffsetMs: Int

    // Position tracking: anchor from librespot events/status + wall-clock extrapolation.
    private var anchorPositionMs: Int64 = 0
    private var anchorAt = Date()
    private var extrapolate = false

    // AirfoilBridge feed for the heartbeat (spec 6.5).
    private var speakerStates: [SpeakerState] = []
    private var airfoilDegraded = false

    // ended-after-drain (spec 6.3 item 5).
    private var draining = false
    // Debounce duplicate metadata/playing reports for one Spotify selection.
    private var lastExternalReport = Date.distantPast
    private var resumeTimer: DispatchSourceTimer?
    private var waitTimer: DispatchSourceTimer?
    private var drainTimer: DispatchSourceTimer?
    private var lastStarvationReport = Date.distantPast

    public init(engine: AudioEngine, librespot: LibrespotClient, supervisor: LibrespotSupervisor,
                cache: VoiceCache, outputLatencyOffsetMs: Int, log: Logger) {
        self.engine = engine
        self.librespot = librespot
        self.supervisor = supervisor
        self.cache = cache
        self.outputLatencyOffsetMs = outputLatencyOffsetMs
        self.log = log

        engine.onFirstMusicSample = { [weak self] _ in
            self?.queue.async { self?.reportStarted() }
        }
        librespot.onEvent = { [weak self] event in
            self?.queue.async { self?.handleLibrespotEvent(event) }
        }
        supervisor.onCrash = { [weak self] in
            self?.queue.async {
                self?.sendError(code: "librespot_restart", message: "daemon exited, supervisor restarting")
            }
        }
        startPositionPolling()
        startDrainWatch()
        startStarvationWatch()
    }

    private func nowMs() -> Int64 { Int64((Date().timeIntervalSince1970 * 1000).rounded()) }

    // MARK: Position (spec 6.3: audible_position = librespot_position - ring_fill)

    public var audiblePositionMs: Int64 {
        var pos = anchorPositionMs
        if extrapolate {
            pos += Int64(Date().timeIntervalSince(anchorAt) * 1000)
        }
        return max(0, pos - engine.ringFillMs)
    }

    private func setAnchor(_ positionMs: Int64, extrapolating: Bool) {
        anchorPositionMs = positionMs
        anchorAt = Date()
        extrapolate = extrapolating
    }

    private func startPositionPolling() {
        let t = DispatchSource.makeTimerSource(queue: queue)
        t.schedule(deadline: .now() + 2, repeating: 2)
        t.setEventHandler { [weak self] in
            guard let self, self.playback == .playing || self.playback == .paused else { return }
            Task {
                guard let st = try? await self.librespot.status(), let track = st.track,
                      let pos = track.position else { return }
                self.queue.async {
                    self.setAnchor(pos, extrapolating: self.playback == .playing)
                    if self.currentURI == nil { self.currentURI = track.uri }
                }
            }
        }
        t.resume()
        positionTimer = t
    }
    private var positionTimer: DispatchSourceTimer?

    // MARK: Commands (spec 8.3)

    public func handle(_ head: EnvelopeHead, _ message: Message) {
        queue.async {
            switch message {
            case .load(let p): self.load(p)
            case .resumeAt(let p): self.resumeAt(p)
            case .pause(let p): self.pauseCmd(p)
            case .seek(let p): self.seekCmd(p)
            case .playVoice(let p): self.playVoice(p)
            case .wait(let p): self.waitCmd(p)
            case .setVolume(let p): self.setVolume(p.volume)
            case .setMode(let p): self.setMode(p.mode)
            case .stop: self.stopAll()
            case .setOffset(let p): self.setOffset(p)
            case .offsetTest(let p): self.offsetTest(p)
            case .soloInject(let p): self.soloInject(p)
            case .soloVoice(let p):
                // Phase 2 (goal §8): boundary interception lands with solo scope.
                self.log.warn("solo_voice not implemented until phase 2", ["element": p.elementId])
            case .welcome, .pong, .register, .state, .ready, .started, .ended,
                 .voiceStarted, .voiceEnded, .waitEnded, .error, .ping, .externalPlayback,
                 .setProvider:
                self.log.debug("ignoring non-command", ["type": head.type])
            }
        }
    }

    private func load(_ p: LoadPayload) {
        cancelTimers()
        draining = false
        playback = .loading
        currentElementID = p.elementId
        currentURI = p.uri
        engine.clearRing()
        engine.setMusicGain(1, fadeMs: 0)
        engine.expectingMusic = false
        setAnchor(p.positionMs, extrapolating: false)

        Task {
            do {
                // The daemon needs seconds after (re)start to authenticate;
                // a load racing that window must wait, not fail as
                // "track unavailable" (R0 finding, prod 2026-07-05).
                for _ in 0..<20 where !(await self.librespot.playbackReady()) {
                    try await Task.sleep(nanoseconds: 500_000_000)
                }
                do {
                    try await self.librespot.playPaused(uri: p.uri)
                } catch {
                    // One local retry: transient daemon stalls (transfer storms)
                    // must not surface as track_unavailable.
                    try await Task.sleep(nanoseconds: 2_000_000_000)
                    try await self.librespot.playPaused(uri: p.uri)
                }
                if p.positionMs > 0 {
                    try await self.librespot.seek(positionMS: p.positionMs)
                }
                try await self.confirmPausedLoaded(uri: p.uri)
                self.queue.async {
                    guard self.currentElementID == p.elementId else { return } // stale
                    self.playback = .paused
                    self.coordinator?.sendMessage(.ready(ReadyPayload(elementId: p.elementId)))
                }
            } catch {
                self.queue.async {
                    guard self.currentElementID == p.elementId else { return }
                    self.playback = .stopped
                    self.sendError(code: "load_failed", message: "\(error)", elementId: p.elementId)
                }
            }
        }
    }

    /// Polls /status until the daemon confirms the paused-loaded state
    /// (spec 6.3 load step 1; live confirmation of seek-while-paused is a
    /// spike-remainder item — this poll is written to tolerate both outcomes).
    private func confirmPausedLoaded(uri: String, attempts: Int = 10) async throws {
        for _ in 0..<attempts {
            if let st = try? await librespot.status(),
               st.paused == true || st.buffering == true,
               st.track?.uri == uri || st.track == nil {
                return
            }
            try await Task.sleep(nanoseconds: 300_000_000)
        }
        throw NSError(domain: "player", code: 1,
                      userInfo: [NSLocalizedDescriptionKey: "daemon did not confirm paused load of \(uri)"])
    }

    private func resumeAt(_ p: ResumeAtPayload) {
        guard p.elementId == currentElementID else { return } // idempotency (spec 7.2)
        guard let clock = coordinator?.clock,
              let tLocal = clock.localDeadline(forCoordinatorMs: p.tCoordMs,
                                               outputLatencyOffsetMs: outputLatencyOffsetMs) else {
            log.warn("resume_at without clock sync, starting immediately", ["element": p.elementId])
            fireResume(elementId: p.elementId)
            return
        }
        let delayMs = tLocal - nowMs()
        let t = DispatchSource.makeTimerSource(flags: .strict, queue: queue)
        t.schedule(deadline: .now() + .milliseconds(Int(max(0, delayMs))), leeway: .milliseconds(1))
        t.setEventHandler { [weak self] in
            self?.fireResume(elementId: p.elementId)
            self?.resumeTimer = nil
        }
        t.resume()
        resumeTimer = t
        log.info("resume armed", ["element": p.elementId, "in_ms": delayMs])
    }

    private func fireResume(elementId: String) {
        guard elementId == currentElementID else { return }
        engine.armStartDetection()
        engine.setMusicGain(1, fadeMs: 0)
        playback = .playing
        engine.expectingMusic = true
        setAnchor(anchorPositionMs, extrapolating: true)
        Task { try? await self.librespot.resume() }
    }

    private func reportStarted() {
        guard let el = currentElementID, playback == .playing else { return }
        let tLocal = nowMs()
        var tCoord = tLocal
        if let off = coordinator?.clock.offsetMs {
            tCoord = tLocal - Int64(off.rounded()) // node = coord + offset (spec 8.5)
        }
        coordinator?.sendMessage(.started(StartedPayload(elementId: el, tFirstSampleCoordMs: tCoord)))
    }

    private func pauseCmd(_ p: PausePayload) {
        guard p.elementId == currentElementID || p.elementId.isEmpty else { return }
        cancelTimers()
        engine.setMusicGain(0, fadeMs: p.fadeMs)
        playback = .paused
        engine.expectingMusic = false
        extrapolate = false
        queue.asyncAfter(deadline: .now() + .milliseconds(Int(p.fadeMs) + 20)) {
            Task { try? await self.librespot.pause() }
        }
    }

    private func seekCmd(_ p: SeekPayload) {
        guard p.elementId == currentElementID else { return }
        engine.clearRing()
        setAnchor(p.positionMs, extrapolating: playback == .playing)
        Task { try? await self.librespot.seek(positionMS: p.positionMs) }
    }

    private func playVoice(_ p: PlayVoicePayload) {
        currentElementID = p.elementId
        playback = .voice
        Task {
            do {
                let file = try await self.cache.fetch(fileURL: p.fileUrl)
                var when: AVAudioTime?
                if let tCoord = p.tCoordMs, let clock = self.coordinator?.clock,
                   let tLocal = clock.localDeadline(forCoordinatorMs: tCoord,
                                                    outputLatencyOffsetMs: self.outputLatencyOffsetMs) {
                    when = AVAudioTime(hostTime: HostClock.hostTime(forUnixMs: tLocal))
                }
                self.coordinator?.sendMessage(.voiceStarted(VoiceStartedPayload(elementId: p.elementId)))
                try self.engine.playInsert(fileURL: file, at: when) {
                    self.queue.async {
                        self.playback = .stopped
                        self.coordinator?.sendMessage(.voiceEnded(VoiceEndedPayload(elementId: p.elementId)))
                    }
                }
            } catch {
                self.queue.async {
                    self.sendError(code: "media_download_failed", message: "\(error)", elementId: p.elementId)
                }
            }
        }
    }

    private func waitCmd(_ p: WaitPayload) {
        currentElementID = p.elementId
        playback = .wait
        let t = DispatchSource.makeTimerSource(queue: queue)
        t.schedule(deadline: .now() + .milliseconds(Int(p.durationMs)))
        t.setEventHandler { [weak self] in
            guard let self else { return }
            self.playback = .stopped
            self.coordinator?.sendMessage(.waitEnded(WaitEndedPayload(elementId: p.elementId)))
            self.waitTimer = nil
        }
        t.resume()
        waitTimer = t
    }

    public func setVolume(_ v: Int) {
        volume = v
        engine.setVolume(v)
    }

    private func setMode(_ m: String) {
        mode = m
        log.info("mode set", ["mode": m])
    }

    private func stopAll() {
        cancelTimers()
        draining = false
        currentElementID = nil
        currentURI = nil
        playback = .stopped
        engine.expectingMusic = false
        // Mode switches yank someone's music away (spec 4.3) — land it softly:
        // raised-cosine fade out, then stop the daemon and drop the tail.
        engine.setMusicGain(0, fadeMs: 250)
        queue.asyncAfter(deadline: .now() + .milliseconds(300)) {
            self.engine.clearRing()
            self.engine.setMusicGain(1, fadeMs: 0) // ready for the next element
        }
        Task { try? await self.librespot.stop() }
    }

    private func setOffset(_ p: SetOffsetPayload) {
        outputLatencyOffsetMs = Int(p.offsetMs)
        log.info("offset set", ["offset_ms": p.offsetMs])
    }

    private func offsetTest(_ p: OffsetTestPayload) {
        guard let clock = coordinator?.clock,
              let tLocal = clock.localDeadline(forCoordinatorMs: p.tCoordMs,
                                               outputLatencyOffsetMs: outputLatencyOffsetMs) else {
            log.warn("offset_test without clock sync, skipping")
            return
        }
        engine.playClicks(count: p.clicks,
                          firstAtHostTime: HostClock.hostTime(forUnixMs: tLocal),
                          intervalMs: p.intervalMs)
    }

    private func soloInject(_ p: SoloInjectPayload) {
        Task { try? await self.librespot.addToQueue(uri: p.uri) }
    }

    // MARK: librespot events

    /// In shared mode, a Spotify selection on this Pulsar becomes a leader
    /// event. Barycenter adopts it at this node's audible position and runs
    /// the normal synchronized load barrier across all homes.
    private func reportExternalSelection(
        _ uri: String?, observedPosition: Int64? = nil, allowSameURI: Bool = false
    ) {
        guard SpotifySelection.shouldReport(
            mode: mode, uri: uri, expectedURI: currentURI,
            playback: playback, allowSameURI: allowSameURI
        ), let uri else { return }
        guard Date().timeIntervalSince(lastExternalReport) > 5 else { return }
        lastExternalReport = Date()
        let positionMs = SpotifySelection.startPosition(
            observedPosition: observedPosition, uri: uri,
            expectedURI: currentURI, audiblePosition: audiblePositionMs)
        log.warn("external playback detected in shared", ["uri": uri, "expected": currentURI ?? "silence"])
        coordinator?.sendMessage(.externalPlayback(
            ExternalPlaybackPayload(uri: uri, positionMs: positionMs)))
    }

    private func handleLibrespotEvent(_ event: LibrespotEvent) {
        switch event {
        case .metadata(let uri, _, let position, _):
            if let position { setAnchor(position, extrapolating: playback == .playing) }
            reportExternalSelection(uri, observedPosition: position)
            if mode != "shared", let uri { currentURI = uri }
        case .seek(let position, _):
            if let position { setAnchor(position, extrapolating: playback == .playing) }
        case .notPlaying, .stopped:
            // Track over at the daemon: the ring tail must finish sounding
            // before we report ended (spec 6.3 item 5).
            if playback == .playing, currentElementID != nil {
                draining = true
                extrapolate = false
            }
        case .paused:
            // Solo UX (spike S4): the daemon stalls the pipe instantly but the
            // ring holds up to 1 s of tail — fade fast so the pause FEELS
            // instant. The ring is NOT cleared: on resume the tail plays first
            // and nothing is lost.
            if mode == "solo" || playback == .playing {
                engine.setMusicGain(0, fadeMs: 250)
                extrapolate = false
            }
        case .playing(let uri):
            // fireResume marks .playing before the daemon resumes. A matching
            // event while stopped/paused therefore means the user selected it.
            reportExternalSelection(uri, allowSameURI: true)
            engine.setMusicGain(1, fadeMs: 120)
            if playback == .playing { setAnchor(anchorPositionMs, extrapolating: true) }
        case .volume(let value, let max):
            // external_volume: true hands volume to our mixer (spec A.2/6.3);
            // the phone's Spotify Connect slider arrives as this event —
            // apply it so solo volume control works naturally. Last writer
            // (phone or coordinator /vol) wins; heartbeat reports the result.
            if let value, let max, max > 0 {
                setVolume(Int((Double(value) / Double(max) * 100).rounded()))
            }
        default:
            break
        }
    }

    private func startDrainWatch() {
        let t = DispatchSource.makeTimerSource(queue: queue)
        t.schedule(deadline: .now() + 0.1, repeating: 0.1)
        t.setEventHandler { [weak self] in
            guard let self, self.draining, let el = self.currentElementID else { return }
            if self.engine.ringFillMs == 0 {
                self.draining = false
                self.playback = .stopped
                self.engine.expectingMusic = false
                self.coordinator?.sendMessage(.ended(EndedPayload(elementId: el, reason: "eof")))
            }
        }
        t.resume()
        drainTimer = t
    }

    // Underruns while music is expected > ~3 s -> audio_starvation + soft
    // restart (spec 6.6); gated on expectingMusic (UNRESOLVED R4).
    private var lastLoggedUnderruns: Int64 = 0
    private var lastLoggedFed: Int64 = 0

    private func startStarvationWatch() {
        let t = DispatchSource.makeTimerSource(queue: queue)
        t.schedule(deadline: .now() + 1, repeating: 1)
        t.setEventHandler { [weak self] in
            guard let self else { return }
            // Dropout telemetry: fed AND starved callbacks within the same
            // second = audible glitch (idle silence is starved-only).
            let u = self.engine.underrunCallbacks
            let f = self.engine.fedCallbacks
            let uDelta = u - self.lastLoggedUnderruns
            let fDelta = f - self.lastLoggedFed
            if uDelta > 0 && fDelta > 0 {
                self.log.warn("audible dropout", [
                    "starved_cbs": uDelta,
                    "fed_cbs": fDelta,
                    "ring_fill_ms": self.engine.ringFillMs,
                ])
            }
            self.lastLoggedUnderruns = u
            self.lastLoggedFed = f
            // Render callbacks run every ~10 ms; ~300 starved in a row ~= 3 s.
            if self.engine.expectingMusic, self.engine.starvedCallbacksStreak > 300,
               Date().timeIntervalSince(self.lastStarvationReport) > 10 {
                self.lastStarvationReport = Date()
                self.sendError(code: "audio_starvation", message: "no samples for >3s while playing")
                self.supervisor.softRestart()
            }
        }
        t.resume()
        starvationTimer = t
    }
    private var starvationTimer: DispatchSourceTimer?

    private func cancelTimers() {
        resumeTimer?.cancel()
        resumeTimer = nil
        waitTimer?.cancel()
        waitTimer = nil
    }

    private func sendError(code: String, message: String, elementId: String? = nil) {
        log.error("node error", ["code": code, "msg": message])
        coordinator?.sendMessage(.error(ErrorPayload(code: code, message: message, elementId: elementId)))
    }

    // MARK: Heartbeat snapshot (spec 8.4 state)

    /// AirfoilBridge pushes live speaker states here (spec 6.2 item 8).
    public func updateSpeakers(_ states: [SpeakerState], degraded: Bool) {
        queue.async {
            self.speakerStates = states
            self.airfoilDegraded = degraded
        }
    }

    public func statePayload(fallbackSpeakers: [SpeakerState], rttMs: Int64) -> StatePayload {
        StatePayload(
            playback: playback.rawValue,
            uri: currentURI,
            positionMs: audiblePositionMs,
            volume: volume,
            degraded: airfoilDegraded,
            underruns: engine.underrunCallbacks,
            rttMs: rttMs,
            speakers: speakerStates.isEmpty ? fallbackSpeakers : speakerStates
        )
    }
}
