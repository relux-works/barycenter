// Wire protocol v1 (spec ch. 8). Contract: protocol/golden/*.json, see
// docs/protocol.md. Explicit CodingKeys everywhere: field names are the
// contract, not a convention.

import Foundation

public enum ProtocolConstants {
    public static let version = 1
}

// MARK: - Envelope

public struct EnvelopeHead: Codable {
    public let v: Int
    public let id: String
    public let ts: Int64
    public let type: String
}

struct Wire<P: Codable>: Codable {
    let v: Int
    let id: String
    let ts: Int64
    let type: String
    let payload: P
}

// MARK: - Payloads: coordinator -> node

public struct SessionCurrent: Codable, Equatable {
    public var elementId: String
    public var kind: String
    public var uri: String?
    public var positionMs: Int64
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", kind, uri, positionMs = "position_ms"
    }
}

public struct SessionSnapshot: Codable, Equatable {
    public var mode: String
    public var state: String
    public var current: SessionCurrent?
    public var volume: Int
    enum CodingKeys: String, CodingKey { case mode, state, current, volume }

    // `current` is nullable-but-present in JSON; encode nil explicitly as null.
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(mode, forKey: .mode)
        try c.encode(state, forKey: .state)
        try c.encode(current, forKey: .current)
        try c.encode(volume, forKey: .volume)
    }
}

public struct WelcomePayload: Codable, Equatable {
    public var sessionSnapshot: SessionSnapshot
    enum CodingKeys: String, CodingKey { case sessionSnapshot = "session_snapshot" }
}

public struct LoadPayload: Codable, Equatable {
    public var elementId: String
    public var uri: String
    public var positionMs: Int64
    public var provider: String?
    public var ref: String?
    public var durationMs: Int64?
    public var gainDb: Double?
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", provider, ref, durationMs = "duration_ms", gainDb = "gain_db", uri, positionMs = "position_ms"
    }
}

public struct ResumeAtPayload: Codable, Equatable {
    public var elementId: String
    public var tCoordMs: Int64
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", tCoordMs = "t_coord_ms"
    }
}

public struct PausePayload: Codable, Equatable {
    public var elementId: String
    public var fadeMs: Int64
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", fadeMs = "fade_ms"
    }
}

public struct SeekPayload: Codable, Equatable {
    public var elementId: String
    public var positionMs: Int64
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", positionMs = "position_ms"
    }
}

public struct PlayVoicePayload: Codable, Equatable {
    public var elementId: String
    public var fileUrl: String
    public var tCoordMs: Int64?
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", fileUrl = "file_url", tCoordMs = "t_coord_ms"
    }
}

public struct WaitPayload: Codable, Equatable {
    public var elementId: String
    public var durationMs: Int64
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", durationMs = "duration_ms"
    }
}

public struct SetVolumePayload: Codable, Equatable {
    public var volume: Int
    enum CodingKeys: String, CodingKey { case volume }
}

public struct SetModePayload: Codable, Equatable {
    public var mode: String
    enum CodingKeys: String, CodingKey { case mode }
}

public struct StopPayload: Codable, Equatable {
    public init() {}
}

public struct SoloInjectPayload: Codable, Equatable {
    public var uri: String
    // v1.1 additive (spec-providers §7); absent provider = "spotify".
    // Optionals: nil is omitted by the synthesized encoder (mirrors Go omitempty).
    public var provider: String?
    public var ref: String?
    public var ctid: String?
    public init(uri: String, provider: String? = nil, ref: String? = nil, ctid: String? = nil) {
        self.uri = uri
        self.provider = provider
        self.ref = ref
        self.ctid = ctid
    }
    enum CodingKeys: String, CodingKey { case uri, provider, ref, ctid }
}

public struct SoloVoicePayload: Codable, Equatable {
    public var elementId: String
    public var fileUrl: String
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", fileUrl = "file_url"
    }
}

public struct PongPayload: Codable, Equatable {
    public var t1: Int64
    public var t2: Int64
    public var t3: Int64
    enum CodingKeys: String, CodingKey { case t1, t2, t3 }
}

// v1 additions beyond the spec ch. 8 catalog (docs/protocol.md).

public struct SetOffsetPayload: Codable, Equatable {
    public var offsetMs: Int64
    enum CodingKeys: String, CodingKey { case offsetMs = "offset_ms" }
}

public struct OffsetTestPayload: Codable, Equatable {
    public var tCoordMs: Int64
    public var clicks: Int
    public var intervalMs: Int64
    enum CodingKeys: String, CodingKey {
        case tCoordMs = "t_coord_ms", clicks, intervalMs = "interval_ms"
    }
}

// MARK: - Payloads: node -> coordinator

public struct RegisterPayload: Codable, Equatable {
    public var nodeId: String
    public var token: String
    public var appVersion: String
    public var librespotVersion: String
    public init(nodeId: String, token: String, appVersion: String, librespotVersion: String) {
        self.nodeId = nodeId
        self.token = token
        self.appVersion = appVersion
        self.librespotVersion = librespotVersion
    }
    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id", token, appVersion = "app_version", librespotVersion = "librespot_version"
    }
}

public struct SpeakerState: Codable, Equatable {
    public var name: String
    public var connected: Bool
    public init(name: String, connected: Bool) {
        self.name = name
        self.connected = connected
    }
    enum CodingKeys: String, CodingKey { case name, connected }
}

public struct StatePayload: Codable, Equatable {
    public var playback: String
    public var uri: String?
    public var positionMs: Int64
    public var volume: Int
    public var degraded: Bool
    public var underruns: Int64
    public var rttMs: Int64
    public var speakers: [SpeakerState]
    // v1.1 additive (spec-providers §7): the node's active provider ("" / nil = spotify).
    public var provider: String?
    public init(playback: String, uri: String?, positionMs: Int64, volume: Int,
                degraded: Bool, underruns: Int64, rttMs: Int64, speakers: [SpeakerState],
                provider: String? = nil) {
        self.playback = playback
        self.uri = uri
        self.positionMs = positionMs
        self.volume = volume
        self.degraded = degraded
        self.underruns = underruns
        self.rttMs = rttMs
        self.speakers = speakers
        self.provider = provider
    }
    enum CodingKeys: String, CodingKey {
        case playback, uri, positionMs = "position_ms", volume, degraded,
             underruns, rttMs = "rtt_ms", speakers, provider
    }

    // `uri` is nullable-but-present (docs/protocol.md). `provider` is additive and
    // omitted when nil (mirrors Go omitempty), so pre-v1.1 state frames stay byte-identical.
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(playback, forKey: .playback)
        try c.encode(uri, forKey: .uri)
        try c.encode(positionMs, forKey: .positionMs)
        try c.encode(volume, forKey: .volume)
        try c.encode(degraded, forKey: .degraded)
        try c.encode(underruns, forKey: .underruns)
        try c.encode(rttMs, forKey: .rttMs)
        try c.encode(speakers, forKey: .speakers)
        try c.encodeIfPresent(provider, forKey: .provider)
    }
}

public struct ReadyPayload: Codable, Equatable {
    public var elementId: String
    public init(elementId: String) { self.elementId = elementId }
    enum CodingKeys: String, CodingKey { case elementId = "element_id" }
}

public struct StartedPayload: Codable, Equatable {
    public var elementId: String
    public var tFirstSampleCoordMs: Int64
    public init(elementId: String, tFirstSampleCoordMs: Int64) {
        self.elementId = elementId
        self.tFirstSampleCoordMs = tFirstSampleCoordMs
    }
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", tFirstSampleCoordMs = "t_first_sample_coord_ms"
    }
}

public struct EndedPayload: Codable, Equatable {
    public var elementId: String
    public var reason: String
    public init(elementId: String, reason: String) {
        self.elementId = elementId
        self.reason = reason
    }
    enum CodingKeys: String, CodingKey { case elementId = "element_id", reason }
}

public struct VoiceStartedPayload: Codable, Equatable {
    public var elementId: String
    enum CodingKeys: String, CodingKey { case elementId = "element_id" }
}

public struct VoiceEndedPayload: Codable, Equatable {
    public var elementId: String
    enum CodingKeys: String, CodingKey { case elementId = "element_id" }
}

public struct WaitEndedPayload: Codable, Equatable {
    public var elementId: String
    enum CodingKeys: String, CodingKey { case elementId = "element_id" }
}

public struct ErrorPayload: Codable, Equatable {
    public var code: String
    public var message: String
    public var elementId: String?
    public init(code: String, message: String, elementId: String? = nil) {
        self.code = code
        self.message = message
        self.elementId = elementId
    }
    enum CodingKeys: String, CodingKey { case code, message, elementId = "element_id" }

    // Optional means omitted, never null (docs/protocol.md).
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(code, forKey: .code)
        try c.encode(message, forKey: .message)
        try c.encodeIfPresent(elementId, forKey: .elementId)
    }
}

public struct PingPayload: Codable, Equatable {
    public var t1: Int64
    public init(t1: Int64) { self.t1 = t1 }
    enum CodingKeys: String, CodingKey { case t1 }
}

/// U9: daemon playback that does not belong to the broadcast (phone takeover).
public struct ExternalPlaybackPayload: Codable, Equatable {
    public var uri: String
    public init(uri: String) { self.uri = uri }
    enum CodingKeys: String, CodingKey { case uri }
}

/// v1.1 (spec-providers §7): switch the node's active provider.
public struct SetProviderPayload: Codable, Equatable {
    public var provider: String
    public init(provider: String) { self.provider = provider }
    enum CodingKeys: String, CodingKey { case provider }
}

// MARK: - Message

public enum Message {
    case welcome(WelcomePayload)
    case load(LoadPayload)
    case resumeAt(ResumeAtPayload)
    case pause(PausePayload)
    case seek(SeekPayload)
    case playVoice(PlayVoicePayload)
    case wait(WaitPayload)
    case setVolume(SetVolumePayload)
    case setMode(SetModePayload)
    case stop(StopPayload)
    case soloInject(SoloInjectPayload)
    case soloVoice(SoloVoicePayload)
    case pong(PongPayload)
    case setOffset(SetOffsetPayload)
    case offsetTest(OffsetTestPayload)
    case register(RegisterPayload)
    case state(StatePayload)
    case ready(ReadyPayload)
    case started(StartedPayload)
    case ended(EndedPayload)
    case voiceStarted(VoiceStartedPayload)
    case voiceEnded(VoiceEndedPayload)
    case waitEnded(WaitEndedPayload)
    case error(ErrorPayload)
    case ping(PingPayload)
    case externalPlayback(ExternalPlaybackPayload)
    case setProvider(SetProviderPayload)

    public var typeName: String {
        switch self {
        case .welcome: return "welcome"
        case .load: return "load"
        case .resumeAt: return "resume_at"
        case .pause: return "pause"
        case .seek: return "seek"
        case .playVoice: return "play_voice"
        case .wait: return "wait"
        case .setVolume: return "set_volume"
        case .setMode: return "set_mode"
        case .stop: return "stop"
        case .soloInject: return "solo_inject"
        case .soloVoice: return "solo_voice"
        case .pong: return "pong"
        case .setOffset: return "set_offset"
        case .offsetTest: return "offset_test"
        case .register: return "register"
        case .state: return "state"
        case .ready: return "ready"
        case .started: return "started"
        case .ended: return "ended"
        case .voiceStarted: return "voice_started"
        case .voiceEnded: return "voice_ended"
        case .waitEnded: return "wait_ended"
        case .error: return "error"
        case .ping: return "ping"
        case .externalPlayback: return "external_playback"
        case .setProvider: return "set_provider"
        }
    }
}

public enum ProtocolError: Error, CustomStringConvertible {
    case unknownType(String)
    case versionMismatch(Int)

    public var description: String {
        switch self {
        case .unknownType(let t): return "unknown message type \"\(t)\""
        case .versionMismatch(let v): return "protocol version \(v), this build speaks \(ProtocolConstants.version)"
        }
    }
}

public enum ProtocolCodec {
    public static func decode(_ data: Data) throws -> (head: EnvelopeHead, message: Message) {
        let dec = JSONDecoder()
        let head = try dec.decode(EnvelopeHead.self, from: data)
        guard head.v == ProtocolConstants.version else {
            throw ProtocolError.versionMismatch(head.v)
        }
        func p<T: Codable>(_ type: T.Type) throws -> T {
            try dec.decode(Wire<T>.self, from: data).payload
        }
        let message: Message
        switch head.type {
        case "welcome": message = .welcome(try p(WelcomePayload.self))
        case "load": message = .load(try p(LoadPayload.self))
        case "resume_at": message = .resumeAt(try p(ResumeAtPayload.self))
        case "pause": message = .pause(try p(PausePayload.self))
        case "seek": message = .seek(try p(SeekPayload.self))
        case "play_voice": message = .playVoice(try p(PlayVoicePayload.self))
        case "wait": message = .wait(try p(WaitPayload.self))
        case "set_volume": message = .setVolume(try p(SetVolumePayload.self))
        case "set_mode": message = .setMode(try p(SetModePayload.self))
        case "stop": message = .stop(try p(StopPayload.self))
        case "solo_inject": message = .soloInject(try p(SoloInjectPayload.self))
        case "solo_voice": message = .soloVoice(try p(SoloVoicePayload.self))
        case "pong": message = .pong(try p(PongPayload.self))
        case "set_offset": message = .setOffset(try p(SetOffsetPayload.self))
        case "offset_test": message = .offsetTest(try p(OffsetTestPayload.self))
        case "register": message = .register(try p(RegisterPayload.self))
        case "state": message = .state(try p(StatePayload.self))
        case "ready": message = .ready(try p(ReadyPayload.self))
        case "started": message = .started(try p(StartedPayload.self))
        case "ended": message = .ended(try p(EndedPayload.self))
        case "voice_started": message = .voiceStarted(try p(VoiceStartedPayload.self))
        case "voice_ended": message = .voiceEnded(try p(VoiceEndedPayload.self))
        case "wait_ended": message = .waitEnded(try p(WaitEndedPayload.self))
        case "error": message = .error(try p(ErrorPayload.self))
        case "ping": message = .ping(try p(PingPayload.self))
        case "external_playback": message = .externalPlayback(try p(ExternalPlaybackPayload.self))
        case "set_provider": message = .setProvider(try p(SetProviderPayload.self))
        default: throw ProtocolError.unknownType(head.type)
        }
        return (head, message)
    }

    public static func encode(id: String, ts: Int64, message: Message) throws -> Data {
        let enc = JSONEncoder()
        func w<T: Codable>(_ payload: T) throws -> Data {
            try enc.encode(Wire(v: ProtocolConstants.version, id: id, ts: ts,
                                type: message.typeName, payload: payload))
        }
        switch message {
        case .welcome(let p): return try w(p)
        case .load(let p): return try w(p)
        case .resumeAt(let p): return try w(p)
        case .pause(let p): return try w(p)
        case .seek(let p): return try w(p)
        case .playVoice(let p): return try w(p)
        case .wait(let p): return try w(p)
        case .setVolume(let p): return try w(p)
        case .setMode(let p): return try w(p)
        case .stop(let p): return try w(p)
        case .soloInject(let p): return try w(p)
        case .soloVoice(let p): return try w(p)
        case .pong(let p): return try w(p)
        case .setOffset(let p): return try w(p)
        case .offsetTest(let p): return try w(p)
        case .register(let p): return try w(p)
        case .state(let p): return try w(p)
        case .ready(let p): return try w(p)
        case .started(let p): return try w(p)
        case .ended(let p): return try w(p)
        case .voiceStarted(let p): return try w(p)
        case .voiceEnded(let p): return try w(p)
        case .waitEnded(let p): return try w(p)
        case .error(let p): return try w(p)
        case .ping(let p): return try w(p)
        case .externalPlayback(let p): return try w(p)
        case .setProvider(let p): return try w(p)
        }
    }
}
