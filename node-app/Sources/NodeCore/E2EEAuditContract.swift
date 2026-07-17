import Foundation

/// Dormant, audit-only E2EE state model. It is not connected to registration,
/// transport, storage, capture, or playback and implements no cryptography.
enum E2EEAuditContract {
    static let name = "e2ee-media-audit.v1"
    static let capability = "e2ee_media_v1"
}

enum E2EEAuditFailure: String, Error, Equatable {
    case downgrade
    case expiredGrant = "expired_grant"
    case foreignTarget = "foreign_target"
    case forkedEpoch = "forked_epoch"
    case invalidSignature = "invalid_signature"
    case malformed
    case nonceReuse = "nonce_reuse"
    case replay
    case staleEpoch = "stale_epoch"
    case tamperedManifest = "tampered_manifest"
    case unknownSuite = "unknown_suite"
}

struct E2EEAuditConfiguration {
    let allowedSuites: Set<String>
    let verify: ((String, String) -> Bool)?

    static let productionDisabled = E2EEAuditConfiguration(allowedSuites: [], verify: nil)
}

struct E2EEAuditMetadata: Codable, Equatable {
    var contract: String
    var capability: String
    var suite: String
    var eventID: String
    var groupID: String
    var actorID: String
    var deviceID: String
    var airID: String
    var targetSnapshotDigest: String
    var objectKind: String
    var objectID: String
    var epoch: UInt64
    var generation: UInt64
    var sequence: UInt64
    var nonce: String
    var expiresAtMS: Int64
    var manifestDigest: String
    var authenticatedDataDigest: String
    var signature: String
    var ciphertextURL: String

    enum CodingKeys: String, CodingKey {
        case contract, capability, suite, nonce, epoch, generation, sequence, signature
        case eventID = "event_id", groupID = "group_id", actorID = "actor_id"
        case deviceID = "device_id", airID = "air_id"
        case targetSnapshotDigest = "target_snapshot_digest"
        case objectKind = "object_kind", objectID = "object_id"
        case expiresAtMS = "expires_at_ms"
        case manifestDigest = "manifest_digest"
        case authenticatedDataDigest = "authenticated_data_digest"
        case ciphertextURL = "ciphertext_url"
    }

    static func decodeCoordinatorVisible(_ data: Data) throws -> E2EEAuditMetadata {
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw E2EEAuditFailure.malformed
        }
        let allowed = Set(CodingKeys.allCases.map(\.rawValue))
        guard Set(object.keys).isSubset(of: allowed) else { throw E2EEAuditFailure.malformed }
        return try JSONDecoder().decode(Self.self, from: data)
    }
}

extension E2EEAuditMetadata.CodingKeys: CaseIterable {}

struct E2EEAuditCommit: Codable, Equatable {
    var contract: String
    var capability: String
    var suite: String
    var eventID: String
    var groupID: String
    var actorID: String
    var deviceID: String
    var airID: String
    var previousEpoch: UInt64
    var epoch: UInt64
    var previousCommitDigest: String
    var commitDigest: String
    var targetSnapshotDigest: String
    var authenticatedDataDigest: String
    var signature: String

    enum CodingKeys: String, CodingKey {
        case contract, capability, suite, epoch, signature
        case eventID = "event_id", groupID = "group_id", actorID = "actor_id"
        case deviceID = "device_id", airID = "air_id", previousEpoch = "previous_epoch"
        case previousCommitDigest = "previous_commit_digest", commitDigest = "commit_digest"
        case targetSnapshotDigest = "target_snapshot_digest"
        case authenticatedDataDigest = "authenticated_data_digest"
    }
}

struct E2EEAuditState {
    var groupID: String
    var airID: String
    var targetSnapshotDigest: String
    var epoch: UInt64
    var commitDigest: String
    private(set) var seenEvents: Set<String> = []
    private(set) var seenNonces: Set<String> = []
    private(set) var lastSequences: [String: UInt64] = [:]

    mutating func remember(eventID: String) { seenEvents.insert(eventID) }
    mutating func remember(nonce: String) { seenNonces.insert(nonce) }

    mutating func accept(
        _ value: E2EEAuditMetadata,
        trustedManifestDigest: String,
        nowMS: Int64,
        configuration: E2EEAuditConfiguration
    ) throws {
        guard value.contract == E2EEAuditContract.name,
              value.capability == E2EEAuditContract.capability else { throw E2EEAuditFailure.downgrade }
        guard configuration.allowedSuites.contains(value.suite) else {
            throw E2EEAuditFailure.unknownSuite
        }
        guard !value.eventID.isEmpty, !value.groupID.isEmpty, !value.actorID.isEmpty,
              !value.deviceID.isEmpty, !value.airID.isEmpty, !value.objectID.isEmpty,
              !value.nonce.isEmpty, value.generation > 0, value.sequence > 0,
              value.targetSnapshotDigest.count == 64, value.manifestDigest.count == 64,
              value.authenticatedDataDigest.count == 64, value.ciphertextURL.hasPrefix("/") else {
            throw E2EEAuditFailure.malformed
        }
        guard ["clip", "track", "saved_cue", "live_ptt"].contains(value.objectKind) else {
            throw E2EEAuditFailure.malformed
        }
        guard value.manifestDigest == trustedManifestDigest else {
            throw E2EEAuditFailure.tamperedManifest
        }
        guard let verify = configuration.verify,
              verify(value.authenticatedDataDigest, value.signature) else {
            throw E2EEAuditFailure.invalidSignature
        }
        guard value.groupID == groupID, value.airID == airID,
              value.targetSnapshotDigest == targetSnapshotDigest else {
            throw E2EEAuditFailure.foreignTarget
        }
        if value.epoch < epoch { throw E2EEAuditFailure.staleEpoch }
        if value.epoch > epoch { throw E2EEAuditFailure.forkedEpoch }
        guard !seenEvents.contains(value.eventID) else { throw E2EEAuditFailure.replay }
        guard !seenNonces.contains(value.nonce) else { throw E2EEAuditFailure.nonceReuse }
        guard value.expiresAtMS > nowMS else {
            throw E2EEAuditFailure.expiredGrant
        }
        let sequenceKey = "\(value.deviceID)/\(value.objectID)/\(value.generation)"
        guard value.sequence > (lastSequences[sequenceKey] ?? 0) else {
            throw E2EEAuditFailure.replay
        }
        seenEvents.insert(value.eventID)
        seenNonces.insert(value.nonce)
        lastSequences[sequenceKey] = value.sequence
    }

    mutating func apply(
        _ value: E2EEAuditCommit,
        configuration: E2EEAuditConfiguration
    ) throws {
        guard value.contract == E2EEAuditContract.name,
              value.capability == E2EEAuditContract.capability else {
            throw E2EEAuditFailure.downgrade
        }
        guard configuration.allowedSuites.contains(value.suite) else {
            throw E2EEAuditFailure.unknownSuite
        }
        guard let verify = configuration.verify,
              verify(value.authenticatedDataDigest, value.signature) else {
            throw E2EEAuditFailure.invalidSignature
        }
        guard value.groupID == groupID, value.airID == airID,
              !value.targetSnapshotDigest.isEmpty else { throw E2EEAuditFailure.foreignTarget }
        guard !seenEvents.contains(value.eventID) else { throw E2EEAuditFailure.replay }
        if value.previousEpoch < epoch || value.epoch <= epoch {
            throw E2EEAuditFailure.staleEpoch
        }
        guard value.previousEpoch == epoch, value.epoch == epoch + 1,
              value.previousCommitDigest == commitDigest else {
            throw E2EEAuditFailure.forkedEpoch
        }
        guard !value.actorID.isEmpty, !value.deviceID.isEmpty,
              value.commitDigest.count == 64 else { throw E2EEAuditFailure.malformed }
        epoch = value.epoch
        commitDigest = value.commitDigest
        targetSnapshotDigest = value.targetSnapshotDigest
        seenEvents.insert(value.eventID)
    }
}
