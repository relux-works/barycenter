import Foundation
import Testing
@testable import NodeCore

private final class LiveNodeSender: MacLiveCaptureSending, @unchecked Sendable {
    var onEvent: (@Sendable (MacLiveCaptureEvent) -> Void)?
    private let lock = NSLock()
    private var phase: MacLiveCapturePhase = .idle
    private var generation: UInt64 = 0
    var accepted: [(LivePTTStartPayload, UInt64)] = []
    var stops: [MacLiveCaptureStopReason] = []

    func currentPhase() -> MacLiveCapturePhase { lock.withLock { phase } }
    func localHoldBegan(
        source: MacLiveHoldSource,
        holdCapabilityAvailable: Bool,
        selectedDeviceID: String?
    ) -> UInt64? {
        guard holdCapabilityAvailable else {
            onEvent?(.fallbackToClip)
            return nil
        }
        let value = lock.withLock { () -> UInt64 in
            guard phase == .idle else { return 0 }
            generation += 1
            phase = .awaitingStart
            return generation
        }
        guard value > 0 else { return nil }
        onEvent?(.phase(.awaitingStart))
        onEvent?(.requestStart(localGeneration: value, source: source))
        return value
    }
    func localHoldHeartbeat(generation: UInt64) {}
    func acceptStart(
        _ payload: LivePTTStartPayload,
        localGeneration: UInt64,
        authorized: Bool
    ) async throws {
        guard authorized, lock.withLock({ phase == .awaitingStart }) else {
            throw MacLiveEncodeError.invalidFrame
        }
        lock.withLock {
            accepted.append((payload, localGeneration))
            phase = .capturing
        }
        onEvent?(.phase(.capturing))
    }
    func localHoldEnded(generation: UInt64) { terminate(.released) }
    func localStop() { terminate(.localStop) }
    func handleSystemSleep() { terminate(.systemSleep) }
    func handleSessionLock() { terminate(.sessionLocked) }
    func handleDisconnect() { terminate(.disconnected) }
    func handleCoordinatorCancel() { terminate(.coordinatorCancelled) }
    func recheckPermission() {}
    func shutdown() { terminate(.appQuit) }
    func acceptedSnapshot() -> [(LivePTTStartPayload, UInt64)] {
        lock.withLock { accepted }
    }

    private func terminate(_ reason: MacLiveCaptureStopReason) {
        let active = lock.withLock { () -> Bool in
            guard phase != .idle else { return false }
            phase = .idle
            stops.append(reason)
            return true
        }
        if active {
            onEvent?(.terminal(reason))
            onEvent?(.phase(.idle))
        }
    }
}

private final class LiveNodeReceiver: MacLiveJitterReceiving, @unchecked Sendable {
    private let lock = NSLock()
    var phase: MacLiveJitterSnapshot.Phase = .idle
    var startPayload: LivePTTStartPayload?
    var frames: [LivePTTBinaryFrame] = []
    var revocations: [String] = []

    func start(_ payload: LivePTTStartPayload, authorized: Bool) -> Bool {
        lock.withLock {
            guard authorized, phase == .idle else { return false }
            startPayload = payload
            phase = .buffering
            return true
        }
    }
    func receive(_ frame: LivePTTBinaryFrame) -> LivePTTFrameDecision {
        lock.withLock {
            guard phase != .idle, frame.sessionId == sessionBytes else { return .stale }
            frames.append(frame)
            if frames.count >= 3 { phase = .playing }
            return .apply
        }
    }
    func end(_ payload: LivePTTEndPayload) {
        lock.withLock {
            if payload.sessionId == startPayload?.sessionId { phase = .idle; startPayload = nil }
        }
    }
    func cancel(_ payload: LivePTTCancelPayload) {
        lock.withLock {
            if payload.sessionId == startPayload?.sessionId { phase = .idle; startPayload = nil }
        }
    }
    func revoke(reason: String) {
        lock.withLock {
            revocations.append(reason)
            phase = .idle
            startPayload = nil
        }
    }
    func snapshot() -> MacLiveJitterSnapshot {
        lock.withLock {
            MacLiveJitterSnapshot(
                phase: phase, sessionId: startPayload?.sessionId,
                generation: startPayload?.generation,
                expectedSequence: UInt32(frames.count + 1),
                highestSequence: UInt32(frames.count),
                encodedFrames: 0, encodedBytes: 0, pcmFrames: 0,
                pcmCapacityFrames: 3_840, receivedFrames: frames.count,
                decodedFrames: frames.count, duplicateFrames: 0, lateFrames: 0,
                fecFrames: 0, plcFrames: 0, failedFrames: 0,
                underrunCallbacks: 0)
        }
    }
    func revocationsSnapshot() -> [String] { lock.withLock { revocations } }
    private var sessionBytes: [UInt8] {
        guard let value = startPayload?.sessionId else { return [] }
        return stride(from: 0, to: 32, by: 2).compactMap { offset in
            let start = value.index(value.startIndex, offsetBy: offset)
            let end = value.index(start, offsetBy: 2)
            return UInt8(value[start..<end], radix: 16)
        }
    }
}

private final class LiveNodeBox: @unchecked Sendable {
    let lock = NSLock()
    var messages: [Message] = []
    var statuses: [MacLivePTTNodeStatus] = []
    func send(_ message: Message) { lock.withLock { messages.append(message) } }
    func status(_ value: MacLivePTTNodeStatus) { lock.withLock { statuses.append(value) } }
}

private func liveNodeStart(generation: Int64 = 7) -> LivePTTStartPayload {
    LivePTTStartPayload(
        sessionId: "00112233445566778899aabbccddeeff", generation: generation,
        senderActorId: 11, senderOrbitId: 12, senderNodeId: "mac-a",
        targetSnapshot: "lts1.fixture", targetSha256: String(repeating: "a", count: 64),
        targetCount: 2, playbackDomain: "personal", playbackDomainId: 11,
        codecProfile: LivePTTConstants.codecProfile, frameMs: 20,
        maxPayloadBytes: 400, jitterBufferMs: 60, startedAtCoordMs: 1_000,
        acceptDeadlineCoordMs: 2_500, maxDurationMs: 300_000,
        mixedVersionPolicy: "require_all",
        lateJoinPolicy: LivePTTConstants.lateJoinPolicy,
        captureAuthority: LivePTTConstants.captureAuthority)
}

private func liveNodeFrame(_ sequence: UInt32) throws -> LivePTTBinaryFrame {
    let session = stride(from: 0, to: 32, by: 2).map { offset -> UInt8 in
        let value = "00112233445566778899aabbccddeeff"
        let start = value.index(value.startIndex, offsetBy: offset)
        return UInt8(value[start..<value.index(start, offsetBy: 2)], radix: 16)!
    }
    return LivePTTBinaryFrame(
        flags: LivePTTBinaryFrame.fecFlag |
            (sequence == 1 ? LivePTTBinaryFrame.startFlag : 0),
        sessionId: session, sequence: sequence,
        captureMonotonicUs: 2_000_000 + UInt64(sequence - 1) * 20_000,
        payload: Data([0xf8, 0xff, UInt8(sequence)]))
}

private func makeLiveNode(
    enabled: Bool = true,
    decision: MacLivePTTIncomingDecision = .allow
) -> (MacLivePTTNode, LiveNodeSender, LiveNodeReceiver, LiveNodeBox) {
    let sender = LiveNodeSender(), receiver = LiveNodeReceiver(), box = LiveNodeBox()
    let node = MacLivePTTNode(
        sender: sender, receiver: receiver,
        featureEnabled: { enabled },
        prepareStart: { _, _ in liveNodeStart() },
        authorizeIncoming: { _ in decision },
        coordinatorNowMs: { 1_100 }, send: { [box] message in box.send(message) })
    node.onStatus = { [box] status in box.status(status) }
    return (node, sender, receiver, box)
}

private func waitLiveNode(_ condition: @escaping () -> Bool) async throws {
    // Swift Testing runs suites concurrently. Hosted macOS runners can leave
    // these private serial queues unscheduled for more than two seconds while
    // the full audio/crypto matrix starts, even though the operation itself is
    // nonblocking. Keep deadlock detection bounded without treating scheduler
    // contention as a product failure.
    let deadline = ContinuousClock.now + .seconds(10)
    while ContinuousClock.now < deadline {
        if condition() { return }
        try await Task.sleep(for: .milliseconds(5))
    }
    Issue.record("timed out waiting for live node")
}

@Suite(.serialized) struct MacLivePTTNodeTests {
    @Test func disabledCapabilityFallsBackAndRejectsIncomingBeforeCapture() async throws {
        let (node, sender, receiver, box) = makeLiveNode(enabled: false)
        #expect(node.holdBegan(
            source: .shortcut, holdAvailable: true, selectedDeviceID: nil) == nil)
        try await waitLiveNode { node.snapshot().fallbackToClip }
        node.handle(.livePTTStart(liveNodeStart()))
        try await waitLiveNode { box.lock.withLock { box.messages.count == 1 } }
        box.lock.withLock {
            guard case .livePTTReject(let payload) = box.messages[0] else {
                Issue.record("missing unsupported rejection"); return
            }
            #expect(payload.code == "unsupported")
            #expect((try? LivePTTValidation.validate(.livePTTReject(payload))) != nil)
        }
        #expect(sender.currentPhase() == .idle)
        #expect(receiver.snapshot().phase == .idle)
    }

    @Test func outgoingHoldStartsOnlyAfterMatchingAcceptAndRejectsConcurrentReceive() async throws {
        let (node, sender, receiver, box) = makeLiveNode()
        let local = try #require(node.holdBegan(
            source: .button, holdAvailable: true, selectedDeviceID: "mic"))
        try await waitLiveNode { box.lock.withLock { box.messages.count == 1 } }
        box.lock.withLock {
            guard case .livePTTStart(let payload) = box.messages[0] else {
                Issue.record("missing start"); return
            }
            #expect(payload == liveNodeStart())
        }
        node.handle(.livePTTAccept(LivePTTAcceptPayload(
            sessionId: liveNodeStart().sessionId, generation: 6, eventSequence: 1,
            acceptedAtCoordMs: 1_101, liveEdgeSequence: 1, bufferFrames: 3)))
        try await Task.sleep(for: .milliseconds(20))
        #expect(sender.acceptedSnapshot().isEmpty)

        node.handle(.livePTTAccept(LivePTTAcceptPayload(
            sessionId: liveNodeStart().sessionId, generation: 7, eventSequence: 1,
            acceptedAtCoordMs: 1_101, liveEdgeSequence: 1, bufferFrames: 3)))
        try await waitLiveNode { sender.currentPhase() == .capturing }
        #expect(sender.acceptedSnapshot().count == 1)
        #expect(sender.acceptedSnapshot()[0].1 == local)
        node.handle(.livePTTStart(liveNodeStart(generation: 8)))
        try await waitLiveNode { box.lock.withLock { box.messages.count == 2 } }
        box.lock.withLock {
            guard case .livePTTReject(let payload) = box.messages[1] else {
                Issue.record("missing busy rejection"); return
            }
            #expect(payload.code == "busy")
        }
        #expect(receiver.snapshot().phase == .idle)
        node.holdEnded(generation: local)
        try await waitLiveNode { node.snapshot().direction == .idle }
    }

    @Test func incomingPolicyBinaryAndTerminalPathsStayGenerationBound() async throws {
        let denied = makeLiveNode(decision: .reject("dnd"))
        denied.0.handle(.livePTTStart(liveNodeStart()))
        try await waitLiveNode { denied.3.lock.withLock { denied.3.messages.count == 1 } }
        denied.3.lock.withLock {
            guard case .livePTTReject(let payload) = denied.3.messages[0] else {
                Issue.record("missing dnd rejection"); return
            }
            #expect(payload.code == "dnd")
        }

        let (node, _, receiver, _) = makeLiveNode()
        node.handle(.livePTTStart(liveNodeStart()))
        try await waitLiveNode { receiver.snapshot().phase == .buffering }
        #expect(node.holdBegan(
            source: .button, holdAvailable: true, selectedDeviceID: nil) == nil)
        for sequence: UInt32 in 1...3 {
            node.handleBinary(try liveNodeFrame(sequence).encoded())
        }
        try await waitLiveNode { receiver.snapshot().phase == .playing }
        #expect(node.snapshot().phase == .playing)
        node.handle(.livePTTEnd(LivePTTEndPayload(
            sessionId: liveNodeStart().sessionId, generation: 7,
            commandSequence: 1, lastSequence: 3, endedAtCoordMs: 1_100,
            drainDeadlineCoordMs: 1_700, reason: "release")))
        try await waitLiveNode { receiver.snapshot().phase == .idle }
        #expect(node.snapshot().direction == .idle)
    }

    @Test func sleepLockDisconnectRollbackAndQuitCleanBothDirections() async throws {
        let actions: [(MacLivePTTNode) -> Void] = [
            { $0.handleSystemSleep() }, { $0.handleSessionLock() },
            { $0.handleDisconnect() }, { $0.rollbackFeature() }, { $0.shutdown() },
        ]
        for action in actions {
            let (node, sender, receiver, _) = makeLiveNode()
            node.handle(.livePTTStart(liveNodeStart()))
            try await waitLiveNode { receiver.snapshot().phase == .buffering }
            action(node)
            try await waitLiveNode {
                receiver.snapshot().phase == .idle && node.snapshot().direction == .idle
            }
            #expect(sender.currentPhase() == .idle)
            #expect(!receiver.revocationsSnapshot().isEmpty)
        }
    }

    @Test func websocketBinaryClassificationAndProductionCapabilityStayFailClosed() throws {
        let frame = try liveNodeFrame(1).encoded()
        #expect(CoordinatorClient.isLivePTTBinary(frame))
        #expect(!CoordinatorClient.isLivePTTBinary(Data(#"{"v":1}"#.utf8)))
        var malformed = frame
        malformed[2] = 9
        #expect(CoordinatorClient.isLivePTTBinary(malformed))
        #expect(throws: LivePTTProtocolError.self) {
            _ = try LivePTTBinaryFrame.decode(malformed)
        }

        let source = try String(contentsOf: URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources/NodeApp/main.swift"))
        #expect(!source.contains("livePTTCapability"))
        #expect(!source.contains("MacLivePTTNode("))
    }

    @Test func statusDeliveryCanReenterTheSerializedSnapshotWithoutDeadlock() async throws {
        let (node, _, _, box) = makeLiveNode()
        node.onStatus = { [node, box] value in
            _ = node.snapshot()
            box.status(value)
        }
        _ = node.holdBegan(
            source: .button, holdAvailable: false, selectedDeviceID: nil)
        try await waitLiveNode { box.lock.withLock { !box.statuses.isEmpty } }
        #expect(node.snapshot().phase == .fallback)
    }
}
