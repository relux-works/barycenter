import Foundation
import Testing

@testable import NodeCore

private final class MemoryMacE2EEKeychainStore: MacE2EEKeychainByteStore, @unchecked Sendable {
  enum Operation: String { case read, add, update, delete }

  private let lock = NSLock()
  private var storage: [String: Data] = [:]
  private var counts: [Operation: Int] = [:]
  private var armWitnessReadFailure = false
  private var pendingReadFailureAccount: String?
  var failure: ((Operation, String, Int) -> Error?)?
  var transform: ((Operation, String, Data) -> Data)?

  func read(account: String) throws -> Data? {
    try perform(.read, account: account) {
      if pendingReadFailureAccount == account {
        pendingReadFailureAccount = nil
        throw MacE2EEKeyStateFailure.unavailable
      }
      return storage[account]
    }
  }

  func add(_ data: Data, account: String) throws {
    try perform(.add, account: account) {
      guard storage[account] == nil else { throw MacE2EEKeyStateFailure.conflict }
      storage[account] = transform?(.add, account, data) ?? data
    }
  }

  func update(_ data: Data, account: String) throws {
    try perform(.update, account: account) {
      guard storage[account] != nil else { throw MacE2EEKeyStateFailure.unavailable }
      storage[account] = transform?(.update, account, data) ?? data
      if armWitnessReadFailure && account.hasPrefix("witness.group.") {
        armWitnessReadFailure = false
        pendingReadFailureAccount = account
      }
    }
  }

  func delete(account: String) throws {
    _ = try perform(.delete, account: account) { storage.removeValue(forKey: account) }
  }

  func failReadAfterNextGroupWitnessUpdate() {
    lock.lock()
    armWitnessReadFailure = true
    lock.unlock()
  }

  func snapshot() -> [String: Data] {
    lock.lock()
    defer { lock.unlock() }
    return storage
  }

  func merge(_ values: [String: Data]) {
    lock.lock()
    storage.merge(values) { _, new in new }
    lock.unlock()
  }

  private func perform<Result>(
    _ operation: Operation, account: String, _ body: () throws -> Result
  ) throws -> Result {
    lock.lock()
    defer { lock.unlock() }
    counts[operation, default: 0] += 1
    if let error = failure?(operation, account, counts[operation]!) { throw error }
    return try body()
  }
}

private struct FixedMacE2EERandomSource: MacE2EERandomSource {
  let byte: UInt8
  func bytes(count: Int) throws -> Data { Data(repeating: byte, count: count) }
}

private struct MacE2EEKeyStateVectors: Decodable {
  struct Transition: Decodable {
    let name: String
    let operation: String
    let expected: String
  }
  struct Crash: Decodable {
    let name: String
    let expected: String
  }
  struct Target: Decodable {
    let name: String
    let active: Bool
    let registered: Int
    let verified: Int
    let supported: Int
    let expected: String
  }
  let contract: String
  let status: String
  let installationRandomHex: String
  let deviceId: String
  let keyFormat: String
  let groupId: String
  let initialEpoch: UInt64
  let nextEpoch: UInt64
  let commitDigest: String
  let nextCommitDigest: String
  let targetSnapshotDigest: String
  let nextTargetSnapshotDigest: String
  let transitions: [Transition]
  let crashVectors: [Crash]
  let targetVectors: [Target]
}

private struct MacE2EERecoveryVectors: Decodable {
  let contract: String
  let status: String
  let transferMaxTtlMs: Int64
  let historyMaxTtlMs: Int64
  let localCleanupMaxGrants: Int
  let failClosed: [String]
}

private func keyStateVectorsURL() -> URL {
  URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent()
    .deletingLastPathComponent()
    .deletingLastPathComponent()
    .appendingPathComponent("../protocol/e2ee-key-state-v1-vectors.json")
    .standardizedFileURL
}

private func loadKeyStateVectors() throws -> MacE2EEKeyStateVectors {
  let decoder = JSONDecoder()
  decoder.keyDecodingStrategy = .convertFromSnakeCase
  return try decoder.decode(
    MacE2EEKeyStateVectors.self, from: Data(contentsOf: keyStateVectorsURL()))
}

private func loadRecoveryVectors() throws -> MacE2EERecoveryVectors {
  let decoder = JSONDecoder()
  decoder.keyDecodingStrategy = .convertFromSnakeCase
  return try decoder.decode(
    MacE2EERecoveryVectors.self,
    from: Data(
      contentsOf: keyStateVectorsURL().deletingLastPathComponent()
        .appendingPathComponent("e2ee-recovery-v1-vectors.json")))
}

private func makeKeyStateRepository(
  randomByte: UInt8 = 0x42
) throws -> (
  repository: MacE2EEKeyStateRepository,
  store: MemoryMacE2EEKeychainStore,
  identity: MacE2EEDeviceIdentityMetadata,
  vectors: MacE2EEKeyStateVectors
) {
  let vectors = try loadKeyStateVectors()
  let store = MemoryMacE2EEKeychainStore()
  let repository = MacE2EEKeyStateRepository(
    store: store, random: FixedMacE2EERandomSource(byte: randomByte))
  let identity = try repository.installDeviceIdentity(
    deviceID: vectors.deviceId, keyFormat: vectors.keyFormat,
    signingPrivateKey: Data(repeating: 0x91, count: 32),
    keyAgreementPrivateKey: Data(repeating: 0xA2, count: 32), createdAtMS: 1000)
  return (repository, store, identity, vectors)
}

private func installGroup(
  _ fixture: (
    repository: MacE2EEKeyStateRepository,
    store: MemoryMacE2EEKeychainStore,
    identity: MacE2EEDeviceIdentityMetadata,
    vectors: MacE2EEKeyStateVectors
  )
) throws -> MacE2EEGroupStateMetadata {
  try fixture.repository.persistGroupState(
    installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
    epoch: fixture.vectors.initialEpoch, previousCommitDigest: "",
    commitDigest: fixture.vectors.commitDigest,
    targetSnapshotDigest: fixture.vectors.targetSnapshotDigest,
    opaqueState: Data(repeating: 0xB3, count: 128), expectedRevision: 0, nowMS: 1100)
}

@Suite struct MacE2EEKeyStateTests {
  @Test func deviceIdentityUsesDistinctRedactedDeviceOnlyBoundary() throws {
    let fixture = try makeKeyStateRepository()
    #expect(fixture.vectors.contract == "e2ee-key-state.v1")
    #expect(fixture.vectors.status == "production-disabled")
    #expect(fixture.identity.installationID == fixture.vectors.installationRandomHex)
    #expect(fixture.identity.revision == 1)

    let lease = try fixture.repository.loadDeviceIdentity(deviceID: fixture.vectors.deviceId)
    let signing = lease.withSigningPrivateKey { Data($0) }
    let agreement = lease.withKeyAgreementPrivateKey { Data($0) }
    #expect(signing == Data(repeating: 0x91, count: 32))
    #expect(agreement == Data(repeating: 0xA2, count: 32))
    #expect(!lease.description.contains(fixture.vectors.deviceId))
    #expect(lease.description.contains("<redacted>"))
    lease.destroy()
    #expect(lease.withSigningPrivateKey { $0.count } == 0)

    let accounts = fixture.store.snapshot().keys.sorted()
    #expect(accounts.count == 6)
    for kind in ["device_metadata", "device_signing", "device_agreement"] {
      #expect(accounts.contains { $0.hasPrefix("state.\(kind).") })
      #expect(accounts.contains { $0.hasPrefix("witness.\(kind).") })
    }
  }

  @Test func partialDeviceIdentityInstallationFailsClosedOnRetry() throws {
    let vectors = try loadKeyStateVectors()
    let store = MemoryMacE2EEKeychainStore()
    let repository = MacE2EEKeyStateRepository(
      store: store, random: FixedMacE2EERandomSource(byte: 0x42))
    store.failure = { operation, account, _ in
      operation == .add && account.hasPrefix("state.device_agreement.")
        ? MacE2EEKeyStateFailure.unavailable : nil
    }
    #expect(throws: MacE2EEKeyStateFailure.unavailable) {
      _ = try repository.installDeviceIdentity(
        deviceID: vectors.deviceId, keyFormat: vectors.keyFormat,
        signingPrivateKey: Data(repeating: 0x91, count: 32),
        keyAgreementPrivateKey: Data(repeating: 0xA2, count: 32), createdAtMS: 1000)
    }
    store.failure = nil
    #expect(throws: MacE2EEKeyStateFailure.rollbackOrClone) {
      _ = try repository.installDeviceIdentity(
        deviceID: vectors.deviceId, keyFormat: vectors.keyFormat,
        signingPrivateKey: Data(repeating: 0x91, count: 32),
        keyAgreementPrivateKey: Data(repeating: 0xA2, count: 32), createdAtMS: 1000)
    }
    let reset = try repository.resetDeviceIdentityForReenrollment(
      expectedDeviceID: vectors.deviceId)
    #expect(reset)
    #expect(throws: MacE2EEKeyStateFailure.notFound) {
      _ = try repository.loadDeviceIdentity(deviceID: vectors.deviceId)
    }
    let replacement = try repository.installDeviceIdentity(
      deviceID: vectors.deviceId, keyFormat: vectors.keyFormat,
      signingPrivateKey: Data(repeating: 0x93, count: 32),
      keyAgreementPrivateKey: Data(repeating: 0xA4, count: 32), createdAtMS: 1100)
    #expect(replacement.revision == 1)
  }

  @Test func sharedEpochGenerationReplayAndForkVectorsFailClosed() throws {
    let fixture = try makeKeyStateRepository()
    let initial = try installGroup(fixture)
    #expect(initial.epoch == fixture.vectors.initialEpoch)
    #expect(initial.sendGeneration == 0)
    #expect(initial.revision == 1)

    let reservation = try fixture.repository.reserveSendGeneration(
      installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
      domain: "media", expectedRevision: initial.revision, nowMS: 1200)
    #expect(reservation.epoch == 7)
    #expect(reservation.generation == 1)
    #expect(reservation.revision == 2)
    #expect(throws: MacE2EEKeyStateFailure.conflict) {
      _ = try fixture.repository.reserveSendGeneration(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        domain: "media", expectedRevision: initial.revision, nowMS: 1201)
    }

    let advanced = try fixture.repository.persistGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
      epoch: fixture.vectors.nextEpoch, previousCommitDigest: fixture.vectors.commitDigest,
      commitDigest: fixture.vectors.nextCommitDigest,
      targetSnapshotDigest: fixture.vectors.nextTargetSnapshotDigest,
      opaqueState: Data(repeating: 0xC4, count: 128), expectedRevision: 2, nowMS: 1300)
    #expect(advanced.epoch == 8)
    #expect(advanced.sendGeneration == 0)
    #expect(advanced.revision == 3)
    #expect(throws: MacE2EEKeyStateFailure.staleEpoch) {
      _ = try fixture.repository.persistGroupState(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        epoch: 8, previousCommitDigest: fixture.vectors.nextCommitDigest,
        commitDigest: fixture.vectors.nextCommitDigest,
        targetSnapshotDigest: fixture.vectors.nextTargetSnapshotDigest,
        opaqueState: Data([1]), expectedRevision: 3, nowMS: 1400)
    }
    #expect(throws: MacE2EEKeyStateFailure.rollbackOrClone) {
      _ = try fixture.repository.persistGroupState(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        epoch: 10, previousCommitDigest: fixture.vectors.nextCommitDigest,
        commitDigest: fixture.vectors.nextCommitDigest,
        targetSnapshotDigest: fixture.vectors.nextTargetSnapshotDigest,
        opaqueState: Data([1]), expectedRevision: 3, nowMS: 1400)
    }
    #expect(throws: MacE2EEKeyStateFailure.rollbackOrClone) {
      _ = try fixture.repository.persistGroupState(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        epoch: 9, previousCommitDigest: fixture.vectors.commitDigest,
        commitDigest: String(repeating: "e", count: 64),
        targetSnapshotDigest: fixture.vectors.nextTargetSnapshotDigest,
        opaqueState: Data([1]), expectedRevision: 3, nowMS: 1400)
    }
    #expect(fixture.vectors.transitions.count == 5)
    try fixture.repository.deleteGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId)
    #expect(throws: MacE2EEKeyStateFailure.notFound) {
      _ = try fixture.repository.loadGroupState(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId)
    }
  }

  @Test func crashAfterRecordBeforeWitnessFailsClosedWithoutGenerationReuse() throws {
    let fixture = try makeKeyStateRepository()
    let initial = try installGroup(fixture)
    fixture.store.failure = { operation, account, _ in
      operation == .update && account.hasPrefix("witness.group.")
        ? MacE2EEKeyStateFailure.unavailable : nil
    }
    #expect(throws: MacE2EEKeyStateFailure.unavailable) {
      _ = try fixture.repository.reserveSendGeneration(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        domain: "live", expectedRevision: initial.revision, nowMS: 1200)
    }
    fixture.store.failure = nil
    #expect(throws: MacE2EEKeyStateFailure.rollbackOrClone) {
      _ = try fixture.repository.loadGroupState(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId)
    }
    #expect(fixture.vectors.crashVectors.first?.expected == "rollback_or_clone")
  }

  @Test func lostReadbackAfterBothWritesConsumesGenerationWithoutReuse() throws {
    let fixture = try makeKeyStateRepository()
    let initial = try installGroup(fixture)
    fixture.store.failReadAfterNextGroupWitnessUpdate()

    #expect(throws: MacE2EEKeyStateFailure.unavailable) {
      _ = try fixture.repository.reserveSendGeneration(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        domain: "live", expectedRevision: initial.revision, nowMS: 1200)
    }

    let recovered = try fixture.repository.loadGroupState(
      installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId)
    #expect(recovered.metadata.sendGeneration == 1)
    #expect(recovered.metadata.revision == 2)
    #expect(throws: MacE2EEKeyStateFailure.conflict) {
      _ = try fixture.repository.reserveSendGeneration(
        installationID: fixture.identity.installationID, groupID: fixture.vectors.groupId,
        domain: "live", expectedRevision: initial.revision, nowMS: 1201)
    }
    #expect(fixture.vectors.crashVectors.last?.expected == "generation-consumed-no-reuse")
  }

  @Test func copiedGroupStateCannotCrossInstallationWitness() throws {
    let source = try makeKeyStateRepository(randomByte: 0x42)
    _ = try installGroup(source)
    let copiedGroup = source.store.snapshot().filter {
      $0.key.hasPrefix("state.group.") || $0.key.hasPrefix("witness.group.")
    }

    let destination = try makeKeyStateRepository(randomByte: 0x43)
    destination.store.merge(copiedGroup)
    #expect(source.identity.installationID != destination.identity.installationID)
    #expect(throws: MacE2EEKeyStateFailure.rollbackOrClone) {
      _ = try destination.repository.loadGroupState(
        installationID: destination.identity.installationID, groupID: source.vectors.groupId)
    }
  }

  @Test func grantsAreMonotonicExpiringAndRevocable() throws {
    let fixture = try makeKeyStateRepository()
    let grant = try fixture.repository.storeGrant(
      installationID: fixture.identity.installationID, grantID: "grant-0001",
      groupID: fixture.vectors.groupId, firstEpoch: 7, lastEpoch: 8,
      expiresAtMS: 5000, opaqueGrant: Data(repeating: 0xD5, count: 64),
      expectedRevision: 0, nowMS: 1500)
    #expect(grant.revision == 1)
    let loaded = try fixture.repository.loadGrant(
      installationID: fixture.identity.installationID, grantID: grant.grantID, nowMS: 2000)
    #expect(loaded.0 == grant)
    #expect(loaded.1.withUnsafeBytes { Data($0) } == Data(repeating: 0xD5, count: 64))
    #expect(throws: MacE2EEKeyStateFailure.replay) {
      _ = try fixture.repository.storeGrant(
        installationID: fixture.identity.installationID, grantID: grant.grantID,
        groupID: fixture.vectors.groupId, firstEpoch: 7, lastEpoch: 7,
        expiresAtMS: 5000, opaqueGrant: Data([1]), expectedRevision: 1, nowMS: 1600)
    }
    #expect(throws: MacE2EEKeyStateFailure.expired) {
      _ = try fixture.repository.loadGrant(
        installationID: fixture.identity.installationID, grantID: grant.grantID, nowMS: 5000)
    }
    try fixture.repository.revokeGrant(
      installationID: fixture.identity.installationID, grantID: grant.grantID)
    #expect(throws: MacE2EEKeyStateFailure.notFound) {
      _ = try fixture.repository.loadGrant(
        installationID: fixture.identity.installationID, grantID: grant.grantID, nowMS: 2000)
    }
  }

  @Test func expiredGrantCleanupIsCallerScopedAndBounded() throws {
    let vectors = try loadRecoveryVectors()
    #expect(vectors.contract == "e2ee-recovery.v1")
    #expect(vectors.status == "production-disabled")
    #expect(vectors.transferMaxTtlMs == 900_000)
    #expect(vectors.historyMaxTtlMs == 2_592_000_000)
    #expect(vectors.localCleanupMaxGrants == 100)
    #expect(vectors.failClosed.count == 10)
    let fixture = try makeKeyStateRepository()
    for (grantID, expiry) in [("grant-expired", Int64(3000)), ("grant-active", Int64(6000))] {
      _ = try fixture.repository.storeGrant(
        installationID: fixture.identity.installationID, grantID: grantID,
        groupID: fixture.vectors.groupId, firstEpoch: 7, lastEpoch: 7,
        expiresAtMS: expiry, opaqueGrant: Data(repeating: 0xD7, count: 32),
        expectedRevision: 0, nowMS: 2000)
    }
    let result = try fixture.repository.cleanupExpiredGrants(
      installationID: fixture.identity.installationID,
      grantIDs: ["grant-expired", "grant-active", "grant-expired"], nowMS: 4000)
    #expect(result == MacE2EEGrantCleanupResult(inspected: 2, deleted: 1))
    #expect(throws: MacE2EEKeyStateFailure.notFound) {
      _ = try fixture.repository.loadGrant(
        installationID: fixture.identity.installationID, grantID: "grant-expired", nowMS: 2500)
    }
    _ = try fixture.repository.loadGrant(
      installationID: fixture.identity.installationID, grantID: "grant-active", nowMS: 4000)
    #expect(throws: MacE2EEKeyStateFailure.invalid) {
      _ = try fixture.repository.cleanupExpiredGrants(
        installationID: fixture.identity.installationID,
        grantIDs: (0...100).map { "grant-\($0)" }, nowMS: 4000)
    }
  }

  @Test func contentKeyCacheIsBoundedExpiringAndClearable() throws {
    let fixture = try makeKeyStateRepository()
    var revision: UInt64 = 0
    for index in 0..<35 {
      revision = try fixture.repository.cacheContentKey(
        installationID: fixture.identity.installationID,
        objectID: String(format: "object-%03d", index), groupID: fixture.vectors.groupId,
        epoch: 7, expiresAtMS: 10_000 + Int64(index),
        key: Data(repeating: UInt8(index), count: 32), expectedRevision: revision,
        nowMS: 2000 + Int64(index))
    }
    #expect(revision == 35)
    #expect(throws: MacE2EEKeyStateFailure.notFound) {
      _ = try fixture.repository.loadContentKey(
        installationID: fixture.identity.installationID, objectID: "object-000", nowMS: 3000)
    }
    let newest = try fixture.repository.loadContentKey(
      installationID: fixture.identity.installationID, objectID: "object-034", nowMS: 3000)
    #expect(newest.0.epoch == 7)
    #expect(newest.1.withUnsafeBytes { $0.count } == 32)
    #expect(throws: MacE2EEKeyStateFailure.expired) {
      _ = try fixture.repository.loadContentKey(
        installationID: fixture.identity.installationID, objectID: "object-034", nowMS: 10_034)
    }
    try fixture.repository.clearContentKeyCache(
      installationID: fixture.identity.installationID)
    #expect(throws: MacE2EEKeyStateFailure.notFound) {
      _ = try fixture.repository.loadContentKey(
        installationID: fixture.identity.installationID, objectID: "object-034", nowMS: 3000)
    }
  }

  @Test func revokedOnlyMemberIsRemovedEndpointNotUnsupportedTarget() throws {
    let vectors = try loadKeyStateVectors()
    for vector in vectors.targetVectors {
      let decision = MacE2EETargetDevicePolicy.decide(
        activeAirMember: vector.active, registeredDevices: vector.registered,
        currentVerifiedDevices: vector.verified, currentSupportedDevices: vector.supported)
      #expect(decision.rawValue == vector.expected, "target vector \(vector.name)")
    }
  }

  @Test func sourceExcludesPreferencesSyncLogsAndRuntimeActivation() throws {
    let source = try String(
      contentsOf: URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        .deletingLastPathComponent().deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/MacE2EEKeyState.swift"), encoding: .utf8)
    #expect(source.contains("kSecAttrAccessibleWhenUnlockedThisDeviceOnly"))
    #expect(source.contains("kSecAttrSynchronizable as String: false"))
    for forbidden in ["UserDefaults", "os_log", "Logger(", "print(", "e2ee_media_v1"] {
      #expect(!source.contains(forbidden), "forbidden source token \(forbidden)")
    }
  }
}
