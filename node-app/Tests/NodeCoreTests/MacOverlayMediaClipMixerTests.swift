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
    var stops: Int { lock.withLock { stopCount } }
}

private func overlayBuffer(seconds: Int = 10) -> AVAudioPCMBuffer {
    let format = AVAudioFormat(standardFormatWithSampleRate: 44_100, channels: 2)!
    let frames = AVAudioFrameCount(44_100 * seconds)
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
            onEnded: { _ in Issue.record("disposed clip must not end") })

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
            onEnded: { _ in })
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
            onEnded: { value in ended.withLock { $0.append(value) } })

        #expect(await eventuallyOverlay { !started.withLock { $0.isEmpty } })
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
            onEnded: { _ in })
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
            onEnded: { _ in Issue.record("cancelled overlay must not report ended") })
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
            onEnded: { _ in })
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
