import Foundation

@testable import NodeCore

final class MemoryProtectedStore: ProtectedStore, @unchecked Sendable {
  enum Kind: String { case read, add, update, delete }
  struct Operation {
    let kind: Kind
    let key: ProtectedStoreKey
  }

  private let lock = NSLock()
  private var storage: [ProtectedStoreKey: Data] = [:]
  private var log: [Operation] = []
  var failure: ((Kind, ProtectedStoreKey, Int) -> Error?)?
  var updateTransform: ((Data, ProtectedStoreKey) -> Data)?
  var failReadAfterEveryUpdate = false
  private var failNextRead = false
  private var counts: [Kind: Int] = [:]

  func read(_ key: ProtectedStoreKey) throws -> Data? {
    try perform(.read, key: key) { storage[key] }
  }
  func add(_ data: Data, for key: ProtectedStoreKey) throws {
    try perform(.add, key: key) {
      guard storage[key] == nil else { throw ProtectedStoreFailure.duplicate }
      storage[key] = data
    }
  }
  func update(_ data: Data, for key: ProtectedStoreKey) throws {
    try perform(.update, key: key) {
      guard storage[key] != nil else { throw ProtectedStoreFailure.unavailable }
      storage[key] = updateTransform?(data, key) ?? data
    }
  }
  func delete(_ key: ProtectedStoreKey) throws {
    _ = try perform(.delete, key: key) { storage.removeValue(forKey: key) }
  }

  func seed(_ data: Data, key: ProtectedStoreKey) {
    lock.withLock { storage[key] = data }
  }
  func data(_ key: ProtectedStoreKey) -> Data? { lock.withLock { storage[key] } }
  func allData() -> [Data] { lock.withLock { Array(storage.values) } }
  func operations() -> [Operation] { lock.withLock { log } }
  func resetLog() {
    lock.withLock {
      log.removeAll()
      counts.removeAll()
    }
  }

  private func perform<T>(_ kind: Kind, key: ProtectedStoreKey, body: () throws -> T) throws -> T {
    lock.lock()
    defer { lock.unlock() }
    let count = counts[kind, default: 0] + 1
    counts[kind] = count
    log.append(Operation(kind: kind, key: key))
    if let error = failure?(kind, key, count) { throw error }
    if kind == .read, failNextRead {
      failNextRead = false
      throw ProtectedStoreFailure.unavailable
    }
    let result = try body()
    if kind == .update, failReadAfterEveryUpdate { failNextRead = true }
    return result
  }
}

final class MemoryCredentialFiles: CredentialFileAccess, @unchecked Sendable {
  private let lock = NSLock()
  var data: Data?
  private(set) var events: [String] = []
  init(data: Data? = nil) { self.data = data }
  func readLegacy(at url: URL) throws -> Data? {
    lock.withLock {
      events.append("read")
      return data
    }
  }
  func deleteLegacy(at url: URL) throws {
    lock.withLock {
      events.append("delete")
      data = nil
    }
  }
}

final class ScriptedTransport: OnboardingHTTPTransport, @unchecked Sendable {
  typealias Handler = @Sendable (URLRequest, Int) async throws -> HTTPTransportResponse
  private let handler: Handler
  private let lock = NSLock()
  private var captured: [URLRequest] = []
  init(_ handler: @escaping Handler) { self.handler = handler }
  func send(_ request: URLRequest, maximumResponseBytes: Int) async throws -> HTTPTransportResponse
  {
    lock.withLock { captured.append(request) }
    return try await handler(request, maximumResponseBytes)
  }
  func requests() -> [URLRequest] { lock.withLock { captured } }
}

func testHTTPResponse(
  request: URLRequest,
  status: Int,
  json: String,
  url: URL? = nil,
  headers: [String: String] = [:]
) -> HTTPTransportResponse {
  var allHeaders = ["Content-Type": "application/json; charset=utf-8"]
  for (key, value) in headers { allHeaders[key] = value }
  let response = HTTPURLResponse(
    url: url ?? request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: allHeaders
  )!
  return HTTPTransportResponse(data: Data(json.utf8), response: response)
}

struct FixedTokenGenerator: ControlTokenGenerator, Sendable {
  let token: String
  func generate() throws -> String { token }
}

actor RequestBarrier {
  private var arrivals = 0
  private var arrivalWaiters: [(Int, CheckedContinuation<Void, Never>)] = []
  private var releaseWaiters: [CheckedContinuation<Void, Never>] = []
  private var released = false

  func arriveAndWait() async {
    arrivals += 1
    let ready = arrivalWaiters.filter { arrivals >= $0.0 }
    arrivalWaiters.removeAll { arrivals >= $0.0 }
    for waiter in ready { waiter.1.resume() }
    guard !released else { return }
    await withCheckedContinuation { releaseWaiters.append($0) }
  }

  func waitForArrivals(_ count: Int) async {
    guard arrivals < count else { return }
    await withCheckedContinuation { arrivalWaiters.append((count, $0)) }
  }

  func releaseAll() {
    released = true
    let waiters = releaseWaiters
    releaseWaiters.removeAll()
    for waiter in waiters { waiter.resume() }
  }
}

actor TestCounter {
  private(set) var value = 0
  func increment() { value += 1 }
}

final class MemoryPasteboard: PasteboardAccess, @unchecked Sendable {
  private let lock = NSLock()
  private var value: String?
  private var count = 0
  private var writes = 0
  private var clears = 0
  private var clearFailuresRemaining = 0

  var changeCount: Int { lock.withLock { count } }
  var writeCount: Int { lock.withLock { writes } }
  var clearCount: Int { lock.withLock { clears } }
  func string() -> String? { lock.withLock { value } }
  func write(_ value: String) throws -> Int {
    lock.withLock {
      self.value = value
      count += 1
      writes += 1
      return count
    }
  }
  func clearIfUnchanged(expectedChangeCount: Int, expectedPayload: String) throws -> Bool {
    try lock.withLock {
      if clearFailuresRemaining > 0 {
        clearFailuresRemaining -= 1
        throw RecoveryExportError.writeFailed
      }
      guard count == expectedChangeCount, value == expectedPayload else { return false }
      value = nil
      count += 1
      clears += 1
      return true
    }
  }
  func replaceExternally(with value: String) {
    lock.withLock {
      self.value = value
      count += 1
    }
  }
  func failNextClears(_ count: Int) {
    lock.withLock { clearFailuresRemaining = count }
  }
}

actor ManualPasteboardSleeper: PasteboardLeaseSleeper {
  private var sleepers: [(Duration, CheckedContinuation<Void, Error>)] = []
  private var entryWaiters: [(Int, CheckedContinuation<Void, Never>)] = []
  private var entries = 0

  var entryCount: Int { entries }

  func sleep(for duration: Duration) async throws {
    entries += 1
    let ready = entryWaiters.filter { entries >= $0.0 }
    entryWaiters.removeAll { entries >= $0.0 }
    for waiter in ready { waiter.1.resume() }
    try await withCheckedThrowingContinuation { sleepers.append((duration, $0)) }
  }

  func waitForEntries(_ count: Int) async {
    guard entries < count else { return }
    await withCheckedContinuation { entryWaiters.append((count, $0)) }
  }

  func wakeAll() {
    let continuations = sleepers
    sleepers.removeAll()
    for continuation in continuations { continuation.1.resume() }
  }

  func wakeNext() {
    guard !sleepers.isEmpty else { return }
    sleepers.removeFirst().1.resume()
  }

  func wake(duration: Duration) {
    guard let index = sleepers.firstIndex(where: { $0.0 == duration }) else { return }
    sleepers.remove(at: index).1.resume()
  }
}

extension NSLock {
  fileprivate func withLock<T>(_ body: () throws -> T) rethrows -> T {
    lock()
    defer { unlock() }
    return try body()
  }
}

let testNodeToken = String(repeating: "a", count: 64)
let testControlToken = String(repeating: "b", count: 64)
let testPendingToken = String(repeating: "c", count: 64)
let testRecoveryID = "rec_" + String(repeating: "d", count: 32)
let testRecoverySecret = "ABCDEFGHJKMNPQRSTVWXYZ23456"

func encodedLegacy(_ credentials: NodeCredentials) -> Data {
  try! JSONEncoder().encode(credentials)
}

func encodedBundle(_ bundle: CredentialBundle) -> Data {
  let encoder = JSONEncoder()
  encoder.outputFormatting = [.sortedKeys]
  return try! encoder.encode(bundle)
}

private struct PreviousControlCapabilityFixture: Encodable {
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

private struct PreviousCredentialBundleFixture: Encodable {
  let version = 1
  let coordinatorOrigin: CoordinatorOrigin?
  let node: NodeCapability?
  let control: PreviousControlCapabilityFixture?
  let recovery: RecoveryMetadata?

  enum CodingKeys: String, CodingKey {
    case version
    case coordinatorOrigin = "coordinator_origin"
    case node, control, recovery
  }
}

func encodedPreviousBundle(
  coordinatorOrigin: CoordinatorOrigin?,
  node: NodeCapability?,
  actorID: Int64,
  orbitID: Int64?,
  role: CredentialRole?,
  controlToken: String,
  recovery: RecoveryMetadata?
) -> Data {
  let fixture = PreviousCredentialBundleFixture(
    coordinatorOrigin: coordinatorOrigin,
    node: node,
    control: PreviousControlCapabilityFixture(
      actorId: actorID, orbitId: orbitID, role: role, controlToken: controlToken),
    recovery: recovery)
  let encoder = JSONEncoder()
  encoder.outputFormatting = [.sortedKeys]
  return try! encoder.encode(fixture)
}
