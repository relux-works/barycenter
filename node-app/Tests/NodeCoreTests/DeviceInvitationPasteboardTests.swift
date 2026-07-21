import Foundation
import Testing

@testable import NodeCore

@Suite("Device invitation pasteboard lease", .serialized)
struct DeviceInvitationPasteboardTests {
  private let code = "ABCDEFGHJKMNPQRSTVWXYZ23456"

  @Test("Copy is explicit and expiry clears only the owned code")
  func explicitCopyAndExpiry() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = DeviceInvitationPasteboardLease(
      pasteboard: pasteboard,
      sleeper: sleeper)
    let updates = await lease.cleanupStatusUpdates()
    var iterator = updates.makeAsyncIterator()

    #expect(await iterator.next() == .idle)
    #expect(pasteboard.string() == nil)
    try await lease.copyExplicitly(code, timeToLive: .seconds(30))
    #expect(await iterator.next() == .leased)
    await sleeper.waitForEntries(1)
    #expect(pasteboard.string() == code)
    await sleeper.wakeAll()
    #expect(await iterator.next() == .idle)
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
  }

  @Test("User clipboard replacement survives automatic and explicit cleanup")
  func replacementSurvivesCleanup() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = DeviceInvitationPasteboardLease(
      pasteboard: pasteboard,
      sleeper: sleeper)

    try await lease.copyExplicitly(code, timeToLive: .seconds(30))
    await sleeper.waitForEntries(1)
    pasteboard.replaceExternally(with: "new user clipboard value")
    try await lease.clearExplicitly()
    #expect(pasteboard.string() == "new user clipboard value")
    #expect(pasteboard.clearCount == 0)
    await sleeper.wakeAll()
    #expect(pasteboard.string() == "new user clipboard value")
  }

  @Test("Explicit hide is idempotent and stale expiry cannot clear a new lease")
  func explicitClearAndReplacementLease() async throws {
    let pasteboard = MemoryPasteboard()
    let sleeper = ManualPasteboardSleeper()
    let lease = DeviceInvitationPasteboardLease(
      pasteboard: pasteboard,
      sleeper: sleeper)

    try await lease.copyExplicitly(code, timeToLive: .seconds(10))
    await sleeper.waitForEntries(1)
    try await lease.clearExplicitly()
    try await lease.clearExplicitly()
    #expect(pasteboard.clearCount == 1)

    let replacement = "23456YZXTWVSRQPNMKJHGFEDCBA"
    try await lease.copyExplicitly(replacement, timeToLive: .seconds(20))
    await sleeper.waitForEntries(2)
    await sleeper.wake(duration: .seconds(10))
    #expect(pasteboard.string() == replacement)
    await sleeper.wake(duration: .seconds(20))
    for _ in 0..<100 where pasteboard.string() != nil {
      await Task.yield()
    }
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 2)
  }

  @Test("Invalid values fail without writing and ordinary state is redacted")
  func validationAndRedaction() async {
    let pasteboard = MemoryPasteboard()
    let lease = DeviceInvitationPasteboardLease(
      pasteboard: pasteboard,
      sleeper: ManualPasteboardSleeper())

    await #expect(throws: DeviceInvitationPasteboardError.invalidCode) {
      try await lease.copyExplicitly("not-an-invite", timeToLive: .seconds(30))
    }
    await #expect(throws: DeviceInvitationPasteboardError.invalidLeaseDuration) {
      try await lease.copyExplicitly(code, timeToLive: .zero)
    }
    await #expect(throws: DeviceInvitationPasteboardError.invalidLeaseDuration) {
      try await lease.copyExplicitly(code, timeToLive: .seconds(301))
    }
    #expect(pasteboard.writeCount == 0)
    for status in [
      DeviceInvitationPasteboardCleanupStatus.idle,
      .leased,
      .automaticCleanupFailed,
    ] {
      #expect(!String(describing: status).contains(code))
      #expect(!String(reflecting: status).contains(code))
    }
  }

  @Test("Termination cleanup is synchronous and remains conditional")
  func synchronousTerminationCleanup() async throws {
    let pasteboard = MemoryPasteboard()
    let clearedLease = DeviceInvitationPasteboardLease(
      pasteboard: pasteboard,
      sleeper: ManualPasteboardSleeper())

    try await clearedLease.copyExplicitly(code, timeToLive: .seconds(30))
    try clearedLease.clearSynchronouslyForTermination()
    #expect(pasteboard.string() == nil)
    #expect(pasteboard.clearCount == 1)
    await #expect(throws: DeviceInvitationPasteboardError.writeFailed) {
      try await clearedLease.copyExplicitly(code, timeToLive: .seconds(30))
    }
    #expect(pasteboard.writeCount == 1)

    let conditionalLease = DeviceInvitationPasteboardLease(
      pasteboard: pasteboard,
      sleeper: ManualPasteboardSleeper())
    try await conditionalLease.copyExplicitly(code, timeToLive: .seconds(30))
    pasteboard.replaceExternally(with: "new user clipboard value")
    try conditionalLease.clearSynchronouslyForTermination()
    #expect(pasteboard.string() == "new user clipboard value")
    #expect(pasteboard.clearCount == 1)
  }
}
