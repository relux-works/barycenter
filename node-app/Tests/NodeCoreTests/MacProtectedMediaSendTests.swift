import Foundation
import Testing

@testable import NodeCore

private struct ProtectedSendVectors: Decodable {
  struct Chunk: Decodable {
    let index: Int
    let offset: Int64
    let size: Int
    let nonce: String
    let sha256: String
  }
  struct Resume: Decodable {
    let interruptedAtChunk: Int
    let expectedGeneration: UInt64
    let expectedSealCount: Int
    let expectedStageCount: Int
    let expectedUploadedChunkIndices: [Int]
  }
  let contract: String
  let status: String
  let fixtureSuite: String
  let fixtureContainer: String
  let sourceSHA256: String
  let manifestSHA256: String
  let ciphertextSHA256: String
  let chunks: [Chunk]
  let resume: Resume
}

private func loadProtectedSendVectors() throws -> ProtectedSendVectors {
  let url = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    .appendingPathComponent("../protocol/macos-protected-media-send-v1-vectors.json")
    .standardizedFileURL
  return try JSONDecoder().decode(ProtectedSendVectors.self, from: Data(contentsOf: url))
}

private final class ProtectedSendMemoryKeychain: MacE2EEKeychainByteStore, @unchecked Sendable {
  private let lock = NSLock()
  private var values: [String: Data] = [:]

  func read(account: String) throws -> Data? {
    lock.lock()
    defer { lock.unlock() }
    return values[account]
  }

  func add(_ data: Data, account: String) throws {
    lock.lock()
    defer { lock.unlock() }
    guard values[account] == nil else { throw MacE2EEKeyStateFailure.conflict }
    values[account] = data
  }

  func update(_ data: Data, account: String) throws {
    lock.lock()
    defer { lock.unlock() }
    guard values[account] != nil else { throw MacE2EEKeyStateFailure.unavailable }
    values[account] = data
  }

  func delete(account: String) throws {
    lock.lock()
    values.removeValue(forKey: account)
    lock.unlock()
  }
}

private struct ProtectedSendFixedRandom: MacE2EERandomSource {
  func bytes(count: Int) throws -> Data { Data(repeating: 0x42, count: count) }
}

private actor ProtectedSendFixtureSealer: MacProtectedMediaSealing {
  nonisolated let productionApproved = false
  private(set) var sealCount = 0
  var duplicateNonce = false
  var verificationResult = true

  func setDuplicateNonce(_ value: Bool) { duplicateNonce = value }
  func setVerificationResult(_ value: Bool) { verificationResult = value }

  func seal(
    sourceURL: URL, context: MacProtectedMediaSealContext,
    identity: MacE2EEDeviceIdentityLease, groupState: MacE2EEGroupStateLease
  ) async throws -> MacProtectedMediaSealedArtifact {
    sealCount += 1
    let source = try Data(contentsOf: sourceURL)
    let midpoint = max(1, source.count / 2)
    let values = [Data(source.prefix(midpoint)), Data(source.dropFirst(midpoint))]
      .filter { !$0.isEmpty }
    let chunks = values.enumerated().map { index, value in
      MacProtectedMediaCiphertextChunk(
        nonce: duplicateNonce ? "fixture-nonce" : "fixture-nonce-\(context.generation)-\(index)",
        ciphertext: Data("fixture-ciphertext-\(index):".utf8) + value)
    }
    return MacProtectedMediaSealedArtifact(
      contract: "e2ee-media-audit.v1", capability: "e2ee_media_v1",
      suite: "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION",
      container: "AUDIT_FIXTURE_CONTAINER_NOT_FOR_PRODUCTION", context: context,
      encryptedManifest: Data("fixture-encrypted-manifest-\(context.generation)".utf8),
      opaqueKeyEnvelopes: Data("fixture-opaque-envelopes".utf8),
      authenticatedManifest: Data("fixture-authenticated-manifest".utf8),
      signature: Data("fixture-signature".utf8), chunks: chunks)
  }

  func verify(_ artifact: MacProtectedMediaSealedArtifact) async -> Bool {
    verificationResult && artifact.signature == Data("fixture-signature".utf8)
  }
}

private actor ProtectedSendFixtureUploader: MacProtectedMediaUploading {
  enum Failure: Error { case injected }
  private(set) var stages: [MacProtectedMediaStageRequest] = []
  private(set) var chunks: [(Int, Int64, String, Data)] = []
  private(set) var finalizeCount = 0
  private(set) var deleteCount = 0
  var failChunkOnce: Int?
  private var didFail = false

  func setFailChunkOnce(_ value: Int?) { failChunkOnce = value }

  func stage(_ request: MacProtectedMediaStageRequest) async throws
    -> MacProtectedMediaRemoteObject
  {
    if let existing = stages.first {
      guard existing == request else { throw Failure.injected }
    } else {
      stages.append(request)
    }
    return MacProtectedMediaRemoteObject(
      objectID: "em_01K123456789ABCDEFGHJKMNPQ", revision: 1)
  }

  func putChunk(
    objectID: String, idempotencyKey: String, index: Int, byteOffset: Int64,
    ciphertextDigest: String, ciphertext: Data
  ) async throws {
    if failChunkOnce == index && !didFail {
      didFail = true
      throw Failure.injected
    }
    if let previous = chunks.first(where: { $0.0 == index }) {
      guard previous.1 == byteOffset, previous.2 == ciphertextDigest,
        previous.3 == ciphertext
      else { throw Failure.injected }
      return
    }
    chunks.append((index, byteOffset, ciphertextDigest, ciphertext))
  }

  func finalize(
    objectID: String, idempotencyKey: String, expectedRevision: UInt64
  ) async throws -> MacProtectedMediaRemoteObject {
    finalizeCount += 1
    return MacProtectedMediaRemoteObject(objectID: objectID, revision: expectedRevision + 1)
  }

  func delete(
    objectID: String, idempotencyKey: String, expectedRevision: UInt64
  ) async throws { deleteCount += 1 }
}

private final class ProtectedSendProgressRecorder: @unchecked Sendable {
  private let lock = NSLock()
  private var values: [MacProtectedMediaSendProgress.Phase] = []

  func append(_ value: MacProtectedMediaSendProgress.Phase) {
    lock.lock()
    values.append(value)
    lock.unlock()
  }

  func snapshot() -> [MacProtectedMediaSendProgress.Phase] {
    lock.lock()
    defer { lock.unlock() }
    return values
  }
}

private struct ProtectedSendFixture {
  let root: URL
  let plaintextRoot: URL
  let ciphertextRoot: URL
  let source: URL
  let keyState: MacE2EEKeyStateRepository
  let identity: MacE2EEDeviceIdentityMetadata
  let group: MacE2EEGroupStateMetadata
  let sealer: ProtectedSendFixtureSealer
  let uploader: ProtectedSendFixtureUploader

  init() throws {
    root = FileManager.default.temporaryDirectory
      .appendingPathComponent("mac-protected-send-\(UUID().uuidString)", isDirectory: true)
    plaintextRoot = root.appendingPathComponent("plaintext", isDirectory: true)
    ciphertextRoot = root.appendingPathComponent("ciphertext", isDirectory: true)
    try FileManager.default.createDirectory(
      at: plaintextRoot, withIntermediateDirectories: true)
    source = plaintextRoot.appendingPathComponent("recording.wav")
    try Data("private audio fixture bytes".utf8).write(to: source)
    keyState = MacE2EEKeyStateRepository(
      store: ProtectedSendMemoryKeychain(), random: ProtectedSendFixedRandom())
    identity = try keyState.installDeviceIdentity(
      deviceID: "dev_01K123456789ABCDEFGHJKMNPQ", keyFormat: "fixture-v1",
      signingPrivateKey: Data(repeating: 0x11, count: 32),
      keyAgreementPrivateKey: Data(repeating: 0x22, count: 32), createdAtMS: 1000)
    group = try keyState.persistGroupState(
      installationID: identity.installationID,
      groupID: "grp_01K123456789ABCDEFGHJKMNPQ", epoch: 7,
      previousCommitDigest: "",
      commitDigest: String(repeating: "b", count: 64),
      targetSnapshotDigest: String(repeating: "a", count: 64),
      opaqueState: Data(repeating: 0x33, count: 64), expectedRevision: 0, nowMS: 1100)
    sealer = ProtectedSendFixtureSealer()
    uploader = ProtectedSendFixtureUploader()
  }

  func request(
    policy: MacProtectedMediaPlaintextPolicy = .appPrivateDeleteOnTerminal,
    kind: MacProtectedMediaKind = .clip,
    recipients: [MacProtectedMediaRecipient]? = nil
  ) -> MacProtectedMediaSendRequest {
    MacProtectedMediaSendRequest(
      draftID: "draft_01K123456789ABCDEFGHJKMNPQ",
      sourceObjectID: "source_01K123456789ABCDEFGHJKMNPQ", sourceURL: source,
      plaintextPolicy: policy, kind: kind, authorDeviceID: identity.deviceID,
      groupID: group.groupID, expectedGroupRevision: group.revision,
      expectedTargetSnapshotDigest: group.targetSnapshotDigest,
      recipients: recipients ?? [
        MacProtectedMediaRecipient(
          deviceID: "dev_01K123456789ABCDEFGHJKMNPQ", verified: true,
          currentMember: true, supportsProtectedMedia: true),
        MacProtectedMediaRecipient(
          deviceID: "dev_01K123456789ABCDEFGHJKMNP2", verified: true,
          currentMember: true, supportsProtectedMedia: true),
      ], declaredDurationMS: 1000, rightsConfirmed: true, targetConfirmed: true,
      expiresAtMS: 10_000)
  }

  func service(auditFixture: Bool = true) throws -> MacProtectedMediaSendService {
    if auditFixture {
      return try MacProtectedMediaSendService(
        auditFixtureKeyState: keyState, sealer: sealer, uploader: uploader,
        ciphertextRoot: ciphertextRoot, plaintextDraftRoot: plaintextRoot)
    }
    return try MacProtectedMediaSendService(
      keyState: keyState, sealer: sealer, uploader: uploader,
      ciphertextRoot: ciphertextRoot, plaintextDraftRoot: plaintextRoot)
  }

  func remove() { try? FileManager.default.removeItem(at: root) }
}

@Suite struct MacProtectedMediaSendTests {
  @Test func productionInitializerCannotEnableAuditFixtureProvider() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    let service = try fixture.service(auditFixture: false)
    await #expect(throws: MacProtectedMediaSendFailure.productionDisabled) {
      _ = try await service.send(fixture.request(), nowMS: 2000)
    }
    #expect(await fixture.sealer.sealCount == 0)
    #expect(await fixture.uploader.stages.isEmpty)
  }

  @Test func fixturePipelinePublishesCiphertextAndCleansOwnedPlaintext() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    let vectors = try loadProtectedSendVectors()
    let service = try fixture.service()
    let progressRecorder = ProtectedSendProgressRecorder()
    let result = try await service.send(fixture.request(), nowMS: 2000) { progress in
      progressRecorder.append(progress.phase)
    }
    #expect(result.generation == 1)
    #expect(result.objectID == "em_01K123456789ABCDEFGHJKMNPQ")
    #expect(!FileManager.default.fileExists(atPath: fixture.source.path))
    #expect(
      !FileManager.default.fileExists(
        atPath: fixture.ciphertextRoot
          .appendingPathComponent("draft_01K123456789ABCDEFGHJKMNPQ").path))
    #expect(await fixture.sealer.sealCount == 1)
    #expect(await fixture.uploader.stages.count == 1)
    #expect(await fixture.uploader.chunks.count == 2)
    #expect(await fixture.uploader.finalizeCount == 1)
    let stage = try #require(await fixture.uploader.stages.first)
    #expect(stage.manifestDigest == vectors.manifestSHA256)
    #expect(stage.ciphertextDigest == vectors.ciphertextSHA256)
    #expect(stage.ciphertextSize == Int64(vectors.chunks.reduce(0) { $0 + $1.size }))
    #expect(vectors.contract == "macos-protected-media-send-v1-vectors")
    #expect(vectors.status == "audit-fixture-only-production-disabled")
    #expect(progressRecorder.snapshot().first == .preparing)
    #expect(progressRecorder.snapshot().last == .published)
    let state = try fixture.keyState.loadGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.group.groupID)
    #expect(state.metadata.sendGeneration == 1)
    state.destroy()
  }

  @Test func interruptedUploadResumesExactCiphertextWithoutGenerationReuse() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    let vectors = try loadProtectedSendVectors()
    await fixture.uploader.setFailChunkOnce(vectors.resume.interruptedAtChunk)
    let service = try fixture.service()
    await #expect(throws: MacProtectedMediaSendFailure.transport) {
      _ = try await service.send(fixture.request(), nowMS: 2000)
    }
    #expect(FileManager.default.fileExists(atPath: fixture.source.path))
    #expect(await fixture.sealer.sealCount == 1)
    #expect(await fixture.uploader.chunks.map(\.0) == [0])

    let result = try await service.send(fixture.request(), nowMS: 2100)
    #expect(result.generation == vectors.resume.expectedGeneration)
    #expect(await fixture.sealer.sealCount == vectors.resume.expectedSealCount)
    #expect(await fixture.uploader.stages.count == vectors.resume.expectedStageCount)
    #expect(await fixture.uploader.chunks.map(\.0) == vectors.resume.expectedUploadedChunkIndices)
    let state = try fixture.keyState.loadGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.group.groupID)
    #expect(state.metadata.sendGeneration == 1)
    state.destroy()
  }

  @Test func unsupportedRecipientFailsBeforeGenerationReservation() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    let service = try fixture.service()
    let recipients = [
      MacProtectedMediaRecipient(
        deviceID: "dev_01K123456789ABCDEFGHJKMNP2", verified: true,
        currentMember: true, supportsProtectedMedia: false)
    ]
    await #expect(throws: MacProtectedMediaSendFailure.unsupportedTarget) {
      _ = try await service.send(fixture.request(recipients: recipients), nowMS: 2000)
    }
    let state = try fixture.keyState.loadGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.group.groupID)
    #expect(state.metadata.sendGeneration == 0)
    #expect(await fixture.sealer.sealCount == 0)
    #expect(FileManager.default.fileExists(atPath: fixture.source.path))
    state.destroy()
  }

  @Test func duplicateNoncesFailClosedAndConsumeReservedGeneration() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    await fixture.sealer.setDuplicateNonce(true)
    let service = try fixture.service()
    await #expect(throws: MacProtectedMediaSendFailure.invalidArtifact) {
      _ = try await service.send(fixture.request(), nowMS: 2000)
    }
    let state = try fixture.keyState.loadGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.group.groupID)
    #expect(state.metadata.sendGeneration == 1)
    #expect(FileManager.default.fileExists(atPath: fixture.source.path))
    #expect(
      !FileManager.default.fileExists(
        atPath: fixture.ciphertextRoot
          .appendingPathComponent("draft_01K123456789ABCDEFGHJKMNPQ").path))
    state.destroy()
  }

  @Test func invalidProviderSignatureFailsClosedBeforeCiphertextPersistence() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    await fixture.sealer.setVerificationResult(false)
    let service = try fixture.service()
    await #expect(throws: MacProtectedMediaSendFailure.invalidArtifact) {
      _ = try await service.send(fixture.request(), nowMS: 2000)
    }
    #expect(
      !FileManager.default.fileExists(
        atPath: fixture.ciphertextRoot
          .appendingPathComponent(fixture.request().draftID).path))
  }

  @Test func sourceAndCiphertextTamperEachFailClosedOnResume() async throws {
    do {
      let fixture = try ProtectedSendFixture()
      defer { fixture.remove() }
      await fixture.uploader.setFailChunkOnce(1)
      let service = try fixture.service()
      await #expect(throws: MacProtectedMediaSendFailure.transport) {
        _ = try await service.send(fixture.request(), nowMS: 2000)
      }
      try Data("modified plaintext fixture".utf8).write(to: fixture.source)
      await #expect(throws: MacProtectedMediaSendFailure.invalidRequest) {
        _ = try await service.send(fixture.request(), nowMS: 2100)
      }
    }
    do {
      let fixture = try ProtectedSendFixture()
      defer { fixture.remove() }
      await fixture.uploader.setFailChunkOnce(1)
      let service = try fixture.service()
      await #expect(throws: MacProtectedMediaSendFailure.transport) {
        _ = try await service.send(fixture.request(), nowMS: 2000)
      }
      let chunk = fixture.ciphertextRoot
        .appendingPathComponent(fixture.request().draftID)
        .appendingPathComponent("chunk-0000.bin")
      var bytes = try Data(contentsOf: chunk)
      bytes[0] ^= 0xff
      try bytes.write(to: chunk)
      await #expect(throws: MacProtectedMediaSendFailure.persistence) {
        _ = try await service.send(fixture.request(), nowMS: 2100)
      }
    }
  }

  @Test func explicitCancelDeletesRemoteStageCiphertextAndOwnedPlaintext() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    await fixture.uploader.setFailChunkOnce(1)
    let service = try fixture.service()
    await #expect(throws: MacProtectedMediaSendFailure.transport) {
      _ = try await service.send(fixture.request(), nowMS: 2000)
    }
    try await service.cancel(draftID: fixture.request().draftID)
    #expect(await fixture.uploader.deleteCount == 1)
    #expect(!FileManager.default.fileExists(atPath: fixture.source.path))
    #expect(
      !FileManager.default.fileExists(
        atPath: fixture.ciphertextRoot
          .appendingPathComponent(fixture.request().draftID).path))
  }

  @Test func expiredCrashRecoveryIsBoundedAndCleansOwnedPlaintext() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    await fixture.uploader.setFailChunkOnce(1)
    let service = try fixture.service()
    await #expect(throws: MacProtectedMediaSendFailure.transport) {
      _ = try await service.send(fixture.request(), nowMS: 2000)
    }
    #expect(try await service.recoverExpiredDrafts(nowMS: 10_001, limit: 1) == 1)
    #expect(!FileManager.default.fileExists(atPath: fixture.source.path))
    #expect(try await service.recoverExpiredDrafts(nowMS: 10_002, limit: 1) == 0)
  }

  @Test func keyStateRepositoryAllowsOnlyOneProtectedSendOwner() throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    _ = try fixture.service()
    #expect(throws: MacE2EEKeyStateFailure.conflict) { _ = try fixture.service() }
  }

  @Test func userOwnedSelectedFileIsRetainedAfterPublication() async throws {
    let fixture = try ProtectedSendFixture()
    defer { fixture.remove() }
    let service = try fixture.service()
    _ = try await service.send(
      fixture.request(policy: .userOwnedRetain), nowMS: 2000)
    #expect(FileManager.default.fileExists(atPath: fixture.source.path))
  }

  @Test func clipTrackAndSavedCueShareTheBoundedProtectedPipeline() async throws {
    for kind in MacProtectedMediaKind.allCases {
      let fixture = try ProtectedSendFixture()
      defer { fixture.remove() }
      let service = try fixture.service()
      let publication = try await service.send(fixture.request(kind: kind), nowMS: 2000)
      #expect(publication.generation == 1)
      #expect(await fixture.uploader.stages.first?.kind == kind)
    }
  }
}
