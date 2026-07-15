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

final class MacInterruptAnchor: @unchecked Sendable {
    let elementID: String
    let loadGeneration: UInt64
    let positionMs: Int64
    var pauseTask: Task<Bool, Never>?

    init(elementID: String, loadGeneration: UInt64, positionMs: Int64) {
        self.elementID = elementID
        self.loadGeneration = loadGeneration
        self.positionMs = positionMs
    }
}

protocol MacInterruptControlling: AnyObject {
    var interruptReady: Bool { get }
    func suspendForInterrupt() -> MacInterruptAnchor?
    func resumeFromInterrupt(
        _ anchor: MacInterruptAnchor,
        fadeInMs: Int64,
        completion: @escaping (Bool) -> Void)
    func abandonInterrupt(_ anchor: MacInterruptAnchor)
}

final class MacOverlayMediaClipMixer: MediaClipMixer, @unchecked Sendable {
    private final class State: @unchecked Sendable {
        let clip: PreparedMediaClip
        let buffer: AVAudioPCMBuffer
        let plan: MediaClipPlayPlan
        let onStarted: (Int64) -> Void
        let onEnded: (Int64) -> Void
        let onFailed: (MediaClipFailure) -> Void
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
        var interruptAnchor: MacInterruptAnchor?
        var resuming = false
        var cancelDuringResume: ((Result<Bool, MediaClipFailure>) -> Void)?

        init(
            clip: PreparedMediaClip,
            buffer: AVAudioPCMBuffer,
            plan: MediaClipPlayPlan,
            onStarted: @escaping (Int64) -> Void,
            onEnded: @escaping (Int64) -> Void,
            onFailed: @escaping (MediaClipFailure) -> Void
        ) {
            self.clip = clip
            self.buffer = buffer
            self.plan = plan
            self.onStarted = onStarted
            self.onEnded = onEnded
            self.onFailed = onFailed
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

    private let audio: MacOverlayAudioRouting
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.macos-overlay-mixer")
    private let nowLocalMs: () -> Int64
    private var active: State?
    private weak var interruptController: MacInterruptControlling?
    private let overlayFrameCounter = RenderAtomicInt64()
    private let interruptFrameCounter = RenderAtomicInt64()
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
    var interruptFrames: Int64 { interruptFrameCounter.load() }
    var limiterHitWindows: Int64 { limiterHitWindowCounter.load() }
    var deliveryCapabilities: [String] {
        queue.sync {
            interruptController == nil
                ? [overlayMixCapability]
                : [overlayMixCapability, interruptResumeCapability]
        }
    }

    func bindInterruptController(_ controller: MacInterruptControlling) {
        queue.sync { interruptController = controller }
    }

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
        onEnded: @escaping (Int64) -> Void,
        onFailed: @escaping (MediaClipFailure) -> Void
    ) throws {
        guard let buffer = clip.decoderHandle as? AVAudioPCMBuffer,
              plan.control.delivery == "overlay" || plan.control.delivery == "interrupt" else {
            throw MediaClipFailure.frozenCode("capability_lost")
        }
        try queue.sync {
            guard active == nil else {
                throw MediaClipFailure.frozenCode("audio_graph_failed")
            }
            if plan.control.interrupt {
                guard interruptController?.interruptReady == true else {
                    throw MediaClipFailure.frozenCode("interrupt_capability_lost")
                }
            } else if plan.control.delivery != "overlay" {
                throw MediaClipFailure.frozenCode("capability_lost")
            }
            let state = State(
                clip: clip,
                buffer: buffer,
                plan: plan,
                onStarted: onStarted,
                onEnded: onEnded,
                onFailed: onFailed)
            active = state
            audio.setOverlayGain(1)
            let when = AVAudioTime(
                hostTime: HostClock.hostTime(forUnixMs: plan.localStartMs))
            audio.scheduleOverlay(buffer, at: when) { [weak self, weak state] in
                guard let self, let state else { return }
                self.queue.async { self.finishNaturally(state) }
            }
            schedulePreControl(state)
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
            self.beginCancel(state, command: command, completion: completion)
        }
    }

    func dispose(_ clip: PreparedMediaClip) {
        queue.async { [weak self] in
            guard let self, let state = self.active, state.clip === clip, !state.started else { return }
            state.cancelTimers()
            state.terminal = true
            if state.duckApplied {
                let restoreMs = state.plan.control.interrupt
                    ? state.plan.control.fadeInMs : state.plan.control.releaseMs
                self.audio.setMusicGain(1, fadeMs: restoreMs)
            }
            if let anchor = state.interruptAnchor {
                self.interruptController?.abandonInterrupt(anchor)
            }
            self.audio.stopOverlay()
            self.audio.setOverlayGain(1)
            self.active = nil
        }
    }

    private func schedulePreControl(_ state: State) {
        let durationMs = state.plan.control.interrupt
            ? state.plan.control.fadeOutMs : state.plan.control.attackMs
        let targetMs = state.plan.localStartMs - durationMs
        let delay = targetMs - nowLocalMs()
        if delay <= 0 {
            applyPreControl(state, scheduledAtMs: targetMs)
            return
        }
        state.preDuckTimer = oneShot(afterMs: delay) { [weak self, weak state] in
            guard let self, let state else { return }
            self.applyPreControl(state, scheduledAtMs: targetMs)
        }
    }

    private func applyPreControl(_ state: State, scheduledAtMs: Int64) {
        guard active === state, !state.terminal, !state.cancelling else { return }
        state.duckApplied = true
        let elapsed = max(0, nowLocalMs() - scheduledAtMs)
        let durationMs = state.plan.control.interrupt
            ? state.plan.control.fadeOutMs : state.plan.control.attackMs
        let remaining = max(0, durationMs - elapsed)
        let target: Float = state.plan.control.interrupt
            ? 0 : Float(pow(10, state.plan.control.duckDB / 20))
        audio.setMusicGain(target, fadeMs: remaining)
    }

    private func scheduleStarted(_ state: State) {
        let delay = max(0, state.plan.localStartMs - nowLocalMs())
        state.startTimer = oneShot(afterMs: delay) { [weak self, weak state] in
            guard let self, let state, self.active === state,
                  !state.terminal, !state.cancelling else { return }
            if state.plan.control.interrupt {
                guard let anchor = self.interruptController?.suspendForInterrupt() else {
                    self.failInterruptStart(state)
                    return
                }
                state.interruptAnchor = anchor
            }
            state.started = true
            state.startedAtMs = self.nowLocalMs()
            state.onStarted(state.startedAtMs!)
            if state.plan.collectTelemetry { self.startTelemetry(state) }
        }
    }

    private func failInterruptStart(_ state: State) {
        guard active === state, !state.terminal else { return }
        state.terminal = true
        state.cancelTimers()
        audio.stopOverlay()
        audio.setOverlayGain(1)
        audio.setMusicGain(1, fadeMs: state.plan.control.fadeInMs)
        active = nil
        state.onFailed(.frozenCode("interrupt_capability_lost"))
    }

    private func finishNaturally(_ state: State) {
        guard active === state, !state.terminal, !state.cancelling else { return }
        if state.plan.control.interrupt {
            finishInterruptNaturally(state)
            return
        }
        state.terminal = true
        state.cancelTimers()
        audio.setMusicGain(1, fadeMs: state.plan.control.releaseMs)
        audio.setOverlayGain(1)
        overlayFrameCounter.add(Int64(state.buffer.frameLength))
        let endedAt = nowLocalMs()
        active = nil
        state.onEnded(endedAt)
        logTelemetry(state, reason: "completed")
    }

    private func finishInterruptNaturally(_ state: State) {
        guard let anchor = state.interruptAnchor,
              let controller = interruptController else {
            failInterruptResume(state, cancelCompletion: nil)
            return
        }
        state.cancelTimers()
        state.resuming = true
        interruptFrameCounter.add(Int64(state.buffer.frameLength))
        controller.resumeFromInterrupt(
            anchor,
            fadeInMs: state.plan.control.fadeInMs
        ) { [weak self, weak state] resumed in
            guard let self, let state else { return }
            self.queue.async {
                guard self.active === state, !state.terminal else { return }
                state.resuming = false
                let cancelCompletion = state.cancelDuringResume
                state.cancelDuringResume = nil
                if resumed {
                    state.terminal = true
                    self.audio.setOverlayGain(1)
                    self.active = nil
                    if let cancelCompletion {
                        cancelCompletion(.success(true))
                        self.logTelemetry(state, reason: "cancelled")
                    } else {
                        state.onEnded(self.nowLocalMs())
                        self.logTelemetry(state, reason: "completed")
                    }
                } else {
                    self.failInterruptResume(state, cancelCompletion: cancelCompletion)
                }
            }
        }
    }

    private func failInterruptResume(
        _ state: State,
        cancelCompletion: ((Result<Bool, MediaClipFailure>) -> Void)?
    ) {
        guard active === state, !state.terminal else { return }
        state.terminal = true
        state.cancelTimers()
        audio.stopOverlay()
        audio.setOverlayGain(1)
        active = nil
        let failure = MediaClipFailure.frozenCode("audio_graph_failed")
        if let cancelCompletion {
            cancelCompletion(.failure(failure))
        } else {
            state.onFailed(failure)
        }
        logTelemetry(state, reason: "resume_failed")
    }

    private func beginCancel(
        _ state: State,
        command: CancelMediaPayload,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    ) {
        guard !state.cancelling else { return }
        let fadeMs = max(command.fadeMs, 0)
        state.cancelling = true
        state.preDuckTimer?.cancel()
        state.startTimer?.cancel()
        state.telemetryTimer?.cancel()
        state.preDuckTimer = nil
        state.startTimer = nil
        state.telemetryTimer = nil
        if state.plan.control.interrupt {
            beginInterruptCancel(
                state,
                fadeMs: fadeMs,
                resumeMain: command.resumeMain,
                completion: completion)
            return
        }
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
            self.logTelemetry(state, reason: "cancelled")
        }
    }

    private func beginInterruptCancel(
        _ state: State,
        fadeMs: Int64,
        resumeMain: Bool,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    ) {
        if state.resuming {
            state.cancelDuringResume = completion
            return
        }
        if state.started, fadeMs > 0 {
            startOverlayFade(state, durationMs: fadeMs)
        } else {
            audio.setOverlayGain(0)
        }
        let waitMs = state.started ? fadeMs : 0
        state.finishTimer = oneShot(afterMs: waitMs) { [weak self, weak state] in
            guard let self, let state, self.active === state, !state.terminal else { return }
            state.finishTimer = nil
            self.audio.stopOverlay()
            self.audio.setOverlayGain(1)
            if let startedAt = state.startedAtMs {
                let playedMs = max(0, self.nowLocalMs() - startedAt)
                let frames = min(
                    Int64(state.buffer.frameLength),
                    Int64(Double(playedMs) * self.audio.sampleRate / 1_000))
                self.interruptFrameCounter.add(frames)
            }
            guard state.started, let anchor = state.interruptAnchor else {
                state.terminal = true
                self.audio.setMusicGain(1, fadeMs: state.plan.control.fadeInMs)
                self.active = nil
                completion(.success(false))
                return
            }
            guard let controller = self.interruptController else {
                self.failInterruptResume(state, cancelCompletion: completion)
                return
            }
            guard resumeMain else {
                controller.abandonInterrupt(anchor)
                state.terminal = true
                self.active = nil
                completion(.success(false))
                return
            }
            state.resuming = true
            controller.resumeFromInterrupt(
                anchor,
                fadeInMs: state.plan.control.fadeInMs
            ) { [weak self, weak state] resumed in
                guard let self, let state else { return }
                self.queue.async {
                    guard self.active === state, !state.terminal else { return }
                    state.resuming = false
                    if resumed {
                        state.terminal = true
                        self.active = nil
                        completion(.success(true))
                        self.logTelemetry(state, reason: "cancelled")
                    } else {
                        self.failInterruptResume(state, cancelCompletion: completion)
                    }
                }
            }
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

    private func logTelemetry(_ state: State, reason: String) {
        guard state.plan.collectTelemetry else { return }
        log.info("macOS media mixer telemetry", [
            "reason": reason,
            "overlay_frames": overlayFrames,
            "interrupt_frames": interruptFrames,
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
