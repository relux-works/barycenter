import Foundation

public enum AirClientError: Error, Equatable, Sendable {
  case invalidConfiguration
  case invalidRequest
  case transport
  case redirectRejected
  case responseTooLarge
  case invalidResponse
  case rejected(status: Int, code: String, retryAfterSeconds: Int?)
}

public enum AirRole: String, Codable, CaseIterable, Sendable {
  case owner, admin, member
}

public enum AirMembershipStatus: String, Codable, Sendable {
  case pendingConfirmation = "pending_confirmation"
  case joined
}

public enum AirInvitePolicy: String, Codable, CaseIterable, Sendable {
  case ownerPrimary = "owner_primary"
  case airAdminPrimary = "air_admin_primary"
  case allMemberPrimaries = "all_member_primaries"
}

public enum AirPlaybackPolicy: String, Codable, CaseIterable, Sendable {
  case ownerPrimary = "owner_primary"
  case airAdminPrimary = "air_admin_primary"
  case allMemberPrimaries = "all_member_primaries"
  case primaryCompanion = "primary_companion"
  case disabled
}

public struct AirCapacity: Equatable, Sendable {
  public let barycenters: Int
  public let onlinePulsars: Int

  public init(barycenters: Int, onlinePulsars: Int) {
    self.barycenters = barycenters
    self.onlinePulsars = onlinePulsars
  }
}

public struct AirPolicy: Equatable, Sendable {
  public let revision: Int64
  public let invite: AirInvitePolicy
  public let overlay: AirPlaybackPolicy
  public let queue: AirPlaybackPolicy
  public let replace: AirPlaybackPolicy

  public init(
    revision: Int64,
    invite: AirInvitePolicy,
    overlay: AirPlaybackPolicy,
    queue: AirPlaybackPolicy,
    replace: AirPlaybackPolicy
  ) {
    self.revision = revision
    self.invite = invite
    self.overlay = overlay
    self.queue = queue
    self.replace = replace
  }
}

public struct AirSummary: Equatable, Identifiable, Sendable {
  public let id: String
  public let title: String
  public let status: String
  public let membershipStatus: AirMembershipStatus
  public let role: AirRole
  public let memberCount: Int
  public let activeMemberCount: Int
  public let onlinePulsarCount: Int
  public let capacity: AirCapacity
  public let policyRevision: Int64
  public let isCurrent: Bool
}

public struct AirList: Equatable, Sendable {
  public let currentAirID: String?
  public let activePointerRevision: Int64
  public let saved: [AirSummary]
}

public struct AirDetail: Equatable, Identifiable, Sendable {
  public let id: String
  public let title: String
  public let status: String
  public let revision: Int64
  public let membershipID: String
  public let membershipStatus: AirMembershipStatus
  public let membershipRevision: Int64
  public let role: AirRole
  public let memberCount: Int
  public let activeMemberCount: Int
  public let onlinePulsarCount: Int
  public let capacity: AirCapacity
  public let policy: AirPolicy
  public let isCurrent: Bool
}

public struct AirInvite: Equatable, Sendable, CustomStringConvertible, CustomDebugStringConvertible {
  public let id: String
  public let revision: Int64
  public let expiresAt: Date
  public let code: String

  public var description: String {
    "AirInvite(id: \(id), revision: \(revision), expiresAt: \(expiresAt), code: <redacted>)"
  }
  public var debugDescription: String { description }
}

public struct AirJoinPreview: Equatable, Sendable {
  public let airID: String
  public let title: String
  public let ownerDisplayName: String
  public let intendedRole: AirRole
  public let membershipRevision: Int64
  public let policy: AirPolicy
  public let memberCount: Int
  public let capacity: AirCapacity
  public let activationWouldSwitch: Bool
}

public struct AirFeatureAvailability: Equatable, Sendable {
  public let enabled: Bool
  public let authorityState: String

  public init(enabled: Bool, authorityState: String) {
    self.enabled = enabled
    self.authorityState = authorityState
  }
}

public protocol AirAppServicing: Sendable {
  func availability() async throws -> AirFeatureAvailability
  func list() async throws -> AirList
  func detail(id: String) async throws -> AirDetail
  func create(title: String, idempotencyKey: String) async throws -> AirDetail
  func issueInvite(airID: String, role: AirRole, idempotencyKey: String) async throws -> AirInvite
  func withdrawInvite(
    airID: String, inviteID: String, revision: Int64, idempotencyKey: String
  ) async throws
  func consumeInvite(code: String, idempotencyKey: String) async throws -> AirJoinPreview
  func confirmJoin(
    airID: String, membershipRevision: Int64, activate: Bool,
    expectedActiveAirID: String?, idempotencyKey: String
  ) async throws
  func declineJoin(
    airID: String, membershipRevision: Int64, idempotencyKey: String
  ) async throws
  func activate(
    airID: String, membershipRevision: Int64, expectedActiveAirID: String?,
    idempotencyKey: String
  ) async throws
  func deactivate(
    airID: String, membershipRevision: Int64, expectedActiveAirID: String,
    idempotencyKey: String
  ) async throws
  func leave(
    airID: String, membershipRevision: Int64, expectedActiveAirID: String?,
    idempotencyKey: String
  ) async throws
  func replacePolicy(airID: String, policy: AirPolicy, idempotencyKey: String) async throws
  func dissolve(airID: String, revision: Int64, idempotencyKey: String) async throws
}

public final class AirAppClient: AirAppServicing, @unchecked Sendable {
  private static let maximumResponseBytes = 64 * 1_024
  private let origin: CoordinatorOrigin
  private let bearer: String
  private let transport: any OnboardingHTTPTransport

  public init(
    bundle: CredentialBundle,
    transport: any OnboardingHTTPTransport = URLSessionOnboardingTransport()
  ) throws {
    guard let origin = bundle.coordinatorOrigin,
      let control = bundle.control,
      CredentialSyntax.lowerHexToken(control.controlToken)
    else { throw AirClientError.invalidConfiguration }
    self.origin = origin
    bearer = control.controlToken
    self.transport = transport
  }

  public func availability() async throws -> AirFeatureAvailability {
    let response = try await request(method: "GET", path: "/healthz", success: [200])
    let wire: AirHealthResponse = try decode(response.data)
    guard !wire.phase2.airAuthorityState.isEmpty else {
      throw AirClientError.invalidResponse
    }
    return AirFeatureAvailability(
      enabled: wire.phase2.airRoomsEnabled,
      authorityState: wire.phase2.airAuthorityState)
  }

  public func list() async throws -> AirList {
    let response = try await request(method: "GET", path: "/v1/airs", success: [200])
    let wire: AirListResponse = try decode(response.data)
    guard wire.activePointerRevision >= 0,
      wire.saved.allSatisfy({ Self.validAirID($0.airID) }),
      wire.currentAirID.map(Self.validAirID) ?? true,
      Set(wire.saved.map(\.airID)).count == wire.saved.count,
      wire.saved.filter(\.isCurrent).count == (wire.currentAirID == nil ? 0 : 1),
      wire.saved.first(where: \.isCurrent)?.airID == wire.currentAirID
    else { throw AirClientError.invalidResponse }
    return AirList(
      currentAirID: wire.currentAirID,
      activePointerRevision: wire.activePointerRevision,
      saved: try wire.saved.map(Self.summary))
  }

  public func detail(id: String) async throws -> AirDetail {
    guard Self.validAirID(id) else { throw AirClientError.invalidRequest }
    let response = try await request(method: "GET", path: "/v1/airs/\(id)", success: [200])
    return try Self.detail(try decode(response.data))
  }

  public func create(title: String, idempotencyKey: String) async throws -> AirDetail {
    let title = title.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !title.isEmpty, title.count <= 80 else { throw AirClientError.invalidRequest }
    let response = try await mutate(
      method: "POST", path: "/v1/airs", key: idempotencyKey,
      body: AirCreateBody(title: title), success: [201])
    return try Self.detail(try decode(response.data))
  }

  public func issueInvite(
    airID: String, role: AirRole, idempotencyKey: String
  ) async throws -> AirInvite {
    guard Self.validAirID(airID), role != .owner else { throw AirClientError.invalidRequest }
    let response = try await mutate(
      method: "POST", path: "/v1/airs/\(airID)/invites", key: idempotencyKey,
      body: AirInviteBody(airRole: role.rawValue), success: [201])
    let wire: AirInviteResponse = try decode(response.data)
    guard Self.validInviteID(wire.inviteID), wire.revision > 0,
      !wire.code.isEmpty, wire.code.utf8.count <= 512,
      let expiresAt = Self.date(wire.expiresAt)
    else { throw AirClientError.invalidResponse }
    return AirInvite(id: wire.inviteID, revision: wire.revision, expiresAt: expiresAt, code: wire.code)
  }

  public func withdrawInvite(
    airID: String, inviteID: String, revision: Int64, idempotencyKey: String
  ) async throws {
    guard Self.validAirID(airID), Self.validInviteID(inviteID), revision > 0
    else { throw AirClientError.invalidRequest }
    _ = try await mutate(
      method: "POST", path: "/v1/airs/\(airID)/invites/\(inviteID)/withdraw",
      key: idempotencyKey, body: AirInviteRevisionBody(inviteRevision: revision), success: [200])
  }

  public func consumeInvite(code: String, idempotencyKey: String) async throws -> AirJoinPreview {
    let code = code.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !code.isEmpty, code.utf8.count <= 512 else { throw AirClientError.invalidRequest }
    let response = try await mutate(
      method: "POST", path: "/v1/air-invites/consume", key: idempotencyKey,
      body: AirConsumeBody(code: code), success: [202])
    let wire: AirJoinResponse = try decode(response.data)
    guard Self.validAirID(wire.airID), Self.validMemberID(wire.membershipID),
      wire.membershipRevision > 0, wire.memberCount >= 0
    else { throw AirClientError.invalidResponse }
    return AirJoinPreview(
      airID: wire.airID, title: wire.title, ownerDisplayName: wire.ownerDisplayName,
      intendedRole: wire.airRole, membershipRevision: wire.membershipRevision,
      policy: try Self.policy(wire.policy), memberCount: wire.memberCount,
      capacity: try Self.capacity(wire.capacity),
      activationWouldSwitch: wire.activationWouldSwitch)
  }

  public func confirmJoin(
    airID: String, membershipRevision: Int64, activate: Bool,
    expectedActiveAirID: String?, idempotencyKey: String
  ) async throws {
    guard Self.validAirID(airID), membershipRevision > 0,
      !activate || Self.validExpectedAirID(expectedActiveAirID)
    else { throw AirClientError.invalidRequest }
    _ = try await mutate(
      method: "POST", path: "/v1/airs/\(airID)/join/confirm", key: idempotencyKey,
      body: AirConfirmBody(
        membershipRevision: membershipRevision, activate: activate,
        expectedActiveAirID: activate ? (expectedActiveAirID ?? "none") : nil), success: [200])
  }

  public func declineJoin(
    airID: String, membershipRevision: Int64, idempotencyKey: String
  ) async throws {
    try await revisionMutation(
      path: "/v1/airs/\(airID)/join/decline", airID: airID,
      membershipRevision: membershipRevision, key: idempotencyKey)
  }

  public func activate(
    airID: String, membershipRevision: Int64, expectedActiveAirID: String?,
    idempotencyKey: String
  ) async throws {
    try await activationMutation(
      path: "/v1/airs/\(airID)/activate", airID: airID,
      membershipRevision: membershipRevision, expectedActiveAirID: expectedActiveAirID,
      key: idempotencyKey)
  }

  public func deactivate(
    airID: String, membershipRevision: Int64, expectedActiveAirID: String,
    idempotencyKey: String
  ) async throws {
    guard Self.validAirID(expectedActiveAirID) else { throw AirClientError.invalidRequest }
    try await activationMutation(
      path: "/v1/airs/\(airID)/deactivate", airID: airID,
      membershipRevision: membershipRevision, expectedActiveAirID: expectedActiveAirID,
      key: idempotencyKey)
  }

  public func leave(
    airID: String, membershipRevision: Int64, expectedActiveAirID: String?,
    idempotencyKey: String
  ) async throws {
    try await activationMutation(
      path: "/v1/airs/\(airID)/leave", airID: airID,
      membershipRevision: membershipRevision, expectedActiveAirID: expectedActiveAirID,
      key: idempotencyKey)
  }

  public func replacePolicy(
    airID: String, policy: AirPolicy, idempotencyKey: String
  ) async throws {
    guard Self.validAirID(airID), policy.revision > 0 else { throw AirClientError.invalidRequest }
    _ = try await mutate(
      method: "PUT", path: "/v1/airs/\(airID)/policy", key: idempotencyKey,
      body: AirPolicyBody(
        policyRevision: policy.revision, invite: policy.invite.rawValue,
        overlay: policy.overlay.rawValue, queue: policy.queue.rawValue,
        replace: policy.replace.rawValue), success: [200])
  }

  public func dissolve(airID: String, revision: Int64, idempotencyKey: String) async throws {
    guard Self.validAirID(airID), revision > 0 else { throw AirClientError.invalidRequest }
    _ = try await mutate(
      method: "POST", path: "/v1/airs/\(airID)/dissolve", key: idempotencyKey,
      body: AirRevisionBody(airRevision: revision), success: [200])
  }

  private func revisionMutation(
    path: String, airID: String, membershipRevision: Int64, key: String
  ) async throws {
    guard Self.validAirID(airID), membershipRevision > 0 else { throw AirClientError.invalidRequest }
    _ = try await mutate(
      method: "POST", path: path, key: key,
      body: AirMembershipRevisionBody(membershipRevision: membershipRevision), success: [200])
  }

  private func activationMutation(
    path: String, airID: String, membershipRevision: Int64,
    expectedActiveAirID: String?, key: String
  ) async throws {
    guard Self.validAirID(airID), membershipRevision > 0,
      Self.validExpectedAirID(expectedActiveAirID)
    else { throw AirClientError.invalidRequest }
    _ = try await mutate(
      method: "POST", path: path, key: key,
      body: AirActivationBody(
        membershipRevision: membershipRevision,
        expectedActiveAirID: expectedActiveAirID ?? "none"), success: [200])
  }

  private func mutate<Body: Encodable>(
    method: String, path: String, key: String, body: Body, success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    guard Self.validIdempotencyKey(key) else { throw AirClientError.invalidRequest }
    let data: Data
    do {
      let encoder = JSONEncoder()
      encoder.outputFormatting = [.sortedKeys]
      data = try encoder.encode(body)
    } catch { throw AirClientError.invalidRequest }
    return try await request(
      method: method, path: path,
      headers: ["Content-Type": "application/json", "Idempotency-Key": key],
      body: data, success: success)
  }

  private func request(
    method: String, path: String, headers: [String: String] = [:],
    body: Data? = nil, success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    guard let url = origin.endpoint(path: path), CredentialSyntax.lowerHexToken(bearer)
    else { throw AirClientError.invalidRequest }
    var request = URLRequest(
      url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: 30)
    request.httpMethod = method
    request.httpBody = body
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    request.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization")
    for (name, value) in headers { request.setValue(value, forHTTPHeaderField: name) }
    let received: HTTPTransportResponse
    do {
      received = try await transport.send(request, maximumResponseBytes: Self.maximumResponseBytes)
    } catch let error as OnboardingClientError where error == .responseTooLarge {
      throw AirClientError.responseTooLarge
    } catch { throw AirClientError.transport }
    guard received.data.count <= Self.maximumResponseBytes,
      let finalURL = received.response.url,
      let finalOrigin = try? CoordinatorOrigin(finalURL.absoluteString), finalOrigin == origin
    else { throw AirClientError.redirectRejected }
    if (300..<400).contains(received.response.statusCode) {
      throw AirClientError.redirectRejected
    }
    guard received.response.value(forHTTPHeaderField: "Content-Type")?
      .split(separator: ";", maxSplits: 1).first?
      .trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "application/json"
    else { throw AirClientError.invalidResponse }
    guard success.contains(received.response.statusCode) else {
      let envelope: AirErrorEnvelope
      do { envelope = try JSONDecoder().decode(AirErrorEnvelope.self, from: received.data) }
      catch { throw AirClientError.invalidResponse }
      throw AirClientError.rejected(
        status: received.response.statusCode, code: envelope.error.code,
        retryAfterSeconds: envelope.error.retryAfterSeconds)
    }
    return received
  }

  private func decode<Value: Decodable>(_ data: Data) throws -> Value {
    do { return try JSONDecoder().decode(Value.self, from: data) }
    catch { throw AirClientError.invalidResponse }
  }

  private static func summary(_ wire: AirSavedResponse) throws -> AirSummary {
    guard wire.memberCount >= 0, wire.activeMemberCount >= 0, wire.onlinePulsarCount >= 0,
      wire.activeMemberCount <= wire.memberCount, wire.policyRevision > 0,
      !wire.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
      ["parked", "active"].contains(wire.status)
    else { throw AirClientError.invalidResponse }
    return AirSummary(
      id: wire.airID, title: wire.title, status: wire.status,
      membershipStatus: wire.membershipStatus, role: wire.airRole,
      memberCount: wire.memberCount, activeMemberCount: wire.activeMemberCount,
      onlinePulsarCount: wire.onlinePulsarCount, capacity: try capacity(wire.capacity),
      policyRevision: wire.policyRevision, isCurrent: wire.isCurrent)
  }

  private static func detail(_ wire: AirDetailResponse) throws -> AirDetail {
    guard validAirID(wire.airID), validMemberID(wire.membershipID),
      wire.revision > 0, wire.membershipRevision > 0,
      wire.memberCount >= 0, wire.activeMemberCount >= 0,
      wire.activeMemberCount <= wire.memberCount, wire.onlinePulsarCount >= 0,
      !wire.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
      ["parked", "active"].contains(wire.status)
    else { throw AirClientError.invalidResponse }
    return AirDetail(
      id: wire.airID, title: wire.title, status: wire.status, revision: wire.revision,
      membershipID: wire.membershipID, membershipStatus: wire.membershipStatus,
      membershipRevision: wire.membershipRevision, role: wire.airRole,
      memberCount: wire.memberCount, activeMemberCount: wire.activeMemberCount,
      onlinePulsarCount: wire.onlinePulsarCount, capacity: try capacity(wire.capacity),
      policy: try policy(wire.policy), isCurrent: wire.isCurrent)
  }

  private static func capacity(_ wire: AirCapacityResponse) throws -> AirCapacity {
    guard wire.barycenters > 0, wire.onlinePulsars > 0 else { throw AirClientError.invalidResponse }
    return AirCapacity(barycenters: wire.barycenters, onlinePulsars: wire.onlinePulsars)
  }

  private static func policy(_ wire: AirPolicyResponse) throws -> AirPolicy {
    let playbackValues: Set<AirPlaybackPolicy> = [
      .airAdminPrimary, .allMemberPrimaries, .primaryCompanion, .disabled,
    ]
    let replaceValues: Set<AirPlaybackPolicy> = [
      .ownerPrimary, .airAdminPrimary, .allMemberPrimaries, .disabled,
    ]
    guard wire.revision > 0,
      playbackValues.contains(wire.overlay), playbackValues.contains(wire.queue),
      replaceValues.contains(wire.replace)
    else { throw AirClientError.invalidResponse }
    return AirPolicy(
      revision: wire.revision, invite: wire.invite, overlay: wire.overlay,
      queue: wire.queue, replace: wire.replace)
  }

  private static func date(_ value: String) -> Date? {
    let milliseconds = ISO8601DateFormatter()
    milliseconds.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return milliseconds.date(from: value) ?? ISO8601DateFormatter().date(from: value)
  }

  private static func validExpectedAirID(_ value: String?) -> Bool {
    value == nil || value == "none" || value.map(validAirID) == true
  }

  private static func validIdempotencyKey(_ value: String) -> Bool {
    (16...128).contains(value.utf8.count) && value.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:-")
        .contains($0)
    } && value.unicodeScalars.first.map { CharacterSet.alphanumerics.contains($0) } == true
  }

  private static func validAirID(_ value: String) -> Bool { validID(value, prefix: "air_") }
  private static func validInviteID(_ value: String) -> Bool { validID(value, prefix: "ai_") }
  private static func validMemberID(_ value: String) -> Bool { validID(value, prefix: "aim_") }
  private static func validID(_ value: String, prefix: String) -> Bool {
    guard value.hasPrefix(prefix) else { return false }
    let suffix = value.dropFirst(prefix.count)
    return suffix.count == 26 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "0123456789ABCDEFGHJKMNPQRSTVWXYZ").contains($0)
    }
  }
}

private struct AirHealthResponse: Decodable {
  struct Phase2: Decodable {
    let airRoomsEnabled: Bool
    let airAuthorityState: String
    enum CodingKeys: String, CodingKey {
      case airRoomsEnabled = "air_rooms_enabled"
      case airAuthorityState = "air_authority_state"
    }
  }
  let phase2: Phase2
}

private struct AirCapacityResponse: Decodable {
  let barycenters: Int
  let onlinePulsars: Int
  enum CodingKeys: String, CodingKey {
    case barycenters
    case onlinePulsars = "online_pulsars"
  }
}

private struct AirPolicyResponse: Decodable {
  let revision: Int64
  let invite: AirInvitePolicy
  let overlay: AirPlaybackPolicy
  let queue: AirPlaybackPolicy
  let replace: AirPlaybackPolicy
}

private struct AirSavedResponse: Decodable {
  let airID: String
  let title: String
  let status: String
  let membershipStatus: AirMembershipStatus
  let airRole: AirRole
  let memberCount: Int
  let activeMemberCount: Int
  let onlinePulsarCount: Int
  let capacity: AirCapacityResponse
  let policyRevision: Int64
  let isCurrent: Bool
  enum CodingKeys: String, CodingKey {
    case airID = "air_id"
    case title, status
    case membershipStatus = "membership_status"
    case airRole = "air_role"
    case memberCount = "member_count"
    case activeMemberCount = "active_member_count"
    case onlinePulsarCount = "online_pulsar_count"
    case capacity
    case policyRevision = "policy_revision"
    case isCurrent = "is_current"
  }
}

private struct AirListResponse: Decodable {
  let currentAirID: String?
  let activePointerRevision: Int64
  let saved: [AirSavedResponse]
  enum CodingKeys: String, CodingKey {
    case currentAirID = "current_air_id"
    case activePointerRevision = "active_pointer_revision"
    case saved
  }
}

private struct AirDetailResponse: Decodable {
  let airID: String
  let title: String
  let status: String
  let revision: Int64
  let membershipID: String
  let membershipStatus: AirMembershipStatus
  let membershipRevision: Int64
  let airRole: AirRole
  let memberCount: Int
  let activeMemberCount: Int
  let onlinePulsarCount: Int
  let capacity: AirCapacityResponse
  let policy: AirPolicyResponse
  let isCurrent: Bool
  enum CodingKeys: String, CodingKey {
    case airID = "air_id"
    case title, status, revision
    case membershipID = "membership_id"
    case membershipStatus = "membership_status"
    case membershipRevision = "membership_revision"
    case airRole = "air_role"
    case memberCount = "member_count"
    case activeMemberCount = "active_member_count"
    case onlinePulsarCount = "online_pulsar_count"
    case capacity, policy
    case isCurrent = "is_current"
  }
}

private struct AirInviteResponse: Decodable {
  let inviteID: String
  let revision: Int64
  let expiresAt: String
  let code: String
  enum CodingKeys: String, CodingKey {
    case inviteID = "invite_id"
    case revision
    case expiresAt = "expires_at"
    case code
  }
}

private struct AirJoinResponse: Decodable {
  let airID: String
  let title: String
  let ownerDisplayName: String
  let airRole: AirRole
  let membershipID: String
  let membershipRevision: Int64
  let policy: AirPolicyResponse
  let memberCount: Int
  let capacity: AirCapacityResponse
  let activationWouldSwitch: Bool
  enum CodingKeys: String, CodingKey {
    case airID = "air_id"
    case title
    case ownerDisplayName = "owner_display_name"
    case airRole = "air_role"
    case membershipID = "membership_id"
    case membershipRevision = "membership_revision"
    case policy, memberCount = "member_count", capacity
    case activationWouldSwitch = "activation_would_switch"
  }
}

private struct AirErrorEnvelope: Decodable {
  struct APIError: Decodable {
    let code: String
    let retryAfterSeconds: Int?
    enum CodingKeys: String, CodingKey {
      case code
      case retryAfterSeconds = "retry_after_seconds"
    }
  }
  let error: APIError
}

private struct AirCreateBody: Encodable { let title: String }
private struct AirInviteBody: Encodable {
  let airRole: String
  enum CodingKeys: String, CodingKey { case airRole = "air_role" }
}
private struct AirConsumeBody: Encodable { let code: String }
private struct AirInviteRevisionBody: Encodable {
  let inviteRevision: Int64
  enum CodingKeys: String, CodingKey { case inviteRevision = "invite_revision" }
}
private struct AirMembershipRevisionBody: Encodable {
  let membershipRevision: Int64
  enum CodingKeys: String, CodingKey { case membershipRevision = "membership_revision" }
}
private struct AirConfirmBody: Encodable {
  let membershipRevision: Int64
  let activate: Bool
  let expectedActiveAirID: String?
  enum CodingKeys: String, CodingKey {
    case membershipRevision = "membership_revision"
    case activate
    case expectedActiveAirID = "expected_active_air_id"
  }
}
private struct AirActivationBody: Encodable {
  let membershipRevision: Int64
  let expectedActiveAirID: String
  enum CodingKeys: String, CodingKey {
    case membershipRevision = "membership_revision"
    case expectedActiveAirID = "expected_active_air_id"
  }
}
private struct AirPolicyBody: Encodable {
  let policyRevision: Int64
  let invite: String
  let overlay: String
  let queue: String
  let replace: String
  enum CodingKeys: String, CodingKey {
    case policyRevision = "policy_revision"
    case invite, overlay, queue, replace
  }
}
private struct AirRevisionBody: Encodable {
  let airRevision: Int64
  enum CodingKeys: String, CodingKey { case airRevision = "air_revision" }
}
