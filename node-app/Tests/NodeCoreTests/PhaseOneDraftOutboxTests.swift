import Foundation
import Testing

@testable import NodeCore

@Suite("Phase 1 durable draft outbox")
struct PhaseOneDraftOutboxTests {
  @Test("Restart retries transmission once without duplicate upload and keeps the frozen intent")
  func restartRetryIsIdempotent() async throws {
    let root = temporaryRoot()
    let mediaRoot = root.appendingPathComponent("CaptureMedia", isDirectory: true)
    let stateURL = root.appendingPathComponent("PhaseOne/outbox.json")
    let store = CaptureMediaStore(
      root: mediaRoot,
      idProvider: { "00000000000000000000000000000001" })
    let draft = try store.importUserDraft(bytes: try cueBytes())
    let service = ScriptedPhaseOneService(failedTransmissionCount: 1)
    let first = try PhaseOneDraftOutbox(
      service: service,
      mediaStore: store,
      stateURL: stateURL,
      recoveredDrafts: [draft])

    await #expect(throws: PhaseOneDraftOutboxError.service("coordinator_unavailable")) {
      _ = try await first.send(
        draftID: draft.id, route: .ownBarycenter, delivery: .overlay)
    }
    #expect(!FileManager.default.fileExists(atPath: draft.fileURL.path))
    let failed = try #require(await first.snapshots().first)
    #expect(failed.state == .retryableFailure)
    #expect(failed.failureCode == "coordinator_unavailable")
    #expect(!failed.localBytesRetained)

    let restartedStore = CaptureMediaStore(root: mediaRoot)
    let recovery = try restartedStore.recover()
    #expect(recovery.retainedDrafts.isEmpty)
    let restarted = try PhaseOneDraftOutbox(
      service: service,
      mediaStore: restartedStore,
      stateURL: stateURL,
      recoveredDrafts: recovery.retainedDrafts)
    let accepted = try await restarted.send(
      draftID: draft.id, route: .ownBarycenter, delivery: .overlay)
    #expect(accepted.state == .accepted)
    #expect(accepted.requestedDelivery == .overlay)
    #expect(accepted.effectiveDelivery == .afterCurrent)
    #expect(accepted.downgradeReason == "mandatory_target_missing_overlay_capability")
    let calls = await service.calls()
    #expect(calls.uploadKeys == ["mac-upload-\(draft.id)"])
    #expect(calls.transmissionKeys == [
      "mac-transmission-\(draft.id)",
      "mac-transmission-\(draft.id)",
    ])
    await #expect(throws: PhaseOneDraftOutboxError.invalidDraft) {
      _ = try await restarted.send(
        draftID: draft.id, route: .currentAir, delivery: .overlay)
    }
  }

  @Test("Explicit delete removes an unsent draft and self-test handles never attach")
  func explicitDeleteAndSelfTestBoundary() async throws {
    let root = temporaryRoot()
    let store = CaptureMediaStore(
      root: root.appendingPathComponent("CaptureMedia"),
      idProvider: { UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased() })
    let draft = try store.importUserDraft(bytes: try cueBytes())
    let service = ScriptedPhaseOneService()
    let outbox = try PhaseOneDraftOutbox(
      service: service,
      mediaStore: store,
      stateURL: root.appendingPathComponent("PhaseOne/outbox.json"),
      recoveredDrafts: [draft])
    try await outbox.delete(draftID: draft.id)
    #expect(!FileManager.default.fileExists(atPath: draft.fileURL.path))
    #expect(await outbox.snapshots().isEmpty)
    #expect((await service.calls()).deletedMediaIDs.isEmpty)

    let partial = try store.begin(.selfTest)
    try cueBytes().write(to: partial.fileURL)
    let selfTest = try store.finalize(store.stop(partial))
    await #expect(throws: PhaseOneDraftOutboxError.invalidDraft) {
      try await outbox.attach(selfTest)
    }
    #expect(FileManager.default.fileExists(atPath: selfTest.fileURL.path))
  }

  private func cueBytes() throws -> Data {
    let root = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent().deletingLastPathComponent()
      .deletingLastPathComponent().deletingLastPathComponent()
    return try Data(contentsOf: root.appendingPathComponent("assets/audio/pulsar-recording-cue.wav"))
  }

  private func temporaryRoot() -> URL {
    FileManager.default.temporaryDirectory
      .appendingPathComponent("phase-one-outbox-tests-\(UUID().uuidString)", isDirectory: true)
  }
}

private actor ScriptedPhaseOneService: PhaseOneAppServicing {
  struct Calls: Sendable {
    var uploadKeys: [String] = []
    var transmissionKeys: [String] = []
    var deletedMediaIDs: [String] = []
  }

  private var recorded = Calls()
  private var failedTransmissionCount: Int

  init(failedTransmissionCount: Int = 0) {
    self.failedTransmissionCount = failedTransmissionCount
  }

  func upload(fileURL: URL, title: String, idempotencyKey: String) async throws
    -> PhaseOneUploadConfirmation
  {
    recorded.uploadKeys.append(idempotencyKey)
    #expect(FileManager.default.fileExists(atPath: fileURL.path))
    return .init(mediaID: "m_" + String(repeating: "A", count: 26), reused: false)
  }

  func transmit(
    mediaID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt {
    recorded.transmissionKeys.append(idempotencyKey)
    if failedTransmissionCount > 0 {
      failedTransmissionCount -= 1
      throw PhaseOneClientError.transport
    }
    return .init(
      transmissionID: "tr_" + String(repeating: "B", count: 26),
      requestedDelivery: delivery,
      effectiveDelivery: .afterCurrent,
      downgradeReason: "mandatory_target_missing_overlay_capability",
      status: "accepted",
      reused: recorded.transmissionKeys.count > 1)
  }

  func deleteMedia(_ mediaID: String) async throws { recorded.deletedMediaIDs.append(mediaID) }
  func presence() async throws -> [PhaseOnePresenceNode] { [] }
  func history(limit: Int, cursor: String?) async throws -> PhaseOneHistoryPage {
    .init(items: [], nextCursor: nil)
  }
  func deleteHistoryItem(_ historyItemID: String) async throws -> PhaseOneHistoryActionReceipt {
    .init(outcome: "media_deleted")
  }
  func reportHistoryItem(
    _ historyItemID: String,
    reason: PhaseOneModerationReason,
    details: String
  ) async throws -> PhaseOneHistoryActionReceipt {
    .init(outcome: "report_received")
  }
  func blockHistoryActor(
    _ historyItemID: String,
    idempotencyKey: String
  ) async throws -> PhaseOneHistoryActionReceipt {
    .init(outcome: "sender_blocked")
  }
  func replayHistoryItem(
    _ historyItemID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt {
    .init(
      transmissionID: "tr_" + String(repeating: "C", count: 26),
      requestedDelivery: delivery,
      effectiveDelivery: delivery,
      downgradeReason: nil,
      status: "accepted",
      reused: false)
  }
  func calls() -> Calls { recorded }
}
