// PlayerCore: executes coordinator commands on this node (spec 6.2 item 5,
// mechanics 6.3): two-step load (play paused + seek), resume_at via T_local,
// pause with fade, voice inserts, wait, offset_test clicks, audible_position
// bookkeeping and the ended-after-ring-drain rule.

import AVFAudio
import Foundation

public final class PlayerCore: MacInterruptControlling {
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
    public weak var coordinator: CoordinatorClient? {
        didSet {
            guard let mediaClips, let coordinator else { return }
            mediaClips.bind(
                send: { [weak coordinator] message in coordinator?.sendMessage(message) },
                clock: { [weak coordinator] in coordinator?.clockSnapshot() },
                outputLatencyOffsetMs: outputLatencyOffsetMs)
        }
    }

    private var mediaClips: MediaClipClient?
    private var presenceStore: NodePresenceStore?

    /// MenuStatus is a thread-safe snapshot for the menu bar (R2).
    public struct MenuStatus {
        public let mode: String
        public let playback: String
        public let uri: String?
        public let title: String?
        public let volume: Int
    }

    public func menuStatus() -> MenuStatus {
        queue.sync {
            MenuStatus(
                mode: mode,
                playback: "\(playback)",
                uri: currentURI,
                title: metadataTitle,
                volume: volume)
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
    private var lastExternalURI: String?
    private var metadataURI: String?
    private var metadataPosition: Int64?
    private var metadataTitle: String?
    private var loadTask: Task<Void, Never>?
    private var voiceTask: Task<Void, Never>?
    // Insertion pauses are chained and retained as a daemon-command barrier.
    // A following load awaits the tail so an old pause cannot overtake it.
    private var insertPauseTask: Task<Void, Never>?
    private var loadGeneration: UInt64 = 0
    private var interruptAnchor: MacInterruptAnchor?
    private var pauseWorkItem: DispatchWorkItem?
    private var resumeTimer: DispatchSourceTimer?
    // Personal pause (2026-07-10): the USER paused this Pulsar in the Spotify
    // app while the shared air was playing. Cleared by any coordinator
    // ownership act (load / pause command / stop / mode switch) — a
    // coordinator-owned pause supersedes the personal one.
    private var pausedLocally = false
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
                self?.cancelTimers()
                self?.mediaClips?.reset()
                self?.sendError(code: "librespot_restart", message: "daemon exited, supervisor restarting")
            }
        }
        startPositionPolling()
        startDrainWatch()
        startStarvationWatch()
    }

    /// Installs the P1 transmission hooks before the coordinator connects.
    /// The base media capability is safe now; exact delivery capabilities are
    /// supplied only by mixer implementations that can actually execute them.
    public func configureTransmissionHooks(
        cacheDirectory: URL,
        nodeToken: String,
        coordinatorURL: URL,
        localStateURL: URL
    ) throws {
        let fetcher = try AuthenticatedMediaClipFetcher(
            cacheDirectory: cacheDirectory,
            nodeToken: nodeToken,
            coordinatorURL: coordinatorURL)
        let mixer = MacOverlayMediaClipMixer(audio: engine, log: log)
        mixer.bindInterruptController(self)
        mediaClips = MediaClipClient(
            fetcher: fetcher,
            mixer: mixer,
            log: log)
        presenceStore = NodePresenceStore(fileURL: localStateURL, log: log)
    }

    /// Canonical register capabilities for this exact build. Later mixer
    /// tasks extend this list through MediaClipMixer.deliveryCapabilities.
    public var advertisedCapabilities: [String] {
        Array(Set(
            [seamlessAdoptionCapability] +
            (mediaClips?.advertisedCapabilities ?? [])
        )).sorted()
    }

    public func stopTransmissionHooks() {
        mediaClips?.stop()
    }

    /// Reconnect owns a fresh command generation. Reset prepared/armed media
    /// before any reissued commands and invalidate an old interrupt token.
    public func applyWelcome(_ payload: WelcomePayload) {
        queue.async {
            self.cancelTimers()
            self.mediaClips?.reset()
            self.setVolume(payload.sessionSnapshot.volume)
            self.mode = payload.sessionSnapshot.mode
        }
    }

    private func nowMs() -> Int64 { Int64((Date().timeIntervalSince1970 * 1000).rounded()) }

    // MARK: Position (spec 6.3: audible_position = librespot_position - ring_fill)

    public var audiblePositionMs: Int64 {
        var pos = anchorPositionMs
        if extrapolate {
            pos += Int64(Date().timeIntervalSince(anchorAt) * 1000)
        }
        return Self.audibleAnchorMs(providerPositionMs: pos, ringFillMs: engine.ringFillMs)
    }

    static func audibleAnchorMs(providerPositionMs: Int64, ringFillMs: Int64) -> Int64 {
        max(0, providerPositionMs - max(0, ringFillMs))
    }

    var interruptReady: Bool {
        queue.sync {
            playback == .playing && currentElementID != nil && interruptAnchor == nil
        }
    }

    func suspendForInterrupt() -> MacInterruptAnchor? {
        queue.sync {
            guard playback == .playing, let elementID = currentElementID,
                  interruptAnchor == nil else { return nil }
            cancelTimers()
            let providerPosition = anchorPositionMs +
                (extrapolate ? Int64(Date().timeIntervalSince(anchorAt) * 1_000) : 0)
            let anchor = MacInterruptAnchor(
                elementID: elementID,
                loadGeneration: loadGeneration,
                positionMs: Self.audibleAnchorMs(
                    providerPositionMs: providerPosition,
                    ringFillMs: engine.ringFillMs))
            playback = .paused
            pausedLocally = false
            draining = false
            setAnchor(anchor.positionMs, extrapolating: false)
            interruptAnchor = anchor
            engine.expectingMusic = false
            engine.clearRing()
            let pauseTask = Task { [weak self, weak anchor] in
                guard let self, let anchor else { return false }
                let paused: Bool
                do {
                    try await self.librespot.pause()
                    paused = true
                } catch {
                    self.log.warn("interrupt provider pause failed", ["err": "\(error)"])
                    paused = false
                }
                self.engine.clearRing()
                return paused && self.queue.sync {
                    self.interruptAnchor === anchor &&
                    self.loadGeneration == anchor.loadGeneration &&
                    self.currentElementID == anchor.elementID
                }
            }
            anchor.pauseTask = pauseTask
            insertPauseTask = Task { _ = await pauseTask.value }
            return anchor
        }
    }

    func resumeFromInterrupt(
        _ anchor: MacInterruptAnchor,
        fadeInMs: Int64,
        completion: @escaping (Bool) -> Void
    ) {
        let operation: Task<Bool, Never> = queue.sync {
            let operation = Task<Bool, Never> { [weak self, weak anchor] in
                guard let self, let anchor else { return false }
                let pauseSucceeded = await anchor.pauseTask?.value ?? false
                guard self.validInterruptAnchor(anchor) else { return false }
                self.engine.clearRing()
                var seekSucceeded = true
                do {
                    try await self.librespot.seek(positionMS: anchor.positionMs)
                } catch {
                    seekSucceeded = false
                    self.log.warn("interrupt resume seek failed", ["err": "\(error)"])
                }
                guard self.validInterruptAnchor(anchor) else { return false }
                self.queue.sync {
                    self.playback = .playing
                    self.pausedLocally = false
                    self.draining = false
                    self.setAnchor(anchor.positionMs, extrapolating: true)
                    self.engine.setMusicGain(0, fadeMs: 0)
                    self.engine.setMusicGain(1, fadeMs: max(fadeInMs, 0))
                    self.engine.expectingMusic = true
                }
                var resumeSucceeded = true
                do {
                    try await self.librespot.resume()
                } catch {
                    resumeSucceeded = false
                    self.log.warn("interrupt provider resume failed", ["err": "\(error)"])
                    self.queue.sync {
                        guard self.loadGeneration == anchor.loadGeneration,
                              self.currentElementID == anchor.elementID else { return }
                        self.playback = .paused
                        self.extrapolate = false
                        self.interruptAnchor = nil
                        self.engine.expectingMusic = false
                    }
                }
                let stillCurrent = self.queue.sync {
                    let valid = self.interruptAnchor === anchor &&
                        self.loadGeneration == anchor.loadGeneration &&
                        self.currentElementID == anchor.elementID
                    if valid { self.interruptAnchor = nil }
                    return valid
                }
                return pauseSucceeded && seekSucceeded && resumeSucceeded && stillCurrent
            }
            insertPauseTask = Task { _ = await operation.value }
            return operation
        }
        Task { completion(await operation.value) }
    }

    func abandonInterrupt(_ anchor: MacInterruptAnchor) {
        queue.sync {
            guard self.interruptAnchor === anchor else { return }
            self.interruptAnchor = nil
        }
    }

    private func validInterruptAnchor(_ anchor: MacInterruptAnchor) -> Bool {
        queue.sync {
            interruptAnchor === anchor && loadGeneration == anchor.loadGeneration &&
            currentElementID == anchor.elementID
        }
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
            case .prepareMedia(let p): self.mediaClips?.prepare(p)
            case .playMediaAt(let p): self.mediaClips?.play(p)
            case .cancelMedia(let p): self.mediaClips?.cancel(p)
            case .presenceUpdate(let p):
                _ = self.presenceStore?.acceptPresence(p)
            case .welcome, .pong, .register, .state, .ready, .started, .ended,
                 .voiceStarted, .voiceEnded, .waitEnded, .error, .ping, .externalPlayback,
                 .setProvider, .userPause, .userResume, .mediaReady,
                 .mediaStarted, .mediaEnded, .mediaFailed, .mediaCancelled, .setDND:
                self.log.debug("ignoring non-command", ["type": head.type])
            }
        }
    }

    private func load(_ p: LoadPayload) {
        cancelTimers()
        pausedLocally = false
        draining = false
        currentElementID = p.elementId
        currentURI = p.uri
        engine.stopInsert()

        if p.adoptPlaying == true {
            // Spotify is already audibly playing this selection on the leader.
            // Relabel it for coordinator accounting without touching the ring
            // or daemon — this is the no-pause handoff.
            playback = .playing
            engine.setMusicGain(1, fadeMs: 0)
            engine.expectingMusic = true
            setAnchor(p.positionMs, extrapolating: true)
            coordinator?.sendMessage(.ready(ReadyPayload(elementId: p.elementId)))
            return
        }

        playback = .loading
        engine.clearRing()
        engine.setMusicGain(1, fadeMs: 0)
        engine.expectingMusic = false
        setAnchor(p.positionMs, extrapolating: false)

        let generation = loadGeneration
        let insertPauseBarrier = insertPauseTask
        loadTask = Task { [weak self] in
            guard let self else { return }
            do {
                if let insertPauseBarrier { await insertPauseBarrier.value }
                try Task.checkCancellation()
                // The daemon needs seconds after (re)start to authenticate;
                // a load racing that window must wait, not fail as
                // "track unavailable" (R0 finding, prod 2026-07-05).
                for _ in 0..<20 where !(await self.librespot.playbackReady()) {
                    try Task.checkCancellation()
                    try await Task.sleep(nanoseconds: 500_000_000)
                }
                try Task.checkCancellation()
                do {
                    try await self.librespot.playPaused(uri: p.uri)
                } catch {
                    try Task.checkCancellation()
                    // One local retry: transient daemon stalls (transfer storms)
                    // must not surface as track_unavailable.
                    try await Task.sleep(nanoseconds: 2_000_000_000)
                    try await self.librespot.playPaused(uri: p.uri)
                }
                try Task.checkCancellation()
                if p.positionMs > 0 {
                    try await self.librespot.seek(positionMS: p.positionMs)
                }
                try await self.confirmPausedLoaded(uri: p.uri)
                self.queue.async {
                    guard self.loadGeneration == generation,
                          self.currentElementID == p.elementId else { return } // stale
                    self.loadTask = nil
                    self.playback = .paused
                    self.coordinator?.sendMessage(.ready(ReadyPayload(elementId: p.elementId)))
                }
            } catch {
                if Task.isCancelled { return }
                self.queue.async {
                    guard self.loadGeneration == generation,
                          self.currentElementID == p.elementId else { return }
                    self.loadTask = nil
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
            try Task.checkCancellation()
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

        if let position = p.positionMs {
            // This home is joining a leader that never stopped. Seek while
            // paused, then arm the ordinary monotonic resume timer.
            let generation = loadGeneration
            engine.clearRing()
            setAnchor(position, extrapolating: false)
            Task {
                do {
                    try await self.librespot.seek(positionMS: position)
                } catch {
                    self.log.warn("catch-up seek failed; resuming best effort", [
                        "element": p.elementId, "err": "\(error)",
                    ])
                }
                self.queue.async {
                    guard self.loadGeneration == generation,
                          self.currentElementID == p.elementId else { return }
                    self.scheduleResume(p)
                }
            }
            return
        }
        scheduleResume(p)
    }

    private func scheduleResume(_ p: ResumeAtPayload) {
        resumeTimer?.cancel()
        resumeTimer = nil
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
        let generation = loadGeneration
        let providerBarrier = insertPauseTask
        pausedLocally = false
        engine.setMusicGain(0, fadeMs: p.fadeMs)
        playback = .paused
        engine.expectingMusic = false
        extrapolate = false
        let element = p.elementId
        let item = DispatchWorkItem { [weak self] in
            guard let self,
                  (element.isEmpty || self.currentElementID == element),
                  self.playback == .paused else { return }
            Task {
                if let providerBarrier { await providerBarrier.value }
                let current = self.queue.sync {
                    self.loadGeneration == generation &&
                    (element.isEmpty || self.currentElementID == element) &&
                    self.playback == .paused
                }
                if current { try? await self.librespot.pause() }
            }
        }
        pauseWorkItem = item
        queue.asyncAfter(
            deadline: .now() + .milliseconds(Int(p.fadeMs) + 20),
            execute: item
        )
    }

    private func seekCmd(_ p: SeekPayload) {
        guard p.elementId == currentElementID else { return }
        interruptAnchor = nil
        let generation = loadGeneration
        let providerBarrier = insertPauseTask
        engine.clearRing()
        setAnchor(p.positionMs, extrapolating: playback == .playing)
        Task {
            if let providerBarrier { await providerBarrier.value }
            let current = self.queue.sync {
                self.loadGeneration == generation && self.currentElementID == p.elementId
            }
            if current { try? await self.librespot.seek(positionMS: p.positionMs) }
        }
    }

    private func playVoice(_ p: PlayVoicePayload) {
        cancelTimers()
        draining = false
        currentElementID = p.elementId
        playback = .voice
        engine.stopInsert()
        engine.expectingMusic = false
        engine.clearRing()
        engine.setMusicGain(0, fadeMs: 0)
        let pauseBarrier = enqueueInsertPause()
        voiceTask = Task { [weak self] in
            guard let self else { return }
            do {
                let file = try await self.cache.fetch(fileURL: p.fileUrl)
                await pauseBarrier.value
                try Task.checkCancellation()
                var when: AVAudioTime?
                if let tCoord = p.tCoordMs, let clock = self.coordinator?.clock,
                   let tLocal = clock.localDeadline(forCoordinatorMs: tCoord,
                                                    outputLatencyOffsetMs: self.outputLatencyOffsetMs) {
                    when = AVAudioTime(hostTime: HostClock.hostTime(forUnixMs: tLocal))
                }
                self.queue.async {
                    guard self.currentElementID == p.elementId,
                          self.playback == .voice else { return }
                    self.voiceTask = nil
                    do {
                        try self.engine.playInsert(fileURL: file, at: when) {
                            self.queue.async {
                                guard self.currentElementID == p.elementId,
                                      self.playback == .voice else { return }
                                self.playback = .stopped
                                self.coordinator?.sendMessage(.voiceEnded(VoiceEndedPayload(elementId: p.elementId)))
                            }
                        }
                        self.coordinator?.sendMessage(.voiceStarted(VoiceStartedPayload(elementId: p.elementId)))
                    } catch {
                        self.sendError(code: "media_download_failed", message: "\(error)", elementId: p.elementId)
                    }
                }
            } catch {
                if Task.isCancelled { return }
                self.queue.async {
                    guard self.currentElementID == p.elementId,
                          self.playback == .voice else { return }
                    self.voiceTask = nil
                    self.sendError(code: "media_download_failed", message: "\(error)", elementId: p.elementId)
                }
            }
        }
    }

    private func waitCmd(_ p: WaitPayload) {
        cancelTimers()
        currentElementID = p.elementId
        playback = .wait
        engine.stopInsert()
        engine.expectingMusic = false
        engine.clearRing()
        engine.setMusicGain(0, fadeMs: 0)
        _ = enqueueInsertPause()
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

    /// UI-safe local volume mutation. The player queue remains the single
    /// owner shared with coordinator and provider events.
    public func setLocalVolume(_ v: Int) {
        queue.async { self.setVolume(v) }
    }

    private func setVolume(_ v: Int) {
        volume = v
        engine.setVolume(v)
    }

    private func setMode(_ m: String) {
        mode = m
        pausedLocally = false
        log.info("mode set", ["mode": m])
    }

    private func stopAll() {
        let wasInsert = playback == .voice || playback == .wait
        cancelTimers()
        let providerBarrier = insertPauseTask
        mediaClips?.reset()
        pausedLocally = false
        engine.stopInsert()
        draining = false
        currentElementID = nil
        currentURI = nil
        playback = .stopped
        engine.expectingMusic = false
        if wasInsert {
            // Voice/wait already silenced the music branch. Reset
            // synchronously so an immediately following load cannot be
            // erased by the ordinary delayed music-stop tail cleanup.
            engine.clearRing()
            engine.setMusicGain(1, fadeMs: 0)
            return
        }
        // Mode switches yank someone's music away (spec 4.3) — land it softly:
        // raised-cosine fade out, then stop the daemon and drop the tail.
        engine.setMusicGain(0, fadeMs: 250)
        queue.asyncAfter(deadline: .now() + .milliseconds(300)) {
            self.engine.clearRing()
            self.engine.setMusicGain(1, fadeMs: 0) // ready for the next element
        }
        Task {
            if let providerBarrier { await providerBarrier.value }
            try? await self.librespot.stop()
        }
    }

    @discardableResult
    private func enqueueInsertPause() -> Task<Void, Never> {
        let previous = insertPauseTask
        let task = Task { [weak self] in
            if let previous { await previous.value }
            guard let self else { return }
            try? await self.librespot.pause()
        }
        insertPauseTask = task
        return task
    }

    private func setOffset(_ p: SetOffsetPayload) {
        outputLatencyOffsetMs = Int(p.offsetMs)
        mediaClips?.setOutputLatencyOffsetMs(outputLatencyOffsetMs)
        log.info("offset set", ["offset_ms": p.offsetMs])
    }

    // MARK: Local DND and privacy-bounded presence

    /// Future UI calls this exact-node mutation. It persists the next revision
    /// before sending and cannot address another node or loosen orbit DND.
    public func setLocalDND(mode: String, mutedUntilCoordMs: Int64? = nil) throws {
        guard let presenceStore else { throw NodePresenceStoreError.persistenceFailed }
        let localNow = nowMs()
        let coordinatorNow: Int64
        if let offset = coordinator?.clockSnapshot().offsetMs {
            coordinatorNow = localNow - Int64(offset.rounded())
        } else {
            coordinatorNow = localNow
        }
        let payload = try presenceStore.nextLocalDND(
            mode: mode,
            mutedUntilCoordMs: mutedUntilCoordMs,
            coordinatorNowMs: coordinatorNow)
        coordinator?.sendMessage(.setDND(payload))
    }

    /// Replays durable intent after each authenticated reconnect. The same
    /// revision/body pair is idempotent in the frozen coordinator contract.
    public func resendLocalDND() {
        guard let payload = presenceStore?.currentLocalDND else { return }
        coordinator?.sendMessage(.setDND(payload))
    }

    public var latestPresence: PresenceUpdatePayload? {
        presenceStore?.latestPresence
    }

    public var localDNDMode: String {
        presenceStore?.currentLocalDND?.mode ?? "allow_all"
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
        _ uri: String?, observedPosition: Int64? = nil, title: String? = nil,
        allowSameURI: Bool = false, playOrigin: String? = nil
    ) {
        guard SpotifySelection.shouldReport(
            mode: mode, uri: uri, expectedURI: currentURI,
            playback: playback, allowSameURI: allowSameURI,
            playOrigin: playOrigin
        ), let uri else { return }
        // Metadata + playing for the SAME selection arrive separately. Only
        // suppress that duplicate; a different track chosen one second later
        // is intentional and must win immediately.
        guard uri != lastExternalURI || Date().timeIntervalSince(lastExternalReport) > 5 else { return }
        lastExternalReport = Date()
        lastExternalURI = uri
        let positionMs = SpotifySelection.startPosition(
            observedPosition: observedPosition, uri: uri,
            expectedURI: currentURI, audiblePosition: audiblePositionMs)
        log.warn("external playback detected in shared", [
            "uri": uri, "expected": currentURI ?? "silence",
            "play_origin": playOrigin ?? "unknown",
        ])
        coordinator?.sendMessage(.externalPlayback(
            ExternalPlaybackPayload(uri: uri, positionMs: positionMs, title: title)))
    }

    private func handleLibrespotEvent(_ event: LibrespotEvent) {
        switch event {
        case .metadata(let uri, let name, let artists, let position, _):
            if let position { setAnchor(position, extrapolating: playback == .playing) }
            metadataURI = uri
            metadataPosition = position
            metadataTitle = SpotifySelection.displayTitle(name: name, artistNames: artists)
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
            // Personal pause (2026-07-10): reaching here with playback still
            // .playing means the DAEMON acted on the user, not on us — every
            // coordinator-driven pause flips `playback` before the daemon
            // echoes the event. Report it and cancel any resume_at in flight:
            // a scheduled fireResume overriding a fresh user pause was one of
            // the ghost-resume mechanics (Timur, 2026-07-10).
            if mode == "shared", playback == .playing, !pausedLocally,
               let el = currentElementID {
                pausedLocally = true
                resumeTimer?.cancel()
                resumeTimer = nil
                log.info("personal pause", ["element": el])
                coordinator?.sendMessage(.userPause(UserPausePayload(elementId: el)))
            }
            // Solo UX (spike S4): the daemon stalls the pipe instantly but the
            // ring holds up to 1 s of tail — fade fast so the pause FEELS
            // instant. The ring is NOT cleared: on resume the tail plays first
            // and nothing is lost.
            if mode == "solo" || playback == .playing {
                engine.setMusicGain(0, fadeMs: 250)
                extrapolate = false
            }
        case .playing(let uri, let playOrigin):
            if pausedLocally {
                pausedLocally = false
                if let uri, uri == currentURI {
                    // Personal resume: play in Spotify returns THIS home to
                    // the air — the coordinator answers with a catch-up load
                    // at the live position. Not an adoption.
                    log.info("personal resume", ["element": currentElementID ?? ""])
                    coordinator?.sendMessage(.userResume(
                        UserResumePayload(elementId: currentElementID ?? "")))
                    engine.setMusicGain(1, fadeMs: 120)
                    return
                }
                // A DIFFERENT track picked while paused is a fresh selection —
                // fall through to the adoption path with the flag cleared.
            }
            // fireResume marks .playing before the daemon resumes. A matching
            // event while stopped/paused therefore means the user selected it.
            if interruptAnchor != nil, playback == .paused {
                engine.setMusicGain(0, fadeMs: 0)
                return
            }
            let insertionActive = playback == .voice || playback == .wait
            let matchesMetadata = uri != nil && uri == metadataURI
            reportExternalSelection(
                uri,
                observedPosition: matchesMetadata ? metadataPosition : nil,
                title: matchesMetadata ? metadataTitle : nil,
                allowSameURI: true,
                playOrigin: playOrigin
            )
            if insertionActive {
                // The coordinator queues this user choice after the accepted
                // voice block. Keep the daemon silent until that boundary.
                engine.setMusicGain(0, fadeMs: 0)
                Task { try? await self.librespot.pause() }
                return
            }
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

    // Underruns while music is expected > ~8 s -> audio_starvation + soft
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
            // Prod 2026-07-10: the 3 s threshold fired during healthy Spotify
            // track/context transitions and restarted Timur's daemon exactly
            // while the next shared load was arming. Give transient buffering
            // a real recovery window; a sustained ~9 s silence is still healed.
            if self.engine.expectingMusic, self.engine.starvedCallbacksStreak > 800,
               Date().timeIntervalSince(self.lastStarvationReport) > 10 {
                self.lastStarvationReport = Date()
                self.sendError(code: "audio_starvation", message: "no samples for >8s while playing")
                self.supervisor.softRestart()
            }
        }
        t.resume()
        starvationTimer = t
    }
    private var starvationTimer: DispatchSourceTimer?

    private func cancelTimers() {
        loadGeneration &+= 1
        interruptAnchor = nil
        loadTask?.cancel()
        loadTask = nil
        voiceTask?.cancel()
        voiceTask = nil
        pauseWorkItem?.cancel()
        pauseWorkItem = nil
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
