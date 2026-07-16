// Wire protocol v1 (spec ch. 8). Contract: protocol/golden/*.json, see
// docs/protocol.md. Explicit CodingKeys everywhere: field names are the
// contract, not a convention.

import Foundation

public enum ProtocolCapabilities {
    public static let interruptResume = "interrupt_resume_v1"
    public static let livePTT = LivePTTConstants.capability
    public static let mediaClip = "media_clip_v1"
    public static let overlayMix = "overlay_mix_v1"
    public static let seamlessAdoption = "seamless_adoption_v1"
    public static let streamTrack = "stream_track_v1"

    /// Register capabilities are non-empty printable ASCII strings in strict
    /// byte order. Unknown names remain valid so additive features survive a
    /// mixed-version rollout.
    public static func areCanonical(_ values: [String]) -> Bool {
        var previous: String?
        for value in values {
            guard !value.isEmpty,
                  value.utf8.allSatisfy({ $0 >= 0x21 && $0 <= 0x7e }) else {
                return false
            }
            if let previous,
               !previous.utf8.lexicographicallyPrecedes(value.utf8) {
                return false
            }
            previous = value
        }
        return true
    }
}

// Source-compatible names used by existing and upcoming client hooks.
public let interruptResumeCapability = ProtocolCapabilities.interruptResume
public let livePTTCapability = ProtocolCapabilities.livePTT
public let mediaClipCapability = ProtocolCapabilities.mediaClip
public let overlayMixCapability = ProtocolCapabilities.overlayMix
public let seamlessAdoptionCapability = ProtocolCapabilities.seamlessAdoption
public let streamTrackCapability = ProtocolCapabilities.streamTrack

public enum ProtocolConstants {
    public static let version = 1
    public static let streamMinimumBufferedMs: Int64 = 2000
    public static let streamLoadReadyTimeoutMs: Int64 = 5000
    public static let streamSeekReadyTimeoutMs: Int64 = 3000
    public static let streamStartDeadlineMs: Int64 = 5000
    public static let streamMixedVersionRequireAll = "require_all"
    public static let streamMixedVersionSupportedOnlyWithReceipts = "supported_only_with_receipts"
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
    public var adoptPlaying: Bool?
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", provider, ref, durationMs = "duration_ms", gainDb = "gain_db",
             adoptPlaying = "adopt_playing", uri, positionMs = "position_ms"
    }
}

public struct ResumeAtPayload: Codable, Equatable {
    public var elementId: String
    public var tCoordMs: Int64
    public var positionMs: Int64?
    enum CodingKeys: String, CodingKey {
        case elementId = "element_id", tCoordMs = "t_coord_ms", positionMs = "position_ms"
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

public struct PrepareMediaPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var mediaId: String
    public var kind: String
    public var delivery: String
    public var fileUrl: String
    public var sha256: String
    public var sizeBytes: Int64
    public var durationMs: Int64
    public var mediaExpiresAtCoordMs: Int64
    public var prepareDeadlineCoordMs: Int64
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation, mediaId = "media_id", kind, delivery,
             fileUrl = "file_url", sha256, sizeBytes = "size_bytes", durationMs = "duration_ms",
             mediaExpiresAtCoordMs = "media_expires_at_coord_ms",
             prepareDeadlineCoordMs = "prepare_deadline_coord_ms"
    }
}

public struct PlayMediaAtPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var tCoordMs: Int64
    public var startDeadlineCoordMs: Int64
    public var delivery: String
    public var duckDb: Double?
    public var attackMs: Int64?
    public var releaseMs: Int64?
    public var fadeOutMs: Int64?
    public var fadeInMs: Int64?
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation, tCoordMs = "t_coord_ms",
             startDeadlineCoordMs = "start_deadline_coord_ms", delivery, duckDb = "duck_db",
             attackMs = "attack_ms", releaseMs = "release_ms", fadeOutMs = "fade_out_ms",
             fadeInMs = "fade_in_ms"
    }
}

public struct CancelMediaPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var reason: String
    public var action: String
    public var resumeMain: Bool
    public var fadeMs: Int64
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation, reason, action,
             resumeMain = "resume_main", fadeMs = "fade_ms"
    }
}

public struct PresenceNode: Codable, Equatable {
    public var orbitId: Int64
    public var slot: String
    public var online: Bool
    public var lastSeenAtCoordMs: Int64
    public var outputState: String
    public var playbackState: String
    public var dndMode: String
    public var dndRevision: Int64
    public var dndUntilCoordMs: Int64?
    public var capabilities: [String]
    public var interruptResumeReady: Bool
    enum CodingKeys: String, CodingKey {
        case orbitId = "orbit_id", slot, online, lastSeenAtCoordMs = "last_seen_at_coord_ms",
             outputState = "output_state", playbackState = "playback_state", dndMode = "dnd_mode",
             dndRevision = "dnd_revision", dndUntilCoordMs = "dnd_until_coord_ms", capabilities,
             interruptResumeReady = "interrupt_resume_ready"
    }
}

public struct PresenceUpdatePayload: Codable, Equatable {
    public var revision: Int64
    public var generatedAtCoordMs: Int64
    public var nodes: [PresenceNode]
    enum CodingKeys: String, CodingKey {
        case revision, generatedAtCoordMs = "generated_at_coord_ms", nodes
    }
}

public struct StreamLoadPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var commandSequence: Int64
    public var mediaId: String
    public var variantManifest: String
    public var variantUrl: String
    public var variantEtag: String
    public var variantSha256: String
    public var variantSizeBytes: Int64
    public var startPositionMs: Int64
    public var minimumBufferedMs: Int64
    public var readyDeadlineCoordMs: Int64
    public var mixedVersionPolicy: String
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", commandSequence = "command_sequence",
             mediaId = "media_id", variantManifest = "variant_manifest",
             variantUrl = "variant_url", variantEtag = "variant_etag",
             variantSha256 = "variant_sha256", variantSizeBytes = "variant_size_bytes",
             startPositionMs = "start_position_ms", minimumBufferedMs = "minimum_buffered_ms",
             readyDeadlineCoordMs = "ready_deadline_coord_ms",
             mixedVersionPolicy = "mixed_version_policy"
    }
}

public struct StreamResumeAtPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var commandSequence: Int64
    public var tCoordMs: Int64
    public var startDeadlineCoordMs: Int64
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", commandSequence = "command_sequence",
             tCoordMs = "t_coord_ms", startDeadlineCoordMs = "start_deadline_coord_ms"
    }
}

public struct StreamSeekPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var commandSequence: Int64
    public var positionMs: Int64
    public var minimumBufferedMs: Int64
    public var readyDeadlineCoordMs: Int64
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", commandSequence = "command_sequence",
             positionMs = "position_ms", minimumBufferedMs = "minimum_buffered_ms",
             readyDeadlineCoordMs = "ready_deadline_coord_ms"
    }
}

public struct StreamPausePayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var commandSequence: Int64
    public var fadeMs: Int64
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", commandSequence = "command_sequence",
             fadeMs = "fade_ms"
    }
}

public struct StreamCancelPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var commandSequence: Int64
    public var reason: String
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", commandSequence = "command_sequence", reason
    }
}

// MARK: - Payloads: node -> coordinator

public struct RegisterPayload: Codable, Equatable {
    public var nodeId: String
    public var token: String
    public var appVersion: String
    public var librespotVersion: String
    public var capabilities: [String]?
    public init(nodeId: String, token: String, appVersion: String, librespotVersion: String,
                capabilities: [String]? = nil) {
        self.nodeId = nodeId
        self.token = token
        self.appVersion = appVersion
        self.librespotVersion = librespotVersion
        self.capabilities = capabilities
    }
    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id", token, appVersion = "app_version",
             librespotVersion = "librespot_version", capabilities
    }
}

public struct MediaReadyPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var decodedDurationMs: Int64
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation,
             decodedDurationMs = "decoded_duration_ms"
    }
}

public struct MediaStartedPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var tFirstSampleCoordMs: Int64
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation,
             tFirstSampleCoordMs = "t_first_sample_coord_ms"
    }
}

public struct MediaEndedPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var tLastSampleCoordMs: Int64
    public var reason: String
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation,
             tLastSampleCoordMs = "t_last_sample_coord_ms", reason
    }
}

public struct MediaFailedPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var stage: String
    public var code: String
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation, stage, code
    }
}

public struct MediaCancelledPayload: Codable, Equatable {
    public var transmissionId: String
    public var generation: Int64
    public var reason: String
    public var action: String
    public var mainResumed: Bool
    enum CodingKeys: String, CodingKey {
        case transmissionId = "transmission_id", generation, reason, action,
             mainResumed = "main_resumed"
    }
}

public struct SetDNDPayload: Codable, Equatable {
    public var revision: Int64
    public var mode: String
    public var mutedUntilCoordMs: Int64?
    enum CodingKeys: String, CodingKey {
        case revision, mode, mutedUntilCoordMs = "muted_until_coord_ms"
    }
}

public struct StreamReadyPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var eventSequence: Int64
    public var audiblePositionMs: Int64
    public var bufferedDurationMs: Int64
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", eventSequence = "event_sequence",
             audiblePositionMs = "audible_position_ms", bufferedDurationMs = "buffered_duration_ms"
    }
}

public struct StreamStartedPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var eventSequence: Int64
    public var audiblePositionMs: Int64
    public var tFirstSampleCoordMs: Int64
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", eventSequence = "event_sequence",
             audiblePositionMs = "audible_position_ms",
             tFirstSampleCoordMs = "t_first_sample_coord_ms"
    }
}

public struct StreamProgressPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var eventSequence: Int64
    public var audiblePositionMs: Int64
    public var bufferedDurationMs: Int64
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", eventSequence = "event_sequence",
             audiblePositionMs = "audible_position_ms", bufferedDurationMs = "buffered_duration_ms"
    }
}

public typealias StreamRebufferPayload = StreamProgressPayload

public struct StreamFailedPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var eventSequence: Int64
    public var stage: String
    public var code: String
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", eventSequence = "event_sequence", stage, code
    }
}

public struct StreamEndedPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var eventSequence: Int64
    public var audiblePositionMs: Int64
    public var tLastSampleCoordMs: Int64
    public var reason: String
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", eventSequence = "event_sequence",
             audiblePositionMs = "audible_position_ms",
             tLastSampleCoordMs = "t_last_sample_coord_ms", reason
    }
}

public struct StreamCancelledPayload: Codable, Equatable {
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var eventSequence: Int64
    public var audiblePositionMs: Int64
    public var reason: String
    enum CodingKeys: String, CodingKey {
        case streamId = "stream_id", playbackGeneration = "playback_generation",
             seekGeneration = "seek_generation", eventSequence = "event_sequence",
             audiblePositionMs = "audible_position_ms", reason
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

/// A track selected through Spotify on this Pulsar while the shared air is active.
public struct ExternalPlaybackPayload: Codable, Equatable {
    public var uri: String
    public var positionMs: Int64?
    public var title: String?
    public init(uri: String, positionMs: Int64? = nil, title: String? = nil) {
        self.uri = uri
        self.positionMs = positionMs
        self.title = title
    }
    enum CodingKeys: String, CodingKey { case uri, positionMs = "position_ms", title }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(uri, forKey: .uri)
        try c.encodeIfPresent(positionMs, forKey: .positionMs)
        try c.encodeIfPresent(title, forKey: .title)
    }
}

/// v1.1 (spec-providers §7): switch the node's active provider.
/// Personal pause (2026-07-10): the user paused/resumed THIS Pulsar via the
/// Spotify app. element_id is what the node believes is current.
public struct UserPausePayload: Codable, Equatable {
    public var elementId: String
    public init(elementId: String) { self.elementId = elementId }
    enum CodingKeys: String, CodingKey { case elementId = "element_id" }
}

public struct UserResumePayload: Codable, Equatable {
    public var elementId: String
    public init(elementId: String) { self.elementId = elementId }
    enum CodingKeys: String, CodingKey { case elementId = "element_id" }
}

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
    case prepareMedia(PrepareMediaPayload)
    case playMediaAt(PlayMediaAtPayload)
    case cancelMedia(CancelMediaPayload)
    case presenceUpdate(PresenceUpdatePayload)
    case streamLoad(StreamLoadPayload)
    case streamResumeAt(StreamResumeAtPayload)
    case streamSeek(StreamSeekPayload)
    case streamPause(StreamPausePayload)
    case streamCancel(StreamCancelPayload)
    case register(RegisterPayload)
    case mediaReady(MediaReadyPayload)
    case mediaStarted(MediaStartedPayload)
    case mediaEnded(MediaEndedPayload)
    case mediaFailed(MediaFailedPayload)
    case mediaCancelled(MediaCancelledPayload)
    case setDND(SetDNDPayload)
    case streamReady(StreamReadyPayload)
    case streamStarted(StreamStartedPayload)
    case streamProgress(StreamProgressPayload)
    case streamRebuffer(StreamRebufferPayload)
    case streamFailed(StreamFailedPayload)
    case streamEnded(StreamEndedPayload)
    case streamCancelled(StreamCancelledPayload)
    case livePTTStart(LivePTTStartPayload)
    case livePTTAccept(LivePTTAcceptPayload)
    case livePTTReject(LivePTTRejectPayload)
    case livePTTEnd(LivePTTEndPayload)
    case livePTTCancel(LivePTTCancelPayload)
    case livePTTFailed(LivePTTFailedPayload)
    case livePTTReceipt(LivePTTReceiptPayload)
    case livePTTState(LivePTTStatePayload)
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
    case userPause(UserPausePayload)
    case userResume(UserResumePayload)
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
        case .prepareMedia: return "prepare_media"
        case .playMediaAt: return "play_media_at"
        case .cancelMedia: return "cancel_media"
        case .presenceUpdate: return "presence_update"
        case .streamLoad: return "stream_load"
        case .streamResumeAt: return "stream_resume_at"
        case .streamSeek: return "stream_seek"
        case .streamPause: return "stream_pause"
        case .streamCancel: return "stream_cancel"
        case .register: return "register"
        case .mediaReady: return "media_ready"
        case .mediaStarted: return "media_started"
        case .mediaEnded: return "media_ended"
        case .mediaFailed: return "media_failed"
        case .mediaCancelled: return "media_cancelled"
        case .setDND: return "set_dnd"
        case .streamReady: return "stream_ready"
        case .streamStarted: return "stream_started"
        case .streamProgress: return "stream_progress"
        case .streamRebuffer: return "stream_rebuffer"
        case .streamFailed: return "stream_failed"
        case .streamEnded: return "stream_ended"
        case .streamCancelled: return "stream_cancelled"
        case .livePTTStart: return "live_ptt_start"
        case .livePTTAccept: return "live_ptt_accept"
        case .livePTTReject: return "live_ptt_reject"
        case .livePTTEnd: return "live_ptt_end"
        case .livePTTCancel: return "live_ptt_cancel"
        case .livePTTFailed: return "live_ptt_failed"
        case .livePTTReceipt: return "live_ptt_receipt"
        case .livePTTState: return "live_ptt_state"
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
        case .userPause: return "user_pause"
        case .userResume: return "user_resume"
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
        case "prepare_media": message = .prepareMedia(try p(PrepareMediaPayload.self))
        case "play_media_at": message = .playMediaAt(try p(PlayMediaAtPayload.self))
        case "cancel_media": message = .cancelMedia(try p(CancelMediaPayload.self))
        case "presence_update": message = .presenceUpdate(try p(PresenceUpdatePayload.self))
        case "stream_load": message = .streamLoad(try p(StreamLoadPayload.self))
        case "stream_resume_at": message = .streamResumeAt(try p(StreamResumeAtPayload.self))
        case "stream_seek": message = .streamSeek(try p(StreamSeekPayload.self))
        case "stream_pause": message = .streamPause(try p(StreamPausePayload.self))
        case "stream_cancel": message = .streamCancel(try p(StreamCancelPayload.self))
        case "register": message = .register(try p(RegisterPayload.self))
        case "media_ready": message = .mediaReady(try p(MediaReadyPayload.self))
        case "media_started": message = .mediaStarted(try p(MediaStartedPayload.self))
        case "media_ended": message = .mediaEnded(try p(MediaEndedPayload.self))
        case "media_failed": message = .mediaFailed(try p(MediaFailedPayload.self))
        case "media_cancelled": message = .mediaCancelled(try p(MediaCancelledPayload.self))
        case "set_dnd": message = .setDND(try p(SetDNDPayload.self))
        case "stream_ready": message = .streamReady(try p(StreamReadyPayload.self))
        case "stream_started": message = .streamStarted(try p(StreamStartedPayload.self))
        case "stream_progress": message = .streamProgress(try p(StreamProgressPayload.self))
        case "stream_rebuffer": message = .streamRebuffer(try p(StreamRebufferPayload.self))
        case "stream_failed": message = .streamFailed(try p(StreamFailedPayload.self))
        case "stream_ended": message = .streamEnded(try p(StreamEndedPayload.self))
        case "stream_cancelled": message = .streamCancelled(try p(StreamCancelledPayload.self))
        case "live_ptt_start": message = .livePTTStart(try p(LivePTTStartPayload.self))
        case "live_ptt_accept": message = .livePTTAccept(try p(LivePTTAcceptPayload.self))
        case "live_ptt_reject": message = .livePTTReject(try p(LivePTTRejectPayload.self))
        case "live_ptt_end": message = .livePTTEnd(try p(LivePTTEndPayload.self))
        case "live_ptt_cancel": message = .livePTTCancel(try p(LivePTTCancelPayload.self))
        case "live_ptt_failed": message = .livePTTFailed(try p(LivePTTFailedPayload.self))
        case "live_ptt_receipt": message = .livePTTReceipt(try p(LivePTTReceiptPayload.self))
        case "live_ptt_state": message = .livePTTState(try p(LivePTTStatePayload.self))
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
        case "user_pause": message = .userPause(try p(UserPausePayload.self))
        case "user_resume": message = .userResume(try p(UserResumePayload.self))
        case "set_provider": message = .setProvider(try p(SetProviderPayload.self))
        default: throw ProtocolError.unknownType(head.type)
        }
        try LivePTTValidation.validate(message)
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
        case .prepareMedia(let p): return try w(p)
        case .playMediaAt(let p): return try w(p)
        case .cancelMedia(let p): return try w(p)
        case .presenceUpdate(let p): return try w(p)
        case .streamLoad(let p): return try w(p)
        case .streamResumeAt(let p): return try w(p)
        case .streamSeek(let p): return try w(p)
        case .streamPause(let p): return try w(p)
        case .streamCancel(let p): return try w(p)
        case .register(let p): return try w(p)
        case .mediaReady(let p): return try w(p)
        case .mediaStarted(let p): return try w(p)
        case .mediaEnded(let p): return try w(p)
        case .mediaFailed(let p): return try w(p)
        case .mediaCancelled(let p): return try w(p)
        case .setDND(let p): return try w(p)
        case .streamReady(let p): return try w(p)
        case .streamStarted(let p): return try w(p)
        case .streamProgress(let p): return try w(p)
        case .streamRebuffer(let p): return try w(p)
        case .streamFailed(let p): return try w(p)
        case .streamEnded(let p): return try w(p)
        case .streamCancelled(let p): return try w(p)
        case .livePTTStart(let p): return try w(p)
        case .livePTTAccept(let p): return try w(p)
        case .livePTTReject(let p): return try w(p)
        case .livePTTEnd(let p): return try w(p)
        case .livePTTCancel(let p): return try w(p)
        case .livePTTFailed(let p): return try w(p)
        case .livePTTReceipt(let p): return try w(p)
        case .livePTTState(let p): return try w(p)
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
        case .userPause(let p): return try w(p)
        case .userResume(let p): return try w(p)
        }
    }
}

public enum StreamGenerationDecision: String, Equatable {
    case apply, duplicate, stale, invalid
}

public enum StreamEventKind: String {
    case ready, started, progress, rebuffer, failed, ended, cancelled
}

/// Mirrors the coordinator/Windows ordering gate. It is intentionally a pure
/// state machine and does not register a decoder or advertise stream_track_v1.
public struct StreamGenerationGuard: Equatable {
    public private(set) var playbackGeneration: Int64 = 0
    public private(set) var seekGeneration: Int64 = 0
    public private(set) var commandSequence: Int64 = 0
    public private(set) var eventSequence: Int64 = 0
    public private(set) var commandKind = ""
    public private(set) var eventKind: StreamEventKind?
    public private(set) var phase = ""

    public init() {}

    public mutating func acceptLoad(playback: Int64, seek: Int64,
                                    command: Int64) -> StreamGenerationDecision {
        guard playback > 0, seek == 0, command == 1 else { return .invalid }
        if playback < playbackGeneration { return .stale }
        if playback == playbackGeneration {
            return seek == seekGeneration && command == commandSequence && commandKind == "load"
                ? .duplicate : .stale
        }
        playbackGeneration = playback
        seekGeneration = 0
        commandSequence = command
        eventSequence = 0
        commandKind = "load"
        eventKind = nil
        phase = "loading"
        return .apply
    }

    public mutating func acceptCommand(playback: Int64, seek: Int64, command: Int64,
                                       kind: String) -> StreamGenerationDecision {
        guard playback == playbackGeneration, seek == seekGeneration else { return .stale }
        if command <= commandSequence {
            if command == commandSequence { return kind == commandKind ? .duplicate : .invalid }
            return .stale
        }
        guard command == commandSequence + 1, phase != "terminal" else { return .invalid }
        switch kind {
        case "resume":
            guard phase == "ready" || phase == "paused_ready" else { return .invalid }
            phase = "ready"
        case "pause":
            if phase == "started" { phase = "paused_ready" }
            else if phase == "rebuffering" { phase = "paused_loading" }
            else { return .invalid }
        case "cancel": break
        default: return .invalid
        }
        commandSequence = command
        commandKind = kind
        return .apply
    }

    public mutating func acceptSeek(playback: Int64, seek: Int64,
                                    command: Int64) -> StreamGenerationDecision {
        guard playback == playbackGeneration else { return .stale }
        if seek <= seekGeneration {
            return seek == seekGeneration && command == commandSequence && commandKind == "seek"
                ? .duplicate : .stale
        }
        guard seek == seekGeneration + 1, command == commandSequence + 1,
              phase != "terminal" else { return .invalid }
        seekGeneration = seek
        commandSequence = command
        eventSequence = 0
        commandKind = "seek"
        eventKind = nil
        phase = "loading"
        return .apply
    }

    public mutating func acceptEvent(playback: Int64, seek: Int64, event: Int64,
                                     kind: StreamEventKind) -> StreamGenerationDecision {
        guard playback == playbackGeneration, seek == seekGeneration else { return .stale }
        if event <= eventSequence {
            if event == eventSequence { return kind == eventKind ? .duplicate : .invalid }
            return .stale
        }
        guard event == eventSequence + 1, phase != "terminal" else { return .invalid }
        switch kind {
        case .ready:
            guard phase == "loading" || phase == "rebuffering" || phase == "paused_loading"
                else { return .invalid }
            phase = phase == "paused_loading" ? "paused_ready" : "ready"
        case .started:
            guard phase == "ready" else { return .invalid }
            phase = "started"
        case .progress:
            guard phase == "started" else { return .invalid }
        case .rebuffer:
            guard phase == "started" else { return .invalid }
            phase = "rebuffering"
        case .failed, .ended, .cancelled: phase = "terminal"
        }
        eventSequence = event
        eventKind = kind
        return .apply
    }

    public mutating func acceptReady(playback: Int64, seek: Int64, event: Int64,
                                     buffered: Int64, minimum: Int64) -> StreamGenerationDecision {
        guard minimum == ProtocolConstants.streamMinimumBufferedMs,
              buffered >= minimum else { return .invalid }
        return acceptEvent(playback: playback, seek: seek, event: event, kind: .ready)
    }
}

public enum StreamContract {
    public static func validate(load: StreamLoadPayload) -> Bool {
        let lowerHex = Set("0123456789abcdef")
        guard !load.streamId.isEmpty, !load.mediaId.isEmpty,
              load.playbackGeneration > 0, load.seekGeneration == 0,
              load.commandSequence == 1, load.variantManifest.hasPrefix("svm1."),
              load.variantManifest.utf8.count <= 512,
              load.variantManifest.utf8.allSatisfy({ byte in
                  (byte >= 0x30 && byte <= 0x39) || (byte >= 0x41 && byte <= 0x5a) ||
                  (byte >= 0x61 && byte <= 0x7a) || byte == 0x2e || byte == 0x5f || byte == 0x2d
              }), load.variantSizeBytes > 0,
              load.startPositionMs >= 0,
              load.minimumBufferedMs == ProtocolConstants.streamMinimumBufferedMs,
              load.readyDeadlineCoordMs > 0,
              load.variantSha256.count == 64,
              load.variantSha256.allSatisfy({ lowerHex.contains($0) }),
              load.variantEtag == "\"sha256-\(load.variantSha256)\"",
              load.variantUrl.hasPrefix("/v1/media/\(load.mediaId)/variants/"),
              !load.variantUrl.contains("://"),
              !load.variantUrl.contains("?"), !load.variantUrl.contains("#"),
              !load.variantUrl.contains("@") else { return false }
        return load.mixedVersionPolicy == ProtocolConstants.streamMixedVersionRequireAll ||
            load.mixedVersionPolicy == ProtocolConstants.streamMixedVersionSupportedOnlyWithReceipts
    }

    public static func validate(ready: StreamReadyPayload) -> Bool {
        !ready.streamId.isEmpty && ready.playbackGeneration > 0 && ready.seekGeneration >= 0 &&
            ready.eventSequence > 0 && ready.audiblePositionMs >= 0 &&
            ready.bufferedDurationMs >= ProtocolConstants.streamMinimumBufferedMs
    }
}
