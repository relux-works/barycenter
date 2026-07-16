import Foundation
import Testing
@testable import NodeCore

private final class LiveRouteFixture: MacLiveAudioRouting {
    let livePCMCapacityFrames: Int
    var pcm: [Float] = []
    var active = false
    var generation: Int64 = 0
    var stops: [(generation: Int64, discard: Bool)] = []
    var livePCMUnderrunCallbacks: Int64 = 0
    var livePCMBufferedFrames: Int { pcm.count }

    init(capacity: Int = 15_360) { livePCMCapacityFrames = capacity }

    func prepareLivePCM() -> Int64 {
        generation += 1
        pcm.removeAll(keepingCapacity: true)
        active = false
        return generation
    }

    func activateLivePCM(generation: Int64) {
        if generation == self.generation { active = true }
    }

    func writeLivePCM(
        generation: Int64,
        samples: UnsafePointer<Float>,
        count: Int
    ) -> Int {
        guard generation == self.generation,
              pcm.count + count <= livePCMCapacityFrames
        else { return 0 }
        pcm.append(contentsOf: UnsafeBufferPointer(start: samples, count: count))
        return count
    }

    func stopLivePCM(generation: Int64, discard: Bool) {
        guard generation == self.generation else { return }
        stops.append((generation, discard))
        active = false
        if discard { pcm.removeAll(keepingCapacity: true) }
    }

    func consumeAll() { pcm.removeAll(keepingCapacity: true) }
}

private final class LiveDecoderFixture: MacLiveOpusDecoding {
    let supportsFEC: Bool
    var calls: [(byte: UInt8, fec: Bool)] = []

    init(supportsFEC: Bool) { self.supportsFEC = supportsFEC }

    func decode(
        packet: Data,
        fec: Bool,
        into output: UnsafeMutableBufferPointer<Float>
    ) throws -> Int {
        if fec && !supportsFEC { throw MacLiveDecodeError.fecUnavailable }
        guard let byte = packet.first else { throw MacLiveDecodeError.invalidPacket }
        calls.append((byte, fec))
        output.baseAddress!.update(
            repeating: Float(byte) / 255, count: output.count)
        return 960
    }

    func reset() {}
}

private let liveSession = "00112233445566778899aabbccddeeff"

private func liveStart(
    generation: Int64 = 7,
    now: Int64 = 1_000
) -> LivePTTStartPayload {
    LivePTTStartPayload(
        sessionId: liveSession,
        generation: generation,
        senderActorId: 11,
        senderOrbitId: 12,
        senderNodeId: "mac-a",
        targetSnapshot: "lts1.fixture",
        targetSha256: String(repeating: "a", count: 64),
        targetCount: 1,
        playbackDomain: "personal",
        playbackDomainId: 11,
        codecProfile: LivePTTConstants.codecProfile,
        frameMs: 20,
        maxPayloadBytes: 400,
        jitterBufferMs: 60,
        startedAtCoordMs: now,
        acceptDeadlineCoordMs: now + 1_500,
        maxDurationMs: 300_000,
        mixedVersionPolicy: "require_all",
        lateJoinPolicy: LivePTTConstants.lateJoinPolicy,
        captureAuthority: LivePTTConstants.captureAuthority)
}

private func liveFrame(_ sequence: UInt32) -> LivePTTBinaryFrame {
    var sessionBytes: [UInt8] = []
    var cursor = liveSession.startIndex
    while cursor < liveSession.endIndex {
        let end = liveSession.index(cursor, offsetBy: 2)
        sessionBytes.append(UInt8(liveSession[cursor..<end], radix: 16)!)
        cursor = end
    }
    var flags = LivePTTBinaryFrame.fecFlag
    if sequence == 1 { flags |= LivePTTBinaryFrame.startFlag }
    return LivePTTBinaryFrame(
        flags: flags,
        sessionId: sessionBytes,
        sequence: sequence,
        captureMonotonicUs: 1_000_000 + UInt64(sequence - 1) * 20_000,
        payload: Data([UInt8(truncatingIfNeeded: sequence), 0x5a]))
}

private func makeReceiver(
    route: LiveRouteFixture,
    decoder: LiveDecoderFixture,
    now: @escaping () -> Int64 = { 1_001 },
    events: EventBox = EventBox()
) -> MacLiveJitterReceiver {
    return MacLiveJitterReceiver(
        route: route,
        decoder: decoder,
        automaticTick: false,
        coordinatorNowMs: now,
        send: { events.events.append($0) })
}

private final class EventBox {
    var events: [Message] = []
}

@Suite struct MacLiveJitterReceiverTests {
    @Test func authorizationBusyAndGenerationAreFailClosed() {
        let route = LiveRouteFixture()
        let decoder = LiveDecoderFixture(supportsFEC: true)
        let receiver = makeReceiver(route: route, decoder: decoder)

        #expect(!receiver.start(liveStart(), authorized: false))
        #expect(receiver.start(liveStart(), authorized: true))
        #expect(!receiver.start(liveStart(generation: 8), authorized: true))
        receiver.revoke(reason: "dnd")
        #expect(!receiver.start(liveStart(generation: 7), authorized: true))
        #expect(receiver.snapshot().phase == .idle)
    }

    @Test func frozenWindowReordersAndUsesFECBeforeAudibleStart() {
        let route = LiveRouteFixture()
        let decoder = LiveDecoderFixture(supportsFEC: true)
        let events = EventBox()
        let receiver = makeReceiver(route: route, decoder: decoder, events: events)
        #expect(receiver.start(liveStart(), authorized: true))

        #expect(receiver.receive(liveFrame(1)) == .apply)
        #expect(receiver.receive(liveFrame(1)) == .duplicate)
        #expect(receiver.receive(liveFrame(3)) == .apply)
        let snapshot = receiver.snapshot()
        #expect(snapshot.phase == .playing)
        #expect(snapshot.decodedFrames == 3)
        #expect(snapshot.fecFrames == 1)
        #expect(snapshot.duplicateFrames == 1)
        #expect(snapshot.encodedFrames == 0)
        #expect(route.active)
        #expect(route.pcm.count == 2_880)
        #expect(decoder.calls.map(\.fec) == [false, true, false])
        #expect(receiver.receive(liveFrame(2)) == .stale)
        #expect(events.events.contains {
            if case .livePTTAccept = $0 { return true }; return false
        })
        #expect(events.events.contains {
            if case .livePTTReceipt(let payload) = $0 {
                return payload.state == "audible_started" && payload.lastSequence == 3
            }
            return false
        })
    }

    @Test func malformedConflictAndOutOfWindowFramesNeverReachDecoder() {
        let route = LiveRouteFixture()
        let decoder = LiveDecoderFixture(supportsFEC: true)
        let receiver = makeReceiver(route: route, decoder: decoder)
        #expect(receiver.start(liveStart(), authorized: true))
        #expect(receiver.receive(liveFrame(1)) == .apply)

        var conflict = liveFrame(1)
        conflict.payload = Data([0x7f])
        #expect(receiver.receive(conflict) == .stale)
        #expect(receiver.receive(liveFrame(10)) == .invalid)
        var badClock = liveFrame(2)
        badClock.captureMonotonicUs += 1
        #expect(receiver.receive(badClock) == .invalid)
        #expect(decoder.calls.isEmpty)
        #expect(receiver.snapshot().encodedFrames == 1)
    }

    @Test func twoPercentLossUsesBoundedPLCWithoutStall() {
        let route = LiveRouteFixture()
        let decoder = LiveDecoderFixture(supportsFEC: false)
        let receiver = makeReceiver(route: route, decoder: decoder)
        #expect(receiver.start(liveStart(), authorized: true))
        for sequence: UInt32 in 1...3 { _ = receiver.receive(liveFrame(sequence)) }
        route.consumeAll()

        for sequence: UInt32 in 4...100 {
            if sequence != 50 && sequence != 75 {
                #expect(receiver.receive(liveFrame(sequence)) == .apply)
            }
            receiver.tick()
            route.consumeAll()
            #expect(receiver.snapshot().encodedFrames <= 9)
            #expect(receiver.snapshot().pcmFrames <= route.livePCMCapacityFrames)
        }
        receiver.tick()
        route.consumeAll()
        receiver.tick()
        route.consumeAll()

        let snapshot = receiver.snapshot()
        #expect(snapshot.phase == .playing)
        #expect(snapshot.expectedSequence == 101)
        #expect(snapshot.decodedFrames == 100)
        #expect(snapshot.receivedFrames == 98)
        #expect(snapshot.plcFrames == 2)
        #expect(snapshot.failedFrames == 0)
    }

    @Test func pcmOverflowFailsAndReleasesRoute() {
        let route = LiveRouteFixture(capacity: 3_840)
        let decoder = LiveDecoderFixture(supportsFEC: true)
        let receiver = makeReceiver(route: route, decoder: decoder)
        #expect(receiver.start(liveStart(), authorized: true))
        for sequence: UInt32 in 1...3 { _ = receiver.receive(liveFrame(sequence)) }
        _ = receiver.receive(liveFrame(4)); receiver.tick()
        _ = receiver.receive(liveFrame(5)); receiver.tick()

        #expect(receiver.snapshot().phase == .idle)
        #expect(route.stops.last?.discard == true)
        #expect(!route.active)
    }

    @Test func endDrainsBeforeClickFreeReleaseAndCancelDiscards() {
        let route = LiveRouteFixture()
        let decoder = LiveDecoderFixture(supportsFEC: true)
        var now: Int64 = 1_100
        let receiver = makeReceiver(
            route: route, decoder: decoder, now: { now })
        #expect(receiver.start(liveStart(), authorized: true))
        for sequence: UInt32 in 1...3 { _ = receiver.receive(liveFrame(sequence)) }
        receiver.end(LivePTTEndPayload(
            sessionId: liveSession, generation: 7, commandSequence: 1,
            lastSequence: 3, endedAtCoordMs: now,
            drainDeadlineCoordMs: now + 600, reason: "release"))
        #expect(receiver.snapshot().phase == .draining)
        #expect(route.stops.isEmpty)
        route.consumeAll(); receiver.tick()
        #expect(receiver.snapshot().phase == .idle)
        #expect(route.stops.last?.discard == false)

        now += 1
        #expect(receiver.start(liveStart(generation: 8, now: now), authorized: true))
        receiver.cancel(LivePTTCancelPayload(
            sessionId: liveSession, generation: 8, commandSequence: 1,
            cancelledAtCoordMs: now, reason: "policy_changed", discardBuffered: true))
        #expect(receiver.snapshot().phase == .idle)
        #expect(route.stops.last?.discard == true)
    }

    @Test func maximumDurationTimeoutDiscardsAndRemovesSessionState() {
        let route = LiveRouteFixture()
        let decoder = LiveDecoderFixture(supportsFEC: true)
        var now: Int64 = 1_001
        let receiver = makeReceiver(
            route: route, decoder: decoder, now: { now })
        #expect(receiver.start(liveStart(), authorized: true))
        for sequence: UInt32 in 1...3 { _ = receiver.receive(liveFrame(sequence)) }
        now = 301_001
        receiver.tick()
        #expect(receiver.snapshot().phase == .idle)
        #expect(route.stops.last?.discard == true)
        #expect(route.pcm.isEmpty)
    }

    @Test func systemAudioConverterDecodesRawOpusPacketIntoFixedFrame() throws {
        let hex = "78822e5d6a3932900000063e520d1d584df8ef6ebf481adb437afaf8cf99755f9d3caaa558fe348729da2831f5db89efef751a0ba587d13c8cdd1cb6b8b3e988ae9c46f86f669d79c54cf35c883c2155388efd5138b2f3c89480e5a5eaba"
        var packet = Data()
        var cursor = hex.startIndex
        while cursor < hex.endIndex {
            let end = hex.index(cursor, offsetBy: 2)
            packet.append(UInt8(hex[cursor..<end], radix: 16)!)
            cursor = end
        }
        let decoder = try #require(MacAVAudioOpusDecoder())
        let storage = UnsafeMutablePointer<Float>.allocate(capacity: 960)
        defer { storage.deallocate() }
        let output = UnsafeMutableBufferPointer(start: storage, count: 960)
        #expect(try decoder.decode(packet: packet, fec: false, into: output) == 960)
        #expect(output.map { abs($0) }.max() ?? 0 > 0.1)
        #expect(try decoder.decode(packet: packet, fec: false, into: output) == 960)
        #expect(output.map { abs($0) }.max() ?? 0 > 0.1)
        #expect(throws: MacLiveDecodeError.fecUnavailable) {
            try decoder.decode(packet: packet, fec: true, into: output)
        }
    }
}
