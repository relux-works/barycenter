import Foundation
import Testing

@testable import NodeCore

@Suite("macOS Air control client")
struct AirAppClientTests {
  private let airA = "air_" + String(repeating: "A", count: 26)
  private let airB = "air_" + String(repeating: "B", count: 26)
  private let memberA = "aim_" + String(repeating: "C", count: 26)
  private let inviteA = "ai_" + String(repeating: "D", count: 26)

  @Test("Coordinator health exposes typed Air availability")
  func availabilityGate() async throws {
    let transport = ScriptedTransport { request, _ in
      #expect(request.httpMethod == "GET")
      #expect(request.url?.path == "/healthz")
      return testHTTPResponse(
        request: request,
        status: 200,
        json: #"{"phase2":{"air_rooms_enabled":false,"air_authority_state":"airs_shadow"}}"#)
    }
    let client = try AirAppClient(bundle: credentialBundle(), transport: transport)
    let availability = try await client.availability()
    #expect(!availability.enabled)
    #expect(availability.authorityState == "airs_shadow")
  }

  @Test("Every lifecycle action uses the common authenticated Air API")
  func lifecycleWireContract() async throws {
    let index = AirRequestIndex()
    let transport = ScriptedTransport { request, maximum in
      #expect(maximum == 64 * 1_024)
      #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer \(testControlToken)")
      #expect(request.url?.query == nil)
      let current = index.next()
      if current >= 2 {
        #expect(request.value(forHTTPHeaderField: "Idempotency-Key")?.hasPrefix("mac-air-test-") == true)
        #expect(request.value(forHTTPHeaderField: "Content-Type") == "application/json")
      }
      switch current {
      case 0:
        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/airs")
        return testHTTPResponse(request: request, status: 200, json: self.listJSON)
      case 1:
        #expect(request.url?.path == "/v1/airs/\(self.airA)")
        return testHTTPResponse(request: request, status: 200, json: self.detailJSON)
      case 2:
        #expect(request.url?.path == "/v1/airs")
        #expect(try self.object(request)["title"] as? String == "Family Air")
        return testHTTPResponse(request: request, status: 201, json: self.detailJSON)
      case 3:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/invites")
        #expect(try self.object(request)["air_role"] as? String == "member")
        return testHTTPResponse(request: request, status: 201, json:
          #"{"invite_id":"\#(self.inviteA)","revision":1,"expires_at":"2026-07-15T12:15:00.000Z","code":"one-time-secret"}"#)
      case 4:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/invites/\(self.inviteA)/withdraw")
        #expect(try self.object(request)["invite_revision"] as? Int == 1)
        return testHTTPResponse(request: request, status: 200, json:
          #"{"invite_id":"\#(self.inviteA)","revision":2,"expires_at":"2026-07-15T12:15:00.000Z"}"#)
      case 5:
        #expect(request.url?.path == "/v1/air-invites/consume")
        #expect(try self.object(request)["code"] as? String == "one-time-secret")
        return testHTTPResponse(request: request, status: 202, json: self.previewJSON)
      case 6:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/join/confirm")
        let body = try self.object(request)
        #expect(body["membership_revision"] as? Int == 1)
        #expect(body["activate"] as? Bool == true)
        #expect(body["expected_active_air_id"] as? String == self.airB)
      case 7:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/join/decline")
      case 8:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/activate")
        #expect(try self.object(request)["expected_active_air_id"] as? String == "none")
      case 9:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/deactivate")
        #expect(try self.object(request)["expected_active_air_id"] as? String == self.airA)
      case 10:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/leave")
      case 11:
        #expect(request.httpMethod == "PUT")
        #expect(request.url?.path == "/v1/airs/\(self.airA)/policy")
        #expect(try self.object(request)["policy_revision"] as? Int == 1)
      case 12:
        #expect(request.url?.path == "/v1/airs/\(self.airA)/dissolve")
        #expect(try self.object(request)["air_revision"] as? Int == 3)
      default:
        Issue.record("unexpected request \(current)")
      }
      return testHTTPResponse(request: request, status: 200, json: "{}")
    }
    let client = try AirAppClient(bundle: credentialBundle(), transport: transport)
    let list = try await client.list()
    #expect(list.currentAirID == airA)
    #expect(list.saved.count == 1)
    #expect(list.saved[0].title == "Family Air")
    let detail = try await client.detail(id: airA)
    #expect(detail.role == .owner)
    #expect(detail.policy.overlay == .primaryCompanion)
    _ = try await client.create(title: " Family Air ", idempotencyKey: key(2))
    let invite = try await client.issueInvite(
      airID: airA, role: .member, idempotencyKey: key(3))
    #expect(invite.code == "one-time-secret")
    #expect(!String(describing: invite).contains("one-time-secret"))
    #expect(!String(reflecting: invite).contains("one-time-secret"))
    try await client.withdrawInvite(
      airID: airA, inviteID: inviteA, revision: 1, idempotencyKey: key(4))
    let preview = try await client.consumeInvite(code: " one-time-secret ", idempotencyKey: key(5))
    #expect(preview.ownerDisplayName == "Ivan")
    #expect(preview.activationWouldSwitch)
    try await client.confirmJoin(
      airID: airA, membershipRevision: 1, activate: true,
      expectedActiveAirID: airB, idempotencyKey: key(6))
    try await client.declineJoin(airID: airA, membershipRevision: 1, idempotencyKey: key(7))
    try await client.activate(
      airID: airA, membershipRevision: 1, expectedActiveAirID: nil, idempotencyKey: key(8))
    try await client.deactivate(
      airID: airA, membershipRevision: 1, expectedActiveAirID: airA, idempotencyKey: key(9))
    try await client.leave(
      airID: airA, membershipRevision: 1, expectedActiveAirID: airA, idempotencyKey: key(10))
    try await client.replacePolicy(
      airID: airA,
      policy: .init(
        revision: 1, invite: .ownerPrimary, overlay: .disabled,
        queue: .primaryCompanion, replace: .airAdminPrimary),
      idempotencyKey: key(11))
    try await client.dissolve(airID: airA, revision: 3, idempotencyKey: key(12))
    #expect(transport.requests().count == 13)
  }

  @Test("Canonical failures and inconsistent active projections are rejected")
  func failureAndProjectionBoundaries() async throws {
    let denied = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 409,
        json: #"{"error":{"code":"active_air_changed"}}"#)
    }
    let deniedClient = try AirAppClient(bundle: credentialBundle(), transport: denied)
    await #expect(throws: AirClientError.rejected(
      status: 409, code: "active_air_changed", retryAfterSeconds: nil
    )) {
      _ = try await deniedClient.list()
    }

    let inconsistent = ScriptedTransport { request, _ in
      let json = self.listJSON.replacingOccurrences(of: "\"is_current\":true", with: "\"is_current\":false")
      return testHTTPResponse(request: request, status: 200, json: json)
    }
    let inconsistentClient = try AirAppClient(bundle: credentialBundle(), transport: inconsistent)
    await #expect(throws: AirClientError.invalidResponse) {
      _ = try await inconsistentClient.list()
    }

    let offline = ScriptedTransport { _, _ in throw AirTestError.offline }
    let offlineClient = try AirAppClient(bundle: credentialBundle(), transport: offline)
    await #expect(throws: AirClientError.transport) { _ = try await offlineClient.list() }
  }

  private func key(_ index: Int) -> String { "mac-air-test-000000\(index)" }

  private func object(_ request: URLRequest) throws -> [String: Any] {
    let body = try #require(request.httpBody)
    return try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
  }

  private func credentialBundle() throws -> CredentialBundle {
    CredentialBundle(
      coordinatorOrigin: try CoordinatorOrigin("https://coord.example"),
      control: ControlCapability(
        actorId: 2, orbitId: 1, role: .primary,
        controlToken: testControlToken, contextStrength: .active))
  }

  private var capacityJSON: String { #"{"barycenters":8,"online_pulsars":20}"# }
  private var policyJSON: String {
    #"{"revision":1,"invite":"air_admin_primary","overlay":"primary_companion","queue":"primary_companion","replace":"air_admin_primary"}"#
  }
  private var listJSON: String {
    #"{"current_air_id":"\#(airA)","active_pointer_revision":2,"saved":[{"air_id":"\#(airA)","title":"Family Air","status":"active","membership_status":"joined","air_role":"owner","member_count":2,"active_member_count":2,"online_pulsar_count":3,"capacity":\#(capacityJSON),"policy_revision":1,"is_current":true}]}"#
  }
  private var detailJSON: String {
    #"{"air_id":"\#(airA)","title":"Family Air","status":"active","revision":3,"membership_id":"\#(memberA)","membership_status":"joined","membership_revision":1,"air_role":"owner","member_count":2,"active_member_count":2,"online_pulsar_count":3,"capacity":\#(capacityJSON),"policy":\#(policyJSON),"is_current":true}"#
  }
  private var previewJSON: String {
    #"{"air_id":"\#(airA)","title":"Family Air","owner_display_name":"Ivan","air_role":"member","membership_id":"\#(memberA)","membership_revision":1,"policy":\#(policyJSON),"member_count":2,"capacity":\#(capacityJSON),"activation_would_switch":true}"#
  }
}

private enum AirTestError: Error { case offline }

private final class AirRequestIndex: @unchecked Sendable {
  private let lock = NSLock()
  private var value = 0
  func next() -> Int {
    lock.withLock {
      defer { value += 1 }
      return value
    }
  }
}
