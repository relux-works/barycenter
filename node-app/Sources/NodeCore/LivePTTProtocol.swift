import Foundation

public enum LivePTTConstants {
    public static let capability = "live_ptt_v1"
    public static let codecProfile = "opus-1.6.1-48k-mono-20ms-24k-cvbr-c5-fec2"
    public static let headerBytes = 40
    public static let maxPayloadBytes = 400
    public static let frameMs = 20
    public static let jitterBufferMs = 60
    public static let maxGapFrames: UInt32 = 8
    public static let maxDurationMs: Int64 = 300_000
    public static let acceptTimeoutMs: Int64 = 1_500
    public static let drainTimeoutMs: Int64 = 600
    public static let lateJoinPolicy = "frozen_targets_no_late_join"
    public static let captureAuthority = "local_user_input_only"
}

public struct LivePTTStartPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64
    public var senderActorId: Int64; public var senderOrbitId: Int64; public var senderNodeId: String
    public var targetSnapshot: String; public var targetSha256: String; public var targetCount: Int
    public var playbackDomain: String; public var playbackDomainId: Int64; public var codecProfile: String
    public var frameMs: Int; public var maxPayloadBytes: Int; public var jitterBufferMs: Int
    public var startedAtCoordMs: Int64; public var acceptDeadlineCoordMs: Int64; public var maxDurationMs: Int64
    public var mixedVersionPolicy: String; public var lateJoinPolicy: String; public var captureAuthority: String
    enum CodingKeys: String, CodingKey {
        case sessionId = "session_id", generation, senderActorId = "sender_actor_id"
        case senderOrbitId = "sender_orbit_id", senderNodeId = "sender_node_id"
        case targetSnapshot = "target_snapshot", targetSha256 = "target_sha256", targetCount = "target_count"
        case playbackDomain = "playback_domain", playbackDomainId = "playback_domain_id"
        case codecProfile = "codec_profile", frameMs = "frame_ms", maxPayloadBytes = "max_payload_bytes"
        case jitterBufferMs = "jitter_buffer_ms", startedAtCoordMs = "started_at_coord_ms"
        case acceptDeadlineCoordMs = "accept_deadline_coord_ms", maxDurationMs = "max_duration_ms"
        case mixedVersionPolicy = "mixed_version_policy", lateJoinPolicy = "late_join_policy"
        case captureAuthority = "capture_authority"
    }
}

public struct LivePTTAcceptPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64; public var eventSequence: Int64
    public var acceptedAtCoordMs: Int64; public var liveEdgeSequence: UInt32; public var bufferFrames: Int
    enum CodingKeys: String, CodingKey { case sessionId = "session_id", generation, eventSequence = "event_sequence", acceptedAtCoordMs = "accepted_at_coord_ms", liveEdgeSequence = "live_edge_sequence", bufferFrames = "buffer_frames" }
}
public struct LivePTTRejectPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64; public var eventSequence: Int64; public var code: String; public var rejectedAtCoordMs: Int64
    enum CodingKeys: String, CodingKey { case sessionId = "session_id", generation, eventSequence = "event_sequence", code, rejectedAtCoordMs = "rejected_at_coord_ms" }
}
public struct LivePTTEndPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64; public var commandSequence: Int64; public var lastSequence: UInt32; public var endedAtCoordMs: Int64; public var drainDeadlineCoordMs: Int64; public var reason: String
    enum CodingKeys: String, CodingKey { case sessionId = "session_id", generation, commandSequence = "command_sequence", lastSequence = "last_sequence", endedAtCoordMs = "ended_at_coord_ms", drainDeadlineCoordMs = "drain_deadline_coord_ms", reason }
}
public struct LivePTTCancelPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64; public var commandSequence: Int64; public var cancelledAtCoordMs: Int64; public var reason: String; public var discardBuffered: Bool
    enum CodingKeys: String, CodingKey { case sessionId = "session_id", generation, commandSequence = "command_sequence", cancelledAtCoordMs = "cancelled_at_coord_ms", reason, discardBuffered = "discard_buffered" }
}
public struct LivePTTFailedPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64; public var eventSequence: Int64; public var stage: String; public var code: String; public var failedAtCoordMs: Int64
    enum CodingKeys: String, CodingKey { case sessionId = "session_id", generation, eventSequence = "event_sequence", stage, code, failedAtCoordMs = "failed_at_coord_ms" }
}
public struct LivePTTReceiptPayload: Codable, Equatable, Sendable {
    public var sessionId: String; public var generation: Int64; public var eventSequence: Int64; public var state: String; public var lastSequence: UInt32?; public var observedAtCoordMs: Int64
    enum CodingKeys: String, CodingKey { case sessionId = "session_id", generation, eventSequence = "event_sequence", state, lastSequence = "last_sequence", observedAtCoordMs = "observed_at_coord_ms" }
}
public struct LivePTTStatePayload: Codable, Equatable, Sendable {
    public var revision: Int64; public var phase: String; public var activeSessionId: String?; public var generation: Int64?; public var speakerActorId: Int64?; public var lastSequence: UInt32?; public var generatedAtCoordMs: Int64
    enum CodingKeys: String, CodingKey { case revision, phase, activeSessionId = "active_session_id", generation, speakerActorId = "speaker_actor_id", lastSequence = "last_sequence", generatedAtCoordMs = "generated_at_coord_ms" }
}

public enum LivePTTFrameDecision: String, Equatable { case apply, duplicate, stale, invalid }

public struct LivePTTBinaryFrame: Equatable, Sendable {
    public static let startFlag: UInt8 = 1, endFlag: UInt8 = 2, fecFlag: UInt8 = 4
    public var flags: UInt8; public var sessionId: [UInt8]; public var sequence: UInt32
    public var captureMonotonicUs: UInt64; public var payload: Data

    public func encoded() throws -> Data {
        guard sessionId.count == 16, sessionId.contains(where: { $0 != 0 }), sequence > 0,
              captureMonotonicUs > 0, !payload.isEmpty, payload.count <= LivePTTConstants.maxPayloadBytes,
              flags & ~UInt8(7) == 0, flags & Self.fecFlag != 0,
              (sequence == 1) == (flags & Self.startFlag != 0) else { throw LivePTTProtocolError.invalidFrame }
        var bytes = [UInt8](repeating: 0, count: LivePTTConstants.headerBytes + payload.count)
        bytes[0] = 0x42; bytes[1] = 0x50; bytes[2] = 1; bytes[3] = flags
        bytes.replaceSubrange(4..<20, with: sessionId)
        Self.put(sequence, into: &bytes, at: 20); Self.put(captureMonotonicUs, into: &bytes, at: 24)
        Self.put(UInt16(payload.count), into: &bytes, at: 32)
        bytes[34] = UInt8(LivePTTConstants.frameMs); bytes[35] = 1; bytes[36] = 1; bytes[37] = 1
        bytes.replaceSubrange(40..<bytes.count, with: payload)
        return Data(bytes)
    }

    public static func decode(_ data: Data) throws -> Self {
        let b = [UInt8](data)
        guard b.count >= 40, b.count <= 440, b[0] == 0x42, b[1] == 0x50, b[2] == 1,
              b[34] == 20, b[35] == 1, b[36] == 1, b[37] == 1, b[38] == 0, b[39] == 0 else { throw LivePTTProtocolError.invalidFrame }
        let length: UInt16 = get(b, at: 32)
        guard length > 0, length <= 400, b.count == 40 + Int(length) else { throw LivePTTProtocolError.invalidFrame }
        let frame = Self(flags: b[3], sessionId: Array(b[4..<20]), sequence: get(b, at: 20), captureMonotonicUs: get(b, at: 24), payload: Data(b[40...]))
        _ = try frame.encoded()
        return frame
    }
    private static func put<T: FixedWidthInteger>(_ value: T, into bytes: inout [UInt8], at offset: Int) { for i in 0..<MemoryLayout<T>.size { bytes[offset + i] = UInt8(truncatingIfNeeded: value >> T((MemoryLayout<T>.size - 1 - i) * 8)) } }
    private static func get<T: FixedWidthInteger>(_ bytes: [UInt8], at offset: Int) -> T { var value: T = 0; for i in 0..<MemoryLayout<T>.size { value = (value << 8) | T(bytes[offset + i]) }; return value }
}

public enum LivePTTProtocolError: Error { case invalidFrame, invalidPayload }

public enum LivePTTValidation {
    private static let tokenCharacters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-"
    private static func common(_ session: String, _ generation: Int64) -> Bool {
        session.count == 32 && session != String(repeating: "0", count: 32) && generation > 0 &&
        session.allSatisfy { "0123456789abcdef".contains($0) }
    }
    public static func validate(_ message: Message) throws {
        switch message {
        case .livePTTStart(let p):
            guard common(p.sessionId, p.generation), p.senderActorId > 0, p.senderOrbitId > 0,
                  !p.senderNodeId.isEmpty, p.senderNodeId.count <= 64,
                  p.senderNodeId.allSatisfy({ Self.tokenCharacters.contains($0) }),
                  p.targetSnapshot.hasPrefix("lts1."), p.targetSnapshot.count <= 128,
                  p.targetSnapshot.allSatisfy({ Self.tokenCharacters.contains($0) }),
                  p.targetSha256.count == 64,
                  p.targetSha256.allSatisfy({ "0123456789abcdef".contains($0) }),
                  (1...64).contains(p.targetCount), ["personal","barycenter","air"].contains(p.playbackDomain),
                  p.playbackDomainId > 0, p.codecProfile == LivePTTConstants.codecProfile,
                  p.frameMs == 20, p.maxPayloadBytes == 400, p.jitterBufferMs == 60,
                  p.startedAtCoordMs > 0, p.acceptDeadlineCoordMs > p.startedAtCoordMs,
                  p.acceptDeadlineCoordMs - p.startedAtCoordMs <= 1500, p.maxDurationMs == 300_000,
                  ["require_all","supported_only_with_receipts"].contains(p.mixedVersionPolicy),
                  p.lateJoinPolicy == LivePTTConstants.lateJoinPolicy,
                  p.captureAuthority == LivePTTConstants.captureAuthority else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTAccept(let p):
            guard common(p.sessionId,p.generation), p.eventSequence == 1, p.acceptedAtCoordMs > 0, p.liveEdgeSequence == 1, p.bufferFrames == 3 else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTReject(let p):
            guard common(p.sessionId,p.generation), p.eventSequence == 1, ["blocked","busy","dnd","expired","policy","unauthorized","unsupported"].contains(p.code), p.rejectedAtCoordMs > 0 else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTEnd(let p):
            guard common(p.sessionId,p.generation), p.commandSequence > 0,
                  p.lastSequence > 0, p.endedAtCoordMs > 0,
                  p.drainDeadlineCoordMs - p.endedAtCoordMs == 600,
                  ["release","lost_release","lock","sleep","permission_revoked","device_lost","disconnect","quit"].contains(p.reason)
            else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTCancel(let p):
            guard common(p.sessionId,p.generation), p.commandSequence > 0,
                  p.cancelledAtCoordMs > 0, p.discardBuffered,
                  ["backpressure","coordinator_restart","generation_replaced","lost_release","policy_changed","sender_disconnect","target_revoked","timeout","user_cancel"].contains(p.reason)
            else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTFailed(let p):
            guard common(p.sessionId,p.generation), p.eventSequence > 0,
                  ["capture","decode","frame","jitter","policy","relay","render","transport"].contains(p.stage),
                  !p.code.isEmpty, p.code.count <= 64,
                  p.code.allSatisfy({ "abcdefghijklmnopqrstuvwxyz0123456789_".contains($0) }),
                  p.failedAtCoordMs > 0
            else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTReceipt(let p):
            guard common(p.sessionId,p.generation), p.eventSequence > 0, ["accepted","audible_started","cancelled","ended","failed","rejected","unsupported"].contains(p.state), p.observedAtCoordMs > 0 else { throw LivePTTProtocolError.invalidPayload }
        case .livePTTState(let p):
            guard p.revision > 0, ["accepting","cancelled","ended","idle","receiving","relaying","starting","terminal"].contains(p.phase), p.generatedAtCoordMs > 0 else { throw LivePTTProtocolError.invalidPayload }
            if p.phase == "idle" { guard p.activeSessionId == nil, p.generation == nil, p.speakerActorId == nil, p.lastSequence == nil else { throw LivePTTProtocolError.invalidPayload } }
            else { guard let session = p.activeSessionId, let generation = p.generation, common(session,generation), (p.speakerActorId ?? 0) > 0 else { throw LivePTTProtocolError.invalidPayload } }
        default: break
        }
    }
}

public struct LivePTTFrameGuard {
    public let sessionId: [UInt8]; public let generation: Int64
    public private(set) var lastSequence: UInt32 = 0; public private(set) var lastCaptureUs: UInt64 = 0
    private var lastFrame = Data(); private var terminal = false
    public init(sessionId: [UInt8], generation: Int64) { self.sessionId = sessionId; self.generation = generation }
    public mutating func accept(_ frame: LivePTTBinaryFrame) -> LivePTTFrameDecision {
        guard generation > 0, frame.sessionId == sessionId, !terminal else { return .stale }
        guard let encoded = try? frame.encoded() else { return .invalid }
        if frame.sequence <= lastSequence { return frame.sequence == lastSequence && encoded == lastFrame ? .duplicate : .stale }
        if lastSequence == 0 { guard frame.sequence == 1 else { return .invalid } }
        else { let gap = frame.sequence - lastSequence; guard gap <= 8, frame.captureMonotonicUs > lastCaptureUs, frame.captureMonotonicUs - lastCaptureUs == UInt64(gap) * 20_000 else { return .invalid } }
        guard frame.sequence <= 15_000 else { return .invalid }
        lastSequence = frame.sequence; lastCaptureUs = frame.captureMonotonicUs; lastFrame = encoded
        terminal = frame.flags & LivePTTBinaryFrame.endFlag != 0
        return .apply
    }
}
