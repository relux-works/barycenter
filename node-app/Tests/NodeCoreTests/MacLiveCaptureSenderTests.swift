import Foundation
import Testing
@testable import NodeCore

private final class LiveCapturePermission: MacMicrophonePermissionAuthorizing {
    var value: MacMicrophonePermission = .granted
    func currentPermission() -> MacMicrophonePermission { value }
    func requestPermission() async -> MacMicrophonePermission { value }
}

private final class LiveCaptureBackend: MacMicrophoneCaptureBackend, @unchecked Sendable {
    var samples: (@Sendable ([Float]) -> Void)?
    var failure: (@Sendable () -> Void)?
    var starts = 0, stops = 0
    var devices = [MacCaptureDevice(id: "mic", name: "Mic", isDefault: true)]
    func availableDevices() -> [MacCaptureDevice] { devices }
    func start(selectedDeviceID: String?, onSamples: @escaping @Sendable ([Float]) -> Void,
               onFailure: @escaping @Sendable () -> Void) throws {
        starts += 1; samples = onSamples; failure = onFailure
    }
    func stop() { if samples != nil { stops += 1 }; samples = nil; failure = nil }
    func emit(_ value: [Float]) { samples?(value) }
    func fail() { failure?() }
}

private final class LiveCaptureEncoder: MacLiveOpusEncoding {
    var calls = 0
    func encode(samples: UnsafeBufferPointer<Float>,
                into output: UnsafeMutableRawBufferPointer) throws -> Int {
        calls += 1; output[0] = UInt8(truncatingIfNeeded: calls); output[1] = 0x5a
        return 2
    }
    func reset() {}
}

private final class LiveCaptureBox: @unchecked Sendable {
    let lock = NSLock(); var frames: [LivePTTBinaryFrame] = []; var controls: [Message] = []
    var acceptFrames = true
    func send(_ frame: LivePTTBinaryFrame) -> Bool { lock.withLock {
        if acceptFrames { frames.append(frame) }; return acceptFrames
    }}
    func control(_ message: Message) { lock.withLock { controls.append(message) } }
}

private func senderStart(generation: Int64 = 9, now: Int64 = 1_000) -> LivePTTStartPayload {
    LivePTTStartPayload(
        sessionId: "00112233445566778899aabbccddeeff", generation: generation,
        senderActorId: 1, senderOrbitId: 2, senderNodeId: "mac",
        targetSnapshot: "lts1.sender", targetSha256: String(repeating: "a", count: 64),
        targetCount: 1, playbackDomain: "personal", playbackDomainId: 1,
        codecProfile: LivePTTConstants.codecProfile, frameMs: 20, maxPayloadBytes: 400,
        jitterBufferMs: 60, startedAtCoordMs: now, acceptDeadlineCoordMs: now + 1_500,
        maxDurationMs: 300_000, mixedVersionPolicy: "require_all",
        lateJoinPolicy: LivePTTConstants.lateJoinPolicy,
        captureAuthority: LivePTTConstants.captureAuthority)
}

private final class LiveCaptureFixture {
    let permission = LiveCapturePermission(), backend = LiveCaptureBackend()
    let encoder = LiveCaptureEncoder(), box = LiveCaptureBox()
    let clock = LockedClock()
    lazy var sender = MacLiveCaptureSender(
            permission: permission, backend: backend, encoder: encoder,
            eventQueue: DispatchQueue(label: "live-capture-events"),
            coordinatorNowMs: { [clock] in clock.value }, monotonicUs: { 2_000_000 },
            trySendFrame: box.send, sendControl: box.control)
}

private final class LockedClock: @unchecked Sendable {
    private let lock = NSLock()
    private var stored: Int64 = 1_001
    var value: Int64 {
        get { lock.withLock { stored } }
        set { lock.withLock { stored = newValue } }
    }
}

@Suite struct MacLiveCaptureSenderTests {
    @Test func onlyCurrentLocalHoldCanOpenMicrophoneAndFallbackStaysClipOnly() async throws {
        let f = LiveCaptureFixture()
        #expect(f.sender.localHoldBegan(
            source: .shortcut, holdCapabilityAvailable: false, selectedDeviceID: nil) == nil)
        await #expect(throws: MacLiveEncodeError.invalidFrame) {
            try await f.sender.acceptStart(senderStart(), localGeneration: 1, authorized: true)
        }
        #expect(f.backend.starts == 0)
        let generation = try #require(f.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: "mic"))
        #expect(f.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil) == nil)
        try await f.sender.acceptStart(senderStart(), localGeneration: generation, authorized: true)
        #expect(f.backend.starts == 1)
        f.sender.localHoldEnded(generation: generation)
        #expect(f.sender.snapshot().phase == .idle)
    }

    @Test func deniedPermissionCancelsAcceptedSessionWithoutOpeningBackend() async throws {
        let f = LiveCaptureFixture()
        f.permission.value = .denied
        let generation = try #require(f.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        await #expect(throws: MacCaptureEngineError.permissionDenied) {
            try await f.sender.acceptStart(
                senderStart(), localGeneration: generation, authorized: true)
        }
        #expect(f.sender.snapshot().phase == .idle)
        #expect(f.backend.starts == 0)
        #expect(endReason(in: f.box) == .permissionRevoked)
    }

    @Test func framesAreBoundedOrderedAndLastFrameIsTerminal() async throws {
        let f = LiveCaptureFixture()
        let generation = try #require(f.sender.localHoldBegan(
            source: .menu, holdCapabilityAvailable: true, selectedDeviceID: nil))
        try await f.sender.acceptStart(senderStart(), localGeneration: generation, authorized: true)
        f.backend.emit([Float](repeating: 0.2, count: 2_880))
        #expect(f.sender.snapshot().sequence == 3)
        f.sender.localHoldEnded(generation: generation)
        #expect(f.sender.snapshot().phase == .idle)
        let frames = f.box.lock.withLock { f.box.frames }
        #expect(frames.map(\.sequence) == [1, 2, 3])
        #expect(frames.allSatisfy { $0.payload.count <= 400 })
        #expect(frames[0].flags & LivePTTBinaryFrame.startFlag != 0)
        #expect(frames[2].flags & LivePTTBinaryFrame.endFlag != 0)
        #expect(frames.map(\.captureMonotonicUs) == [2_000_000, 2_020_000, 2_040_000])
        #expect(f.backend.stops == 1)
        #expect(f.box.lock.withLock { f.box.controls }.contains {
            if case .livePTTEnd(let value) = $0 { return value.lastSequence == 3 }
            return false
        })
    }

    @Test func saturationAndDeviceFailureCannotLeaveCaptureRunning() async throws {
        let f = LiveCaptureFixture(); f.box.acceptFrames = false
        let generation = try #require(f.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        try await f.sender.acceptStart(senderStart(), localGeneration: generation, authorized: true)
        for _ in 0..<12 { f.backend.emit([Float](repeating: 0.1, count: 960)) }
        #expect(f.sender.snapshot().phase == .idle)
        #expect(!f.sender.snapshot().backendActive)
        #expect(f.backend.stops == 1)

        let next = try #require(f.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        try await f.sender.acceptStart(senderStart(generation: 10), localGeneration: next, authorized: true)
        f.backend.fail(); #expect(f.sender.snapshot().phase == .idle)
        #expect(f.backend.stops == 2)
    }

    @Test func oneHundredCyclesAndStaleReleasesAreIdempotent() async throws {
        let f = LiveCaptureFixture()
        var prior: UInt64 = 0
        for cycle in 0..<100 {
            let generation = try #require(f.sender.localHoldBegan(
                source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
            if prior > 0 { f.sender.localHoldEnded(generation: prior) }
            try await f.sender.acceptStart(
                senderStart(generation: Int64(cycle + 1)),
                localGeneration: generation, authorized: true)
            f.backend.emit([Float](repeating: 0.05, count: 1_920))
            f.sender.localHoldEnded(generation: generation)
            #expect(f.sender.snapshot().phase == .idle)
            #expect(!f.sender.snapshot().backendActive)
            prior = generation
        }
        #expect(f.backend.starts == 100); #expect(f.backend.stops == 100)
    }

    @Test func watchdogAndSystemLifecycleEventsAlwaysCloseMicrophone() async throws {
        let lost = LiveCaptureFixture()
        let lostGeneration = try #require(lost.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        try await lost.sender.acceptStart(
            senderStart(), localGeneration: lostGeneration, authorized: true)
        lost.clock.value = 2_502
        lost.sender.runWatchdogCheck()
        #expect(lost.sender.snapshot().phase == .idle)
        #expect(lost.backend.stops == 1)
        #expect(endReason(in: lost.box) == .lostRelease)

        let permission = LiveCaptureFixture()
        let permissionGeneration = try #require(permission.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        try await permission.sender.acceptStart(
            senderStart(), localGeneration: permissionGeneration, authorized: true)
        permission.permission.value = .denied
        permission.sender.recheckPermission()
        #expect(permission.sender.snapshot().phase == .idle)
        #expect(endReason(in: permission.box) == .permissionRevoked)

        for action in [
            { (sender: MacLiveCaptureSender) in sender.handleSystemSleep() },
            { (sender: MacLiveCaptureSender) in sender.handleSessionLock() }
        ] {
            let f = LiveCaptureFixture()
            let generation = try #require(f.sender.localHoldBegan(
                source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
            try await f.sender.acceptStart(
                senderStart(), localGeneration: generation, authorized: true)
            action(f.sender)
            #expect(f.sender.snapshot().phase == .idle)
            #expect(f.backend.stops == 1)
        }

        let disconnected = LiveCaptureFixture()
        let disconnectGeneration = try #require(disconnected.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        try await disconnected.sender.acceptStart(
            senderStart(), localGeneration: disconnectGeneration, authorized: true)
        disconnected.sender.handleDisconnect()
        #expect(disconnected.sender.snapshot().phase == .idle)
        #expect(disconnected.backend.stops == 1)
        #expect(disconnected.box.lock.withLock { disconnected.box.controls }.isEmpty)
    }

    @Test func systemEncoderProducesRepeatedBoundedRawOpusPackets() throws {
        let encoder = try #require(MacAVAudioOpusEncoder())
        let samples = (0..<960).map { index in
            sin(Float(index) * 2 * .pi * 440 / 48_000) * 0.2
        }
        let output = UnsafeMutableRawPointer.allocate(byteCount: 400, alignment: 16)
        defer { output.deallocate() }
        for _ in 0..<2 {
            let count = try samples.withUnsafeBufferPointer { source in
                try encoder.encode(samples: source,
                    into: UnsafeMutableRawBufferPointer(start: output, count: 400))
            }
            #expect((1...400).contains(count))
        }
    }

    @Test func everyEmittedTerminalControlSatisfiesTheFrozenWireContract() async throws {
        let fixtures = [LiveCaptureFixture(), LiveCaptureFixture(), LiveCaptureFixture()]
        let actions: [(LiveCaptureFixture, UInt64) -> Void] = [
            { fixture, generation in fixture.sender.localHoldEnded(generation: generation) },
            { fixture, _ in fixture.sender.localStop() },
            { fixture, _ in fixture.sender.handleSystemSleep() },
        ]
        for (fixture, action) in zip(fixtures, actions) {
            let generation = try #require(fixture.sender.localHoldBegan(
                source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
            try await fixture.sender.acceptStart(
                senderStart(), localGeneration: generation, authorized: true)
            fixture.backend.emit([Float](repeating: 0.1, count: 960))
            action(fixture, generation)
            _ = fixture.sender.snapshot()
            let controls = fixture.box.lock.withLock { fixture.box.controls }
            let terminal = try #require(controls.last)
            #expect((try? LivePTTValidation.validate(terminal)) != nil)
        }

        let denied = LiveCaptureFixture()
        denied.permission.value = .denied
        let generation = try #require(denied.sender.localHoldBegan(
            source: .button, holdCapabilityAvailable: true, selectedDeviceID: nil))
        await #expect(throws: MacCaptureEngineError.permissionDenied) {
            try await denied.sender.acceptStart(
                senderStart(), localGeneration: generation, authorized: true)
        }
        let failure = try #require(denied.box.lock.withLock { denied.box.controls.last })
        #expect((try? LivePTTValidation.validate(failure)) != nil)
    }
}

private func endReason(in box: LiveCaptureBox) -> MacLiveCaptureStopReason? {
    let value = box.lock.withLock { box.controls }.compactMap { message -> String? in
        if case .livePTTEnd(let payload) = message { return payload.reason }
        if case .livePTTCancel(let payload) = message { return payload.reason }
        if case .livePTTFailed(let payload) = message { return payload.code }
        return nil
    }.last
    switch value {
    case "release": return MacLiveCaptureStopReason.released
    case "sleep": return MacLiveCaptureStopReason.systemSleep
    case "lock": return MacLiveCaptureStopReason.sessionLocked
    case "disconnect": return MacLiveCaptureStopReason.disconnected
    case "quit": return MacLiveCaptureStopReason.appQuit
    case "timeout": return MacLiveCaptureStopReason.maximumDuration
    case "user_cancel": return MacLiveCaptureStopReason.localStop
    default: return value.flatMap(MacLiveCaptureStopReason.init(rawValue:))
    }
}
