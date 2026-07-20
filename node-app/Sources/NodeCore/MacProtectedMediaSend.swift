import CryptoKit
import Foundation

public enum MacProtectedMediaSendFailure: String, Error, Equatable, LocalizedError, Sendable {
  case busy
  case cancelled
  case downgradeForbidden = "downgrade_forbidden"
  case invalidArtifact = "invalid_artifact"
  case invalidRequest = "invalid_request"
  case localCleanup = "local_cleanup_failed"
  case persistence = "persistence_failed"
  case productionDisabled = "production_disabled"
  case quotaExceeded = "quota_exceeded"
  case sourceUnavailable = "source_unavailable"
  case staleKeyState = "stale_key_state"
  case targetChanged = "target_changed"
  case transport
  case unsupportedTarget = "unsupported_target"

  public var errorDescription: String? {
    switch self {
    case .productionDisabled:
      "Protected media is not available in this build."
    case .downgradeForbidden, .unsupportedTarget, .targetChanged:
      "Protected sending cannot continue for the selected recipients."
    default:
      "Protected media could not be sent safely."
    }
  }
}

public enum MacProtectedMediaKind: String, Codable, CaseIterable, Sendable {
  case clip
  case savedCue = "saved_cue"
  case track
}

public enum MacProtectedMediaPlaintextPolicy: String, Codable, Sendable {
  /// A file selected outside the app's private draft root is never deleted.
  case userOwnedRetain = "user_owned_retain"
  /// A recording/cue draft under the private root is removed after confirmed
  /// publication, explicit cancellation, or expiry recovery.
  case appPrivateDeleteOnTerminal = "app_private_delete_on_terminal"
}

public struct MacProtectedMediaRecipient: Codable, Equatable, Sendable {
  public let deviceID: String
  public let verified: Bool
  public let currentMember: Bool
  public let supportsProtectedMedia: Bool

  public init(
    deviceID: String, verified: Bool, currentMember: Bool,
    supportsProtectedMedia: Bool
  ) {
    self.deviceID = deviceID
    self.verified = verified
    self.currentMember = currentMember
    self.supportsProtectedMedia = supportsProtectedMedia
  }
}

public struct MacProtectedMediaSendRequest: Sendable {
  public let draftID: String
  public let sourceObjectID: String
  public let sourceURL: URL
  public let plaintextPolicy: MacProtectedMediaPlaintextPolicy
  public let kind: MacProtectedMediaKind
  public let authorDeviceID: String
  public let groupID: String
  public let expectedGroupRevision: UInt64
  public let expectedTargetSnapshotDigest: String
  public let recipients: [MacProtectedMediaRecipient]
  public let declaredDurationMS: Int64
  public let rightsConfirmed: Bool
  public let targetConfirmed: Bool
  public let expiresAtMS: Int64

  public init(
    draftID: String, sourceObjectID: String, sourceURL: URL,
    plaintextPolicy: MacProtectedMediaPlaintextPolicy, kind: MacProtectedMediaKind,
    authorDeviceID: String, groupID: String, expectedGroupRevision: UInt64,
    expectedTargetSnapshotDigest: String, recipients: [MacProtectedMediaRecipient],
    declaredDurationMS: Int64, rightsConfirmed: Bool, targetConfirmed: Bool,
    expiresAtMS: Int64
  ) {
    self.draftID = draftID
    self.sourceObjectID = sourceObjectID
    self.sourceURL = sourceURL
    self.plaintextPolicy = plaintextPolicy
    self.kind = kind
    self.authorDeviceID = authorDeviceID
    self.groupID = groupID
    self.expectedGroupRevision = expectedGroupRevision
    self.expectedTargetSnapshotDigest = expectedTargetSnapshotDigest
    self.recipients = recipients
    self.declaredDurationMS = declaredDurationMS
    self.rightsConfirmed = rightsConfirmed
    self.targetConfirmed = targetConfirmed
    self.expiresAtMS = expiresAtMS
  }
}

public struct MacProtectedMediaSealContext: Equatable, Sendable {
  public let draftID: String
  public let sourceObjectID: String
  public let kind: MacProtectedMediaKind
  public let groupID: String
  public let epoch: UInt64
  public let generation: UInt64
  public let targetSnapshotDigest: String
  public let recipientDeviceIDs: [String]
  public let declaredDurationMS: Int64
  public let expiresAtMS: Int64
}

public struct MacProtectedMediaCiphertextChunk: Equatable, Sendable {
  public let nonce: String
  public let ciphertext: Data

  public init(nonce: String, ciphertext: Data) {
    self.nonce = nonce
    self.ciphertext = ciphertext
  }
}

/// Output of the independently selected local codec/container/crypto layer.
/// NodeCore validates and routes these bytes but deliberately implements no
/// cipher suite itself while the production selection gates remain open.
public struct MacProtectedMediaSealedArtifact: Equatable, Sendable {
  public let contract: String
  public let capability: String
  public let suite: String
  public let container: String
  public let context: MacProtectedMediaSealContext
  public let encryptedManifest: Data
  public let opaqueKeyEnvelopes: Data
  public let authenticatedManifest: Data
  public let signature: Data
  public let chunks: [MacProtectedMediaCiphertextChunk]

  public init(
    contract: String, capability: String, suite: String, container: String,
    context: MacProtectedMediaSealContext, encryptedManifest: Data,
    opaqueKeyEnvelopes: Data, authenticatedManifest: Data, signature: Data,
    chunks: [MacProtectedMediaCiphertextChunk]
  ) {
    self.contract = contract
    self.capability = capability
    self.suite = suite
    self.container = container
    self.context = context
    self.encryptedManifest = encryptedManifest
    self.opaqueKeyEnvelopes = opaqueKeyEnvelopes
    self.authenticatedManifest = authenticatedManifest
    self.signature = signature
    self.chunks = chunks
  }
}

public protocol MacProtectedMediaSealing: Sendable {
  var productionApproved: Bool { get }
  func seal(
    sourceURL: URL, context: MacProtectedMediaSealContext,
    identity: MacE2EEDeviceIdentityLease, groupState: MacE2EEGroupStateLease
  ) async throws -> MacProtectedMediaSealedArtifact
  func verify(_ artifact: MacProtectedMediaSealedArtifact) async -> Bool
}

public struct MacProtectedMediaStageRequest: Equatable, Sendable {
  public let idempotencyKey: String
  public let sourceObjectID: String
  public let kind: MacProtectedMediaKind
  public let authorDeviceID: String
  public let groupID: String
  public let epoch: UInt64
  public let generation: UInt64
  public let targetSnapshotDigest: String
  public let manifestDigest: String
  public let ciphertextDigest: String
  public let ciphertextSize: Int64
  public let chunkCount: Int
  public let declaredDurationMS: Int64
  public let encryptedManifest: Data
  public let opaqueKeyEnvelopes: Data
  public let authenticatedManifest: Data
  public let signature: Data
}

public struct MacProtectedMediaRemoteObject: Codable, Equatable, Sendable {
  public let objectID: String
  public let revision: UInt64

  public init(objectID: String, revision: UInt64) {
    self.objectID = objectID
    self.revision = revision
  }
}

public protocol MacProtectedMediaUploading: Sendable {
  /// Every operation is idempotent for the exact idempotency key and bytes.
  /// Reusing a key with different bytes must fail closed.
  func stage(_ request: MacProtectedMediaStageRequest) async throws
    -> MacProtectedMediaRemoteObject
  func putChunk(
    objectID: String, idempotencyKey: String, index: Int, byteOffset: Int64,
    ciphertextDigest: String, ciphertext: Data
  ) async throws
  func finalize(
    objectID: String, idempotencyKey: String, expectedRevision: UInt64
  ) async throws -> MacProtectedMediaRemoteObject
  func delete(
    objectID: String, idempotencyKey: String, expectedRevision: UInt64
  ) async throws
}

public struct MacProtectedMediaSendProgress: Equatable, Sendable {
  public enum Phase: String, Equatable, Sendable {
    case preparing
    case staging
    case uploading
    case finalizing
    case published
  }
  public let phase: Phase
  public let completedBytes: Int64
  public let totalBytes: Int64
}

public struct MacProtectedMediaPublication: Equatable, Sendable {
  public let draftID: String
  public let objectID: String
  public let revision: UInt64
  public let epoch: UInt64
  public let generation: UInt64
  public let manifestDigest: String
  public let ciphertextDigest: String
}

public actor MacProtectedMediaSendService {
  public static let maximumPlaintextBytes: Int64 = 64 << 20
  public static let maximumCiphertextBytes: Int64 = 64 << 20
  public static let maximumChunkBytes = 1 << 20
  public static let maximumChunks = 1024
  public static let maximumDraftLifetimeMS: Int64 = 24 * 60 * 60 * 1000
  public static let recoveryLimit = 100

  private enum DraftPhase: String, Codable { case prepared, staged, uploading, finalizing }
  private struct StoredChunk: Codable, Equatable {
    let index: Int
    let byteOffset: Int64
    let size: Int
    let digest: String
    let nonce: String
  }
  private struct StoredDraft: Codable {
    let version: Int
    let draftID: String
    let sourceObjectID: String
    let sourcePath: String
    let sourceFingerprint: String
    let plaintextPolicy: MacProtectedMediaPlaintextPolicy
    let kind: MacProtectedMediaKind
    let authorDeviceID: String
    let context: StoredContext
    let contract: String
    let capability: String
    let suite: String
    let container: String
    let encryptedManifest: Data
    let opaqueKeyEnvelopes: Data
    let authenticatedManifest: Data
    let signature: Data
    let manifestDigest: String
    let ciphertextDigest: String
    let ciphertextSize: Int64
    let chunks: [StoredChunk]
    let createdAtMS: Int64
    let expiresAtMS: Int64
    var phase: DraftPhase
    var nextChunkIndex: Int
    var remote: MacProtectedMediaRemoteObject?
  }
  private struct StoredContext: Codable, Equatable {
    let draftID: String
    let sourceObjectID: String
    let kind: MacProtectedMediaKind
    let groupID: String
    let epoch: UInt64
    let generation: UInt64
    let targetSnapshotDigest: String
    let recipientDeviceIDs: [String]
    let declaredDurationMS: Int64
    let expiresAtMS: Int64

    init(_ value: MacProtectedMediaSealContext) {
      draftID = value.draftID
      sourceObjectID = value.sourceObjectID
      kind = value.kind
      groupID = value.groupID
      epoch = value.epoch
      generation = value.generation
      targetSnapshotDigest = value.targetSnapshotDigest
      recipientDeviceIDs = value.recipientDeviceIDs
      declaredDurationMS = value.declaredDurationMS
      expiresAtMS = value.expiresAtMS
    }

    var value: MacProtectedMediaSealContext {
      MacProtectedMediaSealContext(
        draftID: draftID, sourceObjectID: sourceObjectID, kind: kind, groupID: groupID,
        epoch: epoch, generation: generation, targetSnapshotDigest: targetSnapshotDigest,
        recipientDeviceIDs: recipientDeviceIDs, declaredDurationMS: declaredDurationMS,
        expiresAtMS: expiresAtMS)
    }
  }

  private let keyState: MacE2EEKeyStateRepository
  private let sealer: any MacProtectedMediaSealing
  private let uploader: any MacProtectedMediaUploading
  private let ciphertextRoot: URL
  private let plaintextDraftRoot: URL
  private let fileManager: FileManager
  private let fixtureMode: Bool
  private var activeDrafts: Set<String> = []

  public init(
    keyState: MacE2EEKeyStateRepository, sealer: any MacProtectedMediaSealing,
    uploader: any MacProtectedMediaUploading, ciphertextRoot: URL,
    plaintextDraftRoot: URL, fileManager: FileManager = .default
  ) throws {
    try keyState.claimProtectedMediaSendOwnership()
    self.keyState = keyState
    self.sealer = sealer
    self.uploader = uploader
    self.ciphertextRoot = ciphertextRoot.standardizedFileURL
    self.plaintextDraftRoot = plaintextDraftRoot.standardizedFileURL
    self.fileManager = fileManager
    self.fixtureMode = false
    try Self.preparePrivateDirectory(self.ciphertextRoot, fileManager: fileManager)
  }

  /// Repository-only constructor. It is internal so an application target
  /// cannot enable an unapproved provider by configuration or environment.
  init(
    auditFixtureKeyState keyState: MacE2EEKeyStateRepository,
    sealer: any MacProtectedMediaSealing, uploader: any MacProtectedMediaUploading,
    ciphertextRoot: URL, plaintextDraftRoot: URL, fileManager: FileManager = .default
  ) throws {
    try keyState.claimProtectedMediaSendOwnership()
    self.keyState = keyState
    self.sealer = sealer
    self.uploader = uploader
    self.ciphertextRoot = ciphertextRoot.standardizedFileURL
    self.plaintextDraftRoot = plaintextDraftRoot.standardizedFileURL
    self.fileManager = fileManager
    self.fixtureMode = true
    try Self.preparePrivateDirectory(self.ciphertextRoot, fileManager: fileManager)
  }

  public func send(
    _ request: MacProtectedMediaSendRequest, nowMS: Int64,
    progress: (@Sendable (MacProtectedMediaSendProgress) -> Void)? = nil
  ) async throws -> MacProtectedMediaPublication {
    guard sealer.productionApproved || fixtureMode else {
      throw MacProtectedMediaSendFailure.productionDisabled
    }
    try validate(request, nowMS: nowMS)
    guard activeDrafts.insert(request.draftID).inserted else {
      throw MacProtectedMediaSendFailure.busy
    }
    defer { activeDrafts.remove(request.draftID) }

    var draft: StoredDraft
    if fileManager.fileExists(atPath: stateURL(request.draftID).path) {
      draft = try loadDraft(request.draftID)
      try await validateResume(draft, request: request, nowMS: nowMS)
    } else {
      progress?(.init(phase: .preparing, completedBytes: 0, totalBytes: 0))
      draft = try await prepare(request, nowMS: nowMS)
    }
    return try await publish(draft, progress: progress)
  }

  public func cancel(draftID: String) async throws {
    guard Self.validToken(draftID), activeDrafts.insert(draftID).inserted else {
      throw MacProtectedMediaSendFailure.busy
    }
    defer { activeDrafts.remove(draftID) }
    guard fileManager.fileExists(atPath: stateURL(draftID).path) else { return }
    let draft = try loadDraft(draftID)
    if let remote = draft.remote {
      do {
        try await uploader.delete(
          objectID: remote.objectID, idempotencyKey: "mac-protected-delete-\(draftID)",
          expectedRevision: remote.revision)
      } catch { throw MacProtectedMediaSendFailure.transport }
    }
    try cleanup(draft, deletePlaintext: draft.plaintextPolicy == .appPrivateDeleteOnTerminal)
  }

  @discardableResult
  public func recoverExpiredDrafts(nowMS: Int64, limit: Int = recoveryLimit) throws -> Int {
    guard nowMS > 0, (1...Self.recoveryLimit).contains(limit) else {
      throw MacProtectedMediaSendFailure.invalidRequest
    }
    let directories =
      (try? fileManager.contentsOfDirectory(
        at: ciphertextRoot, includingPropertiesForKeys: nil,
        options: [.skipsHiddenFiles])) ?? []
    var removed = 0
    for directory in directories.sorted(by: { $0.lastPathComponent < $1.lastPathComponent }) {
      if removed >= limit { break }
      guard Self.validToken(directory.lastPathComponent),
        fileManager.fileExists(atPath: directory.appendingPathComponent("state.json").path),
        let draft = try? loadDraft(directory.lastPathComponent), draft.expiresAtMS <= nowMS
      else { continue }
      try cleanup(draft, deletePlaintext: draft.plaintextPolicy == .appPrivateDeleteOnTerminal)
      removed += 1
    }
    return removed
  }

  private func prepare(_ request: MacProtectedMediaSendRequest, nowMS: Int64) async throws
    -> StoredDraft
  {
    let sourceFingerprint = try Self.fileDigest(
      request.sourceURL, maximumBytes: Self.maximumPlaintextBytes, fileManager: fileManager)
    let group: MacE2EEGroupStateLease
    do {
      let identity = try keyState.loadDeviceIdentity(deviceID: request.authorDeviceID)
      defer { identity.destroy() }
      group = try keyState.loadGroupState(
        installationID: identity.metadata.installationID, groupID: request.groupID)
    } catch { throw MacProtectedMediaSendFailure.staleKeyState }
    defer { group.destroy() }
    guard group.metadata.revision == request.expectedGroupRevision,
      group.metadata.targetSnapshotDigest == request.expectedTargetSnapshotDigest
    else { throw MacProtectedMediaSendFailure.targetChanged }

    let reservation: MacE2EESendReservation
    do {
      reservation = try keyState.reserveSendGeneration(
        installationID: group.metadata.installationID, groupID: request.groupID,
        domain: "media", expectedRevision: group.metadata.revision, nowMS: nowMS)
    } catch { throw MacProtectedMediaSendFailure.staleKeyState }

    let currentGroup: MacE2EEGroupStateLease
    let identity: MacE2EEDeviceIdentityLease
    do {
      currentGroup = try keyState.loadGroupState(
        installationID: group.metadata.installationID, groupID: request.groupID)
      identity = try keyState.loadDeviceIdentity(deviceID: request.authorDeviceID)
    } catch { throw MacProtectedMediaSendFailure.staleKeyState }
    defer {
      currentGroup.destroy()
      identity.destroy()
    }
    guard currentGroup.metadata.revision == reservation.revision,
      currentGroup.metadata.epoch == reservation.epoch,
      currentGroup.metadata.targetSnapshotDigest == request.expectedTargetSnapshotDigest
    else { throw MacProtectedMediaSendFailure.staleKeyState }

    let context = MacProtectedMediaSealContext(
      draftID: request.draftID, sourceObjectID: request.sourceObjectID, kind: request.kind,
      groupID: request.groupID, epoch: reservation.epoch,
      generation: reservation.generation,
      targetSnapshotDigest: request.expectedTargetSnapshotDigest,
      recipientDeviceIDs: request.recipients.map(\.deviceID).sorted(),
      declaredDurationMS: request.declaredDurationMS, expiresAtMS: request.expiresAtMS)
    let artifact: MacProtectedMediaSealedArtifact
    do {
      artifact = try await sealer.seal(
        sourceURL: request.sourceURL, context: context, identity: identity,
        groupState: currentGroup)
    } catch is CancellationError { throw MacProtectedMediaSendFailure.cancelled } catch {
      throw MacProtectedMediaSendFailure.invalidArtifact
    }
    guard await validate(artifact, expected: context) else {
      throw MacProtectedMediaSendFailure.invalidArtifact
    }
    return try persistPrepared(
      artifact, request: request, authorDeviceID: identity.metadata.deviceID,
      sourceFingerprint: sourceFingerprint, nowMS: nowMS)
  }

  private func publish(
    _ initial: StoredDraft,
    progress: (@Sendable (MacProtectedMediaSendProgress) -> Void)?
  ) async throws -> MacProtectedMediaPublication {
    var draft = initial
    let stageRequest = MacProtectedMediaStageRequest(
      idempotencyKey: "mac-protected-stage-\(draft.draftID)",
      sourceObjectID: draft.sourceObjectID, kind: draft.kind,
      authorDeviceID: draft.authorDeviceID, groupID: draft.context.groupID,
      epoch: draft.context.epoch, generation: draft.context.generation,
      targetSnapshotDigest: draft.context.targetSnapshotDigest,
      manifestDigest: draft.manifestDigest, ciphertextDigest: draft.ciphertextDigest,
      ciphertextSize: draft.ciphertextSize, chunkCount: draft.chunks.count,
      declaredDurationMS: draft.context.declaredDurationMS,
      encryptedManifest: draft.encryptedManifest,
      opaqueKeyEnvelopes: draft.opaqueKeyEnvelopes,
      authenticatedManifest: draft.authenticatedManifest, signature: draft.signature)
    if draft.remote == nil {
      progress?(.init(phase: .staging, completedBytes: 0, totalBytes: draft.ciphertextSize))
      do { draft.remote = try await uploader.stage(stageRequest) } catch {
        throw MacProtectedMediaSendFailure.transport
      }
      draft.phase = .staged
      try save(draft)
    }
    guard let remote = draft.remote else { throw MacProtectedMediaSendFailure.persistence }
    draft.phase = .uploading
    try save(draft)
    var completed = draft.chunks.prefix(draft.nextChunkIndex).reduce(Int64(0)) {
      $0 + Int64($1.size)
    }
    for stored in draft.chunks.dropFirst(draft.nextChunkIndex) {
      try Task.checkCancellation()
      let bytes = try loadChunk(draft.draftID, stored)
      do {
        try await uploader.putChunk(
          objectID: remote.objectID,
          idempotencyKey: "mac-protected-chunk-\(draft.draftID)-\(stored.index)",
          index: stored.index, byteOffset: stored.byteOffset,
          ciphertextDigest: stored.digest, ciphertext: bytes)
      } catch is CancellationError { throw MacProtectedMediaSendFailure.cancelled } catch {
        throw MacProtectedMediaSendFailure.transport
      }
      completed += Int64(stored.size)
      draft.nextChunkIndex = stored.index + 1
      try save(draft)
      progress?(
        .init(
          phase: .uploading, completedBytes: completed, totalBytes: draft.ciphertextSize))
    }
    draft.phase = .finalizing
    try save(draft)
    progress?(
      .init(
        phase: .finalizing, completedBytes: draft.ciphertextSize,
        totalBytes: draft.ciphertextSize))
    let published: MacProtectedMediaRemoteObject
    do {
      published = try await uploader.finalize(
        objectID: remote.objectID, idempotencyKey: "mac-protected-finalize-\(draft.draftID)",
        expectedRevision: remote.revision)
    } catch { throw MacProtectedMediaSendFailure.transport }
    try cleanup(draft, deletePlaintext: draft.plaintextPolicy == .appPrivateDeleteOnTerminal)
    progress?(
      .init(
        phase: .published, completedBytes: draft.ciphertextSize,
        totalBytes: draft.ciphertextSize))
    return MacProtectedMediaPublication(
      draftID: draft.draftID, objectID: published.objectID, revision: published.revision,
      epoch: draft.context.epoch, generation: draft.context.generation,
      manifestDigest: draft.manifestDigest, ciphertextDigest: draft.ciphertextDigest)
  }

  private func validate(_ request: MacProtectedMediaSendRequest, nowMS: Int64) throws {
    guard Self.validToken(request.draftID), Self.validIdentifier(request.sourceObjectID),
      Self.validIdentifier(request.authorDeviceID), Self.validIdentifier(request.groupID),
      request.expectedGroupRevision > 0,
      Self.validDigest(request.expectedTargetSnapshotDigest), request.declaredDurationMS >= 0,
      nowMS > 0, request.expiresAtMS > nowMS,
      request.expiresAtMS - nowMS <= Self.maximumDraftLifetimeMS,
      request.rightsConfirmed, request.targetConfirmed, !request.recipients.isEmpty,
      request.recipients.count <= 64,
      Set(request.recipients.map(\.deviceID)).count == request.recipients.count,
      request.recipients.allSatisfy({ Self.validIdentifier($0.deviceID) })
    else { throw MacProtectedMediaSendFailure.invalidRequest }
    if request.recipients.contains(where: { !$0.currentMember || !$0.verified }) {
      throw MacProtectedMediaSendFailure.targetChanged
    }
    if request.recipients.contains(where: { !$0.supportsProtectedMedia }) {
      throw MacProtectedMediaSendFailure.unsupportedTarget
    }
    if request.plaintextPolicy == .appPrivateDeleteOnTerminal,
      !Self.isOwned(request.sourceURL, by: plaintextDraftRoot)
    {
      throw MacProtectedMediaSendFailure.invalidRequest
    }
  }

  private func validate(
    _ artifact: MacProtectedMediaSealedArtifact, expected: MacProtectedMediaSealContext
  ) async -> Bool {
    guard artifact.contract == "e2ee-media-audit.v1",
      artifact.capability == "e2ee_media_v1", artifact.context == expected,
      !artifact.suite.isEmpty, artifact.suite.utf8.count <= 128,
      !artifact.container.isEmpty, artifact.container.utf8.count <= 128,
      !artifact.encryptedManifest.isEmpty, artifact.encryptedManifest.count <= 1 << 20,
      !artifact.opaqueKeyEnvelopes.isEmpty, artifact.opaqueKeyEnvelopes.count <= 1 << 20,
      !artifact.authenticatedManifest.isEmpty, artifact.authenticatedManifest.count <= 1 << 20,
      !artifact.signature.isEmpty, artifact.signature.count <= 1 << 16,
      !artifact.chunks.isEmpty, artifact.chunks.count <= Self.maximumChunks,
      Set(artifact.chunks.map(\.nonce)).count == artifact.chunks.count,
      artifact.chunks.allSatisfy({
        !$0.nonce.isEmpty && $0.nonce.utf8.count <= 256 && !$0.ciphertext.isEmpty
          && $0.ciphertext.count <= Self.maximumChunkBytes
      }),
      artifact.chunks.reduce(Int64(0), { $0 + Int64($1.ciphertext.count) })
        <= Self.maximumCiphertextBytes
    else { return false }
    return await sealer.verify(artifact)
  }

  private func persistPrepared(
    _ artifact: MacProtectedMediaSealedArtifact, request: MacProtectedMediaSendRequest,
    authorDeviceID: String, sourceFingerprint: String, nowMS: Int64
  ) throws -> StoredDraft {
    let directory = draftDirectory(request.draftID)
    guard !fileManager.fileExists(atPath: directory.path) else {
      throw MacProtectedMediaSendFailure.persistence
    }
    do {
      try Self.preparePrivateDirectory(directory, fileManager: fileManager)
      var chunks: [StoredChunk] = []
      var offset: Int64 = 0
      var whole = SHA256()
      for (index, value) in artifact.chunks.enumerated() {
        let digest = Self.digest(value.ciphertext)
        let metadata = StoredChunk(
          index: index, byteOffset: offset, size: value.ciphertext.count,
          digest: digest, nonce: value.nonce)
        try value.ciphertext.write(to: chunkURL(request.draftID, index), options: [.atomic])
        try fileManager.setAttributes(
          [.posixPermissions: 0o600], ofItemAtPath: chunkURL(request.draftID, index).path)
        chunks.append(metadata)
        offset += Int64(value.ciphertext.count)
        whole.update(data: value.ciphertext)
      }
      let draft = StoredDraft(
        version: 1, draftID: request.draftID, sourceObjectID: request.sourceObjectID,
        sourcePath: request.sourceURL.resolvingSymlinksInPath().standardizedFileURL.path,
        sourceFingerprint: sourceFingerprint, plaintextPolicy: request.plaintextPolicy,
        kind: request.kind, authorDeviceID: authorDeviceID,
        context: StoredContext(artifact.context), contract: artifact.contract,
        capability: artifact.capability, suite: artifact.suite, container: artifact.container,
        encryptedManifest: artifact.encryptedManifest,
        opaqueKeyEnvelopes: artifact.opaqueKeyEnvelopes,
        authenticatedManifest: artifact.authenticatedManifest, signature: artifact.signature,
        manifestDigest: Self.digest(artifact.encryptedManifest),
        ciphertextDigest: whole.finalize().map { String(format: "%02x", $0) }.joined(),
        ciphertextSize: offset, chunks: chunks, createdAtMS: nowMS,
        expiresAtMS: request.expiresAtMS, phase: .prepared, nextChunkIndex: 0,
        remote: nil)
      try save(draft)
      return draft
    } catch let error as MacProtectedMediaSendFailure {
      try? fileManager.removeItem(at: directory)
      throw error
    } catch {
      try? fileManager.removeItem(at: directory)
      throw MacProtectedMediaSendFailure.persistence
    }
  }

  private func validateResume(
    _ draft: StoredDraft, request: MacProtectedMediaSendRequest, nowMS: Int64
  ) async throws {
    guard draft.version == 1, draft.draftID == request.draftID,
      draft.sourceObjectID == request.sourceObjectID,
      draft.sourcePath == request.sourceURL.resolvingSymlinksInPath().standardizedFileURL.path,
      draft.plaintextPolicy == request.plaintextPolicy, draft.kind == request.kind,
      draft.context.groupID == request.groupID,
      draft.context.targetSnapshotDigest == request.expectedTargetSnapshotDigest,
      draft.context.recipientDeviceIDs == request.recipients.map(\.deviceID).sorted(),
      draft.context.declaredDurationMS == request.declaredDurationMS,
      draft.expiresAtMS == request.expiresAtMS, draft.expiresAtMS > nowMS,
      draft.nextChunkIndex >= 0, draft.nextChunkIndex <= draft.chunks.count,
      try Self.fileDigest(
        request.sourceURL, maximumBytes: Self.maximumPlaintextBytes,
        fileManager: fileManager) == draft.sourceFingerprint
    else { throw MacProtectedMediaSendFailure.invalidRequest }
    try validateStoredCiphertext(draft)
    let artifact = MacProtectedMediaSealedArtifact(
      contract: draft.contract, capability: draft.capability, suite: draft.suite,
      container: draft.container, context: draft.context.value,
      encryptedManifest: draft.encryptedManifest,
      opaqueKeyEnvelopes: draft.opaqueKeyEnvelopes,
      authenticatedManifest: draft.authenticatedManifest, signature: draft.signature,
      chunks: try draft.chunks.map {
        MacProtectedMediaCiphertextChunk(
          nonce: $0.nonce, ciphertext: try loadChunk(draft.draftID, $0))
      })
    guard await sealer.verify(artifact) else {
      throw MacProtectedMediaSendFailure.invalidArtifact
    }
  }

  private func validateStoredCiphertext(_ draft: StoredDraft) throws {
    guard draft.contract == "e2ee-media-audit.v1", draft.capability == "e2ee_media_v1",
      Self.validDigest(draft.manifestDigest),
      Self.digest(draft.encryptedManifest) == draft.manifestDigest,
      !draft.opaqueKeyEnvelopes.isEmpty, !draft.authenticatedManifest.isEmpty,
      !draft.signature.isEmpty, !draft.chunks.isEmpty,
      draft.chunks.count <= Self.maximumChunks,
      Set(draft.chunks.map(\.nonce)).count == draft.chunks.count
    else { throw MacProtectedMediaSendFailure.persistence }
    var offset: Int64 = 0
    var whole = SHA256()
    for (index, metadata) in draft.chunks.enumerated() {
      guard metadata.index == index, metadata.byteOffset == offset,
        metadata.size > 0, metadata.size <= Self.maximumChunkBytes,
        !metadata.nonce.isEmpty
      else { throw MacProtectedMediaSendFailure.persistence }
      let bytes = try loadChunk(draft.draftID, metadata)
      whole.update(data: bytes)
      offset += Int64(bytes.count)
    }
    let digest = whole.finalize().map { String(format: "%02x", $0) }.joined()
    guard offset == draft.ciphertextSize, offset <= Self.maximumCiphertextBytes,
      digest == draft.ciphertextDigest
    else { throw MacProtectedMediaSendFailure.persistence }
  }

  private func loadChunk(_ draftID: String, _ metadata: StoredChunk) throws -> Data {
    do {
      let bytes = try Data(contentsOf: chunkURL(draftID, metadata.index), options: .mappedIfSafe)
      guard bytes.count == metadata.size, Self.digest(bytes) == metadata.digest else {
        throw MacProtectedMediaSendFailure.persistence
      }
      return bytes
    } catch let error as MacProtectedMediaSendFailure { throw error } catch {
      throw MacProtectedMediaSendFailure.persistence
    }
  }

  private func save(_ draft: StoredDraft) throws {
    do {
      let encoder = JSONEncoder()
      encoder.outputFormatting = [.sortedKeys]
      try encoder.encode(draft).write(to: stateURL(draft.draftID), options: [.atomic])
      try fileManager.setAttributes(
        [.posixPermissions: 0o600], ofItemAtPath: stateURL(draft.draftID).path)
    } catch { throw MacProtectedMediaSendFailure.persistence }
  }

  private func loadDraft(_ draftID: String) throws -> StoredDraft {
    do {
      let draft = try JSONDecoder().decode(
        StoredDraft.self, from: Data(contentsOf: stateURL(draftID)))
      try validateStoredCiphertext(draft)
      return draft
    } catch let error as MacProtectedMediaSendFailure { throw error } catch {
      throw MacProtectedMediaSendFailure.persistence
    }
  }

  private func cleanup(_ draft: StoredDraft, deletePlaintext: Bool) throws {
    if deletePlaintext {
      let source = URL(fileURLWithPath: draft.sourcePath).standardizedFileURL
      guard Self.isOwned(source, by: plaintextDraftRoot) else {
        throw MacProtectedMediaSendFailure.localCleanup
      }
      do {
        if fileManager.fileExists(atPath: source.path) { try fileManager.removeItem(at: source) }
      } catch { throw MacProtectedMediaSendFailure.localCleanup }
    }
    do { try fileManager.removeItem(at: draftDirectory(draft.draftID)) } catch {
      throw MacProtectedMediaSendFailure.localCleanup
    }
  }

  private func draftDirectory(_ draftID: String) -> URL {
    ciphertextRoot.appendingPathComponent(draftID, isDirectory: true)
  }
  private func stateURL(_ draftID: String) -> URL {
    draftDirectory(draftID).appendingPathComponent("state.json")
  }
  private func chunkURL(_ draftID: String, _ index: Int) -> URL {
    draftDirectory(draftID).appendingPathComponent(String(format: "chunk-%04d.bin", index))
  }

  private static func preparePrivateDirectory(_ url: URL, fileManager: FileManager) throws {
    do {
      try fileManager.createDirectory(
        at: url, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
      try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: url.path)
    } catch { throw MacProtectedMediaSendFailure.persistence }
  }

  private static func fileDigest(
    _ url: URL, maximumBytes: Int64, fileManager: FileManager
  ) throws -> String {
    let canonical = url.resolvingSymlinksInPath().standardizedFileURL
    guard canonical.isFileURL,
      let attributes = try? fileManager.attributesOfItem(atPath: canonical.path),
      (attributes[.type] as? FileAttributeType) == .typeRegular,
      let size = (attributes[.size] as? NSNumber)?.int64Value, size > 0,
      size <= maximumBytes,
      let handle = try? FileHandle(forReadingFrom: canonical)
    else { throw MacProtectedMediaSendFailure.sourceUnavailable }
    defer { try? handle.close() }
    var hasher = SHA256()
    do {
      while let bytes = try handle.read(upToCount: 64 * 1024), !bytes.isEmpty {
        hasher.update(data: bytes)
      }
    } catch { throw MacProtectedMediaSendFailure.sourceUnavailable }
    return hasher.finalize().map { String(format: "%02x", $0) }.joined()
  }

  private static func digest(_ data: Data) -> String {
    SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
  }

  private static func validDigest(_ value: String) -> Bool {
    value.count == 64
      && value.utf8.allSatisfy {
        ($0 >= Character("0").asciiValue! && $0 <= Character("9").asciiValue!)
          || ($0 >= Character("a").asciiValue! && $0 <= Character("f").asciiValue!)
      }
  }

  private static func validToken(_ value: String) -> Bool {
    (16...64).contains(value.utf8.count)
      && value.unicodeScalars.allSatisfy {
        CharacterSet(
          charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
        )
        .contains($0)
      }
  }

  private static func validIdentifier(_ value: String) -> Bool {
    (8...128).contains(value.utf8.count) && !value.contains("/") && !value.contains("\\")
  }

  private static func isOwned(_ child: URL, by root: URL) -> Bool {
    let childPath = child.resolvingSymlinksInPath().standardizedFileURL.path
    let rootPath = root.resolvingSymlinksInPath().standardizedFileURL.path
    return childPath != rootPath && childPath.hasPrefix(rootPath + "/")
  }
}
