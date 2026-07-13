import CryptoKit
import Foundation
import Security

public struct PendingRecoveryRecord: Codable, Equatable, Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public let canonicalCoordinatorOrigin: CoordinatorOrigin
  public let actorId: Int64
  public let recoveryId: String
  public let pendingControlToken: String
  public let everSent: Bool

  public init(
    canonicalCoordinatorOrigin: CoordinatorOrigin,
    actorId: Int64,
    recoveryId: String,
    pendingControlToken: String,
    everSent: Bool
  ) {
    self.canonicalCoordinatorOrigin = canonicalCoordinatorOrigin
    self.actorId = actorId
    self.recoveryId = recoveryId
    self.pendingControlToken = pendingControlToken
    self.everSent = everSent
  }

  enum CodingKeys: String, CodingKey {
    case canonicalCoordinatorOrigin = "canonical_coordinator_origin"
    case actorId = "actor_id"
    case recoveryId = "recovery_id"
    case pendingControlToken = "pending_control_token"
    case everSent = "ever_sent"
  }

  public var description: String {
    "PendingRecoveryRecord(sent: \(everSent), token: <redacted>)"
  }
  public var debugDescription: String { description }

  var sent: PendingRecoveryRecord {
    PendingRecoveryRecord(
      canonicalCoordinatorOrigin: canonicalCoordinatorOrigin,
      actorId: actorId,
      recoveryId: recoveryId,
      pendingControlToken: pendingControlToken,
      everSent: true
    )
  }

  func isSameCandidate(as other: PendingRecoveryRecord) -> Bool {
    canonicalCoordinatorOrigin == other.canonicalCoordinatorOrigin
      && actorId == other.actorId
      && recoveryId == other.recoveryId
      && pendingControlToken == other.pendingControlToken
  }
}

public final class PendingRecoveryRepository: @unchecked Sendable {
  private static let service = "works.relux.pulsar.recovery"
  private static let accountPrefix = "pending-control-v1-"
  private static let processLock = NSRecursiveLock()
  private let store: any ProtectedStore

  public init(store: any ProtectedStore = SystemKeychainStore()) {
    self.store = store
  }

  public func load(origin: CoordinatorOrigin, actorID: Int64) throws -> PendingRecoveryRecord? {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    let dp = key(origin: origin, actorID: actorID, location: .dataProtection)
    let login = key(origin: origin, actorID: actorID, location: .login)
    let left = try readable(dp)
    let right = try readable(login)
    switch (left, right) {
    case (nil, nil): return nil
    case (.some(let data), nil): return try decode(data, expectedOrigin: origin, actorID: actorID)
    case (nil, .some(let data)): return try decode(data, expectedOrigin: origin, actorID: actorID)
    case (.some(let l), .some(let r)):
      guard l == r else { throw CredentialStorageError.conflict }
      return try decode(l, expectedOrigin: origin, actorID: actorID)
    }
  }

  public func createUnsent(_ record: PendingRecoveryRecord) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard !record.everSent,
      CredentialSyntax.recoveryID(record.recoveryId),
      CredentialSyntax.lowerHexToken(record.pendingControlToken),
      record.actorId > 0
    else { throw CredentialStorageError.corrupt }
    guard try load(origin: record.canonicalCoordinatorOrigin, actorID: record.actorId) == nil else {
      throw CredentialStorageError.conflict
    }
    let data = try encode(record)
    let dp = key(
      origin: record.canonicalCoordinatorOrigin, actorID: record.actorId, location: .dataProtection)
    do { try addAndVerify(data, record: record, key: dp) } catch ProtectedStoreFailure
      .missingEntitlement
    {
      let login = key(
        origin: record.canonicalCoordinatorOrigin, actorID: record.actorId, location: .login)
      try addAndVerify(data, record: record, key: login)
    } catch ProtectedStoreFailure.duplicate {
      guard try load(origin: record.canonicalCoordinatorOrigin, actorID: record.actorId) == record
      else {
        throw CredentialStorageError.conflict
      }
    } catch let error as CredentialStorageError {
      throw error
    } catch {
      throw CredentialStorageError.unavailable
    }
  }

  /// The only false→true transition. It uses update on the exact existing
  /// item, then reads that same item back and compares every field.
  public func markSent(_ expectedUnsent: PendingRecoveryRecord) throws -> PendingRecoveryRecord {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    guard !expectedUnsent.everSent else { throw CredentialStorageError.corrupt }
    let located = try locations(
      origin: expectedUnsent.canonicalCoordinatorOrigin, actorID: expectedUnsent.actorId)
    guard !located.isEmpty else { throw CredentialStorageError.verificationFailed }
    guard located.allSatisfy({ $0.record.isSameCandidate(as: expectedUnsent) }) else {
      throw CredentialStorageError.conflict
    }
    let sent = expectedUnsent.sent
    let data = try encode(sent)
    for item in located where !item.record.everSent {
      do { try store.update(data, for: item.key) } catch {
        throw CredentialStorageError.unavailable
      }
      guard let readback = try readable(item.key),
        readback == data,
        try decode(
          readback, expectedOrigin: sent.canonicalCoordinatorOrigin, actorID: sent.actorId) == sent
      else {
        throw CredentialStorageError.verificationFailed
      }
    }
    let verified = try locations(
      origin: sent.canonicalCoordinatorOrigin, actorID: sent.actorId)
    guard !verified.isEmpty, verified.allSatisfy({ $0.data == data && $0.record == sent }) else {
      throw CredentialStorageError.verificationFailed
    }
    return sent
  }

  public func deleteExact(_ expected: PendingRecoveryRecord) throws {
    Self.processLock.lock()
    defer { Self.processLock.unlock() }
    let located = try locations(
      origin: expected.canonicalCoordinatorOrigin, actorID: expected.actorId
    )
    guard !located.isEmpty else { return }
    guard
      located.allSatisfy({ $0.record == expected })
    else {
      throw CredentialStorageError.conflict
    }
    for item in located {
      do { try store.delete(item.key) } catch { throw CredentialStorageError.unavailable }
    }
  }

  private struct Located {
    let key: ProtectedStoreKey
    let data: Data
    let record: PendingRecoveryRecord
  }

  private func locations(origin: CoordinatorOrigin, actorID: Int64) throws -> [Located] {
    let keys = [
      key(origin: origin, actorID: actorID, location: .dataProtection),
      key(origin: origin, actorID: actorID, location: .login),
    ]
    var found: [Located] = []
    for key in keys {
      if let data = try readable(key) {
        found.append(
          Located(
            key: key, data: data,
            record: try decode(data, expectedOrigin: origin, actorID: actorID)))
      }
    }
    guard found.count <= 1 || found.dropFirst().allSatisfy({ $0.data == found[0].data }) else {
      throw CredentialStorageError.conflict
    }
    return found
  }

  private func addAndVerify(_ data: Data, record: PendingRecoveryRecord, key: ProtectedStoreKey)
    throws
  {
    try store.add(data, for: key)
    guard let readback = try readable(key),
      readback == data,
      try decode(
        readback, expectedOrigin: record.canonicalCoordinatorOrigin, actorID: record.actorId)
        == record
    else {
      throw CredentialStorageError.verificationFailed
    }
  }

  private func readable(_ key: ProtectedStoreKey) throws -> Data? {
    do { return try store.read(key) } catch ProtectedStoreFailure.missingEntitlement
      where key.location == .dataProtection
    { return nil } catch { throw CredentialStorageError.unavailable }
  }

  private func encode(_ record: PendingRecoveryRecord) throws -> Data {
    do {
      let encoder = JSONEncoder()
      encoder.outputFormatting = [.sortedKeys]
      return try encoder.encode(record)
    } catch {
      throw CredentialStorageError.corrupt
    }
  }

  private func decode(_ data: Data, expectedOrigin: CoordinatorOrigin, actorID: Int64) throws
    -> PendingRecoveryRecord
  {
    do {
      var parser = StrictJSONParser(data)
      guard let object = try parser.parse().object else { throw CredentialStorageError.corrupt }
      try object.exactKeys([
        "canonical_coordinator_origin", "actor_id", "recovery_id",
        "pending_control_token", "ever_sent",
      ])
      guard object["canonical_coordinator_origin"]?.string != nil,
        object["actor_id"]?.int64 != nil,
        object["recovery_id"]?.string != nil,
        object["pending_control_token"]?.string != nil,
        object["ever_sent"]?.bool != nil
      else { throw CredentialStorageError.corrupt }
    } catch let error as CredentialStorageError {
      throw error
    } catch {
      throw CredentialStorageError.corrupt
    }
    let decoder = JSONDecoder()
    guard let record = try? decoder.decode(PendingRecoveryRecord.self, from: data),
      record.canonicalCoordinatorOrigin == expectedOrigin,
      record.actorId == actorID,
      record.actorId > 0,
      CredentialSyntax.recoveryID(record.recoveryId),
      CredentialSyntax.lowerHexToken(record.pendingControlToken)
    else {
      throw CredentialStorageError.corrupt
    }
    guard try encode(record) == data else { throw CredentialStorageError.corrupt }
    return record
  }

  private func key(
    origin: CoordinatorOrigin,
    actorID: Int64,
    location: ProtectedStoreLocation
  ) -> ProtectedStoreKey {
    let source = Data("\(origin.rawValue)\u{0}\(actorID)".utf8)
    let digest = SHA256.hash(data: source).map { String(format: "%02x", $0) }.joined()
    return ProtectedStoreKey(
      service: Self.service, account: Self.accountPrefix + digest, location: location
    )
  }
}

public protocol ControlTokenGenerator: Sendable {
  func generate() throws -> String
}

public struct SystemControlTokenGenerator: ControlTokenGenerator, Sendable {
  public init() {}
  public func generate() throws -> String {
    var bytes = [UInt8](repeating: 0, count: 32)
    guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else {
      throw CredentialStorageError.unavailable
    }
    return bytes.map { String(format: "%02x", $0) }.joined()
  }
}

struct RecoveryScope: Hashable, Sendable {
  let origin: CoordinatorOrigin
  let actorID: Int64
}

actor RecoveryScopeSerializer {
  static let shared = RecoveryScopeSerializer()
  private struct Waiter {
    let id: UUID
    let continuation: CheckedContinuation<Void, Error>
  }
  private var owners: [RecoveryScope: UUID] = [:]
  private var waiters: [RecoveryScope: [Waiter]] = [:]
  private var queueObservers: [RecoveryScope: [UUID: [CheckedContinuation<Void, Never>]]] = [:]

  func acquire(_ scope: RecoveryScope, id: UUID) async throws {
    try Task.checkCancellation()
    if owners[scope] == nil {
      owners[scope] = id
      do {
        try Task.checkCancellation()
      } catch {
        release(scope, id: id)
        throw error
      }
      return
    }
    do {
      try await withCheckedThrowingContinuation { continuation in
        waiters[scope, default: []].append(Waiter(id: id, continuation: continuation))
        let observers = queueObservers[scope]?[id] ?? []
        queueObservers[scope]?[id] = nil
        if queueObservers[scope]?.isEmpty == true { queueObservers[scope] = nil }
        for observer in observers { observer.resume() }
      }
      try Task.checkCancellation()
    } catch {
      // Release is synchronous on this actor. If owner release promoted this
      // cancelled waiter before its cancellation handler ran, no caller code
      // can execute while the cancelled task still owns the scope.
      release(scope, id: id)
      throw error
    }
  }

  /// Internal synchronization probe used by deterministic concurrency tests.
  /// It exposes no credential state and returns only after this exact owner is
  /// suspended behind an existing scope owner.
  func waitUntilQueued(_ scope: RecoveryScope, id: UUID) async {
    if waiters[scope]?.contains(where: { $0.id == id }) == true { return }
    await withCheckedContinuation { continuation in
      queueObservers[scope, default: [:]][id, default: []].append(continuation)
    }
  }

  func cancel(_ scope: RecoveryScope, id: UUID) {
    if owners[scope] == id { return }
    guard var queued = waiters[scope],
      let index = queued.firstIndex(where: { $0.id == id })
    else {
      // A task cancelled before enqueue observes its own cancellation at the
      // start of acquire. Unknown/late ids are deliberately not retained.
      return
    }
    let waiter = queued.remove(at: index)
    waiters[scope] = queued.isEmpty ? nil : queued
    waiter.continuation.resume(throwing: CancellationError())
  }

  func release(_ scope: RecoveryScope, id: UUID) {
    guard owners[scope] == id else { return }
    var queued = waiters[scope] ?? []
    if !queued.isEmpty {
      let next = queued.removeFirst()
      owners[scope] = next.id
      waiters[scope] = queued.isEmpty ? nil : queued
      next.continuation.resume()
      return
    }
    owners.removeValue(forKey: scope)
    if queued.isEmpty {
      waiters.removeValue(forKey: scope)
    }
  }
}

public enum RecoveryPendingReason: Equatable, Sendable {
  case credentialRejected
  case rateLimited(Int)
  case ambiguousResponse
}

public struct RecoveredCredential: Sendable, CustomStringConvertible, CustomDebugStringConvertible {
  public let bundle: CredentialBundle
  public let hasLimitedContext: Bool
  public var description: String {
    "RecoveredCredential(context: \(hasLimitedContext ? "limited" : "active"), credentials: <redacted>)"
  }
  public var debugDescription: String { description }
}

public enum RecoveryServiceOutcome: Sendable, CustomStringConvertible, CustomDebugStringConvertible
{
  case recovered(RecoveredCredential)
  case pendingRetained(RecoveryPendingReason)
  case needsSecretForExistingGeneration(recoveryID: String)
  case destructiveAbandonAvailable

  public var description: String {
    switch self {
    case .recovered: return "RecoveryServiceOutcome(recovered: <redacted>)"
    case .pendingRetained: return "RecoveryServiceOutcome(pendingRetained: <redacted>)"
    case .needsSecretForExistingGeneration:
      return "RecoveryServiceOutcome(needsSecretForExistingGeneration: <redacted>)"
    case .destructiveAbandonAvailable:
      return "RecoveryServiceOutcome(destructiveAbandonAvailable)"
    }
  }
  public var debugDescription: String { description }
}

public struct DestructiveAbandonConfirmation: Sendable {
  fileprivate let acknowledged: Bool
  public init(acknowledgedWarning: Bool) { acknowledged = acknowledgedWarning }
}

public final class RecoveryService: @unchecked Sendable {
  private let client: OnboardingHTTPClient
  private let credentials: CredentialRepository
  private let pending: PendingRecoveryRepository
  private let generator: any ControlTokenGenerator

  public init(
    client: OnboardingHTTPClient,
    credentials: CredentialRepository = CredentialRepository(),
    pending: PendingRecoveryRepository = PendingRecoveryRepository(),
    generator: any ControlTokenGenerator = SystemControlTokenGenerator()
  ) {
    self.client = client
    self.credentials = credentials
    self.pending = pending
    self.generator = generator
  }

  public func recover(
    actorID: Int64,
    recoveryID: String,
    secret: RecoverySecret
  ) async throws -> RecoveryServiceOutcome {
    guard actorID > 0, CredentialSyntax.recoveryID(recoveryID) else {
      throw OnboardingClientError.invalidRequest
    }
    return try await withSerializedScope(actorID: actorID) {
      try await self.recoverSerialized(
        actorID: actorID, recoveryID: recoveryID, secret: secret)
    }
  }

  /// Resumes protected pending recovery without requiring the one-time secret.
  /// A sent candidate is always probed first; only a definitive 401 (or an
  /// unsent candidate) asks the caller to collect the secret.
  public func resumePending(actorID: Int64) async throws -> RecoveryServiceOutcome {
    guard actorID > 0 else { throw OnboardingClientError.invalidRequest }
    return try await withSerializedScope(actorID: actorID) {
      guard let record = try self.pending.load(origin: self.client.origin, actorID: actorID) else {
        throw OnboardingClientError.invalidRequest
      }
      if try self.activeAlreadyPromoted(record) {
        return .recovered(try self.cleanupPromoted(record))
      }
      guard record.everSent else {
        return .needsSecretForExistingGeneration(recoveryID: record.recoveryId)
      }
      return try await self.probePendingWithoutSecret(record)
    }
  }

  private func recoverSerialized(
    actorID: Int64,
    recoveryID: String,
    secret: RecoverySecret
  ) async throws -> RecoveryServiceOutcome {
    if let existing = try pending.load(origin: client.origin, actorID: actorID) {
      if try activeAlreadyPromoted(existing) {
        return .recovered(try cleanupPromoted(existing))
      }
      if existing.everSent {
        return try await resolveSent(existing, requestedRecoveryID: recoveryID, secret: secret)
      }
      if existing.recoveryId == recoveryID {
        return try await crossSendBarrier(existing, secret: secret)
      }
      try pending.deleteExact(existing)
    }

    let token = try generator.generate()
    guard CredentialSyntax.lowerHexToken(token) else { throw CredentialStorageError.corrupt }
    let record = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: client.origin,
      actorId: actorID,
      recoveryId: recoveryID,
      pendingControlToken: token,
      everSent: false
    )
    try pending.createUnsent(record)
    if Task.isCancelled {
      try? pending.deleteExact(record)
      throw OnboardingClientError.cancelled
    }
    return try await crossSendBarrier(record, secret: secret)
  }

  public func inspectPending(actorID: Int64) throws -> PendingRecoveryRecord? {
    try pending.load(origin: client.origin, actorID: actorID)
  }

  public func abandonPending(
    actorID: Int64,
    confirmation: DestructiveAbandonConfirmation
  ) async throws {
    try await withSerializedScope(actorID: actorID) {
      if let record = try pending.load(origin: client.origin, actorID: actorID) {
        if record.everSent && !confirmation.acknowledged {
          throw CredentialStorageError.conflict
        }
        try pending.deleteExact(record)
      }
    }
  }

  private func withSerializedScope<Value: Sendable>(
    actorID: Int64,
    operation: () async throws -> Value
  ) async throws -> Value {
    let scope = RecoveryScope(origin: client.origin, actorID: actorID)
    let ownerID = UUID()
    try await acquire(scope, ownerID: ownerID)
    do {
      let value = try await operation()
      await RecoveryScopeSerializer.shared.release(scope, id: ownerID)
      return value
    } catch {
      await RecoveryScopeSerializer.shared.release(scope, id: ownerID)
      throw error
    }
  }

  private func acquire(_ scope: RecoveryScope, ownerID: UUID) async throws {
    try await withTaskCancellationHandler {
      try Task.checkCancellation()
      try await RecoveryScopeSerializer.shared.acquire(scope, id: ownerID)
    } onCancel: {
      Task { await RecoveryScopeSerializer.shared.cancel(scope, id: ownerID) }
    }
  }

  private func resolveSent(
    _ record: PendingRecoveryRecord,
    requestedRecoveryID: String,
    secret: RecoverySecret
  ) async throws -> RecoveryServiceOutcome {
    do {
      switch try await client.probe(token: record.pendingControlToken) {
      case .active(let context):
        return .recovered(try promote(record, context: context, limited: false))
      case .limited:
        return .recovered(try promote(record, context: nil, limited: true))
      case .rateLimited(let seconds):
        return .pendingRetained(.rateLimited(seconds))
      case .unauthorized:
        guard requestedRecoveryID == record.recoveryId else {
          return .needsSecretForExistingGeneration(recoveryID: record.recoveryId)
        }
        do {
          let context = try await client.consumeRecovery(
            recoveryID: record.recoveryId,
            secret: secret,
            replacementControlToken: record.pendingControlToken
          )
          return .recovered(try promote(record, context: context, limited: false))
        } catch OnboardingClientError.api(403, .credentialInvalid, _) {
          // Exact 401 probe → 403 retry sequence. State remains.
          return .destructiveAbandonAvailable
        } catch {
          return try retainedOutcome(for: error)
        }
      }
    } catch {
      return try retainedOutcome(for: error)
    }
  }

  private func probePendingWithoutSecret(_ record: PendingRecoveryRecord) async throws
    -> RecoveryServiceOutcome
  {
    do {
      switch try await client.probe(token: record.pendingControlToken) {
      case .active(let context):
        return .recovered(try promote(record, context: context, limited: false))
      case .limited:
        return .recovered(try promote(record, context: nil, limited: true))
      case .rateLimited(let seconds):
        return .pendingRetained(.rateLimited(seconds))
      case .unauthorized:
        return .needsSecretForExistingGeneration(recoveryID: record.recoveryId)
      }
    } catch {
      return try retainedOutcome(for: error)
    }
  }

  private func crossSendBarrier(
    _ unsent: PendingRecoveryRecord,
    secret: RecoverySecret
  ) async throws -> RecoveryServiceOutcome {
    let sent: PendingRecoveryRecord
    do { sent = try pending.markSent(unsent) } catch { throw OnboardingClientError.storage }
    // The transport's send entry point is reachable only after markSent's
    // exact read-back verification returned the true record.
    do {
      let context = try await client.consumeRecovery(
        recoveryID: sent.recoveryId,
        secret: secret,
        replacementControlToken: sent.pendingControlToken
      )
      return .recovered(try promote(sent, context: context, limited: false))
    } catch {
      return try retainedOutcome(for: error)
    }
  }

  private func retainedOutcome(for error: Error) throws -> RecoveryServiceOutcome {
    if case OnboardingClientError.api(_, .credentialInvalid, _) = error {
      return .pendingRetained(.credentialRejected)
    }
    if case OnboardingClientError.api(_, .tooManyAttempts, let retry) = error, let retry {
      return .pendingRetained(.rateLimited(retry))
    }
    if error is CancellationError { return .pendingRetained(.ambiguousResponse) }
    if let clientError = error as? OnboardingClientError {
      switch clientError {
      case .transport, .cancelled, .invalidResponse, .responseTooLarge,
        .redirectRejected, .api:
        return .pendingRetained(.ambiguousResponse)
      default: throw clientError
      }
    }
    return .pendingRetained(.ambiguousResponse)
  }

  private func activeAlreadyPromoted(_ record: PendingRecoveryRecord) throws -> Bool {
    guard let bundle = try credentials.loadBundleWithoutMigration(),
      bundle.control?.controlToken == record.pendingControlToken
    else { return false }
    guard record.everSent else { throw CredentialStorageError.conflict }
    guard bundle.coordinatorOrigin == record.canonicalCoordinatorOrigin,
      bundle.control?.actorId == record.actorId,
      bundle.recovery?.actorId == record.actorId,
      bundle.recovery?.recoveryId == record.recoveryId
    else { throw CredentialStorageError.conflict }
    return true
  }

  private func cleanupPromoted(_ record: PendingRecoveryRecord) throws -> RecoveredCredential {
    guard try activeAlreadyPromoted(record),
      let bundle = try credentials.loadBundleWithoutMigration()
    else {
      throw CredentialStorageError.verificationFailed
    }
    try pending.deleteExact(record)
    return RecoveredCredential(
      bundle: bundle, hasLimitedContext: bundle.control?.contextStrength == .limited)
  }

  private func promote(
    _ record: PendingRecoveryRecord,
    context: ActorCredentialContext?,
    limited: Bool
  ) throws -> RecoveredCredential {
    guard context == nil || context?.actorId == record.actorId else {
      throw OnboardingClientError.invalidResponse
    }
    let bundle: CredentialBundle
    do {
      bundle = try credentials.mutateBundle { bundle in
        guard
          bundle.coordinatorOrigin == nil
            || bundle.coordinatorOrigin == record.canonicalCoordinatorOrigin
        else { throw CredentialStorageError.conflict }
        if bundle.control?.controlToken == record.pendingControlToken {
          guard bundle.control?.actorId == record.actorId,
            bundle.recovery?.actorId == record.actorId,
            bundle.recovery?.recoveryId == record.recoveryId
          else { throw CredentialStorageError.conflict }
        }
        let priorControl = bundle.control
        let orbitID =
          context?.orbitId
          ?? (priorControl?.actorId == record.actorId ? priorControl?.orbitId : nil)
        let role =
          context?.role
          ?? (priorControl?.actorId == record.actorId ? priorControl?.role : nil)
        bundle.coordinatorOrigin = record.canonicalCoordinatorOrigin
        bundle.control = ControlCapability(
          actorId: record.actorId,
          orbitId: orbitID,
          role: role,
          controlToken: record.pendingControlToken,
          contextStrength: limited ? .limited : .active
        )
        bundle.recovery = RecoveryMetadata(
          actorId: record.actorId, recoveryId: record.recoveryId)
      }
    } catch {
      throw OnboardingClientError.storage
    }
    do { try pending.deleteExact(record) } catch { throw OnboardingClientError.storage }
    return RecoveredCredential(bundle: bundle, hasLimitedContext: limited)
  }
}
