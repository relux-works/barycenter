import CryptoKit
import Foundation
import Security

public enum MacE2EEKeyStateFailure: Error, Equatable, LocalizedError {
  case conflict
  case corrupt
  case expired
  case invalid
  case notFound
  case replay
  case rollbackOrClone
  case staleEpoch
  case unavailable
  case unsupported

  public var errorDescription: String? {
    "Protected media key state is unavailable or requires re-verification."
  }
}

public protocol MacE2EEKeychainByteStore: Sendable {
  func read(account: String) throws -> Data?
  func add(_ data: Data, account: String) throws
  func update(_ data: Data, account: String) throws
  func delete(account: String) throws
}

/// Dedicated E2EE Keychain boundary. These items are device-only, unavailable
/// while locked, and excluded from iCloud Keychain. It is deliberately
/// separate from onboarding credentials, which have different availability
/// and recovery semantics.
public final class SystemMacE2EEKeychainStore: MacE2EEKeychainByteStore, @unchecked Sendable {
  static let service = "works.relux.pulsar.e2ee"

  public init() {}

  private func query(account: String) -> [String: Any] {
    [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: Self.service,
      kSecAttrAccount as String: account,
      kSecUseDataProtectionKeychain as String: true,
      kSecAttrSynchronizable as String: false,
    ]
  }

  public func read(account: String) throws -> Data? {
    var value = query(account: account)
    value[kSecReturnData as String] = true
    value[kSecMatchLimit as String] = kSecMatchLimitOne
    var output: CFTypeRef?
    let status = SecItemCopyMatching(value as CFDictionary, &output)
    if status == errSecItemNotFound { return nil }
    guard status == errSecSuccess, let data = output as? Data else {
      throw MacE2EEKeyStateFailure.unavailable
    }
    return data
  }

  public func add(_ data: Data, account: String) throws {
    var value = query(account: account)
    value[kSecValueData as String] = data
    value[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
    let status = SecItemAdd(value as CFDictionary, nil)
    if status == errSecDuplicateItem { throw MacE2EEKeyStateFailure.conflict }
    guard status == errSecSuccess else { throw MacE2EEKeyStateFailure.unavailable }
  }

  public func update(_ data: Data, account: String) throws {
    let status = SecItemUpdate(
      query(account: account) as CFDictionary,
      [kSecValueData as String: data] as CFDictionary)
    guard status == errSecSuccess else { throw MacE2EEKeyStateFailure.unavailable }
  }

  public func delete(account: String) throws {
    let status = SecItemDelete(query(account: account) as CFDictionary)
    guard status == errSecSuccess || status == errSecItemNotFound else {
      throw MacE2EEKeyStateFailure.unavailable
    }
  }
}

public protocol MacE2EERandomSource: Sendable {
  func bytes(count: Int) throws -> Data
}

public struct SystemMacE2EERandomSource: MacE2EERandomSource, Sendable {
  public init() {}

  public func bytes(count: Int) throws -> Data {
    guard count > 0, count <= 4096 else { throw MacE2EEKeyStateFailure.invalid }
    var value = Data(count: count)
    let status = value.withUnsafeMutableBytes { buffer in
      SecRandomCopyBytes(kSecRandomDefault, count, buffer.baseAddress!)
    }
    guard status == errSecSuccess else { throw MacE2EEKeyStateFailure.unavailable }
    return value
  }
}

public final class MacE2EESecretLease: @unchecked Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  private let lock = NSLock()
  private var bytes: Data

  init(_ bytes: Data) { self.bytes = bytes }

  public var description: String { "MacE2EESecretLease(<redacted>)" }
  public var debugDescription: String { description }

  public func withUnsafeBytes<Result>(
    _ body: (UnsafeRawBufferPointer) throws -> Result
  ) rethrows -> Result {
    lock.lock()
    defer { lock.unlock() }
    return try bytes.withUnsafeBytes(body)
  }

  public func destroy() {
    lock.lock()
    _ = bytes.withUnsafeMutableBytes {
      $0.initializeMemory(as: UInt8.self, repeating: 0)
    }
    bytes.removeAll(keepingCapacity: false)
    lock.unlock()
  }

  deinit { destroy() }
}

public struct MacE2EEDeviceIdentityMetadata: Equatable, Sendable {
  public let deviceID: String
  public let installationID: String
  public let keyFormat: String
  public let revision: UInt64
  public let createdAtMS: Int64
}

public final class MacE2EEDeviceIdentityLease: @unchecked Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let metadata: MacE2EEDeviceIdentityMetadata
  private let signing: MacE2EESecretLease
  private let agreement: MacE2EESecretLease

  init(metadata: MacE2EEDeviceIdentityMetadata, signing: Data, agreement: Data) {
    self.metadata = metadata
    self.signing = MacE2EESecretLease(signing)
    self.agreement = MacE2EESecretLease(agreement)
  }

  public var description: String {
    "MacE2EEDeviceIdentityLease(device: <redacted>, keys: <redacted>)"
  }
  public var debugDescription: String { description }

  public func withSigningPrivateKey<Result>(
    _ body: (UnsafeRawBufferPointer) throws -> Result
  ) rethrows -> Result { try signing.withUnsafeBytes(body) }

  public func withKeyAgreementPrivateKey<Result>(
    _ body: (UnsafeRawBufferPointer) throws -> Result
  ) rethrows -> Result { try agreement.withUnsafeBytes(body) }

  public func destroy() {
    signing.destroy()
    agreement.destroy()
  }
}

public struct MacE2EEGroupStateMetadata: Equatable, Sendable {
  public let groupID: String
  public let installationID: String
  public let epoch: UInt64
  public let sendGeneration: UInt64
  public let commitDigest: String
  public let targetSnapshotDigest: String
  public let revision: UInt64
  public let updatedAtMS: Int64
}

public struct MacE2EESendReservation: Equatable, Sendable {
  public let groupID: String
  public let epoch: UInt64
  public let generation: UInt64
  public let domain: String
  public let revision: UInt64
}

public final class MacE2EEGroupStateLease: @unchecked Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let metadata: MacE2EEGroupStateMetadata
  private let state: MacE2EESecretLease

  init(metadata: MacE2EEGroupStateMetadata, state: Data) {
    self.metadata = metadata
    self.state = MacE2EESecretLease(state)
  }

  public var description: String {
    "MacE2EEGroupStateLease(group: <redacted>, epoch: \(metadata.epoch), state: <redacted>)"
  }
  public var debugDescription: String { description }

  public func withOpaqueState<Result>(
    _ body: (UnsafeRawBufferPointer) throws -> Result
  ) rethrows -> Result { try state.withUnsafeBytes(body) }

  public func destroy() { state.destroy() }
}

public struct MacE2EEGrantMetadata: Equatable, Sendable {
  public let grantID: String
  public let groupID: String
  public let firstEpoch: UInt64
  public let lastEpoch: UInt64
  public let expiresAtMS: Int64
  public let revision: UInt64
}

public struct MacE2EEGrantCleanupResult: Equatable, Sendable {
  public let inspected: Int
  public let deleted: Int
}

public struct MacE2EEContentKeyMetadata: Equatable, Sendable {
  public let objectID: String
  public let groupID: String
  public let epoch: UInt64
  public let expiresAtMS: Int64
}

public enum MacE2EETargetDeviceDecision: String, Equatable, Sendable {
  case route
  case removedEndpoint = "removed_endpoint"
  case unsupportedTarget = "unsupported_target"
}

/// EPC-005 boundary: an active Air member whose registered devices are all
/// revoked has no current endpoint. It is not silently reclassified as a
/// legacy/unsupported target and must wait for membership reconciliation.
public enum MacE2EETargetDevicePolicy {
  public static func decide(
    activeAirMember: Bool,
    registeredDevices: Int,
    currentVerifiedDevices: Int,
    currentSupportedDevices: Int
  ) -> MacE2EETargetDeviceDecision {
    guard activeAirMember, registeredDevices >= 0, currentVerifiedDevices >= 0,
      currentSupportedDevices >= 0, currentSupportedDevices <= currentVerifiedDevices,
      currentVerifiedDevices <= registeredDevices
    else { return .removedEndpoint }
    if currentVerifiedDevices == 0 { return .removedEndpoint }
    if currentSupportedDevices == 0 { return .unsupportedTarget }
    return .route
  }
}

public final class MacE2EEKeyStateRepository: @unchecked Sendable {
  public static let maxOpaqueStateBytes = 1 << 20
  public static let maxPrivateKeyBytes = 4096
  public static let maxGrantBytes = 64 << 10
  public static let maxCachedContentKeys = 32
  public static let maxCachedContentKeyBytes = 64 << 10

  private static let processLock = NSRecursiveLock()
  private let store: any MacE2EEKeychainByteStore
  private let random: any MacE2EERandomSource
  private var protectedMediaSendOwnerClaimed = false
  private var e2eeLiveSendOwnerClaimed = false

  public init(
    store: any MacE2EEKeychainByteStore = SystemMacE2EEKeychainStore(),
    random: any MacE2EERandomSource = SystemMacE2EERandomSource()
  ) {
    self.store = store
    self.random = random
  }

  /// A protected-media sender owns generation reservation for this repository
  /// for its entire lifetime. Runtime composition must create one sender and
  /// share it; a second sender could otherwise race a stale expected revision
  /// or attempt to prepare the same draft twice. The claim is intentionally
  /// not releasable: replacing the owner requires replacing the repository and
  /// reloading its witnessed state from Keychain first.
  public func claimProtectedMediaSendOwnership() throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard !protectedMediaSendOwnerClaimed else { throw MacE2EEKeyStateFailure.conflict }
    protectedMediaSendOwnerClaimed = true
  }

  /// Claims the single live-PTT generation-reservation owner for this loaded
  /// repository. The production live factory additionally requires its app
  /// composition to attest cross-process serialization; this in-process claim
  /// alone is deliberately insufficient to enable the dormant runtime.
  func claimE2EELiveSendOwnership() throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard !e2eeLiveSendOwnerClaimed else { throw MacE2EEKeyStateFailure.conflict }
    e2eeLiveSendOwnerClaimed = true
  }

  public func installDeviceIdentity(
    deviceID: String,
    keyFormat: String,
    signingPrivateKey: Data,
    keyAgreementPrivateKey: Data,
    createdAtMS: Int64
  ) throws -> MacE2EEDeviceIdentityMetadata {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard validIdentifier(deviceID, min: 8, max: 128), validLabel(keyFormat, max: 64),
      validSecret(signingPrivateKey, max: Self.maxPrivateKeyBytes),
      validSecret(keyAgreementPrivateKey, max: Self.maxPrivateKeyBytes), createdAtMS > 0
    else { throw MacE2EEKeyStateFailure.invalid }
    let metadataRecord = try loadRecord(
      kind: .deviceMetadata, scope: "device-metadata", installationID: nil)
    let signingRecord = try loadRecord(
      kind: .deviceSigning, scope: "device-signing", installationID: nil)
    let agreementRecord = try loadRecord(
      kind: .deviceAgreement, scope: "device-agreement", installationID: nil)
    if metadataRecord != nil || signingRecord != nil || agreementRecord != nil {
      guard let metadataRecord, let signingRecord, let agreementRecord else {
        throw MacE2EEKeyStateFailure.rollbackOrClone
      }
      let metadata: DevicePayload = try decodePayload(metadataRecord)
      let signing: DeviceSecretPayload = try decodePayload(signingRecord)
      let agreement: DeviceSecretPayload = try decodePayload(agreementRecord)
      try validateDevicePayloads(metadata, signing: signing, agreement: agreement)
      guard metadata.deviceID == deviceID, metadata.keyFormat == keyFormat,
        signing.privateKey == signingPrivateKey, agreement.privateKey == keyAgreementPrivateKey
      else { throw MacE2EEKeyStateFailure.conflict }
      return deviceMetadata(metadata, revision: metadataRecord.revision)
    }
    let installationID = hex(try random.bytes(count: 32))
    let metadata = DevicePayload(
      deviceID: deviceID, installationID: installationID, keyFormat: keyFormat,
      createdAtMS: createdAtMS)
    let signing = DeviceSecretPayload(
      deviceID: deviceID, installationID: installationID, keyFormat: keyFormat,
      role: "signing", privateKey: signingPrivateKey, createdAtMS: createdAtMS)
    let agreement = DeviceSecretPayload(
      deviceID: deviceID, installationID: installationID, keyFormat: keyFormat,
      role: "agreement", privateKey: keyAgreementPrivateKey, createdAtMS: createdAtMS)
    _ = try persist(
      kind: .deviceMetadata, scope: "device-metadata",
      installationID: installationID, payload: metadata,
      expectedRevision: 0, nowMS: createdAtMS)
    _ = try persist(
      kind: .deviceSigning, scope: "device-signing",
      installationID: installationID, payload: signing,
      expectedRevision: 0, nowMS: createdAtMS)
    _ = try persist(
      kind: .deviceAgreement, scope: "device-agreement",
      installationID: installationID, payload: agreement,
      expectedRevision: 0, nowMS: createdAtMS)
    return MacE2EEDeviceIdentityMetadata(
      deviceID: deviceID, installationID: installationID, keyFormat: keyFormat,
      revision: 1, createdAtMS: createdAtMS)
  }

  public func loadDeviceIdentity(deviceID: String) throws -> MacE2EEDeviceIdentityLease {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard validIdentifier(deviceID, min: 8, max: 128) else {
      throw MacE2EEKeyStateFailure.invalid
    }
    let metadataRecord = try loadRecord(
      kind: .deviceMetadata, scope: "device-metadata", installationID: nil)
    let signingRecord = try loadRecord(
      kind: .deviceSigning, scope: "device-signing", installationID: nil)
    let agreementRecord = try loadRecord(
      kind: .deviceAgreement, scope: "device-agreement", installationID: nil)
    if metadataRecord == nil && signingRecord == nil && agreementRecord == nil {
      throw MacE2EEKeyStateFailure.notFound
    }
    guard let record = metadataRecord, let signingRecord, let agreementRecord else {
      throw MacE2EEKeyStateFailure.rollbackOrClone
    }
    let payload: DevicePayload = try decodePayload(record)
    let signing: DeviceSecretPayload = try decodePayload(signingRecord)
    let agreement: DeviceSecretPayload = try decodePayload(agreementRecord)
    try validateDevicePayloads(payload, signing: signing, agreement: agreement)
    guard payload.deviceID == deviceID else { throw MacE2EEKeyStateFailure.corrupt }
    return MacE2EEDeviceIdentityLease(
      metadata: deviceMetadata(payload, revision: record.revision),
      signing: signing.privateKey, agreement: agreement.privateKey)
  }

  /// Clears only the three fixed identity slots so an explicit re-enrollment
  /// can start after a partial install or locally lost identity. Group state,
  /// grants, and cached keys remain installation-bound and therefore cannot be
  /// opened by the replacement identity.
  @discardableResult
  public func resetDeviceIdentityForReenrollment(expectedDeviceID: String) throws -> Bool {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard validIdentifier(expectedDeviceID, min: 8, max: 128) else {
      throw MacE2EEKeyStateFailure.invalid
    }
    let slots: [(Kind, String)] = [
      (.deviceMetadata, "device-metadata"),
      (.deviceSigning, "device-signing"),
      (.deviceAgreement, "device-agreement"),
    ]
    var present = 0
    for (kind, scope) in slots {
      let accounts = try storeAccounts(kind: kind, scope: scope)
      if try store.read(account: accounts.record) != nil { present += 1 }
      if try store.read(account: accounts.witness) != nil { present += 1 }
    }
    if present == 0 { return false }
    if present == slots.count * 2 {
      guard
        let metadataRecord = try loadRecord(
          kind: .deviceMetadata, scope: "device-metadata", installationID: nil),
        let signingRecord = try loadRecord(
          kind: .deviceSigning, scope: "device-signing", installationID: nil),
        let agreementRecord = try loadRecord(
          kind: .deviceAgreement, scope: "device-agreement", installationID: nil)
      else { throw MacE2EEKeyStateFailure.rollbackOrClone }
      let metadata: DevicePayload = try decodePayload(metadataRecord)
      let signing: DeviceSecretPayload = try decodePayload(signingRecord)
      let agreement: DeviceSecretPayload = try decodePayload(agreementRecord)
      try validateDevicePayloads(metadata, signing: signing, agreement: agreement)
      guard metadata.deviceID == expectedDeviceID else {
        throw MacE2EEKeyStateFailure.conflict
      }
    }
    for (kind, scope) in slots {
      try deleteSlot(kind: kind, scope: scope)
    }
    return true
  }

  public func persistGroupState(
    installationID: String,
    groupID: String,
    epoch: UInt64,
    previousCommitDigest: String,
    commitDigest: String,
    targetSnapshotDigest: String,
    opaqueState: Data,
    expectedRevision: UInt64,
    nowMS: Int64
  ) throws -> MacE2EEGroupStateMetadata {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(groupID, min: 8, max: 128), epoch > 0,
      validDigest(commitDigest), validDigest(targetSnapshotDigest),
      validSecret(opaqueState, max: Self.maxOpaqueStateBytes), nowMS > 0
    else { throw MacE2EEKeyStateFailure.invalid }
    let scope = "group/\(groupID)"
    var generation: UInt64 = 0
    if let current = try loadRecord(kind: .group, scope: scope, installationID: installationID) {
      let payload: GroupPayload = try decodePayload(current)
      try validateGroupPayload(payload, expectedGroupID: groupID)
      guard current.revision == expectedRevision else { throw MacE2EEKeyStateFailure.conflict }
      if epoch <= payload.epoch { throw MacE2EEKeyStateFailure.staleEpoch }
      guard validDigest(previousCommitDigest), previousCommitDigest == payload.commitDigest,
        epoch == payload.epoch + 1, payload.groupID == groupID
      else { throw MacE2EEKeyStateFailure.rollbackOrClone }
      generation = 0
    } else {
      guard expectedRevision == 0 else { throw MacE2EEKeyStateFailure.conflict }
      guard previousCommitDigest.isEmpty else { throw MacE2EEKeyStateFailure.rollbackOrClone }
    }
    let payload = GroupPayload(
      groupID: groupID, epoch: epoch, sendGeneration: generation,
      commitDigest: commitDigest, targetSnapshotDigest: targetSnapshotDigest,
      opaqueState: opaqueState, updatedAtMS: nowMS)
    let record = try persist(
      kind: .group, scope: scope, installationID: installationID,
      payload: payload, expectedRevision: expectedRevision, nowMS: nowMS)
    return groupMetadata(payload, installationID: installationID, revision: record.revision)
  }

  public func loadGroupState(
    installationID: String, groupID: String
  ) throws -> MacE2EEGroupStateLease {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(groupID, min: 8, max: 128) else {
      throw MacE2EEKeyStateFailure.invalid
    }
    let scope = "group/\(groupID)"
    guard
      let record = try loadRecord(
        kind: .group, scope: scope,
        installationID: installationID)
    else { throw MacE2EEKeyStateFailure.notFound }
    let payload: GroupPayload = try decodePayload(record)
    try validateGroupPayload(payload, expectedGroupID: groupID)
    return MacE2EEGroupStateLease(
      metadata: groupMetadata(
        payload, installationID: installationID,
        revision: record.revision), state: payload.opaqueState)
  }

  /// Reserves a generation only after group state and its independent witness
  /// have both been read back. A crash may consume a generation, but can never
  /// cause the caller to reuse one.
  public func reserveSendGeneration(
    installationID: String,
    groupID: String,
    domain: String,
    expectedRevision: UInt64,
    nowMS: Int64
  ) throws -> MacE2EESendReservation {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(groupID, min: 8, max: 128),
      validLabel(domain, max: 32), nowMS > 0
    else {
      throw MacE2EEKeyStateFailure.invalid
    }
    let scope = "group/\(groupID)"
    guard
      let current = try loadRecord(
        kind: .group, scope: scope,
        installationID: installationID)
    else { throw MacE2EEKeyStateFailure.notFound }
    guard current.revision == expectedRevision else { throw MacE2EEKeyStateFailure.conflict }
    var payload: GroupPayload = try decodePayload(current)
    try validateGroupPayload(payload, expectedGroupID: groupID)
    guard payload.sendGeneration < UInt64.max else { throw MacE2EEKeyStateFailure.replay }
    payload.sendGeneration += 1
    payload.updatedAtMS = nowMS
    let record = try persist(
      kind: .group, scope: scope, installationID: installationID,
      payload: payload, expectedRevision: expectedRevision, nowMS: nowMS)
    return MacE2EESendReservation(
      groupID: groupID, epoch: payload.epoch, generation: payload.sendGeneration,
      domain: domain, revision: record.revision)
  }

  public func storeGrant(
    installationID: String,
    grantID: String,
    groupID: String,
    firstEpoch: UInt64,
    lastEpoch: UInt64,
    expiresAtMS: Int64,
    opaqueGrant: Data,
    expectedRevision: UInt64,
    nowMS: Int64
  ) throws -> MacE2EEGrantMetadata {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(grantID, min: 8, max: 128),
      validIdentifier(groupID, min: 8, max: 128), firstEpoch > 0,
      lastEpoch >= firstEpoch, expiresAtMS > nowMS, nowMS > 0,
      validSecret(opaqueGrant, max: Self.maxGrantBytes)
    else { throw MacE2EEKeyStateFailure.invalid }
    let scope = "grant/\(grantID)"
    if let current = try loadRecord(
      kind: .grant, scope: scope,
      installationID: installationID)
    {
      guard current.revision == expectedRevision else { throw MacE2EEKeyStateFailure.conflict }
      let previous: GrantPayload = try decodePayload(current)
      try validateGrantPayload(previous, expectedGrantID: grantID)
      guard previous.grantID == grantID, previous.groupID == groupID,
        firstEpoch == previous.firstEpoch, lastEpoch >= previous.lastEpoch,
        expiresAtMS >= previous.expiresAtMS
      else { throw MacE2EEKeyStateFailure.replay }
    } else if expectedRevision != 0 {
      throw MacE2EEKeyStateFailure.conflict
    }
    let payload = GrantPayload(
      grantID: grantID, groupID: groupID, firstEpoch: firstEpoch,
      lastEpoch: lastEpoch, expiresAtMS: expiresAtMS, opaqueGrant: opaqueGrant)
    let record = try persist(
      kind: .grant, scope: scope,
      installationID: installationID, payload: payload,
      expectedRevision: expectedRevision, nowMS: nowMS)
    return grantMetadata(payload, revision: record.revision)
  }

  public func loadGrant(
    installationID: String, grantID: String, nowMS: Int64
  ) throws -> (MacE2EEGrantMetadata, MacE2EESecretLease) {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(grantID, min: 8, max: 128), nowMS > 0 else {
      throw MacE2EEKeyStateFailure.invalid
    }
    guard
      let record = try loadRecord(
        kind: .grant, scope: "grant/\(grantID)",
        installationID: installationID)
    else { throw MacE2EEKeyStateFailure.notFound }
    let payload: GrantPayload = try decodePayload(record)
    try validateGrantPayload(payload, expectedGrantID: grantID)
    guard payload.expiresAtMS > nowMS else { throw MacE2EEKeyStateFailure.expired }
    return (
      grantMetadata(payload, revision: record.revision),
      MacE2EESecretLease(payload.opaqueGrant)
    )
  }

  public func revokeGrant(installationID: String, grantID: String) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(grantID, min: 8, max: 128) else {
      throw MacE2EEKeyStateFailure.invalid
    }
    try deleteSlot(kind: .grant, scope: "grant/\(grantID)")
  }

  /// Deletes only caller-enumerated expired grants. The explicit list and hard
  /// cap keep cleanup bounded because Keychain does not expose a trusted grant
  /// index to this repository.
  public func cleanupExpiredGrants(
    installationID: String, grantIDs: [String], nowMS: Int64
  ) throws -> MacE2EEGrantCleanupResult {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard !grantIDs.isEmpty, grantIDs.count <= 100, nowMS > 0 else {
      throw MacE2EEKeyStateFailure.invalid
    }
    var seen = Set<String>()
    var inspected = 0
    var deleted = 0
    for grantID in grantIDs {
      guard validIdentifier(grantID, min: 8, max: 128) else {
        throw MacE2EEKeyStateFailure.invalid
      }
      guard seen.insert(grantID).inserted else { continue }
      inspected += 1
      guard
        let record = try loadRecord(
          kind: .grant, scope: "grant/\(grantID)", installationID: installationID)
      else { continue }
      let payload: GrantPayload = try decodePayload(record)
      try validateGrantPayload(payload, expectedGrantID: grantID)
      if payload.expiresAtMS <= nowMS {
        try deleteSlot(kind: .grant, scope: "grant/\(grantID)")
        deleted += 1
      }
    }
    return MacE2EEGrantCleanupResult(inspected: inspected, deleted: deleted)
  }

  public func deleteGroupState(installationID: String, groupID: String) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(groupID, min: 8, max: 128) else {
      throw MacE2EEKeyStateFailure.invalid
    }
    try deleteSlot(kind: .group, scope: "group/\(groupID)")
  }

  public func cacheContentKey(
    installationID: String,
    objectID: String,
    groupID: String,
    epoch: UInt64,
    expiresAtMS: Int64,
    key: Data,
    expectedRevision: UInt64,
    nowMS: Int64
  ) throws -> UInt64 {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(objectID, min: 8, max: 128),
      validIdentifier(groupID, min: 8, max: 128), epoch > 0,
      expiresAtMS > nowMS, validSecret(key, max: 4096), nowMS > 0
    else { throw MacE2EEKeyStateFailure.invalid }
    let scope = "content-cache"
    var entries: [ContentKeyPayload.Entry] = []
    if let current = try loadRecord(
      kind: .contentCache, scope: scope,
      installationID: installationID)
    {
      guard current.revision == expectedRevision else { throw MacE2EEKeyStateFailure.conflict }
      entries = try decodePayload(ContentKeyPayload.self, from: current).entries
      try validateContentKeyEntries(entries)
    } else if expectedRevision != 0 {
      throw MacE2EEKeyStateFailure.conflict
    }
    entries.removeAll { $0.expiresAtMS <= nowMS || $0.objectID == objectID }
    entries.append(
      .init(
        objectID: objectID, groupID: groupID, epoch: epoch,
        expiresAtMS: expiresAtMS, key: key, cachedAtMS: nowMS))
    entries.sort { ($0.cachedAtMS, $0.objectID) < ($1.cachedAtMS, $1.objectID) }
    while entries.count > Self.maxCachedContentKeys
      || entries.reduce(0, { $0 + $1.key.count }) > Self.maxCachedContentKeyBytes
    { entries.removeFirst() }
    let record = try persist(
      kind: .contentCache, scope: scope,
      installationID: installationID, payload: ContentKeyPayload(entries: entries),
      expectedRevision: expectedRevision, nowMS: nowMS)
    return record.revision
  }

  public func loadContentKey(
    installationID: String, objectID: String, nowMS: Int64
  ) throws -> (MacE2EEContentKeyMetadata, MacE2EESecretLease) {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    guard validIdentifier(objectID, min: 8, max: 128), nowMS > 0 else {
      throw MacE2EEKeyStateFailure.invalid
    }
    guard
      let record = try loadRecord(
        kind: .contentCache, scope: "content-cache",
        installationID: installationID)
    else { throw MacE2EEKeyStateFailure.notFound }
    let payload: ContentKeyPayload = try decodePayload(record)
    try validateContentKeyEntries(payload.entries)
    guard let value = payload.entries.first(where: { $0.objectID == objectID }) else {
      throw MacE2EEKeyStateFailure.notFound
    }
    guard value.expiresAtMS > nowMS else { throw MacE2EEKeyStateFailure.expired }
    return (
      MacE2EEContentKeyMetadata(
        objectID: value.objectID, groupID: value.groupID, epoch: value.epoch,
        expiresAtMS: value.expiresAtMS), MacE2EESecretLease(value.key)
    )
  }

  public func clearContentKeyCache(installationID: String) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try requireInstallation(installationID)
    try deleteSlot(kind: .contentCache, scope: "content-cache")
  }

  private enum Kind: String, Codable {
    case deviceMetadata = "device_metadata"
    case deviceSigning = "device_signing"
    case deviceAgreement = "device_agreement"
    case group
    case grant
    case contentCache = "content_cache"
  }

  private struct Record: Codable, Equatable {
    let version: Int
    let kind: Kind
    let installationID: String
    let scope: String
    let revision: UInt64
    let payloadDigest: String
    let payload: Data
    let createdAtMS: Int64
    let updatedAtMS: Int64
  }

  private struct Witness: Codable, Equatable {
    let version: Int
    let kind: Kind
    let installationID: String
    let scope: String
    let revision: UInt64
    let recordDigest: String
  }

  private struct DevicePayload: Codable {
    let deviceID: String
    let installationID: String
    let keyFormat: String
    let createdAtMS: Int64
  }

  private struct DeviceSecretPayload: Codable {
    let deviceID: String
    let installationID: String
    let keyFormat: String
    let role: String
    let privateKey: Data
    let createdAtMS: Int64
  }

  private struct GroupPayload: Codable {
    let groupID: String
    let epoch: UInt64
    var sendGeneration: UInt64
    let commitDigest: String
    let targetSnapshotDigest: String
    let opaqueState: Data
    var updatedAtMS: Int64
  }

  private struct GrantPayload: Codable {
    let grantID: String
    let groupID: String
    let firstEpoch: UInt64
    let lastEpoch: UInt64
    let expiresAtMS: Int64
    let opaqueGrant: Data
  }

  private struct ContentKeyPayload: Codable {
    struct Entry: Codable {
      let objectID: String
      let groupID: String
      let epoch: UInt64
      let expiresAtMS: Int64
      let key: Data
      let cachedAtMS: Int64
    }
    let entries: [Entry]
  }

  private func requireInstallation(_ installationID: String) throws {
    guard validDigest(installationID) else { throw MacE2EEKeyStateFailure.invalid }
    guard
      let metadataRecord = try loadRecord(
        kind: .deviceMetadata, scope: "device-metadata", installationID: installationID),
      let signingRecord = try loadRecord(
        kind: .deviceSigning, scope: "device-signing", installationID: installationID),
      let agreementRecord = try loadRecord(
        kind: .deviceAgreement, scope: "device-agreement", installationID: installationID)
    else { throw MacE2EEKeyStateFailure.rollbackOrClone }
    let metadata: DevicePayload = try decodePayload(metadataRecord)
    let signing: DeviceSecretPayload = try decodePayload(signingRecord)
    let agreement: DeviceSecretPayload = try decodePayload(agreementRecord)
    try validateDevicePayloads(metadata, signing: signing, agreement: agreement)
    guard metadata.installationID == installationID else {
      throw MacE2EEKeyStateFailure.rollbackOrClone
    }
  }

  private func validateDevicePayloads(
    _ metadata: DevicePayload,
    signing: DeviceSecretPayload,
    agreement: DeviceSecretPayload
  ) throws {
    guard validIdentifier(metadata.deviceID, min: 8, max: 128),
      validDigest(metadata.installationID), validLabel(metadata.keyFormat, max: 64),
      metadata.createdAtMS > 0,
      signing.deviceID == metadata.deviceID, agreement.deviceID == metadata.deviceID,
      signing.installationID == metadata.installationID,
      agreement.installationID == metadata.installationID,
      signing.keyFormat == metadata.keyFormat, agreement.keyFormat == metadata.keyFormat,
      signing.createdAtMS == metadata.createdAtMS,
      agreement.createdAtMS == metadata.createdAtMS,
      signing.role == "signing", agreement.role == "agreement",
      validSecret(signing.privateKey, max: Self.maxPrivateKeyBytes),
      validSecret(agreement.privateKey, max: Self.maxPrivateKeyBytes)
    else { throw MacE2EEKeyStateFailure.rollbackOrClone }
  }

  private func validateGroupPayload(
    _ payload: GroupPayload, expectedGroupID: String
  ) throws {
    guard payload.groupID == expectedGroupID,
      validIdentifier(payload.groupID, min: 8, max: 128), payload.epoch > 0,
      validDigest(payload.commitDigest), validDigest(payload.targetSnapshotDigest),
      validSecret(payload.opaqueState, max: Self.maxOpaqueStateBytes),
      payload.updatedAtMS > 0
    else { throw MacE2EEKeyStateFailure.rollbackOrClone }
  }

  private func validateGrantPayload(
    _ payload: GrantPayload, expectedGrantID: String
  ) throws {
    guard payload.grantID == expectedGrantID,
      validIdentifier(payload.grantID, min: 8, max: 128),
      validIdentifier(payload.groupID, min: 8, max: 128),
      payload.firstEpoch > 0, payload.lastEpoch >= payload.firstEpoch,
      payload.expiresAtMS > 0, validSecret(payload.opaqueGrant, max: Self.maxGrantBytes)
    else { throw MacE2EEKeyStateFailure.rollbackOrClone }
  }

  private func validateContentKeyEntries(_ entries: [ContentKeyPayload.Entry]) throws {
    let objectIDs = Set(entries.map(\.objectID))
    guard entries.count <= Self.maxCachedContentKeys, objectIDs.count == entries.count,
      entries.reduce(0, { $0 + $1.key.count }) <= Self.maxCachedContentKeyBytes,
      entries.allSatisfy({
        validIdentifier($0.objectID, min: 8, max: 128)
          && validIdentifier($0.groupID, min: 8, max: 128) && $0.epoch > 0
          && $0.expiresAtMS > $0.cachedAtMS && $0.cachedAtMS > 0 && validSecret($0.key, max: 4096)
      })
    else { throw MacE2EEKeyStateFailure.rollbackOrClone }
  }

  private func persist<Payload: Encodable>(
    kind: Kind, scope: String, installationID: String, payload: Payload,
    expectedRevision: UInt64, nowMS: Int64
  ) throws -> Record {
    let existing = try loadRecord(kind: kind, scope: scope, installationID: installationID)
    if let existing {
      guard existing.revision == expectedRevision else { throw MacE2EEKeyStateFailure.conflict }
    } else if expectedRevision != 0 {
      throw MacE2EEKeyStateFailure.conflict
    }
    let payloadData = try canonicalEncode(payload)
    let record = Record(
      version: 1, kind: kind, installationID: installationID, scope: scope,
      revision: expectedRevision + 1, payloadDigest: digest(payloadData), payload: payloadData,
      createdAtMS: existing?.createdAtMS ?? nowMS, updatedAtMS: nowMS)
    let recordData = try canonicalEncode(record)
    let accounts = try storeAccounts(kind: kind, scope: scope)
    if existing == nil {
      try store.add(recordData, account: accounts.record)
    } else {
      try store.update(recordData, account: accounts.record)
    }
    guard try store.read(account: accounts.record) == recordData else {
      throw MacE2EEKeyStateFailure.unavailable
    }
    let witness = Witness(
      version: 1, kind: kind, installationID: installationID, scope: scope,
      revision: record.revision, recordDigest: digest(recordData))
    let witnessData = try canonicalEncode(witness)
    if existing == nil {
      try store.add(witnessData, account: accounts.witness)
    } else {
      try store.update(witnessData, account: accounts.witness)
    }
    guard try store.read(account: accounts.witness) == witnessData,
      try loadRecord(kind: kind, scope: scope, installationID: installationID) == record
    else { throw MacE2EEKeyStateFailure.unavailable }
    return record
  }

  private func loadRecord(
    kind: Kind, scope: String, installationID: String?
  ) throws -> Record? {
    let accounts = try storeAccounts(kind: kind, scope: scope)
    let recordData = try store.read(account: accounts.record)
    let witnessData = try store.read(account: accounts.witness)
    switch (recordData, witnessData) {
    case (nil, nil): return nil
    case (.some, nil), (nil, .some): throw MacE2EEKeyStateFailure.rollbackOrClone
    case (.some(let rawRecord), .some(let rawWitness)):
      let record: Record = try canonicalDecode(rawRecord)
      let witness: Witness = try canonicalDecode(rawWitness)
      guard record.version == 1, witness.version == 1, record.kind == kind,
        witness.kind == kind, record.scope == scope, witness.scope == scope,
        record.installationID == witness.installationID,
        installationID == nil || record.installationID == installationID,
        record.revision == witness.revision, record.revision > 0,
        record.payloadDigest == digest(record.payload), witness.recordDigest == digest(rawRecord),
        record.createdAtMS > 0, record.updatedAtMS >= record.createdAtMS
      else { throw MacE2EEKeyStateFailure.rollbackOrClone }
      return record
    }
  }

  private func deleteSlot(kind: Kind, scope: String) throws {
    let accounts = try storeAccounts(kind: kind, scope: scope)
    // Delete the record first. A crash before witness deletion leaves a
    // detectable partial state rather than resurrecting a secret.
    try store.delete(account: accounts.record)
    try store.delete(account: accounts.witness)
  }

  private func storeAccounts(kind: Kind, scope: String) throws -> (record: String, witness: String)
  {
    guard validIdentifier(scope, min: 3, max: 256) else { throw MacE2EEKeyStateFailure.invalid }
    let token = digest(Data(scope.utf8))
    return ("state.\(kind.rawValue).\(token)", "witness.\(kind.rawValue).\(token)")
  }

  private func canonicalEncode<Value: Encodable>(_ value: Value) throws -> Data {
    do {
      let encoder = JSONEncoder()
      encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
      return try encoder.encode(value)
    } catch { throw MacE2EEKeyStateFailure.corrupt }
  }

  private func canonicalDecode<Value: Codable>(_ data: Data) throws -> Value {
    do {
      let value = try JSONDecoder().decode(Value.self, from: data)
      guard try canonicalEncode(value) == data else { throw MacE2EEKeyStateFailure.corrupt }
      return value
    } catch let failure as MacE2EEKeyStateFailure { throw failure } catch {
      throw MacE2EEKeyStateFailure.corrupt
    }
  }

  private func decodePayload<Value: Codable>(_ record: Record) throws -> Value {
    try canonicalDecode(record.payload)
  }

  private func decodePayload<Value: Codable>(_ type: Value.Type, from record: Record) throws
    -> Value
  {
    try decodePayload(record)
  }

  private func deviceMetadata(_ payload: DevicePayload, revision: UInt64)
    -> MacE2EEDeviceIdentityMetadata
  {
    MacE2EEDeviceIdentityMetadata(
      deviceID: payload.deviceID, installationID: payload.installationID,
      keyFormat: payload.keyFormat, revision: revision, createdAtMS: payload.createdAtMS)
  }

  private func groupMetadata(
    _ payload: GroupPayload, installationID: String, revision: UInt64
  ) -> MacE2EEGroupStateMetadata {
    MacE2EEGroupStateMetadata(
      groupID: payload.groupID, installationID: installationID, epoch: payload.epoch,
      sendGeneration: payload.sendGeneration, commitDigest: payload.commitDigest,
      targetSnapshotDigest: payload.targetSnapshotDigest, revision: revision,
      updatedAtMS: payload.updatedAtMS)
  }

  private func grantMetadata(_ payload: GrantPayload, revision: UInt64)
    -> MacE2EEGrantMetadata
  {
    MacE2EEGrantMetadata(
      grantID: payload.grantID, groupID: payload.groupID, firstEpoch: payload.firstEpoch,
      lastEpoch: payload.lastEpoch, expiresAtMS: payload.expiresAtMS, revision: revision)
  }

  private func validSecret(_ value: Data, max: Int) -> Bool {
    !value.isEmpty && value.count <= max
  }

  private func validIdentifier(_ value: String, min: Int, max: Int) -> Bool {
    value.utf8.count >= min && value.utf8.count <= max
      && value.utf8.allSatisfy {
        ($0 >= 0x30 && $0 <= 0x39) || ($0 >= 0x41 && $0 <= 0x5A) || ($0 >= 0x61 && $0 <= 0x7A)
          || $0 == 0x2D || $0 == 0x2E || $0 == 0x2F || $0 == 0x5F
      }
  }

  private func validLabel(_ value: String, max: Int) -> Bool {
    validIdentifier(value, min: 1, max: max) && !value.contains("/")
  }

  private func validDigest(_ value: String) -> Bool {
    value.utf8.count == 64
      && value.utf8.allSatisfy {
        ($0 >= 0x30 && $0 <= 0x39) || ($0 >= 0x61 && $0 <= 0x66)
      }
  }

  private func digest(_ data: Data) -> String { SHA256.hash(data: data).hex }
  private func hex(_ data: Data) -> String { data.map { String(format: "%02x", $0) }.joined() }
}

extension SHA256.Digest {
  fileprivate var hex: String { map { String(format: "%02x", $0) }.joined() }
}
