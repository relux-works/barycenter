import AppKit
import Darwin
import Foundation

public enum RecoveryExportError: Error, Equatable, LocalizedError {
  case invalidMaterial
  case invalidLeaseDuration
  case writeFailed

  public var errorDescription: String? { "Recovery material could not be exported." }
}

enum RecoveryFileWriteResult: Equatable {
  case written(Int)
  case interrupted
  case failed
}

protocol RecoveryExportFileOperations: Sendable {
  func openExclusiveNoFollow(_ url: URL) -> Int32
  func write(_ descriptor: Int32, buffer: UnsafeRawBufferPointer) -> RecoveryFileWriteResult
  func synchronize(_ descriptor: Int32) -> Bool
  func close(_ descriptor: Int32) -> Bool
  func truncate(_ descriptor: Int32) -> Bool
  func remove(_ url: URL) -> Bool
}

struct SystemRecoveryExportFileOperations: RecoveryExportFileOperations, Sendable {
  func openExclusiveNoFollow(_ url: URL) -> Int32 {
    url.withUnsafeFileSystemRepresentation { path in
      guard let path else { return -1 }
      return Darwin.open(path, O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW, S_IRUSR | S_IWUSR)
    }
  }

  func write(_ descriptor: Int32, buffer: UnsafeRawBufferPointer) -> RecoveryFileWriteResult {
    guard let baseAddress = buffer.baseAddress else { return .written(0) }
    let count = Darwin.write(descriptor, baseAddress, buffer.count)
    if count < 0 { return errno == EINTR ? .interrupted : .failed }
    return .written(count)
  }

  func synchronize(_ descriptor: Int32) -> Bool { Darwin.fsync(descriptor) == 0 }
  func close(_ descriptor: Int32) -> Bool { Darwin.close(descriptor) == 0 }
  func truncate(_ descriptor: Int32) -> Bool { Darwin.ftruncate(descriptor, 0) == 0 }
  func remove(_ url: URL) -> Bool {
    url.withUnsafeFileSystemRepresentation { path in
      guard let path else { return false }
      return Darwin.unlink(path) == 0
    }
  }
}

public enum RecoveryExportHelper {
  /// Explicitly reveals all three required values. No export is produced by
  /// displaying material; callers must invoke this action deliberately.
  public static func payload(for material: OneTimeRecoveryMaterial) throws -> String {
    guard material.actorId > 0, CredentialSyntax.recoveryID(material.recoveryId) else {
      throw RecoveryExportError.invalidMaterial
    }
    let object: [String: Any] = [
      "actor_id": material.actorId,
      "recovery_id": material.recoveryId,
      "recovery_secret": material.secret.reveal(),
    ]
    guard JSONSerialization.isValidJSONObject(object),
      let data = try? JSONSerialization.data(withJSONObject: object, options: [.sortedKeys]),
      let value = String(data: data, encoding: .utf8)
    else {
      throw RecoveryExportError.invalidMaterial
    }
    return value
  }

  /// Writes directly to the destination selected by the user. Atomic/temp,
  /// autosave, recent-document, and sidecar behavior are intentionally absent.
  public static func save(_ material: OneTimeRecoveryMaterial, to selectedURL: URL) throws {
    try save(material, to: selectedURL, fileOperations: SystemRecoveryExportFileOperations())
  }

  static func save(
    _ material: OneTimeRecoveryMaterial,
    to selectedURL: URL,
    fileOperations: any RecoveryExportFileOperations
  ) throws {
    let data = Data(try payload(for: material).utf8)
    let descriptor = fileOperations.openExclusiveNoFollow(selectedURL)
    guard descriptor >= 0 else { throw RecoveryExportError.writeFailed }
    var descriptorOwned = true

    func cleanUpFailure() {
      if descriptorOwned {
        _ = fileOperations.truncate(descriptor)
        descriptorOwned = false
        _ = fileOperations.close(descriptor)
      }
      _ = fileOperations.remove(selectedURL)
    }

    var wroteAll = true
    data.withUnsafeBytes { rawBuffer in
      var offset = 0
      while offset < rawBuffer.count {
        let remaining = UnsafeRawBufferPointer(rebasing: rawBuffer[offset...])
        switch fileOperations.write(descriptor, buffer: remaining) {
        case .interrupted:
          continue
        case .failed:
          wroteAll = false
          return
        case .written(let count):
          guard count > 0, count <= remaining.count else {
            wroteAll = false
            return
          }
          offset += count
        }
      }
    }
    guard wroteAll else {
      cleanUpFailure()
      throw RecoveryExportError.writeFailed
    }
    guard fileOperations.synchronize(descriptor) else {
      cleanUpFailure()
      throw RecoveryExportError.writeFailed
    }

    // Descriptor ownership is unspecified after close returns, including on
    // failure. Call it exactly once and never retry or use the descriptor.
    descriptorOwned = false
    guard fileOperations.close(descriptor) else {
      _ = fileOperations.remove(selectedURL)
      throw RecoveryExportError.writeFailed
    }
  }
}

public protocol PasteboardAccess: Sendable {
  @discardableResult func write(_ value: String) throws -> Int
  @discardableResult func clearIfUnchanged(
    expectedChangeCount: Int,
    expectedPayload: String
  ) throws -> Bool
}

public final class SystemPasteboardAccess: PasteboardAccess, @unchecked Sendable {
  private let pasteboard: NSPasteboard
  public init(pasteboard: NSPasteboard = .general) { self.pasteboard = pasteboard }
  public func write(_ value: String) throws -> Int {
    try onMain {
      self.pasteboard.clearContents()
      guard self.pasteboard.setString(value, forType: .string) else {
        throw RecoveryExportError.writeFailed
      }
      return self.pasteboard.changeCount
    }
  }
  public func clearIfUnchanged(
    expectedChangeCount: Int,
    expectedPayload: String
  ) throws -> Bool {
    onMain {
      guard self.pasteboard.changeCount == expectedChangeCount,
        self.pasteboard.string(forType: .string) == expectedPayload
      else { return false }
      self.pasteboard.clearContents()
      return true
    }
  }

  private func onMain<T>(_ operation: @escaping () throws -> T) rethrows -> T {
    if Thread.isMainThread { return try operation() }
    return try DispatchQueue.main.sync(execute: operation)
  }
}

public protocol PasteboardLeaseSleeper: Sendable {
  func sleep(for duration: Duration) async throws
}

public struct SystemPasteboardLeaseSleeper: PasteboardLeaseSleeper, Sendable {
  public init() {}
  public func sleep(for duration: Duration) async throws { try await Task.sleep(for: duration) }
}

public enum RecoveryPasteboardCleanupStatus: Equatable, Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  case idle
  case leased
  case automaticCleanupFailed

  public var description: String {
    switch self {
    case .idle: return "RecoveryPasteboardCleanupStatus(idle)"
    case .leased: return "RecoveryPasteboardCleanupStatus(leased)"
    case .automaticCleanupFailed:
      return "RecoveryPasteboardCleanupStatus(automaticCleanupFailed)"
    }
  }
  public var debugDescription: String { description }
}

/// Bounded clipboard lease. Expiry and explicit clear erase only the exact
/// leased payload at the same change count, so newer clipboard data survives.
public actor RecoveryPasteboardLease {
  public static let maximumTimeToLive: Duration = .seconds(300)
  static let maximumClearRetryDelay: Duration = .seconds(60)
  static let maximumAutomaticClearRetries = 3
  private static let defaultClearRetryDelays: [Duration] = [
    .seconds(1), .seconds(5), .seconds(30),
  ]

  private struct Lease {
    let id: UUID
    let changeCount: Int
    let payload: String
    var nextRetryIndex: Int
  }

  private let pasteboard: any PasteboardAccess
  private let sleeper: any PasteboardLeaseSleeper
  private let clearRetryDelays: [Duration]
  private var lease: Lease?
  private var expiryTask: Task<Void, Never>?
  public private(set) var cleanupStatus: RecoveryPasteboardCleanupStatus = .idle
  private var statusContinuations:
    [UUID: AsyncStream<RecoveryPasteboardCleanupStatus>.Continuation] = [:]

  public init(
    pasteboard: any PasteboardAccess = SystemPasteboardAccess(),
    sleeper: any PasteboardLeaseSleeper = SystemPasteboardLeaseSleeper()
  ) {
    self.pasteboard = pasteboard
    self.sleeper = sleeper
    clearRetryDelays = Self.defaultClearRetryDelays
  }

  init(
    pasteboard: any PasteboardAccess,
    sleeper: any PasteboardLeaseSleeper,
    clearRetryDelays: [Duration]
  ) {
    self.pasteboard = pasteboard
    self.sleeper = sleeper
    self.clearRetryDelays = Array(
      clearRetryDelays.prefix(Self.maximumAutomaticClearRetries)
    ).map { delay in
      max(.nanoseconds(1), min(delay, Self.maximumClearRetryDelay))
    }
  }

  deinit {
    expiryTask?.cancel()
    for continuation in statusContinuations.values { continuation.finish() }
  }

  /// Emits the current generic cleanup state immediately and every subsequent
  /// state change. Values never contain lease or recovery material.
  public func cleanupStatusUpdates() -> AsyncStream<RecoveryPasteboardCleanupStatus> {
    let observerID = UUID()
    let pair = AsyncStream<RecoveryPasteboardCleanupStatus>.makeStream(
      bufferingPolicy: .bufferingNewest(1))
    statusContinuations[observerID] = pair.continuation
    pair.continuation.yield(cleanupStatus)
    pair.continuation.onTermination = { [weak self] _ in
      Task { await self?.removeStatusContinuation(observerID) }
    }
    return pair.stream
  }

  public func copyExplicitly(
    _ material: OneTimeRecoveryMaterial,
    timeToLive: Duration = .seconds(120)
  ) throws {
    guard timeToLive > .zero, timeToLive <= Self.maximumTimeToLive else {
      throw RecoveryExportError.invalidLeaseDuration
    }
    let payload = try RecoveryExportHelper.payload(for: material)
    let changeCount = try pasteboard.write(payload)
    let id = UUID()
    expiryTask?.cancel()
    lease = Lease(id: id, changeCount: changeCount, payload: payload, nextRetryIndex: 0)
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
    let resolved: Bool
    do {
      // `false` is also terminal: the same atomic operation proved that a
      // newer clipboard value replaced the lease.
      resolved = try pasteboard.clearIfUnchanged(
        expectedChangeCount: current.changeCount,
        expectedPayload: current.payload)
    } catch {
      scheduleRetry(for: current)
      throw RecoveryExportError.writeFailed
    }
    _ = resolved
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
    lease = nil
    expiryTask?.cancel()
    expiryTask = nil
    setCleanupStatus(.idle)
  }

  private func setCleanupStatus(_ status: RecoveryPasteboardCleanupStatus) {
    guard cleanupStatus != status else { return }
    cleanupStatus = status
    for continuation in statusContinuations.values { continuation.yield(status) }
  }

  private func removeStatusContinuation(_ id: UUID) {
    statusContinuations.removeValue(forKey: id)
  }
}
