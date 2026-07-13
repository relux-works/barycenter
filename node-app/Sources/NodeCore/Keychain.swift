import Foundation
import Security

public enum ProtectedStoreLocation: String, Codable, Hashable, Sendable {
  case dataProtection
  case login
}

public struct ProtectedStoreKey: Hashable, Sendable {
  public let service: String
  public let account: String
  public let location: ProtectedStoreLocation

  public init(service: String, account: String, location: ProtectedStoreLocation) {
    self.service = service
    self.account = account
    self.location = location
  }
}

public enum ProtectedStoreFailure: Error, Equatable, LocalizedError {
  case missingEntitlement
  case duplicate
  case unavailable
  case structural

  public var errorDescription: String? { "Protected credential storage is unavailable." }
}

/// Injectable protected byte store. Production is backed by Security.framework;
/// tests use isolated in-memory implementations and never touch user Keychain.
public protocol ProtectedStore: Sendable {
  func read(_ key: ProtectedStoreKey) throws -> Data?
  func add(_ data: Data, for key: ProtectedStoreKey) throws
  func update(_ data: Data, for key: ProtectedStoreKey) throws
  func delete(_ key: ProtectedStoreKey) throws
}

public final class SystemKeychainStore: ProtectedStore, @unchecked Sendable {
  public init() {}

  private func query(_ key: ProtectedStoreKey) -> [String: Any] {
    var value: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword,
      kSecAttrService as String: key.service,
      kSecAttrAccount as String: key.account,
    ]
    if key.location == .dataProtection {
      value[kSecUseDataProtectionKeychain as String] = true
    }
    return value
  }

  public func read(_ key: ProtectedStoreKey) throws -> Data? {
    var value = query(key)
    value[kSecReturnData as String] = true
    value[kSecMatchLimit as String] = kSecMatchLimitOne
    var out: CFTypeRef?
    let status = SecItemCopyMatching(value as CFDictionary, &out)
    if status == errSecItemNotFound { return nil }
    try check(status)
    guard let data = out as? Data else { throw ProtectedStoreFailure.structural }
    return data
  }

  public func add(_ data: Data, for key: ProtectedStoreKey) throws {
    var value = query(key)
    value[kSecValueData as String] = data
    value[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
    let status = SecItemAdd(value as CFDictionary, nil)
    if status == errSecDuplicateItem { throw ProtectedStoreFailure.duplicate }
    try check(status)
  }

  public func update(_ data: Data, for key: ProtectedStoreKey) throws {
    let status = SecItemUpdate(
      query(key) as CFDictionary,
      [kSecValueData as String: data] as CFDictionary
    )
    // An update is never implemented as delete-then-add. Failure leaves the
    // prior good item intact and is returned structurally.
    try check(status)
  }

  public func delete(_ key: ProtectedStoreKey) throws {
    let status = SecItemDelete(query(key) as CFDictionary)
    if status == errSecItemNotFound { return }
    try check(status)
  }

  private func check(_ status: OSStatus) throws {
    guard status != errSecSuccess else { return }
    if status == errSecMissingEntitlement { throw ProtectedStoreFailure.missingEntitlement }
    throw ProtectedStoreFailure.unavailable
  }
}

public protocol CredentialFileAccess: Sendable {
  func readLegacy(at url: URL) throws -> Data?
  func deleteLegacy(at url: URL) throws
}

public struct SystemCredentialFileAccess: CredentialFileAccess, Sendable {
  public init() {}

  public func readLegacy(at url: URL) throws -> Data? {
    guard FileManager.default.fileExists(atPath: url.path) else { return nil }
    return try Data(contentsOf: url)
  }

  public func deleteLegacy(at url: URL) throws {
    if FileManager.default.fileExists(atPath: url.path) {
      try FileManager.default.removeItem(at: url)
    }
  }
}

public enum CredentialStorageError: Error, Equatable, LocalizedError {
  case corrupt
  case conflict
  case unsupportedVersion
  case unavailable
  case verificationFailed

  public var errorDescription: String? {
    switch self {
    case .conflict: return "Conflicting protected credentials require manual resolution."
    case .unsupportedVersion: return "The protected credential format is not supported."
    default: return "Protected credentials could not be read or saved safely."
    }
  }
}

public final class CredentialRepository: @unchecked Sendable {
  static let service = "works.relux.pulsar"
  static let legacyAccount = "node-credentials"
  static let previousDestinationAccount = "onboarding-credential-bundle-v1"
  static let destinationAccount = "onboarding-credential-bundle-v2"
  private static let processLock = NSRecursiveLock()

  private let store: any ProtectedStore
  private let files: any CredentialFileAccess

  public init(
    store: any ProtectedStore = SystemKeychainStore(),
    files: any CredentialFileAccess = SystemCredentialFileAccess()
  ) {
    self.store = store
    self.files = files
  }

  public func loadBundle(besideConfig configPath: String) throws -> CredentialBundle? {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try migratePreviousDestination()
    let destination = try readDestination()
    let legacyURL = NodeCredentials.fileURL(besideConfig: configPath)
    let sources = try readLegacySources(fileURL: legacyURL)

    guard !sources.isEmpty else { return destination?.bundle }
    let uniqueNodes = Set(sources.map(\.credentials))
    guard uniqueNodes.count == 1, let legacyNode = uniqueNodes.first else {
      throw CredentialStorageError.conflict
    }

    var candidate = destination?.bundle ?? .legacy(legacyNode)
    if let existing = candidate.node?.legacyView, existing != legacyNode {
      throw CredentialStorageError.conflict
    }
    if candidate.node == nil {
      candidate.node = CredentialBundle.legacy(legacyNode).node
    }

    let candidateData = try encoded(candidate)
    try saveBundle(candidate)
    guard try readDestination()?.data == candidateData else {
      throw CredentialStorageError.verificationFailed
    }

    // Each source is removed only after the distinct destination has been
    // read through the same abstraction and compared field-for-field.
    for source in sources {
      switch source.kind {
      case .keychain(let key): try store.delete(key)
      case .file: try files.deleteLegacy(at: legacyURL)
      }
    }
    return candidate
  }

  public func loadBundleWithoutMigration() throws -> CredentialBundle? {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try migratePreviousDestination()
    return try readDestination()?.bundle
  }

  public func saveNode(_ credentials: NodeCredentials) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    var bundle = try loadBundleWithoutMigration() ?? CredentialBundle()
    bundle.node = CredentialBundle.legacy(credentials).node
    try saveBundle(bundle)
  }

  public func saveBundle(_ bundle: CredentialBundle) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try migratePreviousDestination()
    try validate(bundle)
    let data = try encoded(bundle)
    try writeCurrentDestination(data, bundle: bundle)
  }

  private func writeCurrentDestination(_ data: Data, bundle: CredentialBundle) throws {
    let dp = destinationKey(.dataProtection)
    let login = destinationKey(.login)

    let dpExisting = try readable(dp)
    let loginExisting = try readable(login)
    if let dpExisting, let loginExisting {
      guard dpExisting == loginExisting else {
        throw CredentialStorageError.conflict
      }
      _ = try decodedBundle(dpExisting)
      // Keep the verified login copy until DP contains and verifies the new
      // complete value. A corrupt or failed update therefore cannot destroy
      // the only prior-good destination.
      try updateAndVerify(data, bundle: bundle, key: dp)
      do { try store.delete(login) } catch { throw CredentialStorageError.unavailable }
      return
    }
    if dpExisting != nil {
      try updateAndVerify(data, bundle: bundle, key: dp)
      return
    }
    if loginExisting != nil {
      try updateAndVerify(data, bundle: bundle, key: login)
      return
    }

    do {
      try addAndVerify(data, bundle: bundle, key: dp)
    } catch ProtectedStoreFailure.missingEntitlement {
      try addAndVerify(data, bundle: bundle, key: login)
    } catch ProtectedStoreFailure.duplicate {
      try resolveDuplicate(data, bundle: bundle, key: dp)
    } catch let error as CredentialStorageError {
      throw error
    } catch {
      throw CredentialStorageError.unavailable
    }
  }

  public func clearDestinations() throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try? store.delete(destinationKey(.dataProtection))
    try? store.delete(destinationKey(.login))
    try? store.delete(previousDestinationKey(.dataProtection))
    try? store.delete(previousDestinationKey(.login))
  }

  /// Cross-instance atomic read-modify-write used when only one capability is
  /// changing. This prevents a concurrent pair save and control promotion from
  /// losing one another's fields.
  @discardableResult
  public func mutateBundle(
    _ mutation: (inout CredentialBundle) throws -> Void
  ) throws -> CredentialBundle {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    try migratePreviousDestination()
    var bundle = try readDestination()?.bundle ?? CredentialBundle()
    try mutation(&bundle)
    let expectedData = try encoded(bundle)
    try saveBundle(bundle)
    guard try readDestination()?.data == expectedData else {
      throw CredentialStorageError.verificationFailed
    }
    return bundle
  }

  private struct Destination {
    let key: ProtectedStoreKey
    let data: Data
    let bundle: CredentialBundle
  }

  private func readDestination() throws -> Destination? {
    let dp = destinationKey(.dataProtection)
    let login = destinationKey(.login)
    let dpData = try readable(dp)
    let loginData = try readable(login)
    switch (dpData, loginData) {
    case (nil, nil): return nil
    case (.some(let data), nil):
      return Destination(key: dp, data: data, bundle: try decodedBundle(data))
    case (nil, .some(let data)):
      return Destination(key: login, data: data, bundle: try decodedBundle(data))
    case (.some(let left), .some(let right)):
      guard left == right else { throw CredentialStorageError.conflict }
      return Destination(key: dp, data: left, bundle: try decodedBundle(left))
    }
  }

  private struct PreviousDestination {
    let keys: [ProtectedStoreKey]
    let bundle: CredentialBundle
  }

  private func migratePreviousDestination() throws {
    guard let previous = try readPreviousDestination() else { return }
    if let current = try readDestination() {
      guard current.bundle == previous.bundle else { throw CredentialStorageError.conflict }
    } else {
      let data = try encoded(previous.bundle)
      try writeCurrentDestination(data, bundle: previous.bundle)
      guard try readDestination()?.data == data else {
        throw CredentialStorageError.verificationFailed
      }
    }
    for key in previous.keys {
      do { try store.delete(key) } catch { throw CredentialStorageError.unavailable }
    }
  }

  private func readPreviousDestination() throws -> PreviousDestination? {
    let dp = previousDestinationKey(.dataProtection)
    let login = previousDestinationKey(.login)
    let dpData = try readable(dp)
    let loginData = try readable(login)
    switch (dpData, loginData) {
    case (nil, nil): return nil
    case (.some(let data), nil):
      return PreviousDestination(keys: [dp], bundle: try decodedPreviousBundle(data))
    case (nil, .some(let data)):
      return PreviousDestination(keys: [login], bundle: try decodedPreviousBundle(data))
    case (.some(let left), .some(let right)):
      guard left == right else { throw CredentialStorageError.conflict }
      return PreviousDestination(keys: [dp, login], bundle: try decodedPreviousBundle(left))
    }
  }

  private enum LegacyKind {
    case keychain(ProtectedStoreKey)
    case file
  }

  private struct LegacySource {
    let kind: LegacyKind
    let credentials: NodeCredentials
  }

  private func readLegacySources(fileURL: URL) throws -> [LegacySource] {
    let decoder = JSONDecoder()
    var sources: [LegacySource] = []
    for location in [ProtectedStoreLocation.dataProtection, .login] {
      let key = legacyKey(location)
      if let data = try readable(key) {
        guard let value = try? decoder.decode(NodeCredentials.self, from: data) else {
          throw CredentialStorageError.corrupt
        }
        sources.append(LegacySource(kind: .keychain(key), credentials: value))
      }
    }
    if let data = try files.readLegacy(at: fileURL) {
      guard let value = try? decoder.decode(NodeCredentials.self, from: data) else {
        throw CredentialStorageError.corrupt
      }
      sources.append(LegacySource(kind: .file, credentials: value))
    }
    return sources
  }

  private func readable(_ key: ProtectedStoreKey) throws -> Data? {
    do { return try store.read(key) } catch ProtectedStoreFailure.missingEntitlement
      where key.location == .dataProtection
    { return nil } catch { throw CredentialStorageError.unavailable }
  }

  private func addAndVerify(_ data: Data, bundle: CredentialBundle, key: ProtectedStoreKey) throws {
    do { try store.add(data, for: key) } catch ProtectedStoreFailure.duplicate {
      try resolveDuplicate(data, bundle: bundle, key: key)
      return
    } catch { throw error }
    guard let readback = try readable(key), readback == data,
      try decodedBundle(readback) == bundle
    else {
      throw CredentialStorageError.verificationFailed
    }
  }

  private func resolveDuplicate(_ data: Data, bundle: CredentialBundle, key: ProtectedStoreKey)
    throws
  {
    guard let existing = try readable(key) else { throw CredentialStorageError.verificationFailed }
    _ = try decodedBundle(existing)
    guard existing == data else { throw CredentialStorageError.conflict }
    _ = bundle
  }

  private func updateAndVerify(_ data: Data, bundle: CredentialBundle, key: ProtectedStoreKey)
    throws
  {
    do { try store.update(data, for: key) } catch { throw CredentialStorageError.unavailable }
    guard let readback = try readable(key), readback == data,
      try decodedBundle(readback) == bundle
    else {
      throw CredentialStorageError.verificationFailed
    }
  }

  private func encoded(_ bundle: CredentialBundle) throws -> Data {
    do {
      let encoder = JSONEncoder()
      encoder.outputFormatting = [.sortedKeys]
      return try encoder.encode(bundle)
    } catch {
      throw CredentialStorageError.corrupt
    }
  }

  private func decodedBundle(_ data: Data) throws -> CredentialBundle {
    try validateProtectedSchema(data, version: CredentialBundle.currentVersion)
    let decoder = JSONDecoder()
    guard let bundle = try? decoder.decode(CredentialBundle.self, from: data) else {
      throw CredentialStorageError.corrupt
    }
    try validate(bundle)
    guard try encoded(bundle) == data else { throw CredentialStorageError.corrupt }
    return bundle
  }

  private struct PreviousControlCapability: Codable {
    let actorId: Int64
    let orbitId: Int64?
    let role: CredentialRole?
    let controlToken: String

    enum CodingKeys: String, CodingKey {
      case actorId = "actor_id"
      case orbitId = "orbit_id"
      case role
      case controlToken = "control_token"
    }
  }

  private struct PreviousCredentialBundle: Codable {
    let version: Int
    let coordinatorOrigin: CoordinatorOrigin?
    let node: NodeCapability?
    let control: PreviousControlCapability?
    let recovery: RecoveryMetadata?

    enum CodingKeys: String, CodingKey {
      case version
      case coordinatorOrigin = "coordinator_origin"
      case node, control, recovery
    }
  }

  private func decodedPreviousBundle(_ data: Data) throws -> CredentialBundle {
    try validateProtectedSchema(data, version: 1)
    guard let previous = try? JSONDecoder().decode(PreviousCredentialBundle.self, from: data),
      previous.version == 1
    else {
      throw CredentialStorageError.corrupt
    }
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    guard (try? encoder.encode(previous)) == data else { throw CredentialStorageError.corrupt }
    let control = previous.control.map {
      ControlCapability(
        actorId: $0.actorId,
        orbitId: $0.orbitId,
        role: $0.role,
        controlToken: $0.controlToken,
        contextStrength: $0.orbitId == nil ? .limited : .active)
    }
    let migrated = CredentialBundle(
      coordinatorOrigin: previous.coordinatorOrigin,
      node: previous.node,
      control: control,
      recovery: previous.recovery)
    try validate(migrated)
    return migrated
  }

  private func validateProtectedSchema(_ data: Data, version: Int) throws {
    do {
      var parser = StrictJSONParser(data)
      guard let root = try parser.parse().object, root["version"]?.int64 == Int64(version) else {
        throw CredentialStorageError.corrupt
      }
      try root.exactKeys(
        ["version"], optional: ["coordinator_origin", "node", "control", "recovery"])
      if let origin = root["coordinator_origin"] {
        guard origin.string != nil else { throw CredentialStorageError.corrupt }
      }
      if let nodeValue = root["node"] {
        guard let node = nodeValue.object else { throw CredentialStorageError.corrupt }
        try node.exactKeys(["orbit_id", "slot", "node_token", "ws_url"])
        guard node["orbit_id"]?.int64 != nil, node["slot"]?.string != nil,
          node["node_token"]?.string != nil, node["ws_url"]?.string != nil
        else { throw CredentialStorageError.corrupt }
      }
      if let controlValue = root["control"] {
        guard let control = controlValue.object else { throw CredentialStorageError.corrupt }
        var required: Set<String> = ["actor_id", "control_token"]
        if version == CredentialBundle.currentVersion { required.insert("context_strength") }
        try control.exactKeys(required, optional: ["orbit_id", "role"])
        guard control["actor_id"]?.int64 != nil, control["control_token"]?.string != nil,
          version != CredentialBundle.currentVersion || control["context_strength"]?.string != nil,
          control["orbit_id"].map({ $0.int64 != nil }) ?? true,
          control["role"].map({ $0.string != nil }) ?? true
        else { throw CredentialStorageError.corrupt }
      }
      if let recoveryValue = root["recovery"] {
        guard let recovery = recoveryValue.object else { throw CredentialStorageError.corrupt }
        try recovery.exactKeys(
          ["actor_id", "recovery_id", "explicit_backup_acknowledged"])
        guard recovery["actor_id"]?.int64 != nil, recovery["recovery_id"]?.string != nil,
          recovery["explicit_backup_acknowledged"]?.bool != nil
        else { throw CredentialStorageError.corrupt }
      }
    } catch let error as CredentialStorageError {
      throw error
    } catch {
      throw CredentialStorageError.corrupt
    }
  }

  private func validate(_ bundle: CredentialBundle) throws {
    guard bundle.version == CredentialBundle.currentVersion else {
      throw CredentialStorageError.unsupportedVersion
    }
    guard bundle.node != nil || bundle.control != nil else {
      throw CredentialStorageError.corrupt
    }
    if let origin = bundle.coordinatorOrigin {
      guard (try? CoordinatorOrigin(origin.rawValue)) == origin else {
        throw CredentialStorageError.corrupt
      }
    }
    if let node = bundle.node {
      guard node.orbitId > 0, CredentialSyntax.slot(node.slot),
        CredentialSyntax.lowerHexToken(node.nodeToken),
        CredentialSyntax.canonicalWebSocketURL(node.wsUrl, origin: bundle.coordinatorOrigin)
      else {
        throw CredentialStorageError.corrupt
      }
    }
    if let control = bundle.control {
      guard control.actorId > 0,
        control.orbitId.map({ $0 > 0 }) ?? true,
        (control.orbitId == nil) == (control.role == nil),
        control.contextStrength != .active || control.orbitId != nil,
        CredentialSyntax.lowerHexToken(control.controlToken),
        bundle.coordinatorOrigin != nil,
        bundle.node.map({ control.orbitId == nil || $0.orbitId == control.orbitId }) ?? true
      else {
        throw CredentialStorageError.corrupt
      }
    }
    if let recovery = bundle.recovery {
      guard recovery.actorId > 0, CredentialSyntax.recoveryID(recovery.recoveryId),
        bundle.coordinatorOrigin != nil,
        bundle.control?.actorId == recovery.actorId
      else {
        throw CredentialStorageError.corrupt
      }
    }
  }

  private func destinationKey(_ location: ProtectedStoreLocation) -> ProtectedStoreKey {
    ProtectedStoreKey(service: Self.service, account: Self.destinationAccount, location: location)
  }

  private func previousDestinationKey(_ location: ProtectedStoreLocation) -> ProtectedStoreKey {
    ProtectedStoreKey(
      service: Self.service, account: Self.previousDestinationAccount, location: location)
  }

  private func legacyKey(_ location: ProtectedStoreLocation) -> ProtectedStoreKey {
    ProtectedStoreKey(service: Self.service, account: Self.legacyAccount, location: location)
  }
}

/// Existing startup facade retained source-compatible for the pair-only app.
public enum CredentialsStore {
  private static let repository = CredentialRepository()

  public static func save(_ credentials: NodeCredentials) throws {
    try repository.saveNode(credentials)
  }

  public static func saveBundle(_ bundle: CredentialBundle) throws {
    try repository.saveBundle(bundle)
  }

  public static func loadBundle(besideConfig configPath: String) throws -> CredentialBundle? {
    try repository.loadBundle(besideConfig: configPath)
  }

  public static func load(besideConfig configPath: String) -> NodeCredentials? {
    (try? repository.loadBundle(besideConfig: configPath))?.nodeCredentials
  }

  public static func loadFromKeychain() -> NodeCredentials? {
    (try? repository.loadBundleWithoutMigration())?.nodeCredentials
  }

  public static func clear() {
    try? repository.clearDestinations()
  }
}
