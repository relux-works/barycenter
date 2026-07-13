import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct RecoveryExportTests {
  @Test func explicitExportContainsExactlyThreeCanonicalFieldsAndSaveIsDirect() throws {
    let entered = "abcd-efgh-jkmn-pqrs-tvwx-yz23-456"
    let material = OneTimeRecoveryMaterial(
      actorId: 9, recoveryId: testRecoveryID,
      secret: try RecoverySecret(validated: entered)
    )
    let payload = try RecoveryExportHelper.payload(for: material)
    let object = try #require(
      JSONSerialization.jsonObject(with: Data(payload.utf8)) as? [String: Any]
    )
    #expect(Set(object.keys) == ["actor_id", "recovery_id", "recovery_secret"])
    #expect(object["actor_id"] as? Int == 9)
    #expect(object["recovery_id"] as? String == testRecoveryID)
    #expect(object["recovery_secret"] as? String == testRecoverySecret)

    let selected = URL(fileURLWithPath: "/explicit-user-selection/recovery.json")
    let fileOperations = ScriptedRecoveryFileOperations(
      writeResults: [.interrupted, .written(3)])
    try RecoveryExportHelper.save(
      material, to: selected, fileOperations: fileOperations)
    #expect(String(decoding: fileOperations.writtenData, as: UTF8.self) == payload)
    #expect(fileOperations.openedURLs == [selected])
    #expect(fileOperations.events.first == "open")
    #expect(fileOperations.events.suffix(2) == ["sync", "close"])
    #expect(!fileOperations.events.contains("truncate"))
    #expect(!fileOperations.events.contains("remove"))
  }

  @Test func directSaveRequiresWriteSyncAndCheckedCloseAndCleansEveryFailure() throws {
    let material = try recoveryMaterial()
    let selected = URL(fileURLWithPath: "/explicit-user-selection/recovery.json")

    let openFailure = ScriptedRecoveryFileOperations(openResult: -1)
    #expect(throws: RecoveryExportError.writeFailed) {
      try RecoveryExportHelper.save(
        material, to: selected, fileOperations: openFailure)
    }
    #expect(openFailure.events == ["open"])

    for operations in [
      ScriptedRecoveryFileOperations(writeResults: [.written(0)]),
      ScriptedRecoveryFileOperations(writeResults: [.failed]),
    ] {
      #expect(throws: RecoveryExportError.writeFailed) {
        try RecoveryExportHelper.save(
          material, to: selected, fileOperations: operations)
      }
      #expect(operations.events.suffix(3) == ["truncate", "close", "remove"])
      #expect(operations.events.filter { $0 == "close" }.count == 1)
      #expect(!operations.events.contains("sync"))
    }

    let syncFailure = ScriptedRecoveryFileOperations(synchronizeResult: false)
    #expect(throws: RecoveryExportError.writeFailed) {
      try RecoveryExportHelper.save(
        material, to: selected, fileOperations: syncFailure)
    }
    #expect(syncFailure.events.suffix(4) == ["sync", "truncate", "close", "remove"])

    let closeFailure = ScriptedRecoveryFileOperations(closeResult: false)
    #expect(throws: RecoveryExportError.writeFailed) {
      try RecoveryExportHelper.save(
        material, to: selected, fileOperations: closeFailure)
    }
    #expect(closeFailure.events.suffix(3) == ["sync", "close", "remove"])
    #expect(!closeFailure.events.contains("truncate"))
    #expect(closeFailure.events.filter { $0 == "close" }.count == 1)

    let cleanupFailure = ScriptedRecoveryFileOperations(
      writeResults: [.failed], closeResult: false, truncateResult: false,
      removeResult: false)
    #expect(throws: RecoveryExportError.writeFailed) {
      try RecoveryExportHelper.save(
        material, to: selected, fileOperations: cleanupFailure)
    }
    #expect(cleanupFailure.events.suffix(3) == ["truncate", "close", "remove"])
    #expect(!String(describing: RecoveryExportError.writeFailed).contains(testRecoverySecret))
    #expect(!selected.lastPathComponent.contains(testRecoverySecret))
    #expect(cleanupFailure.openedURLs == [selected])
  }

  @Test func copyIsExplicitAndExpiryClearsOnlyTheOwnedPayload() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(pasteboard: pasteboard, sleeper: sleeper)
    let material = try recoveryMaterial()
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.writeCount == 0)
    #expect(pasteboard.string() == nil)

    try await lease.copyExplicitly(material, timeToLive: .seconds(30))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    #expect(pasteboard.writeCount == 1)
    #expect(pasteboard.string() == (try RecoveryExportHelper.payload(for: material)))
    await sleeper.wakeAll()
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
  }

  @Test func newerClipboardDataSurvivesTTLAndExplicitClearIsIdempotent() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(pasteboard: pasteboard, sleeper: sleeper)
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(30))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    pasteboard.replaceExternally(with: "new user clipboard value")
    await sleeper.wakeAll()
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == "new user clipboard value")
    #expect(pasteboard.clearCount == 0)

    let secondSleeper = ManualPasteboardSleeper()
    let secondLease = RecoveryPasteboardLease(pasteboard: pasteboard, sleeper: secondSleeper)
    try await secondLease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(30))
    await secondSleeper.waitForEntries(1)
    try await secondLease.clearExplicitly()
    try await secondLease.clearExplicitly()
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
    await secondSleeper.wakeAll()
  }

  @Test func warningsAndOrdinaryDescriptionsNeverContainSecretCanary() throws {
    let canary = testRecoverySecret
    let material = try recoveryMaterial()
    #expect(
      RecoveryWarningCopy.unrecoverableEnglish
        == "Loss of the sole installation plus an unsaved recovery secret is unrecoverable.")
    #expect(
      RecoveryWarningCopy.unrecoverableRussian
        == "Потеря единственной установки вместе с несохранённым секретом восстановления необратима."
    )
    #expect(RecoveryWarningCopy.dismissedUnsavedEnglish.contains("is now gone"))
    #expect(RecoveryWarningCopy.dismissedUnsavedRussian.contains("теперь утрачен"))
    #expect(!String(describing: material).contains(canary))
    #expect(!String(reflecting: material).contains(canary))
    #expect(!String(describing: RecoveryExportError.writeFailed).contains(canary))
    for status in [
      RecoveryPasteboardCleanupStatus.idle, .leased, .automaticCleanupFailed,
    ] {
      #expect(!String(describing: status).contains(canary))
      #expect(!String(reflecting: status).contains(canary))
    }
  }

  @Test func concurrentCopyContendersKeepWinnerLeasedAcrossOldExpiry() async throws {
    let pasteboard = FirstWriteGatedPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(pasteboard: pasteboard, sleeper: sleeper)
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    let first = try recoveryMaterial()
    let second = OneTimeRecoveryMaterial(
      actorId: 10, recoveryId: "rec_" + String(repeating: "e", count: 32),
      secret: try RecoverySecret(validated: testRecoverySecret))
    let firstCopy = Task {
      try await lease.copyExplicitly(first, timeToLive: .seconds(10))
    }
    await pasteboard.waitUntilFirstWriteEntered()
    let secondCopy = Task {
      try await lease.copyExplicitly(second, timeToLive: .seconds(20))
    }
    pasteboard.releaseFirstWrite()
    try await firstCopy.value
    try await secondCopy.value
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(2)
    #expect(pasteboard.string() == (try RecoveryExportHelper.payload(for: second)))
    await sleeper.wake(duration: .seconds(10))
    #expect(pasteboard.string() == (try RecoveryExportHelper.payload(for: second)))
    #expect(pasteboard.clearCount == 0)
    await sleeper.wake(duration: .seconds(20))
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)

    await #expect(throws: RecoveryExportError.invalidLeaseDuration) {
      try await lease.copyExplicitly(first, timeToLive: .zero)
    }
    await #expect(throws: RecoveryExportError.invalidLeaseDuration) {
      try await lease.copyExplicitly(first, timeToLive: .seconds(301))
    }
  }

  @Test func copyThenClearAndClearThenCopyHavePinnedTTLOrderings() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(pasteboard: pasteboard, sleeper: sleeper)
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    try await lease.clearExplicitly()
    #expect(await statusIterator.next() == .idle)
    await sleeper.wake(duration: .seconds(10))
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)

    try await lease.clearExplicitly()
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(20))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(2)
    #expect(pasteboard.string() != nil)
    await sleeper.wake(duration: .seconds(20))
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 2)
  }

  @Test func atomicConditionalClearPreservesReplacementAtFormerTOCTOUBoundary() async throws {
    let pasteboard = ConditionalClearGatedPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(pasteboard: pasteboard, sleeper: sleeper)
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    await sleeper.wake(duration: .seconds(10))
    await pasteboard.waitUntilConditionalClearEntered()
    pasteboard.replaceExternally(with: "newer external clipboard value")
    pasteboard.releaseConditionalClear()
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == "newer external clipboard value")
    #expect(pasteboard.clearCount == 0)
  }

  @Test func expiryClearFailureRetainsExactLeaseAndRetriesWithBoundedDelay() async throws {
    let pasteboard = MemoryPasteboard()
    pasteboard.failNextClears(1)
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(
      pasteboard: pasteboard, sleeper: sleeper, clearRetryDelays: [.seconds(1)])
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)

    await sleeper.wake(duration: .seconds(10))
    await sleeper.waitForEntries(2)
    #expect(pasteboard.string() != nil)
    #expect(pasteboard.clearCount == 0)

    await sleeper.wake(duration: .seconds(1))
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
  }

  @Test func explicitClearFailureSurfacesAndKeepsRetryAuthorityUntilResolved() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(
      pasteboard: pasteboard, sleeper: sleeper, clearRetryDelays: [.seconds(2)])
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    await sleeper.waitForEntries(1)
    pasteboard.failNextClears(1)

    await #expect(throws: RecoveryExportError.writeFailed) {
      try await lease.clearExplicitly()
    }
    await sleeper.waitForEntries(2)
    #expect(pasteboard.string() != nil)
    #expect(pasteboard.clearCount == 0)

    try await lease.clearExplicitly()
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
    await sleeper.wake(duration: .seconds(2))
    #expect(pasteboard.clearCount == 1)
  }

  @Test func automaticClearRetriesAreCappedInCountAndDelay() async throws {
    let pasteboard = MemoryPasteboard()
    pasteboard.failNextClears(4)
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(
      pasteboard: pasteboard, sleeper: sleeper,
      clearRetryDelays: [.seconds(999), .seconds(2), .seconds(3), .seconds(4)])
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)

    await sleeper.wake(duration: .seconds(10))
    await sleeper.waitForEntries(2)
    await sleeper.wake(duration: RecoveryPasteboardLease.maximumClearRetryDelay)
    await sleeper.waitForEntries(3)
    await sleeper.wake(duration: .seconds(2))
    await sleeper.waitForEntries(4)
    await sleeper.wake(duration: .seconds(3))
    #expect(await statusIterator.next() == .automaticCleanupFailed)

    #expect(await sleeper.entryCount == 4)
    #expect(pasteboard.string() != nil)
    #expect(pasteboard.clearCount == 0)
    #expect(await lease.cleanupStatus == .automaticCleanupFailed)
    try await lease.clearExplicitly()
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
    #expect(await lease.cleanupStatus == .idle)
    #expect(await statusIterator.next() == .idle)
  }

  @Test func terminalCleanupFailureRetainsLeaseUntilExternalReplacementIsProven() async throws {
    let pasteboard = MemoryPasteboard()
    pasteboard.failNextClears(1)
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(
      pasteboard: pasteboard, sleeper: sleeper, clearRetryDelays: [])
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    await sleeper.wake(duration: .seconds(10))
    #expect(await statusIterator.next() == .automaticCleanupFailed)
    #expect(await lease.cleanupStatus == .automaticCleanupFailed)
    #expect(pasteboard.string() != nil)

    pasteboard.replaceExternally(with: "newer external clipboard value")
    try await lease.clearExplicitly()
    #expect(await statusIterator.next() == .idle)
    #expect(pasteboard.string() == "newer external clipboard value")
    #expect(pasteboard.clearCount == 0)
    #expect(await lease.cleanupStatus == .idle)
  }

  @Test func copyingNewPayloadResetsTerminalCleanupFailureForExactNewLease() async throws {
    let pasteboard = MemoryPasteboard()
    pasteboard.failNextClears(1)
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(
      pasteboard: pasteboard, sleeper: sleeper, clearRetryDelays: [])
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    try await lease.copyExplicitly(try recoveryMaterial(), timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    await sleeper.wake(duration: .seconds(10))
    #expect(await statusIterator.next() == .automaticCleanupFailed)
    #expect(await lease.cleanupStatus == .automaticCleanupFailed)

    let replacement = OneTimeRecoveryMaterial(
      actorId: 10, recoveryId: "rec_" + String(repeating: "e", count: 32),
      secret: try RecoverySecret(validated: testRecoverySecret))
    try await lease.copyExplicitly(replacement, timeToLive: .seconds(20))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(2)
    #expect(await lease.cleanupStatus == .leased)
    #expect(pasteboard.string() == (try RecoveryExportHelper.payload(for: replacement)))

    await sleeper.wake(duration: .seconds(20))
    #expect(await statusIterator.next() == .idle)
    #expect(await lease.cleanupStatus == .idle)
    #expect(pasteboard.string() == nil)
  }

  @Test func newerCopyIgnoresStaleRetryAndResetsCleanupStatus() async throws {
    let pasteboard = MemoryPasteboard()
    pasteboard.failNextClears(1)
    let sleeper = ManualPasteboardSleeper()
    let lease = RecoveryPasteboardLease(
      pasteboard: pasteboard, sleeper: sleeper, clearRetryDelays: [.seconds(1)])
    let updates = await lease.cleanupStatusUpdates()
    var statusIterator = updates.makeAsyncIterator()
    #expect(await statusIterator.next() == .idle)
    let first = try recoveryMaterial()
    let second = OneTimeRecoveryMaterial(
      actorId: 10, recoveryId: "rec_" + String(repeating: "e", count: 32),
      secret: try RecoverySecret(validated: testRecoverySecret))

    try await lease.copyExplicitly(first, timeToLive: .seconds(10))
    #expect(await statusIterator.next() == .leased)
    await sleeper.waitForEntries(1)
    await sleeper.wake(duration: .seconds(10))
    await sleeper.waitForEntries(2)
    try await lease.copyExplicitly(second, timeToLive: .seconds(20))
    await sleeper.waitForEntries(3)
    #expect(await lease.cleanupStatus == .leased)

    await sleeper.wake(duration: .seconds(1))
    #expect(await lease.cleanupStatus == .leased)
    #expect(pasteboard.string() == (try RecoveryExportHelper.payload(for: second)))
    #expect(pasteboard.clearCount == 0)

    await sleeper.wake(duration: .seconds(20))
    #expect(await statusIterator.next() == .idle)
    #expect(await lease.cleanupStatus == .idle)
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
  }

  private func recoveryMaterial() throws -> OneTimeRecoveryMaterial {
    OneTimeRecoveryMaterial(
      actorId: 9, recoveryId: testRecoveryID,
      secret: try RecoverySecret(validated: testRecoverySecret)
    )
  }
}

private final class ScriptedRecoveryFileOperations: RecoveryExportFileOperations,
  @unchecked Sendable
{
  private let lock = NSLock()
  private let openResult: Int32
  private var writeResults: [RecoveryFileWriteResult]
  private let synchronizeResult: Bool
  private let closeResult: Bool
  private let truncateResult: Bool
  private let removeResult: Bool
  private(set) var events: [String] = []
  private(set) var openedURLs: [URL] = []
  private(set) var writtenData = Data()

  init(
    openResult: Int32 = 41,
    writeResults: [RecoveryFileWriteResult] = [],
    synchronizeResult: Bool = true,
    closeResult: Bool = true,
    truncateResult: Bool = true,
    removeResult: Bool = true
  ) {
    self.openResult = openResult
    self.writeResults = writeResults
    self.synchronizeResult = synchronizeResult
    self.closeResult = closeResult
    self.truncateResult = truncateResult
    self.removeResult = removeResult
  }

  func openExclusiveNoFollow(_ url: URL) -> Int32 {
    lock.withLock {
      events.append("open")
      openedURLs.append(url)
      return openResult
    }
  }

  func write(_ descriptor: Int32, buffer: UnsafeRawBufferPointer) -> RecoveryFileWriteResult {
    lock.withLock {
      events.append("write")
      let result = writeResults.isEmpty ? .written(buffer.count) : writeResults.removeFirst()
      if case .written(let count) = result, count > 0, count <= buffer.count,
        let baseAddress = buffer.baseAddress
      {
        writtenData.append(baseAddress.assumingMemoryBound(to: UInt8.self), count: count)
      }
      return result
    }
  }

  func synchronize(_ descriptor: Int32) -> Bool {
    lock.withLock {
      events.append("sync")
      return synchronizeResult
    }
  }

  func close(_ descriptor: Int32) -> Bool {
    lock.withLock {
      events.append("close")
      return closeResult
    }
  }

  func truncate(_ descriptor: Int32) -> Bool {
    lock.withLock {
      events.append("truncate")
      return truncateResult
    }
  }

  func remove(_ url: URL) -> Bool {
    lock.withLock {
      events.append("remove")
      return removeResult
    }
  }
}

private final class FirstWriteGatedPasteboard: PasteboardAccess, @unchecked Sendable {
  private let stateLock = NSLock()
  private var value: String?
  private var change = 0
  private var clears = 0
  private let gateLock = NSLock()
  private var firstWrite = true
  private var entered = false
  private var enteredWaiters: [CheckedContinuation<Void, Never>] = []
  private let release = DispatchSemaphore(value: 0)

  var clearCount: Int {
    stateLock.lock()
    defer { stateLock.unlock() }
    return clears
  }

  func string() -> String? {
    stateLock.lock()
    defer { stateLock.unlock() }
    return value
  }

  func write(_ value: String) throws -> Int {
    gateLock.lock()
    let shouldBlock = firstWrite
    firstWrite = false
    if shouldBlock {
      entered = true
      let waiters = enteredWaiters
      enteredWaiters.removeAll()
      gateLock.unlock()
      for waiter in waiters { waiter.resume() }
      release.wait()
    } else {
      gateLock.unlock()
    }
    stateLock.lock()
    self.value = value
    change += 1
    let result = change
    stateLock.unlock()
    return result
  }

  func clearIfUnchanged(expectedChangeCount: Int, expectedPayload: String) throws -> Bool {
    stateLock.lock()
    defer { stateLock.unlock() }
    guard change == expectedChangeCount, value == expectedPayload else { return false }
    value = nil
    change += 1
    clears += 1
    return true
  }

  func waitUntilFirstWriteEntered() async {
    await withCheckedContinuation { continuation in
      gateLock.lock()
      if entered {
        gateLock.unlock()
        continuation.resume()
      } else {
        enteredWaiters.append(continuation)
        gateLock.unlock()
      }
    }
  }

  func releaseFirstWrite() { release.signal() }
}

private final class ConditionalClearGatedPasteboard: PasteboardAccess, @unchecked Sendable {
  private let stateLock = NSLock()
  private var value: String?
  private var change = 0
  private var clears = 0
  private let gateLock = NSLock()
  private var entered = false
  private var enteredWaiters: [CheckedContinuation<Void, Never>] = []
  private let release = DispatchSemaphore(value: 0)

  var clearCount: Int {
    stateLock.lock()
    defer { stateLock.unlock() }
    return clears
  }

  func string() -> String? {
    stateLock.lock()
    defer { stateLock.unlock() }
    return value
  }

  func write(_ value: String) throws -> Int {
    stateLock.lock()
    self.value = value
    change += 1
    let result = change
    stateLock.unlock()
    return result
  }

  func clearIfUnchanged(expectedChangeCount: Int, expectedPayload: String) throws -> Bool {
    gateLock.lock()
    entered = true
    let waiters = enteredWaiters
    enteredWaiters.removeAll()
    gateLock.unlock()
    for waiter in waiters { waiter.resume() }
    release.wait()

    stateLock.lock()
    defer { stateLock.unlock() }
    guard change == expectedChangeCount, value == expectedPayload else { return false }
    value = nil
    change += 1
    clears += 1
    return true
  }

  func replaceExternally(with value: String) {
    stateLock.lock()
    self.value = value
    change += 1
    stateLock.unlock()
  }

  func waitUntilConditionalClearEntered() async {
    await withCheckedContinuation { continuation in
      gateLock.lock()
      if entered {
        gateLock.unlock()
        continuation.resume()
      } else {
        enteredWaiters.append(continuation)
        gateLock.unlock()
      }
    }
  }

  func releaseConditionalClear() { release.signal() }
}
