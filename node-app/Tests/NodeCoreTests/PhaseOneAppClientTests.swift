import Foundation
import Testing

@testable import NodeCore

@Suite("Phase 1 macOS data client")
struct PhaseOneAppClientTests {
  @Test("Upload, transmission, presence and history use canonical authenticated contracts")
  func canonicalWireContracts() async throws {
    let repositoryRoot = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent().deletingLastPathComponent()
      .deletingLastPathComponent().deletingLastPathComponent()
    let file = repositoryRoot.appendingPathComponent("assets/audio/pulsar-recording-cue.wav")
    let size = try Data(contentsOf: file).count
    let uploadID = "up_" + String(repeating: "A", count: 26)
    let mediaID = "m_" + String(repeating: "B", count: 26)
    let transmissionID = "tr_" + String(repeating: "C", count: 26)
    let historyID = "hi_" + String(repeating: "D", count: 26)
    let requestIndex = PhaseOneRequestIndex()
    let transport = ScriptedTransport { request, maximum in
      let current = requestIndex.next()
      #expect(maximum == 64 * 1_024)
      switch current {
      case 0:
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/media/uploads")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-upload-00000000000000000000000000000001")
        let object = try #require(
          JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(object["kind"] as? String == "voice_clip")
        #expect(object["size_bytes"] as? Int == size)
        return testHTTPResponse(
          request: request, status: 201,
          json: #"{"upload_id":"\#(uploadID)","media_id":"\#(mediaID)","upload_token":"\#(testNodeToken)","upload_offset":0,"upload_length":\#(size),"expires_at":"2026-07-15T00:00:00Z","status":"open"}"#)
      case 1:
        #expect(request.httpMethod == "PUT")
        #expect(request.url?.path == "/v1/media/uploads/\(uploadID)")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testNodeToken)")
        #expect(request.value(forHTTPHeaderField: "Upload-Offset") == "0")
        #expect(request.httpBody?.count == size)
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"upload_id":"\#(uploadID)","media_id":"\#(mediaID)","upload_offset":\#(size),"upload_length":\#(size),"expires_at":"2026-07-15T00:00:00Z","status":"completed"}"#)
      case 2:
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/transmissions")
        let object = try #require(
          JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(object["media_id"] as? String == mediaID)
        #expect(object["include_origin"] as? Bool == false)
        #expect((object["audience"] as? [String: Any])?["kind"] as? String == "own_barycenter")
        return testHTTPResponse(
          request: request, status: 201,
          json: #"{"transmission_id":"\#(transmissionID)","media_id":"\#(mediaID)","audience":{"kind":"own_barycenter","target_count":2},"origin_kind":"microphone","include_origin":false,"requested_delivery":"overlay","effective_delivery":"after_current","downgrade_reason":"mandatory_target_missing_overlay_capability","accepted_at":"2026-07-15T00:00:00.000Z","expires_at":"2026-07-15T00:05:00.000Z","status":"accepted","target_counts":{"accepted":2,"preparing":0,"ready":0,"scheduled":0,"playing":0,"cancelling":0,"played":0,"missed_offline":0,"missed_dnd":0,"missed_not_ready":0,"blocked":0,"failed":0,"cancelled":0,"expired":0},"can_cancel":true,"reused":false}"#)
      case 3:
        #expect(request.url?.path == "/v1/presence")
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"contract":"p1-history-presence-telegram-v1","revision":1,"generated_at":"2026-07-15T00:00:00.000Z","orbit_dnd":{"mode":"allow_all","revision":0},"nodes":[{"orbit_id":1,"slot":"a","online":true,"output_state":"ready","playback_state":"playing","local_dnd":{"mode":"allow_all","revision":0},"effective_dnd":{"mode":"allow_all","source":"none"},"capabilities":["media_clip_v1"],"interrupt_resume_ready":false}]}"#)
      case 4:
        #expect(request.url?.path == "/v1/history")
        #expect(request.url?.query == "limit=20")
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"contract":"p1-history-presence-telegram-v1","items":[{"history_item_id":"\#(historyID)","item_kind":"transmission","direction":"sent","occurred_at":"2026-07-15T00:00:00.000Z","media":{"media_id":"\#(mediaID)","kind":"voice_clip","title":"Pulsar recording","duration_ms":160,"content_available":true},"audience":{"kind":"own_barycenter","target_count":2},"requested_delivery":"overlay","effective_delivery":"after_current","downgrade_reason":"mandatory_target_missing_overlay_capability","status":"partial","reason_code":"partial_delivery","target_counts":{"played":1,"other":1},"actions":["delete","replay"]}],"next_cursor":"opaque-cursor"}"#)
      case 5:
        #expect(request.url?.path == "/v1/history/\(historyID)/actions/delete")
        #expect(String(decoding: request.httpBody!, as: UTF8.self) == "{}")
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"history_item_id":"\#(historyID)","media_id":"\#(mediaID)","deleted":true}"#)
      case 6:
        #expect(request.httpMethod == "DELETE")
        #expect(request.url?.path == "/v1/media/\(mediaID)")
        let response = HTTPURLResponse(
          url: request.url!, statusCode: 204, httpVersion: "HTTP/1.1", headerFields: [:])!
        return HTTPTransportResponse(data: Data(), response: response)
      default:
        Issue.record("unexpected request")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)

    let upload = try await client.upload(
      fileURL: file,
      title: "Pulsar recording",
      idempotencyKey: "mac-upload-00000000000000000000000000000001")
    #expect(upload.mediaID == mediaID)
    let receipt = try await client.transmit(
      mediaID: mediaID,
      route: .ownBarycenter,
      delivery: .overlay,
      originKind: .microphone,
      idempotencyKey: "mac-transmission-00000000000000000000000000000001")
    #expect(receipt.requestedDelivery == .overlay)
    #expect(receipt.effectiveDelivery == .afterCurrent)
    #expect(receipt.downgradeReason == "mandatory_target_missing_overlay_capability")
    let presence = try await client.presence()
    #expect(presence == [
      .init(slot: "a", online: true, outputState: "ready", playbackState: "playing", effectiveDND: "allow_all"),
    ])
    let history = try await client.history(limit: 20, cursor: nil)
    #expect(history.items.first?.id == historyID)
    #expect(history.items.first?.effectiveDelivery == "after_current")
    #expect(history.items.first?.playedCount == 1)
    #expect(history.nextCursor == "opaque-cursor")
    try await client.deleteHistoryItem(historyID)
    try await client.deleteMedia(mediaID)
    #expect(transport.requests().count == 7)
  }

  @Test("Redirects and self-test upload attempts are rejected before false success")
  func rejectionBoundaries() async throws {
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 200, json: "{}",
        url: URL(string: "https://attacker.example/v1/presence"))
    }
    let client = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)
    await #expect(throws: PhaseOneClientError.redirectRejected) {
      _ = try await client.presence()
    }
    #expect(throws: PhaseOneClientError.invalidConfiguration) {
      _ = try PhaseOneAppClient(bundle: CredentialBundle())
    }
  }

  private func credentialBundle() throws -> CredentialBundle {
    CredentialBundle(
      coordinatorOrigin: try CoordinatorOrigin("https://coord.example"),
      node: NodeCapability(
        orbitId: 1, slot: "a", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example/ws"),
      control: ControlCapability(
        actorId: 2, orbitId: 1, role: .primary,
        controlToken: testControlToken, contextStrength: .active))
  }
}

private final class PhaseOneRequestIndex: @unchecked Sendable {
  private let lock = NSLock()
  private var value = 0

  func next() -> Int {
    lock.withLock {
      defer { value += 1 }
      return value
    }
  }
}
