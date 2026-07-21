import Foundation

public enum DeviceInvitationPasteboardError: Error, Equatable, LocalizedError {
  case invalidCode
  case invalidLeaseDuration
  case writeFailed

  public var errorDescription: String? {
    "The device invitation could not be copied safely."
  }
}

public enum DeviceInvitationPasteboardCleanupStatus: Equatable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  case idle
  case leased
  case automaticCleanupFailed

  public var description: String {
    switch self {
    case .idle: return "DeviceInvitationPasteboardCleanupStatus(idle)"
    case .leased: return "DeviceInvitationPasteboardCleanupStatus(leased)"
    case .automaticCleanupFailed:
      return "DeviceInvitationPasteboardCleanupStatus(automaticCleanupFailed)"
    }
  }

  public var debugDescription: String { description }
}

/// A bounded lease for one canonical device-invitation code. Automatic and
/// explicit cleanup clear only the exact value written by this lease at the
/// same pasteboard change count, so later user clipboard data always wins.
public actor DeviceInvitationPasteboardLease {
  public static let maximumTimeToLive: Duration = .seconds(300)
  static let maximumClearRetryDelay: Duration = .seconds(60)
  static let maximumAutomaticClearRetries = 3
  private static let defaultClearRetryDelays: [Duration] = [
    .seconds(1), .seconds(5), .seconds(30),
  ]

  private struct Lease {
    let id: UUID
    var nextRetryIndex: Int
  }

  private nonisolated let pasteboardState: DeviceInvitationPasteboardState
  private let sleeper: any PasteboardLeaseSleeper
  private let clearRetryDelays: [Duration]
  private var lease: Lease?
  private var expiryTask: Task<Void, Never>?
  public private(set) var cleanupStatus: DeviceInvitationPasteboardCleanupStatus = .idle
  private var statusContinuations:
    [UUID: AsyncStream<DeviceInvitationPasteboardCleanupStatus>.Continuation] = [:]

  public init(
    pasteboard: any PasteboardAccess = SystemPasteboardAccess(),
    sleeper: any PasteboardLeaseSleeper = SystemPasteboardLeaseSleeper()
  ) {
    pasteboardState = DeviceInvitationPasteboardState(pasteboard: pasteboard)
    self.sleeper = sleeper
    clearRetryDelays = Self.defaultClearRetryDelays
  }

  init(
    pasteboard: any PasteboardAccess,
    sleeper: any PasteboardLeaseSleeper,
    clearRetryDelays: [Duration]
  ) {
    pasteboardState = DeviceInvitationPasteboardState(pasteboard: pasteboard)
    self.sleeper = sleeper
    self.clearRetryDelays = Array(
      clearRetryDelays.prefix(Self.maximumAutomaticClearRetries)
    ).map { delay in
      max(.nanoseconds(1), min(delay, Self.maximumClearRetryDelay))
    }
  }

  deinit {
    expiryTask?.cancel()
    try? pasteboardState.clearSynchronously()
    for continuation in statusContinuations.values { continuation.finish() }
  }

  /// Used by process termination paths that cannot await an actor hop. It uses
  /// the same exact change-count and payload check as ordinary cleanup.
  public nonisolated func clearSynchronouslyForTermination() throws {
    try pasteboardState.clearSynchronously()
  }

  /// Emits only generic cleanup state; invitation payloads never enter the
  /// stream, descriptions, errors, or logs.
  public func cleanupStatusUpdates() -> AsyncStream<DeviceInvitationPasteboardCleanupStatus> {
    let observerID = UUID()
    let pair = AsyncStream<DeviceInvitationPasteboardCleanupStatus>.makeStream(
      bufferingPolicy: .bufferingNewest(1))
    statusContinuations[observerID] = pair.continuation
    pair.continuation.yield(cleanupStatus)
    pair.continuation.onTermination = { [weak self] _ in
      Task { await self?.removeStatusContinuation(observerID) }
    }
    return pair.stream
  }

  public func copyExplicitly(
    _ code: String,
    timeToLive: Duration = .seconds(120)
  ) throws {
    guard CredentialSyntax.canonicalHumanCode(code) else {
      throw DeviceInvitationPasteboardError.invalidCode
    }
    guard timeToLive > .zero, timeToLive <= Self.maximumTimeToLive else {
      throw DeviceInvitationPasteboardError.invalidLeaseDuration
    }
    let id = UUID()
    do {
      _ = try pasteboardState.write(code, id: id)
    } catch {
      throw DeviceInvitationPasteboardError.writeFailed
    }
    expiryTask?.cancel()
    lease = Lease(id: id, nextRetryIndex: 0)
    setCleanupStatus(.leased)
    scheduleClear(id: id, after: timeToLive)
  }

  public func clearExplicitly() throws {
    expiryTask?.cancel()
    expiryTask = nil
    guard let id = lease?.id else { return }
    try clearIfStillOwned(id: id)
  }

  private func expire(id: UUID) {
    guard lease?.id == id else { return }
    expiryTask = nil
    try? clearIfStillOwned(id: id)
  }

  private func clearIfStillOwned(id: UUID) throws {
    guard let current = lease, current.id == id else { return }
    do {
      // A resolved external replacement is terminal too: the same atomic
      // operation proved that a newer clipboard payload replaced this lease.
      _ = try pasteboardState.clearIfOwned(id: id)
    } catch {
      scheduleRetry(for: current)
      throw DeviceInvitationPasteboardError.writeFailed
    }
    retire(id: id)
  }

  private func scheduleRetry(for current: Lease) {
    guard lease?.id == current.id,
      current.nextRetryIndex < clearRetryDelays.count
    else {
      expiryTask = nil
      if lease?.id == current.id { setCleanupStatus(.automaticCleanupFailed) }
      return
    }
    let retryIndex = current.nextRetryIndex
    lease?.nextRetryIndex += 1
    scheduleClear(id: current.id, after: clearRetryDelays[retryIndex])
  }

  private func scheduleClear(id: UUID, after delay: Duration) {
    expiryTask?.cancel()
    expiryTask = Task { [weak self, sleeper] in
      do { try await sleeper.sleep(for: delay) } catch { return }
      await self?.expire(id: id)
    }
  }

  private func retire(id: UUID) {
    guard lease?.id == id else { return }
    pasteboardState.retire(id: id)
    lease = nil
    expiryTask?.cancel()
    expiryTask = nil
    setCleanupStatus(.idle)
  }

  private func setCleanupStatus(_ status: DeviceInvitationPasteboardCleanupStatus) {
    guard cleanupStatus != status else { return }
    cleanupStatus = status
    for continuation in statusContinuations.values { continuation.yield(status) }
  }

  private func removeStatusContinuation(_ id: UUID) {
    statusContinuations.removeValue(forKey: id)
  }
}

private final class DeviceInvitationPasteboardState: @unchecked Sendable {
  private struct Entry {
    let id: UUID
    let changeCount: Int
    let payload: String
  }

  private let lock = NSLock()
  private let pasteboard: any PasteboardAccess
  private var entry: Entry?
  private var acceptsWrites = true

  init(pasteboard: any PasteboardAccess) {
    self.pasteboard = pasteboard
  }

  func write(_ payload: String, id: UUID) throws -> Int {
    lock.lock()
    defer { lock.unlock() }
    guard acceptsWrites else {
      throw DeviceInvitationPasteboardError.writeFailed
    }
    let changeCount = try pasteboard.write(payload)
    entry = Entry(id: id, changeCount: changeCount, payload: payload)
    return changeCount
  }

  func clearIfOwned(id: UUID) throws -> Bool {
    lock.lock()
    defer { lock.unlock() }
    guard let current = entry, current.id == id else { return false }
    _ = try pasteboard.clearIfUnchanged(
      expectedChangeCount: current.changeCount,
      expectedPayload: current.payload)
    entry = nil
    return true
  }

  func clearSynchronously() throws {
    lock.lock()
    defer { lock.unlock() }
    // Termination cleanup is a one-way boundary. Closing writes while holding
    // the same lock as `write` ensures a queued copy cannot land after the
    // process lifecycle has synchronously cleared the pasteboard.
    acceptsWrites = false
    guard let current = entry else { return }
    _ = try pasteboard.clearIfUnchanged(
      expectedChangeCount: current.changeCount,
      expectedPayload: current.payload)
    entry = nil
  }

  func retire(id: UUID) {
    lock.lock()
    defer { lock.unlock() }
    if entry?.id == id { entry = nil }
  }
}
