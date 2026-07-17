import Foundation
import Testing

@testable import NodeCore

@Suite("macOS automation administration client")
struct AutomationAdminClientTests {
  @Test("Calendar rules preserve spring gaps, earliest fall folds and wrapping quiet hours")
  func calendarRules() throws {
    #expect(try AutomationScheduleRules.parseWeekdays("Sun,Mon") == [0, 1])
    #expect(AutomationScheduleRules.formatWeekdays([0, 1, 6]) == "Sun,Mon,Sat")
    let quiet = try AutomationScheduleRules.parseQuietHours("Sun 22:00-07:00; Mon 08:00-09:00")
    #expect(AutomationScheduleRules.formatQuietHours(quiet) == "Sun 22:00-07:00; Mon 08:00-09:00")

    let spring = schedule(
      timezone: "Europe/Berlin", weekdays: [0], localTime: "02:30", enabled: true)
    let springStart = try #require(ISO8601DateFormatter().date(from: "2026-03-28T23:00:00Z"))
    let springNext = try #require(AutomationScheduleRules.nextRun(for: spring, after: springStart))
    #expect(ISO8601DateFormatter().string(from: springNext) == "2026-04-05T00:30:00Z")

    let fall = schedule(
      timezone: "Europe/Berlin", weekdays: [0], localTime: "02:30", enabled: true)
    let fallStart = try #require(ISO8601DateFormatter().date(from: "2026-10-24T23:59:00Z"))
    let fallNext = try #require(AutomationScheduleRules.nextRun(for: fall, after: fallStart))
    #expect(ISO8601DateFormatter().string(from: fallNext) == "2026-10-25T00:30:00Z")
    #expect(AutomationScheduleRules.isQuiet(
      at: try #require(ISO8601DateFormatter().date(from: "2026-10-25T22:30:00Z")),
      timezone: "UTC", windows: [quiet[0]]))
  }

  @Test("Feature and schedule APIs send authenticated CAS and idempotency contracts")
  func featureAndScheduleContracts() async throws {
    let index = LockedIndex()
    let scheduleID = "sch_" + String(repeating: "A", count: 26)
    let cueID = "cq_" + String(repeating: "B", count: 26)
    let transport = AdminScriptTransport { request, maximum in
      #expect(maximum == 64 * 1_024)
      #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(adminControlToken)")
      switch index.next() {
      case 0:
        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/automation/status")
        return response(request, 200, featureJSON(revision: 7))
      case 1:
        #expect(request.httpMethod == "PUT")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-feature-test-0001")
        let body = try #require(JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(body["expected_revision"] as? Int == 7)
        #expect(body["timezone"] as? String == "Asia/Yerevan")
        return response(request, 200, featureJSON(revision: 8))
      case 2:
        #expect(request.url?.path == "/v1/automation/schedules")
        return response(request, 200, "{\"schedules\":[" + scheduleJSON(scheduleID: scheduleID, cueID: cueID, revision: 2) + "]}")
      case 3:
        #expect(request.httpMethod == "POST")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-schedule-create-0001")
        let body = try #require(JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(body["policy_revision"] as? Int == 8)
        #expect((body["audience"] as? [String: Any])?["kind"] as? String == "own_barycenter")
        #expect(body["expected_revision"] == nil)
        return response(request, 201, "{\"schedule\":" + scheduleJSON(scheduleID: scheduleID, cueID: cueID, revision: 1) + "}")
      case 4:
        #expect(request.httpMethod == "PUT")
        let body = try #require(JSONSerialization.jsonObject(with: request.httpBody!) as? [String: Any])
        #expect(body["expected_revision"] as? Int == 2)
        return response(request, 200, "{\"schedule\":" + scheduleJSON(scheduleID: scheduleID, cueID: cueID, revision: 3) + "}")
      case 5:
        #expect(request.url?.path == "/v1/automation/schedules/\(scheduleID)/disable")
        return response(request, 200, "{\"schedule\":" + scheduleJSON(scheduleID: scheduleID, cueID: cueID, revision: 3, enabled: false) + "}")
      case 6:
        #expect(request.httpMethod == "DELETE")
        return response(request, 200, "{\"schedule\":" + scheduleJSON(scheduleID: scheduleID, cueID: cueID, revision: 4, enabled: false) + "}")
      default:
        Issue.record("unexpected automation request")
        return response(request, 500, #"{"error":{"code":"unexpected"}}"#)
      }
    }
    let client = try AutomationAdminClient(bundle: bundle(), transport: transport)
    var feature = try await client.feature()
    #expect(feature.revision == 7)
    feature.timezone = "Asia/Yerevan"
    feature = try await client.replaceFeature(feature, idempotencyKey: "mac-feature-test-0001")
    #expect(feature.revision == 8)
    let listed = try await client.schedules()
    #expect(listed.map(\.id) == [scheduleID])
    let draft = AutomationScheduleDraft(
      cueID: cueID, displayName: "Morning", timezone: "Asia/Yerevan",
      weekdays: [1, 2, 3, 4, 5], localTime: "09:00", policyRevision: 8)
    _ = try await client.createSchedule(draft, idempotencyKey: "mac-schedule-create-0001")
    _ = try await client.replaceSchedule(
      listed[0], with: draft, idempotencyKey: "mac-schedule-replace-0001")
    _ = try await client.setScheduleEnabled(
      listed[0], enabled: false, idempotencyKey: "mac-schedule-toggle-0001")
    _ = try await client.deleteSchedule(listed[0], idempotencyKey: "mac-schedule-delete-0001")
    #expect(index.value == 7)
  }

  @Test("Principal issuance is one-time, redacted and revocation/history use canonical paths")
  func principalContracts() async throws {
    let index = LockedIndex()
    let principalID = "ap_" + String(repeating: "C", count: 26)
    let cueID = "cq_" + String(repeating: "D", count: 26)
    let historyID = "hi_" + String(repeating: "E", count: 26)
    let secret = String(repeating: "a", count: 64)
    let transport = AdminScriptTransport { request, _ in
      switch index.next() {
      case 0:
        return response(request, 200, "{\"principals\":[" + principalJSON(principalID: principalID, cueID: cueID, revision: 2) + "]}")
      case 1:
        #expect(request.httpMethod == "POST")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-principal-issue-0001")
        return response(request, 201, "{\"principal\":" + principalJSON(principalID: principalID, cueID: cueID, revision: 1) + ",\"secret\":\"" + secret + "\",\"secret_available\":true}")
      case 2:
        #expect(request.httpMethod == "POST")
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key") == "mac-principal-replay-0001")
        return response(request, 201, "{\"principal\":" + principalJSON(principalID: principalID, cueID: cueID, revision: 1) + ",\"secret_available\":false}")
      case 3:
        #expect(request.url?.path == "/v1/automation/principals/\(principalID)/revoke")
        return response(request, 200, "{\"principal\":" + principalJSON(principalID: principalID, cueID: cueID, revision: 3, revokedAt: "2026-08-02T00:00:00Z") + "}")
      case 4:
        #expect(request.url?.path == "/v1/history/\(historyID)/actions/cancel")
        return response(request, 200, "{\"history_item_id\":\"" + historyID + "\",\"status\":\"cancelled\"}")
      default:
        Issue.record("unexpected principal request")
        return response(request, 500, #"{"error":{"code":"unexpected"}}"#)
      }
    }
    let client = try AutomationAdminClient(bundle: bundle(), transport: transport)
    let principals = try await client.principals()
    let issue = try await client.issuePrincipal(
      .init(
        displayName: "Build agent", allowedCueIDs: [cueID],
        expiresAt: try #require(ISO8601DateFormatter().date(from: "2026-08-01T00:00:00Z"))),
      idempotencyKey: "mac-principal-issue-0001")
    #expect(issue.secretAvailable)
    #expect(issue.secret?.revealForExplicitCopy() == secret)
    #expect(!issue.description.contains(secret))
    #expect(!String(reflecting: issue).contains(secret))
    let replay = try await client.issuePrincipal(
      .init(
        displayName: "Build agent", allowedCueIDs: [cueID],
        expiresAt: try #require(ISO8601DateFormatter().date(from: "2026-08-01T00:00:00Z"))),
      idempotencyKey: "mac-principal-replay-0001")
    #expect(!replay.secretAvailable)
    #expect(replay.secret == nil)
    let revoked = try await client.revokePrincipal(
      principals[0], idempotencyKey: "mac-principal-revoke-0001")
    #expect(revoked.revokedAt != nil)
    try await client.cancelHistory(historyID)
    #expect(index.value == 5)
  }

  @Test("macOS source keeps secrets out of projections and retains explicit authorization controls")
  func sourceSafetyBoundaries() throws {
    let root = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent().deletingLastPathComponent()
      .deletingLastPathComponent().deletingLastPathComponent()
    let ui = try String(contentsOf:
      root.appendingPathComponent("node-app/Sources/NodeAppUI/PulsarMainWindow.swift"),
      encoding: .utf8)
    let model = try String(contentsOf:
      root.appendingPathComponent("node-app/Sources/NodeAppUI/PulsarShellModel.swift"),
      encoding: .utf8)
    let composition = try String(contentsOf:
      root.appendingPathComponent("node-app/Sources/NodeApp/MacAutomationAdminAppComposition.swift"),
      encoding: .utf8)
    let main = try String(contentsOf:
      root.appendingPathComponent("node-app/Sources/NodeApp/main.swift"), encoding: .utf8)

    #expect(ui.contains(".confirmationDialog("))
    #expect(ui.contains("One-time secret available"))
    #expect(ui.contains("Manual soundboard"))
    #expect(!ui.contains("text: $secret"))
    #expect(!model.contains("public var secret: String"))
    #expect(composition.contains("private var secret: AutomationPrincipalSecret?"))
    #expect(composition.contains("apply(result)"))
    #expect(composition.contains("clearAuthorizationProjection"))
    #expect(composition.contains("pasteboard.changeCount == changeCount"))
    #expect(main.contains("openAutomationAdmin: { showShellSection(.automation) }"))
  }

  private func schedule(
    timezone: String, weekdays: [Int], localTime: String, enabled: Bool
  ) -> AutomationSchedule {
    AutomationSchedule(
      id: "sch_" + String(repeating: "A", count: 26),
      cueID: "cq_" + String(repeating: "B", count: 26), displayName: "Schedule",
      timezone: timezone, weekdays: weekdays, localTime: localTime,
      audience: .ownBarycenter, targetDigests: [], airID: nil,
      additionalQuietHours: [], policyVersion: "policy-v1", policyRevision: 1,
      enabled: enabled, revision: 1,
      createdAt: Date(timeIntervalSince1970: 0), updatedAt: Date(timeIntervalSince1970: 0))
  }

  private func bundle() throws -> CredentialBundle {
    CredentialBundle(
      coordinatorOrigin: try CoordinatorOrigin("https://coordinator.example"),
      control: .init(
        actorId: 1, orbitId: 2, role: .primary,
        controlToken: adminControlToken, contextStrength: .active))
  }
}

private let adminControlToken = String(repeating: "1", count: 64)

private final class LockedIndex: @unchecked Sendable {
  private let lock = NSLock()
  private var storage = 0
  func next() -> Int {
    lock.withLock {
      defer { storage += 1 }
      return storage
    }
  }
  var value: Int { lock.withLock { storage } }
}

private final class AdminScriptTransport: OnboardingHTTPTransport, @unchecked Sendable {
  private let handler: @Sendable (URLRequest, Int) throws -> HTTPTransportResponse
  init(_ handler: @escaping @Sendable (URLRequest, Int) throws -> HTTPTransportResponse) {
    self.handler = handler
  }
  func send(_ request: URLRequest, maximumResponseBytes: Int) async throws -> HTTPTransportResponse {
    try handler(request, maximumResponseBytes)
  }
}

private func response(_ request: URLRequest, _ status: Int, _ json: String) -> HTTPTransportResponse {
  HTTPTransportResponse(
    data: Data(json.utf8),
    response: HTTPURLResponse(
      url: request.url!, statusCode: status, httpVersion: "HTTP/1.1",
      headerFields: ["Content-Type": "application/json"])!)
}

private func featureJSON(revision: Int) -> String {
  jsonString([
    "soundboard_enabled": true, "automation_enabled": true,
    "emergency_disabled": false, "timezone": "UTC", "quiet_hours": [],
    "policy_version": "policy-v1", "revision": revision, "policy_valid": true,
    "updated_at": "2026-07-17T00:00:00Z",
  ])
}

private func scheduleJSON(
  scheduleID: String, cueID: String, revision: Int, enabled: Bool = true
) -> String {
  jsonString([
    "schedule_id": scheduleID, "cue_id": cueID, "display_name": "Morning",
    "timezone": "Asia/Yerevan", "weekdays": [1, 2, 3, 4, 5],
    "local_time": "09:00",
    "audience": ["kind": "own_barycenter", "target_digests": [], "air_id": ""],
    "delivery": "overlay", "additional_quiet_hours": [],
    "policy_version": "policy-v1", "policy_revision": 8,
    "enabled": enabled, "revision": revision,
    "created_at": "2026-07-17T00:00:00Z", "updated_at": "2026-07-17T00:00:00Z",
  ])
}

private func principalJSON(
  principalID: String, cueID: String, revision: Int, revokedAt: String = ""
) -> String {
  jsonString([
    "principal_id": principalID, "display_name": "Build agent", "permission": "trigger",
    "bound_air_id": "", "max_target_count": 1, "allowed_cue_ids": [cueID],
    "allowed_audience_kinds": ["own_barycenter"], "allowed_target_digests": [],
    "issued_at": "2026-07-17T00:00:00Z", "expires_at": "2026-08-17T00:00:00Z",
    "disabled_at": "", "revoked_at": revokedAt, "revision": revision,
  ])
}

private func jsonString(_ object: [String: Any]) -> String {
  String(
    decoding: try! JSONSerialization.data(withJSONObject: object, options: [.sortedKeys]),
    as: UTF8.self)
}
