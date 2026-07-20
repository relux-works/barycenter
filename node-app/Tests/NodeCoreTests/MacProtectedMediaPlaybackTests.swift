import CryptoKit
import Foundation
import Testing

@testable import NodeCore

private func protectedPlaybackHash(_ data: Data) -> String {
  SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
}

private struct ProtectedPlaybackVectors: Decodable {
  struct Chunk: Decodable {
    let index: Int
    let offset: Int64
    let size: Int
    let ciphertextSHA256: String
    let authenticatedPlaintextSHA256: String
  }
  let contract: String
  let status: String
  let platformProducers: [String]
  let ciphertextSHA256: String
  let chunks: [Chunk]
  let failClosed: [[String: String]]
}

private func loadProtectedPlaybackVectors() throws -> ProtectedPlaybackVectors {
  let url = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    .appendingPathComponent("../protocol/macos-protected-media-playback-v1-vectors.json")
    .standardizedFileURL
  return try JSONDecoder().decode(ProtectedPlaybackVectors.self, from: Data(contentsOf: url))
}

private final class ProtectedPlaybackMemoryKeychain: MacE2EEKeychainByteStore,
  @unchecked Sendable
{
  private let lock = NSLock()
  private var values: [String: Data] = [:]

  func read(account: String) throws -> Data? {
    lock.withLock { values[account] }
  }

  func add(_ data: Data, account: String) throws {
    try lock.withLock {
      guard values[account] == nil else { throw MacE2EEKeyStateFailure.conflict }
      values[account] = data
    }
  }

  func update(_ data: Data, account: String) throws {
    try lock.withLock {
      guard values[account] != nil else { throw MacE2EEKeyStateFailure.unavailable }
      values[account] = data
    }
  }

  func delete(account: String) throws {
    _ = lock.withLock { values.removeValue(forKey: account) }
  }
}

private struct ProtectedPlaybackFixedRandom: MacE2EERandomSource {
  func bytes(count: Int) throws -> Data { Data(repeating: 0x52, count: count) }
}

private final class ProtectedPlaybackTransport: MacProtectedMediaPlaybackTransport,
  @unchecked Sendable
{
  private let lock = NSLock()
  private var storedRoute: MacProtectedMediaPlaybackRoute
  private var bodies: [ClosedRange<Int64>: Data]
  private var manifestCalls = 0
  private var rangeCalls = 0
  private var corrupt = false
  private var failure: MacProtectedMediaPlaybackFailure?

  init(route: MacProtectedMediaPlaybackRoute, ciphertext: [Data]) {
    storedRoute = route
    var offset: Int64 = 0
    bodies = Dictionary(uniqueKeysWithValues: ciphertext.map { body in
      defer { offset += Int64(body.count) }
      return (offset...(offset + Int64(body.count) - 1), body)
    })
  }

  func replaceRoute(_ route: MacProtectedMediaPlaybackRoute) {
    lock.withLock { storedRoute = route }
  }

  func setCorrupt(_ value: Bool) { lock.withLock { corrupt = value } }
  func setFailure(_ value: MacProtectedMediaPlaybackFailure?) {
    lock.withLock { failure = value }
  }

  var counts: (manifest: Int, range: Int) {
    lock.withLock { (manifestCalls, rangeCalls) }
  }

  func fetchManifest(
    objectID: String, recipientDeviceID: String, requestedAtMS: Int64
  ) async throws -> MacProtectedMediaPlaybackRoute {
    try lock.withLock {
      manifestCalls += 1
      if let failure { throw failure }
      return storedRoute
    }
  }

  func fetchRange(_ request: MacProtectedMediaRangeRequest) async throws
    -> (ciphertext: Data, etag: String)
  {
    try lock.withLock {
      rangeCalls += 1
      if let failure { throw failure }
      guard let body = bodies[request.start...request.end] else {
        throw MacProtectedMediaPlaybackFailure.transport
      }
      if corrupt { return (Data(repeating: 0xff, count: body.count), request.etag) }
      return (body, request.etag)
    }
  }
}

private actor ProtectedPlaybackFixtureOpener: MacProtectedMediaOpening {
  nonisolated let productionApproved = false
  private(set) var openCount = 0
  private(set) var decryptCount = 0
  private(set) var sawHistoryGrant = false
  var rejectOpen = false
  var rejectChunk = false

  func setRejectOpen(_ value: Bool) { rejectOpen = value }
  func setRejectChunk(_ value: Bool) { rejectChunk = value }

  func open(
    route: MacProtectedMediaPlaybackRoute, identity: MacE2EEDeviceIdentityLease,
    groupState: MacE2EEGroupStateLease, historyGrant: MacE2EESecretLease?
  ) async throws -> MacProtectedMediaOpenLease {
    openCount += 1
    sawHistoryGrant = historyGrant != nil
    if rejectOpen { throw MacProtectedMediaPlaybackFailure.invalidAuthentication }
    guard route.signature == Data("fixture-signature".utf8) else {
      throw MacProtectedMediaPlaybackFailure.invalidAuthentication
    }
    return MacProtectedMediaOpenLease(opaqueState: Data("fixture-open-key-canary".utf8))
  }

  func authenticateAndDecrypt(
    ciphertext: Data, chunk: MacStreamChunk, route: MacProtectedMediaPlaybackRoute,
    lease: MacProtectedMediaOpenLease
  ) async throws -> Data {
    decryptCount += 1
    if rejectChunk { throw MacProtectedMediaPlaybackFailure.invalidAuthentication }
    let prefix = Data("fixture-cipher-v1:".utf8)
    guard ciphertext.starts(with: prefix) else {
      throw MacProtectedMediaPlaybackFailure.invalidAuthentication
    }
    return Data(ciphertext.dropFirst(prefix.count).map { $0 ^ 0xa5 })
  }
}

private struct ProtectedPlaybackFixture {
  let root: URL
  let cacheRoot: URL
  let keyState: MacE2EEKeyStateRepository
  let identity: MacE2EEDeviceIdentityMetadata
  let group: MacE2EEGroupStateMetadata
  let plaintext: [Data]
  let ciphertext: [Data]
  let opener: ProtectedPlaybackFixtureOpener
  let secret = Data(repeating: 0x77, count: 32)

  init(epoch: UInt64 = 7) throws {
    root = FileManager.default.temporaryDirectory.appendingPathComponent(
      "mac-protected-playback-\(UUID().uuidString)", isDirectory: true)
    cacheRoot = root.appendingPathComponent("ciphertext-cache", isDirectory: true)
    keyState = MacE2EEKeyStateRepository(
      store: ProtectedPlaybackMemoryKeychain(), random: ProtectedPlaybackFixedRandom())
    identity = try keyState.installDeviceIdentity(
      deviceID: "dev_01K123456789ABCDEFGHJKMNPQ", keyFormat: "fixture-v1",
      signingPrivateKey: Data("private-signing-key-canary".utf8),
      keyAgreementPrivateKey: Data("private-agreement-key-canary".utf8),
      createdAtMS: 1000)
    group = try keyState.persistGroupState(
      installationID: identity.installationID,
      groupID: "grp_01K123456789ABCDEFGHJKMNPQ", epoch: epoch,
      previousCommitDigest: "", commitDigest: String(repeating: "b", count: 64),
      targetSnapshotDigest: String(repeating: "a", count: 64),
      opaqueState: Data("opaque-group-key-canary".utf8), expectedRevision: 0,
      nowMS: 1100)
    plaintext = [Data("clear-audio-mac".utf8), Data("clear-audio-windows".utf8)]
    ciphertext = plaintext.map {
      Data("fixture-cipher-v1:".utf8) + Data($0.map { $0 ^ 0xa5 })
    }
    opener = ProtectedPlaybackFixtureOpener()
  }

  func streamManifest(identity suffix: String = "shared-mac-windows") -> MacStreamManifest {
    var offset: Int64 = 0
    let chunks = ciphertext.enumerated().map { index, body -> MacStreamChunk in
      defer { offset += Int64(body.count) }
      return MacStreamChunk(
        index: index, start: offset, end: offset + Int64(body.count) - 1,
        sha256: protectedPlaybackHash(body))
    }
    let whole = ciphertext.reduce(into: Data()) { $0.append($1) }
    return MacStreamManifest(
      identity: "svm1.protected.\(suffix)",
      variantUrl: "/v1/media/em_01K123456789ABCDEFGHJKMNPQ/variants/protected",
      etag: "\"sha256-\(protectedPlaybackHash(whole))\"",
      sha256: protectedPlaybackHash(whole), sizeBytes: Int64(whole.count),
      durationMs: 20_000, chunks: chunks,
      seekMap: [
        MacStreamSeekPoint(timeMs: 0, offset: 0),
        MacStreamSeekPoint(timeMs: 10_000, offset: chunks[1].start),
      ])
  }

  func route(
    contract: String = "e2ee-media-audit.v1", epoch: UInt64? = nil,
    generation: UInt64 = 3, target: String? = nil, expiresAtMS: Int64 = 20_000,
    stream: MacStreamManifest? = nil
  ) -> MacProtectedMediaPlaybackRoute {
    let encrypted = Data("fixture-encrypted-manifest".utf8)
    return MacProtectedMediaPlaybackRoute(
      contract: contract, capability: "e2ee_media_v1",
      suite: "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION",
      container: "AUDIT_FIXTURE_CONTAINER_NOT_FOR_PRODUCTION",
      objectID: "em_01K123456789ABCDEFGHJKMNPQ",
      sourceObjectID: "source_01K123456789ABCDEFGHJKMNPQ", objectKind: .track,
      authorDeviceID: "dev_01K123456789ABCDEFGHJKMNP2",
      recipientDeviceID: identity.deviceID, groupID: group.groupID,
      epoch: epoch ?? group.epoch, generation: generation,
      targetSnapshotDigest: target ?? group.targetSnapshotDigest,
      expiresAtMS: expiresAtMS, manifestDigest: protectedPlaybackHash(encrypted),
      encryptedManifest: encrypted, opaqueKeyEnvelope: Data("opaque-envelope".utf8),
      authenticatedManifest: Data("authenticated-manifest".utf8),
      signature: Data("fixture-signature".utf8),
      streamManifest: stream ?? streamManifest())
  }

  func request(
    epoch: UInt64? = nil, generation: UInt64 = 3, target: String? = nil,
    historyGrantID: String? = nil, policyAllowed: Bool = true
  ) -> MacProtectedMediaPlaybackRequest {
    MacProtectedMediaPlaybackRequest(
      objectID: "em_01K123456789ABCDEFGHJKMNPQ",
      recipientDeviceID: identity.deviceID, groupID: group.groupID,
      expectedGroupRevision: group.revision, expectedEpoch: epoch ?? group.epoch,
      expectedGeneration: generation,
      expectedTargetSnapshotDigest: target ?? group.targetSnapshotDigest,
      historyGrantID: historyGrantID, policyAllowed: policyAllowed,
      dndAllowed: true, senderBlocked: false)
  }

  func service(
    transport: ProtectedPlaybackTransport, production: Bool = false
  ) throws -> MacProtectedMediaPlaybackService {
    if production {
      return try MacProtectedMediaPlaybackService(
        keyState: keyState, opener: opener, transport: transport,
        cacheRoot: cacheRoot, cacheInstallationSecret: secret)
    }
    return try MacProtectedMediaPlaybackService(
      auditFixtureKeyState: keyState, opener: opener, transport: transport,
      cacheRoot: cacheRoot, cacheInstallationSecret: secret,
      currentTimeMS: { 2_000 })
  }

  func remove() { try? FileManager.default.removeItem(at: root) }
}

private final class ProtectedPlaybackDecoder: MacStreamCandidateDecoder, @unchecked Sendable {
  private let lock = NSLock()
  private var received: Data?

  func decode(_ request: MacStreamDecodeRequest) async throws {
    let value = try await request.chunks.readChunk(
      index: request.chunks.chunkIndex(forTimeMs: request.startPositionMs))
    lock.withLock { received = value }
  }

  var value: Data? { lock.withLock { received } }
}

private final class ProtectedPlaybackClock: MacStreamDeadlineClock, @unchecked Sendable {
  func localDeadline(coordinatorMs: Int64) -> Int64? { coordinatorMs }
  func coordinatorNowMs() -> Int64 { 2_000 }
  func localNowMs() -> Int64 { 2_000 }
}

private func protectedPlaybackLoad(_ manifest: MacStreamManifest) -> StreamLoadPayload {
  StreamLoadPayload(
    streamId: "protected-stream", playbackGeneration: 1, seekGeneration: 0,
    commandSequence: 1, mediaId: "em_01K123456789ABCDEFGHJKMNPQ",
    variantManifest: manifest.identity,
    variantUrl: manifest.variantUrl, variantEtag: manifest.etag,
    variantSha256: manifest.sha256, variantSizeBytes: manifest.sizeBytes,
    startPositionMs: 0, minimumBufferedMs: ProtocolConstants.streamMinimumBufferedMs,
    readyDeadlineCoordMs: 10_000,
    mixedVersionPolicy: ProtocolConstants.streamMixedVersionRequireAll)
}

private func protectedPlaybackEventually(_ predicate: @escaping @Sendable () -> Bool) async -> Bool {
  for _ in 0..<400 {
    if predicate() { return true }
    try? await Task.sleep(for: .milliseconds(5))
  }
  return predicate()
}

private func protectedPlaybackDiskContains(root: URL, needle: Data) -> Bool {
  guard let enumerator = FileManager.default.enumerator(
    at: root, includingPropertiesForKeys: [.isRegularFileKey])
  else { return false }
  for case let url as URL in enumerator {
    if let body = try? Data(contentsOf: url), body.range(of: needle) != nil { return true }
  }
  return false
}

@Suite(.serialized) struct MacProtectedMediaPlaybackTests {
  @Test func sharedMacWindowsFixtureFreezesExactAuthenticatedRanges() throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let vectors = try loadProtectedPlaybackVectors()
    let manifest = fixture.streamManifest()
    #expect(vectors.contract == "macos-protected-media-playback-v1-vectors")
    #expect(vectors.status == "audit-fixture-only-production-disabled")
    #expect(Set(vectors.platformProducers) == ["macos-fixture", "windows-fixture"])
    #expect(vectors.ciphertextSHA256 == manifest.sha256)
    #expect(vectors.failClosed.count == 8)
    #expect(vectors.chunks.count == manifest.chunks.count)
    for (vector, chunk) in zip(vectors.chunks, manifest.chunks) {
      #expect(vector.index == chunk.index)
      #expect(vector.offset == chunk.start)
      #expect(vector.size == Int(chunk.end - chunk.start + 1))
      #expect(vector.ciphertextSHA256 == chunk.sha256)
      #expect(
        vector.authenticatedPlaintextSHA256
          == protectedPlaybackHash(fixture.plaintext[vector.index]))
    }
  }

  @Test func productionRemainsDarkWithoutApprovedProvider() async throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let transport = ProtectedPlaybackTransport(
      route: fixture.route(), ciphertext: fixture.ciphertext)
    let service = try fixture.service(transport: transport, production: true)
    await #expect(throws: MacProtectedMediaPlaybackFailure.productionDisabled) {
      _ = try await service.prepare(fixture.request(), nowMS: 2_000)
    }
    #expect(transport.counts == (0, 0))
    #expect(await fixture.opener.openCount == 0)
  }

  @Test func incrementalAuthenticatedPlaybackAndCiphertextOnlyRestartCache() async throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let route = fixture.route()
    let transport = ProtectedPlaybackTransport(route: route, ciphertext: fixture.ciphertext)
    let first = try await fixture.service(transport: transport).prepare(
      fixture.request(), nowMS: 2_000)
    #expect(try await first.chunks.readChunk(index: 0) == fixture.plaintext[0])
    #expect(transport.counts.range == 1)
    #expect(await fixture.opener.decryptCount == 1)

    let restarted = try await fixture.service(transport: transport).prepare(
      fixture.request(), nowMS: 2_100)
    #expect(try await restarted.chunks.readChunk(index: 0) == fixture.plaintext[0])
    #expect(transport.counts.range == 1, "restart uses verified ciphertext cache")
    #expect(await fixture.opener.decryptCount == 2, "cached ciphertext is authenticated again")
    #expect(!protectedPlaybackDiskContains(root: fixture.root, needle: fixture.plaintext[0]))
    #expect(
      !protectedPlaybackDiskContains(
        root: fixture.root, needle: Data("opaque-group-key-canary".utf8)))
    #expect(
      !protectedPlaybackDiskContains(
        root: fixture.root, needle: Data("fixture-open-key-canary".utf8)))
  }

  @Test func ciphertextTamperAndUnauthenticatedPlaintextNeverReachDecoder() async throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let transport = ProtectedPlaybackTransport(
      route: fixture.route(), ciphertext: fixture.ciphertext)
    transport.setCorrupt(true)
    let prepared = try await fixture.service(transport: transport).prepare(
      fixture.request(), nowMS: 2_000)
    await #expect(throws: MacStreamFailure.self) {
      _ = try await prepared.chunks.readChunk(index: 0)
    }
    #expect(transport.counts.range == 2, "bounded integrity retry")
    #expect(await fixture.opener.decryptCount == 0)

    transport.setCorrupt(false)
    await fixture.opener.setRejectChunk(true)
    await #expect(throws: MacStreamFailure.self) {
      _ = try await prepared.chunks.readChunk(index: 0)
    }
    #expect(await fixture.opener.decryptCount == 1)
    #expect(await prepared.cache.stats().bytes == 0, "failed authentication purges cache")
  }

  @Test func downgradeExpiryWrongTargetAndLocalPolicyFailClosedBeforeRanges() async throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let transport = ProtectedPlaybackTransport(
      route: fixture.route(contract: "legacy-media.v0"), ciphertext: fixture.ciphertext)
    let service = try fixture.service(transport: transport)
    await #expect(throws: MacProtectedMediaPlaybackFailure.downgradeForbidden) {
      _ = try await service.prepare(fixture.request(), nowMS: 2_000)
    }

    transport.replaceRoute(fixture.route(expiresAtMS: 1_999))
    await #expect(throws: MacProtectedMediaPlaybackFailure.expired) {
      _ = try await service.prepare(fixture.request(), nowMS: 2_000)
    }

    transport.replaceRoute(fixture.route(target: String(repeating: "c", count: 64)))
    await #expect(throws: MacProtectedMediaPlaybackFailure.targetChanged) {
      _ = try await service.prepare(fixture.request(), nowMS: 2_000)
    }

    await #expect(throws: MacProtectedMediaPlaybackFailure.blocked) {
      _ = try await service.prepare(
        fixture.request(policyAllowed: false), nowMS: 2_000)
    }
    #expect(transport.counts.range == 0)
  }

  @Test func historicalEpochRequiresLiveBoundedGrant() async throws {
    let fixture = try ProtectedPlaybackFixture(epoch: 8)
    defer { fixture.remove() }
    let grantID = "grant_01K123456789ABCDEFGHJKMNPQ"
    _ = try fixture.keyState.storeGrant(
      installationID: fixture.identity.installationID, grantID: grantID,
      groupID: fixture.group.groupID, firstEpoch: 6, lastEpoch: 7,
      expiresAtMS: 10_000, opaqueGrant: Data("opaque-history-grant".utf8),
      expectedRevision: 0, nowMS: 1_500)
    let route = fixture.route(epoch: 7)
    let transport = ProtectedPlaybackTransport(route: route, ciphertext: fixture.ciphertext)
    let service = try fixture.service(transport: transport)
    await #expect(throws: MacProtectedMediaPlaybackFailure.missingGrant) {
      _ = try await service.prepare(fixture.request(epoch: 7), nowMS: 2_000)
    }
    let prepared = try await service.prepare(
      fixture.request(epoch: 7, historyGrantID: grantID), nowMS: 2_000)
    #expect(try await prepared.chunks.readChunk(index: 1) == fixture.plaintext[1])
    #expect(await fixture.opener.sawHistoryGrant)
    try fixture.keyState.revokeGrant(
      installationID: fixture.identity.installationID, grantID: grantID)
    await #expect(throws: MacStreamFailure.self) {
      _ = try await prepared.chunks.readChunk(index: 0)
    }
    #expect(await prepared.cache.stats().bytes == 0)
  }

  @Test func membershipChangeAndExplicitRevocationPersistAsTombstones() async throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let route = fixture.route()
    let transport = ProtectedPlaybackTransport(route: route, ciphertext: fixture.ciphertext)
    let prepared = try await fixture.service(transport: transport).prepare(
      fixture.request(), nowMS: 2_000)
    _ = try fixture.keyState.persistGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.group.groupID,
      epoch: fixture.group.epoch + 1, previousCommitDigest: fixture.group.commitDigest,
      commitDigest: String(repeating: "d", count: 64),
      targetSnapshotDigest: String(repeating: "e", count: 64),
      opaqueState: Data("rotated-group-key".utf8),
      expectedRevision: fixture.group.revision, nowMS: 2_100)
    await #expect(throws: MacStreamFailure.self) {
      _ = try await prepared.chunks.readChunk(index: 0)
    }
    #expect(await prepared.cache.stats().bytes == 0)

    let freshFixture = try ProtectedPlaybackFixture()
    defer { freshFixture.remove() }
    let freshRoute = freshFixture.route()
    let freshTransport = ProtectedPlaybackTransport(
      route: freshRoute, ciphertext: freshFixture.ciphertext)
    let fresh = try await freshFixture.service(transport: freshTransport).prepare(
      freshFixture.request(), nowMS: 2_000)
    try await fresh.revoke()
    await #expect(throws: MacStreamFailure.self) {
      _ = try await fresh.chunks.readChunk(index: 0)
    }
    let restarted = try await freshFixture.service(transport: freshTransport).prepare(
      freshFixture.request(), nowMS: 2_100)
    await #expect(throws: MacStreamFailure.self) {
      _ = try await restarted.chunks.readChunk(index: 0)
    }
    #expect(freshTransport.counts.range == 0)
  }

  @Test func boundedCandidatePlayerReceivesOnlyAuthenticatedChunkReader() async throws {
    let fixture = try ProtectedPlaybackFixture()
    defer { fixture.remove() }
    let route = fixture.route()
    let transport = ProtectedPlaybackTransport(route: route, ciphertext: fixture.ciphertext)
    let prepared = try await fixture.service(transport: transport).prepare(
      fixture.request(), nowMS: 2_000)
    let decoder = ProtectedPlaybackDecoder()
    let player = prepared.makeCandidatePlayer(
      decoder: decoder, clock: ProtectedPlaybackClock(), send: { _ in })
    try player.load(protectedPlaybackLoad(route.streamManifest), manifest: route.streamManifest)
    #expect(await protectedPlaybackEventually { decoder.value == fixture.plaintext[0] })
    #expect(decoder.value != fixture.ciphertext[0])
    #expect(player.snapshot().ringBytes <= player.snapshot().ringCeilingBytes)
  }
}
