import AVFAudio
import Foundation
import Testing
@testable import NodeCore

private final class StubMacOverlayAudio: MacOverlayAudioRouting, @unchecked Sendable {
    let sampleRate = 44_100.0
    var ringFillMs: Int64 = 500
    var underrunCallbacks: Int64 = 0
    var limiterReductionDB: Float = 0
    private let lock = NSLock()
    private var completion: (() -> Void)?
    private(set) var musicGains: [(Float, Int64)] = []
    private(set) var overlayGains: [Float] = []
    private(set) var scheduledFrames: [AVAudioFrameCount] = []
    private(set) var scheduledBufferIDs: [ObjectIdentifier] = []
    private(set) var scheduledHostTimes: [UInt64] = []
    private(set) var stopCount = 0

    func setMusicGain(_ target: Float, fadeMs: Int64) {
        lock.withLock { musicGains.append((target, fadeMs)) }
    }

    func scheduleOverlay(
        _ buffer: AVAudioPCMBuffer,
        at when: AVAudioTime?,
        completion: @escaping () -> Void
    ) {
        lock.withLock {
            scheduledFrames.append(buffer.frameLength)
            scheduledBufferIDs.append(ObjectIdentifier(buffer))
            scheduledHostTimes.append(when?.hostTime ?? 0)
            self.completion = completion
        }
    }

    func stopOverlay() {
        lock.withLock { stopCount += 1 }
    }

    func setOverlayGain(_ gain: Float) {
        lock.withLock { overlayGains.append(gain) }
    }

    func finishScheduledBuffer() {
        let callback = lock.withLock { completion }
        callback?()
    }

    var musicGainSnapshot: [(Float, Int64)] { lock.withLock { musicGains } }
    var overlayGainSnapshot: [Float] { lock.withLock { overlayGains } }
    var scheduledFrameSnapshot: [AVAudioFrameCount] { lock.withLock { scheduledFrames } }
    var scheduledBufferIDSnapshot: [ObjectIdentifier] { lock.withLock { scheduledBufferIDs } }
    var stops: Int { lock.withLock { stopCount } }
}

private final class StubMacInterruptController: MacInterruptControlling, @unchecked Sendable {
    private let lock = NSLock()
    var ready = true
    var resumeResult = true
    var holdResume = false
    let anchor = MacInterruptAnchor(
        elementID: "el_interrupt", loadGeneration: 7, positionMs: 9_950)
    private var heldCompletion: ((Bool) -> Void)?
    private(set) var suspendCount = 0
    private(set) var resumeCalls: [(Int64, Int64)] = []
    private(set) var abandonCount = 0

    var interruptReady: Bool { lock.withLock { ready } }

    func suspendForInterrupt() -> MacInterruptAnchor? {
        lock.withLock {
            guard ready else { return nil }
            suspendCount += 1
            return anchor
        }
    }

    func resumeFromInterrupt(
        _ anchor: MacInterruptAnchor,
        fadeInMs: Int64,
        completion: @escaping (Bool) -> Void
    ) {
        let result = lock.withLock { () -> Bool? in
            resumeCalls.append((anchor.positionMs, fadeInMs))
            if holdResume {
                heldCompletion = completion
                return nil
            }
            return resumeResult
        }
        if let result { completion(result) }
    }

    func abandonInterrupt(_ anchor: MacInterruptAnchor) {
        lock.withLock { abandonCount += 1 }
    }

    func completeHeldResume(_ result: Bool) {
        let callback = lock.withLock { () -> ((Bool) -> Void)? in
            defer { heldCompletion = nil }
            return heldCompletion
        }
        callback?(result)
    }

    var suspends: Int { lock.withLock { suspendCount } }
    var resumes: [(Int64, Int64)] { lock.withLock { resumeCalls } }
    var abandons: Int { lock.withLock { abandonCount } }
}

private func overlayBuffer(seconds: Int = 10) -> AVAudioPCMBuffer {
    let format = AVAudioFormat(standardFormatWithSampleRate: 44_100, channels: 2)!
    let frames = AVAudioFrameCount(44_100 * seconds)
    let buffer = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: frames)!
    buffer.frameLength = frames
    return buffer
}

private func overlayBuffer(frames: AVAudioFrameCount) -> AVAudioPCMBuffer {
    let format = AVAudioFormat(standardFormatWithSampleRate: 44_100, channels: 2)!
    let buffer = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: frames)!
    buffer.frameLength = frames
    return buffer
}

private func overlayMixerPlan(startMs: Int64, releaseMs: Int64 = 600) -> MediaClipPlayPlan {
    let payload = PlayMediaAtPayload(
        transmissionId: "tr_macos_overlay",
        generation: 1,
        tCoordMs: startMs,
        startDeadlineCoordMs: startMs + 100,
        delivery: "overlay",
        duckDb: -12,
        attackMs: 250,
        releaseMs: releaseMs,
        fadeOutMs: nil,
        fadeInMs: nil)
    return MediaClipPlayPlan(
        payload: payload,
        localStartMs: startMs,
        localStartDeadlineMs: startMs + 100,
        control: MixerControlParameters(payload)!)
}

private func interruptMixerPlan(startMs: Int64) -> MediaClipPlayPlan {
    let payload = PlayMediaAtPayload(
        transmissionId: "tr_macos_interrupt",
        generation: 1,
        tCoordMs: startMs,
        startDeadlineCoordMs: startMs + 100,
        delivery: "interrupt",
        duckDb: nil,
        attackMs: nil,
        releaseMs: nil,
        fadeOutMs: 250,
        fadeInMs: 120)
    return MediaClipPlayPlan(
        payload: payload,
        localStartMs: startMs,
        localStartDeadlineMs: startMs + 100,
        control: MixerControlParameters(payload)!)
}

private func preparedOverlay(_ buffer: AVAudioPCMBuffer) -> PreparedMediaClip {
    PreparedMediaClip(
        localURL: URL(fileURLWithPath: "/tmp/macos-overlay.wav"),
        decodedDurationMs: Int64(Double(buffer.frameLength) / 44_100 * 1_000),
        decoderHandle: buffer)
}

private func eventuallyOverlay(
    timeoutIterations: Int = 300,
    _ predicate: @escaping () -> Bool
) async -> Bool {
    for _ in 0..<timeoutIterations {
        if predicate() { return true }
        try? await Task.sleep(nanoseconds: 5_000_000)
    }
    return predicate()
}

@Suite struct MacOverlayMediaClipMixerTests {
    @Test func audibleInterruptAnchorSubtractsQueuedRingTail() {
        #expect(PlayerCore.audibleAnchorMs(providerPositionMs: 10_000, ringFillMs: 50) == 9_950)
        #expect(PlayerCore.audibleAnchorMs(providerPositionMs: 20, ringFillMs: 50) == 0)
        #expect(PlayerCore.audibleAnchorMs(providerPositionMs: 20, ringFillMs: -1) == 20)
    }

    @Test func macOSRuns100SequentialOverlaysWithoutRetainedClipOwners() async throws {
        let audio = StubMacOverlayAudio()
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil),
            nowLocalMs: { now })
        let buffer = overlayBuffer(frames: 1)
        let started = NSLockBox(0)
        let ended = NSLockBox(0)
        weak var lastClip: PreparedMediaClip?

        for iteration in 0..<100 {
            var clip: PreparedMediaClip? = preparedOverlay(buffer)
            lastClip = clip
            try mixer.arm(
                clip!,
                plan: overlayMixerPlan(startMs: now),
                onStarted: { _ in started.withLock { $0 += 1 } },
                onEnded: { _ in ended.withLock { $0 += 1 } },
                onFailed: { failure in Issue.record("overlay failed: \(failure)") })
            #expect(await eventuallyOverlay {
                started.withLock { $0 == iteration + 1 }
            })
            audio.finishScheduledBuffer()
            #expect(await eventuallyOverlay {
                ended.withLock { $0 == iteration + 1 }
            })
            clip = nil
            #expect(await eventuallyOverlay { lastClip == nil },
                    "iteration \(iteration) retained a prepared clip owner")
        }
        #expect(audio.scheduledFrameSnapshot.count == 100)
        #expect(audio.scheduledFrameSnapshot.allSatisfy { $0 == 1 })
    }

    @Test func maximumP1ClipUsesOneBoundedPreparedPCMBuffer() async throws {
        let expectedFrames = AVAudioFrameCount(44_100 * 180)
        let expectedBytes = Int64(expectedFrames) * 2 * Int64(MemoryLayout<Float>.size)
        #expect(expectedBytes == MediaClipLimits.maximumDecodedPCMBytes)
        #expect(expectedBytes < 64 * 1_024 * 1_024)

        let audio = StubMacOverlayAudio()
        let controller = StubMacInterruptController()
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil))
        mixer.bindInterruptController(controller)
        let buffer = overlayBuffer(frames: expectedFrames)
        let clip = preparedOverlay(buffer)
        let start = Int64((Date().timeIntervalSince1970 * 1_000).rounded()) + 1_000
        try mixer.arm(
            clip,
            plan: interruptMixerPlan(startMs: start),
            onStarted: { _ in Issue.record("maximum fixture must remain armed") },
            onEnded: { _ in },
            onFailed: { failure in Issue.record("maximum fixture failed: \(failure)") })
        #expect(audio.scheduledBufferIDSnapshot == [ObjectIdentifier(buffer)])
        #expect(audio.scheduledFrameSnapshot == [expectedFrames])
        mixer.dispose(clip)
        #expect(await eventuallyOverlay { audio.stops == 1 })
    }

    @Test func interruptWithoutExactControllerFailsBeforeSchedulingAndNeverFallsBack() throws {
        let audio = StubMacOverlayAudio()
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil))
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let clip = preparedOverlay(overlayBuffer(seconds: 1))
        do {
            try mixer.arm(
                clip,
                plan: interruptMixerPlan(startMs: now),
                onStarted: { _ in },
                onEnded: { _ in },
                onFailed: { _ in })
            Issue.record("unbound exact interrupt unexpectedly armed")
        } catch let failure as MediaClipFailure {
            #expect(failure.code == "interrupt_capability_lost")
        }
        #expect(audio.scheduledFrameSnapshot.isEmpty)
        #expect(audio.musicGainSnapshot.isEmpty)
    }

    @Test func interruptFadesAtTMinus250ResumesExactAnchorOnceAndReusesGraph() async throws {
        let audio = StubMacOverlayAudio()
        let controller = StubMacInterruptController()
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil),
            nowLocalMs: { now })
        mixer.bindInterruptController(controller)
        #expect(mixer.deliveryCapabilities == [overlayMixCapability, interruptResumeCapability])
        let clip = preparedOverlay(overlayBuffer(seconds: 1))
        let started = NSLockBox<[Int64]>([])
        let ended = NSLockBox(0)
        try mixer.arm(
            clip,
            plan: interruptMixerPlan(startMs: now + 250),
            onStarted: { value in started.withLock { $0.append(value) } },
            onEnded: { _ in ended.withLock { $0 += 1 } },
            onFailed: { failure in Issue.record("interrupt failed: \(failure)") })

        #expect(audio.musicGainSnapshot.first?.0 == 0)
        #expect(audio.musicGainSnapshot.first?.1 == 250)
        #expect(await eventuallyOverlay { started.withLock { $0.count == 1 } })
        let interruptStart = try #require(started.withLock { $0.first })
        #expect(abs(interruptStart - (now + 250)) <= 500)
        #expect(controller.suspends == 1)
        audio.finishScheduledBuffer()
        #expect(await eventuallyOverlay { ended.withLock { $0 == 1 } })
        #expect(controller.resumes.count == 1)
        #expect(controller.resumes.first?.0 == 9_950)
        #expect(controller.resumes.first?.1 == 120)
        #expect(mixer.interruptFrames == Int64(overlayBuffer(seconds: 1).frameLength))

        let replacement = preparedOverlay(overlayBuffer(seconds: 1))
        try mixer.arm(
            replacement,
            plan: overlayMixerPlan(startMs: now),
            onStarted: { _ in },
            onEnded: { _ in },
            onFailed: { _ in })
        mixer.dispose(replacement)
    }

    @Test func interruptResumeFailureIsDistinctAndLeavesGraphReusable() async throws {
        let audio = StubMacOverlayAudio()
        let controller = StubMacInterruptController()
        controller.resumeResult = false
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil))
        mixer.bindInterruptController(controller)
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let clip = preparedOverlay(overlayBuffer(seconds: 1))
        let started = NSLockBox(false)
        let ended = NSLockBox(false)
        let failureCode = NSLockBox<String?>(nil)
        try mixer.arm(
            clip,
            plan: interruptMixerPlan(startMs: now),
            onStarted: { _ in started.withLock { $0 = true } },
            onEnded: { _ in ended.withLock { $0 = true } },
            onFailed: { failure in failureCode.withLock { $0 = failure.code } })
        #expect(await eventuallyOverlay { started.withLock { $0 } })
        audio.finishScheduledBuffer()
        #expect(await eventuallyOverlay { failureCode.withLock { $0 != nil } })
        #expect(failureCode.withLock { $0 } == "audio_graph_failed")
        #expect(!ended.withLock { $0 })
        #expect(audio.stops == 1)

        let replacement = preparedOverlay(overlayBuffer(seconds: 1))
        try mixer.arm(
            replacement,
            plan: overlayMixerPlan(startMs: now),
            onStarted: { _ in },
            onEnded: { _ in },
            onFailed: { _ in })
        mixer.dispose(replacement)
    }

    @Test func activeInterruptCancelFadesAndAcknowledgesOneResume() async throws {
        let audio = StubMacOverlayAudio()
        let controller = StubMacInterruptController()
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil))
        mixer.bindInterruptController(controller)
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let clip = preparedOverlay(overlayBuffer(seconds: 1))
        let started = NSLockBox(false)
        let ended = NSLockBox(false)
        try mixer.arm(
            clip,
            plan: interruptMixerPlan(startMs: now),
            onStarted: { _ in started.withLock { $0 = true } },
            onEnded: { _ in ended.withLock { $0 = true } },
            onFailed: { failure in Issue.record("interrupt failed: \(failure)") })
        #expect(await eventuallyOverlay { started.withLock { $0 } })

        let cancelled = NSLockBox<[Bool]>([])
        mixer.cancel(
            clip,
            command: CancelMediaPayload(
                transmissionId: "tr_macos_interrupt", generation: 1,
                reason: "media_deleted", action: "fade_stop",
                resumeMain: true, fadeMs: 10)
        ) { result in
            if case .success(let resumed) = result {
                cancelled.withLock { $0.append(resumed) }
            }
        }
        #expect(await eventuallyOverlay { cancelled.withLock { $0 == [true] } })
        #expect(controller.resumes.count == 1)
        #expect(!ended.withLock { $0 })
        #expect(audio.overlayGainSnapshot.contains { $0 < 1 })
        #expect(audio.stops == 1)
    }

    @Test func cancelDuringResumeProducesOneCancelledTerminalState() async throws {
        let audio = StubMacOverlayAudio()
        let controller = StubMacInterruptController()
        controller.holdResume = true
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil))
        mixer.bindInterruptController(controller)
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let clip = preparedOverlay(overlayBuffer(seconds: 1))
        let started = NSLockBox(false)
        let ended = NSLockBox(0)
        try mixer.arm(
            clip,
            plan: interruptMixerPlan(startMs: now),
            onStarted: { _ in started.withLock { $0 = true } },
            onEnded: { _ in ended.withLock { $0 += 1 } },
            onFailed: { failure in Issue.record("interrupt failed: \(failure)") })
        #expect(await eventuallyOverlay { started.withLock { $0 } })
        audio.finishScheduledBuffer()
        #expect(await eventuallyOverlay { controller.resumes.count == 1 })

        let cancelled = NSLockBox<[Bool]>([])
        mixer.cancel(
            clip,
            command: CancelMediaPayload(
                transmissionId: "tr_macos_interrupt", generation: 1,
                reason: "coordinator_restarted", action: "fade_stop",
                resumeMain: true, fadeMs: 0)
        ) { result in
            if case .success(let resumed) = result {
                cancelled.withLock { $0.append(resumed) }
            }
        }
        controller.completeHeldResume(true)
        #expect(await eventuallyOverlay { cancelled.withLock { $0 == [true] } })
        #expect(ended.withLock { $0 } == 0)
        controller.completeHeldResume(true)
        try? await Task.sleep(nanoseconds: 10_000_000)
        #expect(cancelled.withLock { $0 } == [true])
    }

    @Test func preparationConvertsDecodedAudioOffRenderToEngineFormat() throws {
        let audio = StubMacOverlayAudio()
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil))
        let inputFormat = AVAudioFormat(
            standardFormatWithSampleRate: 48_000,
            channels: 2)!
        let input = AVAudioPCMBuffer(
            pcmFormat: inputFormat,
            frameCapacity: 4_800)!
        input.frameLength = 4_800
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("mac-overlay-\(UUID().uuidString).wav")
        defer { try? FileManager.default.removeItem(at: url) }
        do {
            let file = try AVAudioFile(forWriting: url, settings: inputFormat.settings)
            try file.write(from: input)
        }

        let prepared = try mixer.prepare(localURL: url, delivery: "overlay")
        let output = try #require(prepared.decoderHandle as? AVAudioPCMBuffer)
        #expect(output.format.sampleRate == 44_100)
        #expect(output.format.channelCount == 2)
        #expect(output.frameLength > 0)
        #expect(prepared.decodedDurationMs == 100)
    }

    @Test func preDuckStartsAtTMinus250AndDisposeRestoresReusableGraph() async throws {
        let audio = StubMacOverlayAudio()
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil),
            nowLocalMs: { now })
        let clip = preparedOverlay(overlayBuffer(seconds: 1))

        try mixer.arm(
            clip,
            plan: overlayMixerPlan(startMs: now + 250),
            onStarted: { _ in Issue.record("future clip must remain armed") },
            onEnded: { _ in Issue.record("disposed clip must not end") },
            onFailed: { _ in Issue.record("disposed clip must not fail") })

        let duck = try #require(audio.musicGainSnapshot.first)
        #expect(abs(duck.0 - Float(pow(10, -12.0 / 20))) < 0.0001)
        #expect(duck.1 == 250)
        mixer.dispose(clip)
        #expect(await eventuallyOverlay { audio.stops == 1 })
        #expect(audio.musicGainSnapshot.last?.0 == 1)
        #expect(audio.musicGainSnapshot.last?.1 == 600)

        let replacement = preparedOverlay(overlayBuffer(seconds: 1))
        try mixer.arm(
            replacement,
            plan: overlayMixerPlan(startMs: now + 250),
            onStarted: { _ in },
            onEnded: { _ in },
            onFailed: { _ in })
        mixer.dispose(replacement)
    }

    @Test func tenSecondOverlayUsesFrozenGainOrderAndLeavesGraphReusable() async throws {
        let audio = StubMacOverlayAudio()
        audio.limiterReductionDB = 2
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil),
            nowLocalMs: { now })
        let buffer = overlayBuffer()
        let clip = preparedOverlay(buffer)
        let started = NSLockBox<[Int64]>([])
        let ended = NSLockBox<[Int64]>([])

        try mixer.arm(
            clip,
            plan: overlayMixerPlan(startMs: now),
            onStarted: { value in started.withLock { $0.append(value) } },
            onEnded: { value in ended.withLock { $0.append(value) } },
            onFailed: { failure in Issue.record("overlay failed: \(failure)") })

        #expect(await eventuallyOverlay { !started.withLock { $0.isEmpty } })
        let overlayStart = try #require(started.withLock { $0.first })
        #expect(abs(overlayStart - now) <= 200)
        #expect(audio.scheduledFrameSnapshot == [buffer.frameLength])
        #expect(mixer.deliveryCapabilities == [overlayMixCapability])
        let duck = try #require(audio.musicGainSnapshot.first)
        #expect(abs(duck.0 - Float(pow(10, -12.0 / 20))) < 0.0001)
        #expect(duck.1 == 0, "late-at-T pre-duck catches up instead of restarting attack")

        try await Task.sleep(nanoseconds: 70_000_000)
        audio.finishScheduledBuffer()
        #expect(await eventuallyOverlay { !ended.withLock { $0.isEmpty } })
        #expect(mixer.overlayFrames == Int64(buffer.frameLength))
        #expect(mixer.limiterHitWindows > 0)
        #expect(audio.musicGainSnapshot.last?.0 == 1)
        #expect(audio.musicGainSnapshot.last?.1 == 600)

        // Natural completion clears ownership immediately while the main-only
        // release ramp continues, so the next prepared clip can arm.
        let next = preparedOverlay(overlayBuffer(seconds: 1))
        try mixer.arm(
            next,
            plan: overlayMixerPlan(startMs: now),
            onStarted: { _ in },
            onEnded: { _ in },
            onFailed: { _ in })
    }

    @Test func cancellationFadesOverlayReleasesDuckAndAcknowledgesOnce() async throws {
        let audio = StubMacOverlayAudio()
        let now = Int64((Date().timeIntervalSince1970 * 1_000).rounded())
        let mixer = MacOverlayMediaClipMixer(
            audio: audio,
            log: Logger(level: .error, path: nil),
            nowLocalMs: { Int64((Date().timeIntervalSince1970 * 1_000).rounded()) })
        let clip = preparedOverlay(overlayBuffer(seconds: 1))
        let started = NSLockBox(false)
        try mixer.arm(
            clip,
            plan: overlayMixerPlan(startMs: now, releaseMs: 30),
            onStarted: { _ in started.withLock { $0 = true } },
            onEnded: { _ in Issue.record("cancelled overlay must not report ended") },
            onFailed: { failure in Issue.record("overlay failed: \(failure)") })
        #expect(await eventuallyOverlay { started.withLock { $0 } })

        let cancelled = NSLockBox(0)
        mixer.cancel(
            clip,
            command: CancelMediaPayload(
                transmissionId: "tr_macos_overlay", generation: 1,
                reason: "media_deleted", action: "fade_stop",
                resumeMain: false, fadeMs: 10)
        ) { result in
            if case .success(false) = result {
                cancelled.withLock { $0 += 1 }
            }
        }
        #expect(await eventuallyOverlay { cancelled.withLock { $0 == 1 } })
        let gains = audio.overlayGainSnapshot
        #expect(gains.first == 1)
        #expect(gains.contains { $0 < 1 })
        #expect(gains.last == 1, "graph resets overlay gain for the next clip")
        #expect(audio.musicGainSnapshot.last?.0 == 1)
        #expect(audio.musicGainSnapshot.last?.1 == 30)
        #expect(audio.stops == 1)

        let replacement = preparedOverlay(overlayBuffer(seconds: 1))
        try mixer.arm(
            replacement,
            plan: overlayMixerPlan(startMs: now),
            onStarted: { _ in },
            onEnded: { _ in },
            onFailed: { _ in })
    }
}

private final class NSLockBox<Value>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: Value

    init(_ value: Value) { self.value = value }

    func withLock<Result>(_ body: (inout Value) -> Result) -> Result {
        lock.lock()
        defer { lock.unlock() }
        return body(&value)
    }
}
