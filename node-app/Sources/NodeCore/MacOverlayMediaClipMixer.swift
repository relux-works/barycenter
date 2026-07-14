import AVFAudio
import Foundation

protocol MacOverlayAudioRouting: AnyObject {
    var sampleRate: Double { get }
    var ringFillMs: Int64 { get }
    var underrunCallbacks: Int64 { get }
    var limiterReductionDB: Float { get }
    func setMusicGain(_ target: Float, fadeMs: Int64)
    func scheduleOverlay(
        _ buffer: AVAudioPCMBuffer,
        at when: AVAudioTime?,
        completion: @escaping () -> Void)
    func stopOverlay()
    func setOverlayGain(_ gain: Float)
}

extension AudioEngine: MacOverlayAudioRouting {}

final class MacOverlayMediaClipMixer: MediaClipMixer, @unchecked Sendable {
    private final class State: @unchecked Sendable {
        let clip: PreparedMediaClip
        let buffer: AVAudioPCMBuffer
        let plan: MediaClipPlayPlan
        let onStarted: (Int64) -> Void
        let onEnded: (Int64) -> Void
        var preDuckTimer: DispatchSourceTimer?
        var startTimer: DispatchSourceTimer?
        var fadeTimer: DispatchSourceTimer?
        var finishTimer: DispatchSourceTimer?
        var telemetryTimer: DispatchSourceTimer?
        var duckApplied = false
        var started = false
        var cancelling = false
        var terminal = false
        var startedAtMs: Int64?

        init(
            clip: PreparedMediaClip,
            buffer: AVAudioPCMBuffer,
            plan: MediaClipPlayPlan,
            onStarted: @escaping (Int64) -> Void,
            onEnded: @escaping (Int64) -> Void
        ) {
            self.clip = clip
            self.buffer = buffer
            self.plan = plan
            self.onStarted = onStarted
            self.onEnded = onEnded
        }

        func cancelTimers() {
            preDuckTimer?.cancel()
            startTimer?.cancel()
            fadeTimer?.cancel()
            finishTimer?.cancel()
            telemetryTimer?.cancel()
            preDuckTimer = nil
            startTimer = nil
            fadeTimer = nil
            finishTimer = nil
            telemetryTimer = nil
        }
    }

    let deliveryCapabilities = [overlayMixCapability]

    private let audio: MacOverlayAudioRouting
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.macos-overlay-mixer")
    private let nowLocalMs: () -> Int64
    private var active: State?
    private let overlayFrameCounter = RenderAtomicInt64()
    private let limiterHitWindowCounter = RenderAtomicInt64()

    init(
        audio: MacOverlayAudioRouting,
        log: Logger,
        nowLocalMs: @escaping () -> Int64 = {
            Int64((Date().timeIntervalSince1970 * 1000).rounded())
        }
    ) {
        self.audio = audio
        self.log = log
        self.nowLocalMs = nowLocalMs
    }

    var overlayFrames: Int64 { overlayFrameCounter.load() }
    var limiterHitWindows: Int64 { limiterHitWindowCounter.load() }

    func prepare(localURL: URL, delivery: String) throws -> PreparedMediaClip {
        let decoded = try PreparedOnlyMacMediaClipMixer().prepare(
            localURL: localURL, delivery: delivery)
        guard let input = decoded.decoderHandle as? AVAudioPCMBuffer,
              let targetFormat = AVAudioFormat(
                standardFormatWithSampleRate: audio.sampleRate,
                channels: 2) else {
            throw MediaClipFailure.frozenCode("decode_failed")
        }
        if input.format == targetFormat {
            return decoded
        }
        guard let converter = AVAudioConverter(from: input.format, to: targetFormat) else {
            throw MediaClipFailure.frozenCode("decode_failed")
        }
        let ratio = targetFormat.sampleRate / input.format.sampleRate
        let capacity = AVAudioFrameCount(ceil(Double(input.frameLength) * ratio)) + 32
        guard let output = AVAudioPCMBuffer(
            pcmFormat: targetFormat,
            frameCapacity: capacity) else {
            throw MediaClipFailure.frozenCode("decode_failed")
        }
        var supplied = false
        var conversionError: NSError?
        let status = converter.convert(to: output, error: &conversionError) { _, outputStatus in
            if supplied {
                outputStatus.pointee = .endOfStream
                return nil
            }
            supplied = true
            outputStatus.pointee = .haveData
            return input
        }
        guard conversionError == nil, status != .error, output.frameLength > 0 else {
            throw MediaClipFailure.frozenCode("decode_failed")
        }
        let duration = Int64(
            (Double(output.frameLength) / targetFormat.sampleRate * 1_000).rounded(.up))
        return PreparedMediaClip(
            localURL: localURL,
            decodedDurationMs: duration,
            decoderHandle: output)
    }

    func arm(
        _ clip: PreparedMediaClip,
        plan: MediaClipPlayPlan,
        onStarted: @escaping (Int64) -> Void,
        onEnded: @escaping (Int64) -> Void
    ) throws {
        guard plan.control.delivery == "overlay", !plan.control.interrupt,
              let buffer = clip.decoderHandle as? AVAudioPCMBuffer else {
            throw MediaClipFailure.frozenCode("capability_lost")
        }
        try queue.sync {
            guard active == nil else {
                throw MediaClipFailure.frozenCode("audio_graph_failed")
            }
            let state = State(
                clip: clip,
                buffer: buffer,
                plan: plan,
                onStarted: onStarted,
                onEnded: onEnded)
            active = state
            audio.setOverlayGain(1)
            let when = AVAudioTime(
                hostTime: HostClock.hostTime(forUnixMs: plan.localStartMs))
            audio.scheduleOverlay(buffer, at: when) { [weak self, weak state] in
                guard let self, let state else { return }
                self.queue.async { self.finishNaturally(state) }
            }
            schedulePreDuck(state)
            scheduleStarted(state)
        }
    }

    func cancel(
        _ clip: PreparedMediaClip,
        command: CancelMediaPayload,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    ) {
        queue.async { [weak self] in
            guard let self else { return }
            guard let state = self.active, state.clip === clip, !state.terminal else {
                completion(.success(false))
                return
            }
            self.beginCancel(state, fadeMs: max(command.fadeMs, 0), completion: completion)
        }
    }

    func dispose(_ clip: PreparedMediaClip) {
        queue.async { [weak self] in
            guard let self, let state = self.active, state.clip === clip, !state.started else { return }
            state.cancelTimers()
            state.terminal = true
            if state.duckApplied {
                self.audio.setMusicGain(1, fadeMs: state.plan.control.releaseMs)
            }
            self.audio.stopOverlay()
            self.audio.setOverlayGain(1)
            self.active = nil
        }
    }

    private func schedulePreDuck(_ state: State) {
        let targetMs = state.plan.localStartMs - 250
        let delay = targetMs - nowLocalMs()
        if delay <= 0 {
            applyPreDuck(state, scheduledAtMs: targetMs)
            return
        }
        state.preDuckTimer = oneShot(afterMs: delay) { [weak self, weak state] in
            guard let self, let state else { return }
            self.applyPreDuck(state, scheduledAtMs: targetMs)
        }
    }

    private func applyPreDuck(_ state: State, scheduledAtMs: Int64) {
        guard active === state, !state.terminal, !state.cancelling else { return }
        state.duckApplied = true
        let elapsed = max(0, nowLocalMs() - scheduledAtMs)
        let remaining = max(0, state.plan.control.attackMs - elapsed)
        let target = Float(pow(10, state.plan.control.duckDB / 20))
        audio.setMusicGain(target, fadeMs: remaining)
    }

    private func scheduleStarted(_ state: State) {
        let delay = max(0, state.plan.localStartMs - nowLocalMs())
        state.startTimer = oneShot(afterMs: delay) { [weak self, weak state] in
            guard let self, let state, self.active === state,
                  !state.terminal, !state.cancelling else { return }
            state.started = true
            state.startedAtMs = self.nowLocalMs()
            state.onStarted(state.startedAtMs!)
            self.startTelemetry(state)
        }
    }

    private func finishNaturally(_ state: State) {
        guard active === state, !state.terminal, !state.cancelling else { return }
        state.terminal = true
        state.cancelTimers()
        audio.setMusicGain(1, fadeMs: state.plan.control.releaseMs)
        audio.setOverlayGain(1)
        overlayFrameCounter.add(Int64(state.buffer.frameLength))
        let endedAt = nowLocalMs()
        active = nil
        state.onEnded(endedAt)
        logTelemetry(reason: "completed")
    }

    private func beginCancel(
        _ state: State,
        fadeMs: Int64,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    ) {
        guard !state.cancelling else { return }
        state.cancelling = true
        state.preDuckTimer?.cancel()
        state.startTimer?.cancel()
        state.telemetryTimer?.cancel()
        state.preDuckTimer = nil
        state.startTimer = nil
        state.telemetryTimer = nil
        if state.duckApplied {
            audio.setMusicGain(1, fadeMs: state.plan.control.releaseMs)
        }
        if state.started, fadeMs > 0 {
            startOverlayFade(state, durationMs: fadeMs)
        } else {
            audio.setOverlayGain(0)
        }
        let releaseMs = state.duckApplied ? state.plan.control.releaseMs : 0
        let overlayFadeMs = state.started ? fadeMs : 0
        let waitMs = max(overlayFadeMs, releaseMs)
        state.finishTimer = oneShot(afterMs: waitMs) { [weak self, weak state] in
            guard let self, let state, self.active === state, !state.terminal else { return }
            state.terminal = true
            state.cancelTimers()
            self.audio.stopOverlay()
            self.audio.setOverlayGain(1)
            if let startedAt = state.startedAtMs {
                let playedMs = max(0, self.nowLocalMs() - startedAt)
                let frames = min(
                    Int64(state.buffer.frameLength),
                    Int64(Double(playedMs) * self.audio.sampleRate / 1000))
                self.overlayFrameCounter.add(frames)
            }
            self.active = nil
            completion(.success(false))
            self.logTelemetry(reason: "cancelled")
        }
    }

    private func startOverlayFade(_ state: State, durationMs: Int64) {
        let startedAt = nowLocalMs()
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now(), repeating: .milliseconds(5), leeway: .milliseconds(1))
        timer.setEventHandler { [weak self, weak state] in
            guard let self, let state, self.active === state, !state.terminal else { return }
            let elapsed = max(0, self.nowLocalMs() - startedAt)
            let progress = min(1, Double(elapsed) / Double(max(durationMs, 1)))
            let eased = 0.5 * (1 - cos(.pi * progress))
            self.audio.setOverlayGain(Float(1 - eased))
            if progress >= 1 {
                state.fadeTimer?.cancel()
                state.fadeTimer = nil
            }
        }
        timer.resume()
        state.fadeTimer = timer
    }

    private func startTelemetry(_ state: State) {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now(), repeating: .milliseconds(50))
        timer.setEventHandler { [weak self, weak state] in
            guard let self, let state, self.active === state, !state.terminal else { return }
            if self.audio.limiterReductionDB > 0.01 {
                self.limiterHitWindowCounter.add(1)
            }
        }
        timer.resume()
        state.telemetryTimer = timer
    }

    private func logTelemetry(reason: String) {
        log.info("macOS overlay mixer telemetry", [
            "reason": reason,
            "overlay_frames": overlayFrames,
            "limiter_hit_windows": limiterHitWindows,
            "ring_fill_ms": audio.ringFillMs,
            "underruns": audio.underrunCallbacks,
        ])
    }

    private func oneShot(afterMs delayMs: Int64, handler: @escaping () -> Void) -> DispatchSourceTimer {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(
            deadline: .now() + .milliseconds(Int(max(delayMs, 0))),
            leeway: .milliseconds(1))
        timer.setEventHandler(handler: handler)
        timer.resume()
        return timer
    }
}
