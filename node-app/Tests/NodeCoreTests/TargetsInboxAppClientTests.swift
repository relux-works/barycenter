import Foundation
import Testing

@testable import NodeCore

@Suite("Phase 2 macOS targets and inbox client")
struct TargetsInboxAppClientTests {
  @Test("Projection and explicit actions use authenticated capability endpoints without autoplay")
  func projectionAndActions() async throws {
    let inboxID = "ib_" + String(repeating: "A", count: 26)
    let historyID = "hi_" + String(repeating: "B", count: 26)
    let blockID = "bl_" + String(repeating: "C", count: 26)
    let targetRef = "trf_" + String(repeating: "x", count: 43)
    let inboxCursor = "ic_" + String(repeating: "1", count: 64)
    let historyCursor = "hc_" + String(repeating: "2", count: 64)
    let receiptCursor = "rc_" + String(repeating: "3", count: 64)
    let transport = ScriptedTransport { request, maximum in
      #expect(maximum == 64 * 1_024)
      #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
      let path = try #require(request.url?.path)
      switch (request.httpMethod, path) {
      case ("GET", "/v1/transmission-targets"):
        return testHTTPResponse(request: request, status: 200, json: """
          {"contract":"p2-targets-inbox-parity.v1","targets":[{
            "reference":"\(targetRef)","kind":"pulsar","label":"safe",
            "capability_state":"mixed","capabilities":["media_clip_v1"],
            "expires_at":"2026-07-17T00:00:00.000Z","presentation":{
              "label":{"key":"target.pulsar","en":"Home, Pulsar a","ru":"Дом, Пульсар a"},
              "capability_state":{"key":"capability_state.mixed","en":"Some differ","ru":"Есть отличия"},
              "capabilities":["media_clip_v1"]}}]}
          """)
      case ("GET", "/v1/inbox"):
        #expect(request.url?.query?.contains("limit=20") == true)
        return testHTTPResponse(request: request, status: 200, json: """
          {"contract":"p2-targets-inbox-parity.v1","items":[{
            "id":"\(inboxID)","history_item_id":"\(historyID)",
            "media":{"title":"Missed clip"},"availability":"available",
            "expires_at":"2026-07-17T00:00:00.000Z","actions":["replay","dismiss","report","block_actor"],
            "presentation":{
              "sender":{"key":"sender.named","en":"Alice","ru":"Алиса"},
              "source":{"key":"origin.named","en":"From Home","ru":"Из Дома"},
              "requested_delivery":{"key":"delivery.interrupt","en":"Interrupt","ru":"Прервать"},
              "effective_delivery":{"key":"delivery.after_current","en":"After current","ru":"После текущего"},
              "receipt":{"key":"receipt.reason.offline","en":"Missed offline","ru":"Пропущено офлайн"},
              "actions":[
                {"action":"replay","label":{"key":"action.replay","en":"Replay","ru":"Повторить"}},
                {"action":"dismiss","label":{"key":"action.dismiss","en":"Dismiss","ru":"Убрать"}},
                {"action":"report","label":{"key":"action.report","en":"Report","ru":"Пожаловаться"}},
                {"action":"block_actor","label":{"key":"action.block_actor","en":"Mute sender","ru":"Заглушить"}}]}}],
            "next_cursor":"\(inboxCursor)"}
          """)
      case ("GET", "/v1/history"):
        return testHTTPResponse(request: request, status: 200, json: """
          {"contract":"p1-history-presence-telegram-v1","items":[{
            "history_item_id":"\(historyID)","media":{"title":"Missed clip"},
            "target_counts":{"played":1,"other":2},"actions":["delete","report","block_actor"],
            "presentation":{"status":{"key":"receipt.partial","en":"Partial","ru":"Частично"},
              "actions":[
                {"action":"delete","label":{"key":"action.delete","en":"Delete","ru":"Удалить"}},
                {"action":"report","label":{"key":"action.report","en":"Report","ru":"Пожаловаться"}},
                {"action":"block_actor","label":{"key":"action.block_actor","en":"Mute sender","ru":"Заглушить"}}]}}],
            "next_cursor":"\(historyCursor)"}
          """)
      case ("GET", "/v1/content-policy/acceptance"):
        return testHTTPResponse(request: request, status: 200, json:
          #"{"contract":"p2-content-policy-consent.v1","current":true,"terms_accepted":true}"#)
      case ("GET", "/v1/history/\(historyID)/receipts"):
        return testHTTPResponse(request: request, status: 200, json: """
          {"contract":"p2-targets-inbox-parity.v1","history_item_id":"\(historyID)",
           "items":[{"target_label":"Home, Pulsar a","presentation":{"status":{
             "key":"receipt.played","en":"Played","ru":"Воспроизведено"}}}],
           "next_cursor":"\(receiptCursor)"}
          """)
      case ("DELETE", "/v1/inbox/\(inboxID)"):
        return testHTTPResponse(request: request, status: 200, json:
          #"{"contract":"p2-targets-inbox-parity.v1","item":{"id":"\#(inboxID)","availability":"dismissed"}}"#)
      case ("POST", "/v1/inbox/\(inboxID)/replays"):
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-inbox-replay-test")
        return testHTTPResponse(request: request, status: 201, json: """
          {"contract":"p2-targets-inbox-parity.v1","history_item_id":"\(historyID)",
           "requested_delivery":"after_current","effective_delivery":"after_current","reused":false}
          """)
      case ("POST", "/v1/history/\(historyID)/actions/delete"):
        return testHTTPResponse(request: request, status: 200, json:
          #"{"history_item_id":"\#(historyID)","deleted":true}"#)
      case ("POST", "/v1/history/\(historyID)/actions/report"):
        return testHTTPResponse(request: request, status: 201, json:
          #"{"history_item_id":"\#(historyID)","reused":false}"#)
      case ("POST", "/v1/history/\(historyID)/actions/block_actor"):
        return testHTTPResponse(request: request, status: 201, json:
          #"{"block_id":"\#(blockID)","reused":false}"#)
      case ("POST", "/v1/transmissions"):
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-explicit-send-test")
        let body = try #require(request.httpBody)
        let json = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        let audience = try #require(json["audience"] as? [String: Any])
        #expect(audience["kind"] as? String == "explicit")
        #expect(json["include_origin"] as? Bool == false)
        return testHTTPResponse(request: request, status: 201, json: """
          {"transmission_id":"tr_\(String(repeating: "D", count: 26))",
           "media_id":"m_\(String(repeating: "E", count: 26))",
           "requested_delivery":"overlay","effective_delivery":"overlay",
           "status":"accepted","reused":false}
          """)
      default:
        Issue.record("unexpected request \(request.httpMethod ?? "") \(path)")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try TargetsInboxAppClient(bundle: credentialBundle(), transport: transport)
    let projection = try await client.projection()
    #expect(projection.targets.first?.reference == targetRef)
    #expect(projection.inbox.items.first?.historyItemID == historyID)
    #expect(projection.history.items.first?.playedCount == 1)
    #expect(projection.contentPolicyState == "current")
    let receipts = try await client.receipts(historyItemID: historyID, cursor: nil)
    #expect(receipts.items.first?.targetLabel == "Home, Pulsar a")
    #expect(try await client.dismissInbox(inboxID) == "inbox_dismissed")
    #expect(try await client.replayInbox(
      inboxID, delivery: "after_current", idempotencyKey: "mac-inbox-replay-test") == "replay_accepted")
    #expect(try await client.deleteHistory(historyID) == "media_deleted")
    #expect(try await client.reportHistory(historyID, reason: "spam", details: "") == "report_received")
    #expect(try await client.muteHistorySender(
      historyID, idempotencyKey: "mac-mute-sender-test") == "sender_blocked")
    let phaseOne = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)
    let explicit = try await phaseOne.transmitExplicit(
      mediaID: "m_" + String(repeating: "E", count: 26),
      targetReferences: [targetRef], includeOrigin: false, delivery: .overlay,
      originKind: .microphone, idempotencyKey: "mac-explicit-send-test")
    #expect(explicit.effectiveDelivery == .overlay)
    #expect(!transport.requests().contains { $0.url?.path.contains("playback") == true })
  }

  @Test("Missing current policy is represented as required and transport failures stay explicit")
  func policyAndTransportFailure() async throws {
    let transport = ScriptedTransport { request, _ in
      switch request.url?.path {
      case "/v1/transmission-targets":
        return testHTTPResponse(request: request, status: 200, json:
          #"{"contract":"p2-targets-inbox-parity.v1","targets":[]}"#)
      case "/v1/inbox":
        return testHTTPResponse(request: request, status: 200, json:
          #"{"contract":"p2-targets-inbox-parity.v1","items":[]}"#)
      case "/v1/history":
        return testHTTPResponse(request: request, status: 200, json:
          #"{"contract":"p1-history-presence-telegram-v1","items":[]}"#)
      case "/v1/content-policy/acceptance":
        return testHTTPResponse(request: request, status: 428, json:
          #"{"error":{"code":"content_policy_acceptance_required"}}"#)
      default:
        Issue.record("unexpected policy projection request")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try TargetsInboxAppClient(bundle: credentialBundle(), transport: transport)
    #expect(try await client.projection().contentPolicyState == "required")

    let offlineTransport = ScriptedTransport { _, _ in throw TargetsInboxTestError.offline }
    let offline = try TargetsInboxAppClient(
      bundle: credentialBundle(), transport: offlineTransport)
    await #expect(throws: TargetsInboxClientError.transport) {
      _ = try await offline.projection()
    }
  }

  @Test("Malformed opaque capabilities and duplicate stable IDs fail closed")
  func malformedCapabilitiesFailClosed() async throws {
    let inboxID = "ib_" + String(repeating: "A", count: 26)
    let historyID = "hi_" + String(repeating: "B", count: 26)
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: """
        {"contract":"p2-targets-inbox-parity.v1","items":[
          {"id":"\(inboxID)","history_item_id":"\(historyID)",
           "media":{"title":"One"},"availability":"available",
           "expires_at":"2026-07-17T00:00:00.000Z","actions":[],
           "presentation":{"sender":{"key":"sender.one","en":"One","ru":"Один"},
             "source":{"key":"source.one","en":"One","ru":"Один"},
             "requested_delivery":{"key":"delivery.overlay","en":"Overlay","ru":"Микс"},
             "effective_delivery":{"key":"delivery.overlay","en":"Overlay","ru":"Микс"},
             "receipt":{"key":"receipt.one","en":"One","ru":"Один"},"actions":[]}},
          {"id":"\(inboxID)","history_item_id":"\(historyID)",
           "media":{"title":"Duplicate"},"availability":"available",
           "expires_at":"2026-07-17T00:00:00.000Z","actions":[],
           "presentation":{"sender":{"key":"sender.one","en":"One","ru":"Один"},
             "source":{"key":"source.one","en":"One","ru":"Один"},
             "requested_delivery":{"key":"delivery.overlay","en":"Overlay","ru":"Микс"},
             "effective_delivery":{"key":"delivery.overlay","en":"Overlay","ru":"Микс"},
             "receipt":{"key":"receipt.one","en":"One","ru":"Один"},"actions":[]}}],
         "next_cursor":"ic_\(String(repeating: "A", count: 64))"}
        """)
    }
    let client = try TargetsInboxAppClient(bundle: credentialBundle(), transport: transport)

    await #expect(throws: TargetsInboxClientError.invalidRequest) {
      _ = try await client.inbox(cursor: "ic_unsafe")
    }
    #expect(transport.requests().isEmpty)
    await #expect(throws: TargetsInboxClientError.invalidResponse) {
      _ = try await client.inbox(cursor: nil)
    }
    #expect(transport.requests().count == 1)
    await #expect(throws: TargetsInboxClientError.invalidRequest) {
      _ = try await client.replayInbox(
        inboxID, delivery: "overlay", idempotencyKey: ".invalid-leading-key")
    }
    #expect(transport.requests().count == 1)
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

private enum TargetsInboxTestError: Error { case offline }
