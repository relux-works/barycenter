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
    let reportID = "rp_" + String(repeating: "E", count: 26)
    let blockID = "bl_" + String(repeating: "F", count: 26)
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
        #expect(object["rights_acknowledged"] as? Bool == true)
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
        #expect(request.url?.path == "/v1/history/\(historyID)/actions/report")
        let object = try #require(
          JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(object["reason"] as? String == "harassment")
        #expect(object["details"] as? String == "policy evidence")
        return testHTTPResponse(
          request: request, status: 201,
          json: #"{"id":"\#(reportID)","media_id":"\#(mediaID)","history_item_id":"\#(historyID)","reason":"harassment","status":"received","created_at":"2026-07-15T00:00:00Z","updated_at":"2026-07-15T00:00:00Z","reused":false}"#)
      case 7:
        #expect(request.url?.path == "/v1/history/\(historyID)/actions/block_actor")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-history-block-test")
        return testHTTPResponse(
          request: request, status: 201,
          json: #"{"block_id":"\#(blockID)","scope":"actor","subject_ref":"opaque","display_name":"Sender","created_at":"2026-07-15T00:00:00Z","revision":1,"reused":false}"#)
      case 8:
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

    await #expect(throws: PhaseOneClientError.invalidRequest) {
      _ = try await client.upload(
        fileURL: file,
        title: "Pulsar recording",
        idempotencyKey: "mac-upload-without-rights-000000000000000001",
        rightsAcknowledged: false)
    }
    let upload = try await client.upload(
      fileURL: file,
      title: "Pulsar recording",
      idempotencyKey: "mac-upload-00000000000000000000000000000001",
      rightsAcknowledged: true)
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
    let deleted = try await client.deleteHistoryItem(historyID)
    #expect(deleted.outcome == "media_deleted")
    let report = try await client.reportHistoryItem(
      historyID, reason: .harassment, details: "policy evidence")
    #expect(report == .init(outcome: "report_received"))
    let block = try await client.blockHistoryActor(
      historyID, idempotencyKey: "mac-history-block-test")
    #expect(block == .init(outcome: "sender_blocked"))
    try await client.deleteMedia(mediaID)
    #expect(transport.requests().count == 9)
  }

  @Test("Versioned RU and EN content policy is displayed before exact server-owned acceptance")
  func versionedContentPolicyConsent() async throws {
    let policyHash = "a4d59ec7e9bfd8aeb2ec5d84356517580bde8df4540e6a2162f9206cd7ecd30e"
    let localeHash = "a25d1b46b530fb64f18224618701f67ed80ace9ce5c1b1cfb1a7c3d70a1988ca"
    let index = PhaseOneRequestIndex()
    let transport = ScriptedTransport { request, _ in
      switch index.next() {
      case 0:
        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/content-policy")
        #expect(request.url?.query == "locale=en")
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"contract":"p2-content-policy-consent.v1","version":"1.0","policy_hash":"\#(policyHash)","locale":"en","locale_hash":"\#(localeHash)","effective_at":"2026-07-13T20:00:00Z","terms_url":"https://barycenter.live/legal/terms","content_guidelines_url":"https://barycenter.live/legal/content-guidelines","title":"Upload and sharing rights","rights_text":"Only content with rights. Acceptance does not prove ownership.","consent_text":"I accept the current Terms and Content Guidelines.","controlling_language":"en"}"#)
      case 1:
        #expect(request.httpMethod == "PUT")
        #expect(request.url?.path == "/v1/content-policy/acceptance")
        let object = try #require(
          JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(object["version"] as? String == "1.0")
        #expect(object["policy_hash"] as? String == policyHash)
        #expect(object["locale"] as? String == "en")
        #expect(object["terms_accepted"] as? Bool == true)
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"contract":"p2-content-policy-consent.v1","version":"1.0","policy_hash":"\#(policyHash)","locale":"en","accepted_at":"2026-07-16T12:00:00Z","revision":1,"current":true,"terms_accepted":true}"#)
      case 2:
        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/content-policy/acceptance")
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"contract":"p2-content-policy-consent.v1","version":"1.0","policy_hash":"\#(policyHash)","locale":"en","accepted_at":"2026-07-16T12:00:00Z","revision":1,"current":true,"terms_accepted":true}"#)
      case 3:
        #expect(request.httpMethod == "DELETE")
        #expect(request.url?.path == "/v1/content-policy/acceptance")
        #expect(request.url?.query == "locale=en")
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"contract":"p2-content-policy-consent.v1","version":"1.0","policy_hash":"\#(policyHash)","locale":"en","accepted_at":"2026-07-16T12:00:00Z","revoked_at":"2026-07-16T12:01:00Z","revision":2,"current":false,"terms_accepted":true}"#)
      default:
        Issue.record("unexpected content-policy request")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)
    let policy = try await client.contentPolicy(locale: .en)
    #expect(policy.policyHash == policyHash)
    #expect(policy.termsURL.absoluteString == "https://barycenter.live/legal/terms")
    #expect(policy.rightsText.contains("does not prove ownership"))
    let grant = try await client.acceptContentPolicy(policy)
    #expect(grant.current && grant.termsAccepted && grant.revision == 1)
    let current = try await client.currentContentPolicyGrant()
    #expect(current == grant)
    let revoked = try await client.revokeContentPolicy(locale: .en)
    #expect(!revoked.current && revoked.revokedAt != nil && revoked.revision == 2)
    #expect(transport.requests().count == 4)
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

  @Test("Repeated moderation outcomes and bounded report input remain exact")
  func repeatedModerationOutcomes() async throws {
    let historyID = "hi_" + String(repeating: "H", count: 26)
    let reportID = "rp_" + String(repeating: "J", count: 26)
    let blockID = "bl_" + String(repeating: "K", count: 26)
    let requestIndex = PhaseOneRequestIndex()
    let transport = ScriptedTransport { request, _ in
      switch requestIndex.next() {
      case 0:
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"id":"\#(reportID)","media_id":"m_MMMMMMMMMMMMMMMMMMMMMMMMMM","history_item_id":"\#(historyID)","reason":"spam","status":"received","created_at":"2026-07-15T00:00:00Z","updated_at":"2026-07-15T00:00:00Z","reused":true}"#)
      case 1:
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"block_id":"\#(blockID)","scope":"actor","subject_ref":"opaque","display_name":"Sender","created_at":"2026-07-15T00:00:00Z","revision":1,"reused":true}"#)
      default:
        Issue.record("invalid moderation request reached transport")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)
    let report = try await client.reportHistoryItem(historyID, reason: .spam, details: "")
    #expect(report == .init(outcome: "report_already_received", reused: true))
    let block = try await client.blockHistoryActor(
      historyID, idempotencyKey: "mac-history-block-repeated")
    #expect(block == .init(outcome: "sender_already_blocked", reused: true))
    await #expect(throws: PhaseOneClientError.invalidRequest) {
      _ = try await client.reportHistoryItem(
        historyID, reason: .other, details: String(repeating: "x", count: 2_001))
    }
    await #expect(throws: PhaseOneClientError.invalidRequest) {
      _ = try await client.reportHistoryItem(
        historyID, reason: .other, details: " leading whitespace")
    }
    #expect(transport.requests().count == 2)
  }

  @Test("Moderation denial and offline failures remain exact and privacy-safe")
  func moderationFailureContracts() async throws {
    let historyID = "hi_" + String(repeating: "N", count: 26)
    let denied = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 403,
        json: #"{"error":{"code":"insufficient_capability","retry_after_seconds":null}}"#)
    }
    let deniedClient = try PhaseOneAppClient(bundle: credentialBundle(), transport: denied)
    await #expect(
      throws: PhaseOneClientError.rejected(
        status: 403, code: "insufficient_capability", retryAfterSeconds: nil)
    ) {
      _ = try await deniedClient.reportHistoryItem(historyID, reason: .spam, details: "")
    }
    #expect(denied.requests().count == 1)

    let offline = ScriptedTransport { _, _ in throw PhaseOneTestTransportError.offline }
    let offlineClient = try PhaseOneAppClient(bundle: credentialBundle(), transport: offline)
    await #expect(throws: PhaseOneClientError.transport) {
      _ = try await offlineClient.blockHistoryActor(
        historyID, idempotencyKey: "mac-history-block-offline")
    }
    #expect(offline.requests().count == 1)
  }

  @Test("Long tracks require rights and resume in bounded authenticated chunks")
  func resumableLongTrackUpload() async throws {
    let root = FileManager.default.temporaryDirectory
      .appendingPathComponent("phase-one-track-\(UUID().uuidString)", isDirectory: true)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: root) }
    let file = root.appendingPathComponent("long-track.mp3")
    FileManager.default.createFile(atPath: file.path, contents: nil)
    let writer = try FileHandle(forWritingTo: file)
    let resumedOffset = Int64(PhaseOneAppClient.streamTrackChunkBytes)
    let total = resumedOffset * 2 + 17
    try writer.truncate(atOffset: UInt64(total))
    try writer.close()

    let uploadID = "up_" + String(repeating: "A", count: 26)
    let mediaID = "m_" + String(repeating: "B", count: 26)
    let index = PhaseOneRequestIndex()
    let progress = TrackProgressRecorder()
    let transport = ScriptedTransport { request, maximum in
      #expect(maximum == 64 * 1_024)
      switch index.next() {
      case 0:
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/media/uploads")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "track:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        let object = try #require(
          JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(object["kind"] as? String == "audio_track")
        #expect(object["size_bytes"] as? Int == Int(total))
        #expect(object["rights_acknowledged"] as? Bool == true)
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"upload_id":"\#(uploadID)","media_id":"\#(mediaID)","upload_token":"\#(testNodeToken)","upload_offset":\#(resumedOffset),"upload_length":\#(total),"status":"open","reused":true}"#)
      case 1:
        #expect(request.httpMethod == "PUT")
        #expect(request.url?.path == "/v1/media/uploads/\(uploadID)")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testNodeToken)")
        #expect(request.value(forHTTPHeaderField: "Upload-Offset") == String(resumedOffset))
        #expect(request.httpBody?.count == PhaseOneAppClient.streamTrackChunkBytes)
        #expect(request.timeoutInterval == 15 * 60)
        let offset = resumedOffset + Int64(PhaseOneAppClient.streamTrackChunkBytes)
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"upload_id":"\#(uploadID)","media_id":"\#(mediaID)","upload_offset":\#(offset),"upload_length":\#(total),"status":"open"}"#)
      case 2:
        let offset = resumedOffset + Int64(PhaseOneAppClient.streamTrackChunkBytes)
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testNodeToken)")
        #expect(request.value(forHTTPHeaderField: "Upload-Offset") == String(offset))
        #expect(request.httpBody?.count == 17)
        return testHTTPResponse(
          request: request, status: 200,
          json: #"{"upload_id":"\#(uploadID)","media_id":"\#(mediaID)","upload_offset":\#(total),"upload_length":\#(total),"status":"completed"}"#)
      default:
        Issue.record("unexpected long-track request")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)

    await #expect(throws: PhaseOneClientError.invalidRequest) {
      _ = try await client.uploadTrack(
        fileURL: file, title: "Long track",
        idempotencyKey: "track:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        rightsAcknowledged: false)
    }
    let receipt = try await client.uploadTrack(
      fileURL: file, title: "Long track",
      idempotencyKey: "track:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      rightsAcknowledged: true,
      progress: { progress.append(offset: $0, length: $1) })
    #expect(receipt == .init(mediaID: mediaID, reused: true))
    #expect(progress.values() == [resumedOffset, resumedOffset * 2, total])
    #expect(transport.requests().count == 3)
  }

  @Test("Soundboard CRUD, automation history, and interrupt confirmation stay on canonical control auth")
  func soundboardContracts() async throws {
    let cueID = "cq_" + String(repeating: "A", count: 26)
    let mediaID = "m_" + String(repeating: "B", count: 26)
    let executionID = "mx_" + String(repeating: "C", count: 26)
    let transmissionID = "tr_" + String(repeating: "D", count: 26)
    let historyID = "hi_" + String(repeating: "E", count: 26)
    let hash = String(repeating: "a", count: 64)
    let token = "fc_" + String(repeating: "b", count: 64)
    let index = PhaseOneRequestIndex()
    let transport = ScriptedTransport { request, _ in
      #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
      switch index.next() {
      case 0:
        #expect(request.httpMethod == "GET" && request.url?.path == "/v1/soundboard/cues")
        return testHTTPResponse(request: request, status: 200,
          json: #"{"order_revision":1,"cues":[{"cue_id":"\#(cueID)","title":"Bell","source_kind":"media","media_id":"\#(mediaID)","source_sha256":"\#(hash)","source_bytes":1024,"source_duration_ms":500,"state":"active","revision":1,"source_generation":1,"position":0}]}"#)
      case 1:
        #expect(request.httpMethod == "POST" && request.url?.path == "/v1/soundboard/cues")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-soundboard-create-test")
        return testHTTPResponse(request: request, status: 201,
          json: #"{"cue":{"cue_id":"\#(cueID)","title":"Bell","source_kind":"media","media_id":"\#(mediaID)","source_sha256":"\#(hash)","source_bytes":1024,"source_duration_ms":500,"state":"active","revision":1,"source_generation":1},"order_revision":1,"replayed":false}"#)
      case 2:
        #expect(request.url?.path == "/v1/history")
        return testHTTPResponse(request: request, status: 200,
          json: #"{"contract":"p1-history-presence-telegram-v1","items":[{"history_item_id":"\#(historyID)","item_kind":"automation_attempt","direction":"sent","occurred_at":"2026-07-17T00:00:00.000Z","media":{"title":"Bell"},"status":"denied","reason_code":"automation_disabled","automation":{"trigger_kind":"schedule","schedule_id":"sch_AAAAAAAAAAAAAAAAAAAAAAAAAA","schedule_label":"Morning","schedule_revision":2,"cue_id":"\#(cueID)","cue_label":"Bell","cue_revision":1,"resolved_target_count":0,"outcome":"denied","reason_code":"automation_disabled"},"actions":["disable_schedule","emergency_disable_automation"]}]}"#)
      case 3:
        #expect(request.url?.path == "/v1/soundboard/cues/\(cueID)/trigger")
        return testHTTPResponse(request: request, status: 409,
          json: #"{"error":{"code":"requires_confirmation","message":"confirm","details":{"confirmation_token":"\#(token)","alternatives":[{"delivery":"overlay","available":false},{"delivery":"after_current","available":true}]}}}"#)
      case 4:
        let object = try #require(JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        let fallback = try #require(object["fallback_confirmation"] as? [String: Any])
        #expect(fallback["token"] as? String == token)
        #expect(fallback["delivery"] as? String == "after_current")
        return testHTTPResponse(request: request, status: 201,
          json: #"{"execution_id":"\#(executionID)","transmission_id":"\#(transmissionID)","requested_delivery":"interrupt","effective_delivery":"after_current","downgrade_reason":"confirmed_fallback","status":"accepted","reused":false}"#)
      default:
        Issue.record("unexpected soundboard request")
        return testHTTPResponse(request: request, status: 500, json: "{}")
      }
    }
    let client = try PhaseOneAppClient(bundle: credentialBundle(), transport: transport)
    #expect(try await client.soundboardCues().cues.first?.id == cueID)
    _ = try await client.createSoundboardMediaCue(
      title: "Bell", mediaID: mediaID, idempotencyKey: "mac-soundboard-create-test")
    let history = try await client.history(limit: 30, cursor: nil)
    #expect(history.items.first?.automation?.scheduleLabel == "Morning")
    #expect(history.items.first?.actions.contains("emergency_disable_automation") == true)
    let key = "mac-soundboard-trigger-test"
    do {
      _ = try await client.triggerSoundboardCue(
        cueID, intent: .init(route: .ownBarycenter, delivery: .interrupt, includeOrigin: true),
        idempotencyKey: key)
      Issue.record("confirmation challenge was not surfaced")
    } catch let challenge as PhaseOneConfirmationChallenge {
      #expect(challenge.token == token && challenge.alternatives == [.afterCurrent])
    }
    let receipt = try await client.triggerSoundboardCue(
      cueID, intent: .init(
        route: .ownBarycenter, delivery: .interrupt, includeOrigin: true,
        fallback: .init(token: token, delivery: .afterCurrent)), idempotencyKey: key)
    #expect(receipt.executionID == executionID)
    #expect(receipt.transmission.effectiveDelivery == .afterCurrent)
    #expect(transport.requests().count == 5)
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

private enum PhaseOneTestTransportError: Error {
  case offline
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

private final class TrackProgressRecorder: @unchecked Sendable {
  private let lock = NSLock()
  private var offsets: [Int64] = []

  func append(offset: Int64, length: Int64) {
    lock.withLock {
      #expect(offset >= 0 && offset <= length)
      offsets.append(offset)
    }
  }

  func values() -> [Int64] { lock.withLock { offsets } }
}
