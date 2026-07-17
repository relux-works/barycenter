import Foundation

public enum AutomationAudience: String, Codable, CaseIterable, Sendable {
  case ownBarycenter = "own_barycenter"
  case currentAir = "current_air"
  case explicit
}

public struct AutomationQuietWindow: Codable, Equatable, Sendable {
  public let weekday: Int
  public let startMinute: Int
  public let endMinute: Int

  public init(weekday: Int, startMinute: Int, endMinute: Int) {
    self.weekday = weekday
    self.startMinute = startMinute
    self.endMinute = endMinute
  }

  enum CodingKeys: String, CodingKey {
    case weekday
    case startMinute = "start_minute"
    case endMinute = "end_minute"
  }
}

public struct AutomationFeature: Equatable, Sendable {
  public var soundboardEnabled: Bool
  public var automationEnabled: Bool
  public var emergencyDisabled: Bool
  public var timezone: String
  public var quietHours: [AutomationQuietWindow]
  public let policyVersion: String
  public let revision: Int64
  public let updatedAt: Date

  public init(
    soundboardEnabled: Bool,
    automationEnabled: Bool,
    emergencyDisabled: Bool,
    timezone: String,
    quietHours: [AutomationQuietWindow],
    policyVersion: String,
    revision: Int64,
    updatedAt: Date
  ) {
    self.soundboardEnabled = soundboardEnabled
    self.automationEnabled = automationEnabled
    self.emergencyDisabled = emergencyDisabled
    self.timezone = timezone
    self.quietHours = quietHours
    self.policyVersion = policyVersion
    self.revision = revision
    self.updatedAt = updatedAt
  }
}

public struct AutomationSchedule: Equatable, Identifiable, Sendable {
  public let id: String
  public let cueID: String
  public let displayName: String
  public let timezone: String
  public let weekdays: [Int]
  public let localTime: String
  public let audience: AutomationAudience
  public let targetDigests: [String]
  public let airID: String?
  public let additionalQuietHours: [AutomationQuietWindow]
  public let policyVersion: String
  public let policyRevision: Int64
  public let enabled: Bool
  public let revision: Int64
  public let createdAt: Date
  public let updatedAt: Date
}

public struct AutomationScheduleDraft: Equatable, Sendable {
  public let cueID: String
  public let displayName: String
  public let timezone: String
  public let weekdays: [Int]
  public let localTime: String
  public let audience: AutomationAudience
  public let targetReferences: [String]
  public let airID: String?
  public let additionalQuietHours: [AutomationQuietWindow]
  public let policyRevision: Int64

  public init(
    cueID: String, displayName: String, timezone: String, weekdays: [Int],
    localTime: String, audience: AutomationAudience = .ownBarycenter,
    targetReferences: [String] = [], airID: String? = nil,
    additionalQuietHours: [AutomationQuietWindow] = [], policyRevision: Int64
  ) {
    self.cueID = cueID
    self.displayName = displayName
    self.timezone = timezone
    self.weekdays = weekdays
    self.localTime = localTime
    self.audience = audience
    self.targetReferences = targetReferences
    self.airID = airID
    self.additionalQuietHours = additionalQuietHours
    self.policyRevision = policyRevision
  }
}

public struct AutomationPrincipal: Equatable, Identifiable, Sendable {
  public let id: String
  public let displayName: String
  public let permission: String
  public let airID: String?
  public let maxTargetCount: Int
  public let allowedCueIDs: [String]
  public let allowedAudiences: [AutomationAudience]
  public let targetDigests: [String]
  public let issuedAt: Date
  public let expiresAt: Date
  public let disabledAt: Date?
  public let revokedAt: Date?
  public let revision: Int64
}

public struct AutomationPrincipalDraft: Equatable, Sendable {
  public let displayName: String
  public let allowedCueIDs: [String]
  public let allowedAudiences: [AutomationAudience]
  public let targetReferences: [String]
  public let airID: String?
  public let maxTargetCount: Int
  public let expiresAt: Date

  public init(
    displayName: String, allowedCueIDs: [String],
    allowedAudiences: [AutomationAudience] = [.ownBarycenter],
    targetReferences: [String] = [], airID: String? = nil,
    maxTargetCount: Int = 1, expiresAt: Date
  ) {
    self.displayName = displayName
    self.allowedCueIDs = allowedCueIDs
    self.allowedAudiences = allowedAudiences
    self.targetReferences = targetReferences
    self.airID = airID
    self.maxTargetCount = maxTargetCount
    self.expiresAt = expiresAt
  }
}

/// One-time automation credential. It is deliberately neither Codable nor
/// Equatable and its textual representations always redact the payload.
public final class AutomationPrincipalSecret: @unchecked Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  private let value: String

  fileprivate init(validated value: String) { self.value = value }
  public func revealForExplicitCopy() -> String { value }
  public var description: String { "AutomationPrincipalSecret(<redacted>)" }
  public var debugDescription: String { description }
}

public struct AutomationPrincipalIssue: Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public let principal: AutomationPrincipal
  public let secret: AutomationPrincipalSecret?
  public let secretAvailable: Bool

  public var description: String {
    "AutomationPrincipalIssue(principal: \(principal.displayName), secret: <redacted>)"
  }
  public var debugDescription: String { description }
}

public protocol AutomationAdminServicing: Sendable {
  func feature() async throws -> AutomationFeature
  func replaceFeature(_ feature: AutomationFeature, idempotencyKey: String) async throws
    -> AutomationFeature
  func schedules() async throws -> [AutomationSchedule]
  func createSchedule(_ draft: AutomationScheduleDraft, idempotencyKey: String) async throws
    -> AutomationSchedule
  func replaceSchedule(
    _ current: AutomationSchedule, with draft: AutomationScheduleDraft,
    idempotencyKey: String
  ) async throws -> AutomationSchedule
  func setScheduleEnabled(
    _ current: AutomationSchedule, enabled: Bool, idempotencyKey: String
  ) async throws -> AutomationSchedule
  func deleteSchedule(_ current: AutomationSchedule, idempotencyKey: String) async throws
    -> AutomationSchedule
  func principals() async throws -> [AutomationPrincipal]
  func issuePrincipal(_ draft: AutomationPrincipalDraft, idempotencyKey: String) async throws
    -> AutomationPrincipalIssue
  func revokePrincipal(_ current: AutomationPrincipal, idempotencyKey: String) async throws
    -> AutomationPrincipal
  func cancelHistory(_ historyID: String) async throws
}

public enum AutomationScheduleRules {
  private static let weekdayNames = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]

  public static func validTimezone(_ value: String) -> Bool {
    !value.isEmpty && value != "Local" && value.utf8.count <= 128
      && value == value.trimmingCharacters(in: .whitespacesAndNewlines)
      && TimeZone(identifier: value) != nil
  }

  public static func validLocalTime(_ value: String) -> Bool {
    guard value.utf8.count == 5, value[value.index(value.startIndex, offsetBy: 2)] == ":" else {
      return false
    }
    let parts = value.split(separator: ":", omittingEmptySubsequences: false)
    return parts.count == 2 && Int(parts[0]).map { (0...23).contains($0) } == true
      && Int(parts[1]).map { (0...59).contains($0) } == true
  }

  public static func validWeekdays(_ values: [Int]) -> Bool {
    !values.isEmpty && values.count <= 7 && Set(values).count == values.count
      && values.allSatisfy { (0...6).contains($0) }
  }

  public static func validQuietWindows(_ values: [AutomationQuietWindow]) -> Bool {
    guard values.count <= 128 else { return false }
    var occupied = Set<Int>()
    for window in values {
      guard (0...6).contains(window.weekday), (0...1439).contains(window.startMinute),
        (0...1439).contains(window.endMinute), window.startMinute != window.endMinute
      else { return false }
      var minute = window.startMinute
      while minute != window.endMinute {
        let day = minute < window.startMinute ? (window.weekday + 1) % 7 : window.weekday
        guard occupied.insert(day * 1_440 + minute).inserted else { return false }
        minute = (minute + 1) % 1_440
      }
    }
    return true
  }

  public static func parseWeekdays(_ value: String) throws -> [Int] {
    let clean = value.trimmingCharacters(in: .whitespacesAndNewlines)
    if clean.isEmpty { return Array(0...6) }
    let lookup = Dictionary(uniqueKeysWithValues: weekdayNames.enumerated().map { ($1.lowercased(), $0) })
    let values = try clean.split(separator: ",", omittingEmptySubsequences: false).map { raw -> Int in
      guard let value = lookup[raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()]
      else { throw PhaseOneClientError.invalidRequest }
      return value
    }
    guard validWeekdays(values) else { throw PhaseOneClientError.invalidRequest }
    return values
  }

  public static func parseQuietHours(_ value: String) throws -> [AutomationQuietWindow] {
    let clean = value.trimmingCharacters(in: .whitespacesAndNewlines)
    if clean.isEmpty { return [] }
    let lookup = Dictionary(uniqueKeysWithValues: weekdayNames.enumerated().map { ($1.lowercased(), $0) })
    let values = try clean.split(separator: ";", omittingEmptySubsequences: false).map { raw -> AutomationQuietWindow in
      let fields = raw.split(whereSeparator: { $0.isWhitespace })
      guard fields.count == 2, let weekday = lookup[fields[0].lowercased()] else {
        throw PhaseOneClientError.invalidRequest
      }
      let times = fields[1].split(separator: "-", omittingEmptySubsequences: false)
      guard times.count == 2 else { throw PhaseOneClientError.invalidRequest }
      let start = String(times[0]), end = String(times[1])
      guard validLocalTime(start), validLocalTime(end) else {
        throw PhaseOneClientError.invalidRequest
      }
      return AutomationQuietWindow(
        weekday: weekday, startMinute: minute(start), endMinute: minute(end))
    }
    guard validQuietWindows(values) else { throw PhaseOneClientError.invalidRequest }
    return values
  }

  public static func formatWeekdays(_ values: [Int]) -> String {
    values.compactMap { weekdayNames.indices.contains($0) ? weekdayNames[$0] : nil }
      .joined(separator: ",")
  }

  public static func formatQuietHours(_ values: [AutomationQuietWindow]) -> String {
    values.compactMap { value in
      guard weekdayNames.indices.contains(value.weekday) else { return nil }
      return String(
        format: "%@ %02d:%02d-%02d:%02d", weekdayNames[value.weekday],
        value.startMinute / 60, value.startMinute % 60,
        value.endMinute / 60, value.endMinute % 60)
    }.joined(separator: "; ")
  }

  public static func nextRun(for schedule: AutomationSchedule, after now: Date) -> Date? {
    guard schedule.enabled, validTimezone(schedule.timezone), validLocalTime(schedule.localTime),
      validWeekdays(schedule.weekdays), let zone = TimeZone(identifier: schedule.timezone)
    else { return nil }
    let wantedMinute = minute(schedule.localTime)
    let allowed = Set(schedule.weekdays)
    var candidate = Date(timeIntervalSince1970: floor(now.timeIntervalSince1970 / 60) * 60 + 60)
    let limit = candidate.addingTimeInterval((8 * 24 + 3) * 3_600)
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = zone
    while candidate <= limit {
      let parts = calendar.dateComponents([.weekday, .hour, .minute], from: candidate)
      let weekday = (parts.weekday ?? 1) - 1
      if allowed.contains(weekday), (parts.hour ?? -1) * 60 + (parts.minute ?? -1) == wantedMinute {
        return candidate
      }
      candidate.addTimeInterval(60)
    }
    return nil
  }

  public static func isQuiet(
    at instant: Date, timezone: String, windows: [AutomationQuietWindow]
  ) -> Bool {
    guard let zone = TimeZone(identifier: timezone) else { return true }
    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = zone
    let parts = calendar.dateComponents([.weekday, .hour, .minute], from: instant)
    let weekday = (parts.weekday ?? 1) - 1
    let minute = (parts.hour ?? 0) * 60 + (parts.minute ?? 0)
    return windows.contains { window in
      if window.endMinute > window.startMinute {
        return weekday == window.weekday && minute >= window.startMinute && minute < window.endMinute
      }
      return (weekday == window.weekday && minute >= window.startMinute)
        || (weekday == (window.weekday + 1) % 7 && minute < window.endMinute)
    }
  }

  private static func minute(_ value: String) -> Int {
    let parts = value.split(separator: ":")
    return (Int(parts[0]) ?? 0) * 60 + (Int(parts[1]) ?? 0)
  }
}

public final class AutomationAdminClient: AutomationAdminServicing, @unchecked Sendable {
  private static let maximumResponseBytes = 64 * 1_024
  private let origin: CoordinatorOrigin
  private let token: String
  private let transport: any OnboardingHTTPTransport

  public init(
    bundle: CredentialBundle,
    transport: any OnboardingHTTPTransport = URLSessionOnboardingTransport()
  ) throws {
    guard let origin = bundle.coordinatorOrigin, origin.isSecureForCredentials,
      let control = bundle.control, control.contextStrength == .active,
      control.orbitId != nil, CredentialSyntax.lowerHexToken(control.controlToken)
    else { throw PhaseOneClientError.invalidConfiguration }
    self.origin = origin
    token = control.controlToken
    self.transport = transport
  }

  public func feature() async throws -> AutomationFeature {
    try decodeFeature(try await request(method: "GET", path: "/v1/automation/status", success: [200]).data)
  }

  public func replaceFeature(_ feature: AutomationFeature, idempotencyKey: String) async throws
    -> AutomationFeature
  {
    guard Self.validKey(idempotencyKey), AutomationScheduleRules.validTimezone(feature.timezone),
      AutomationScheduleRules.validQuietWindows(feature.quietHours), feature.revision >= 0
    else { throw PhaseOneClientError.invalidRequest }
    let body = FeatureMutationWire(
      soundboardEnabled: feature.soundboardEnabled,
      automationEnabled: feature.automationEnabled,
      emergencyDisabled: feature.emergencyDisabled,
      timezone: feature.timezone, quietHours: feature.quietHours,
      expectedRevision: feature.revision)
    return try decodeFeature(try await request(
      method: "PUT", path: "/v1/automation/status",
      headers: ["Idempotency-Key": idempotencyKey], body: body, success: [200]).data)
  }

  public func schedules() async throws -> [AutomationSchedule] {
    let response: ScheduleListWire = try decode(
      try await request(method: "GET", path: "/v1/automation/schedules", success: [200]).data)
    guard response.schedules.count <= 128 else { throw PhaseOneClientError.invalidResponse }
    var seen = Set<String>()
    let values = try response.schedules.map { wire -> AutomationSchedule in
      let value = try validate(wire)
      guard seen.insert(value.id).inserted else { throw PhaseOneClientError.invalidResponse }
      return value
    }
    return values.sorted { $0.displayName.localizedStandardCompare($1.displayName) == .orderedAscending }
  }

  public func createSchedule(_ draft: AutomationScheduleDraft, idempotencyKey: String) async throws
    -> AutomationSchedule
  {
    guard Self.validKey(idempotencyKey), Self.valid(draft) else {
      throw PhaseOneClientError.invalidRequest
    }
    return try decodeScheduleEnvelope(try await request(
      method: "POST", path: "/v1/automation/schedules",
      headers: ["Idempotency-Key": idempotencyKey], body: ScheduleMutationWire(draft, revision: nil),
      success: [201]).data)
  }

  public func replaceSchedule(
    _ current: AutomationSchedule, with draft: AutomationScheduleDraft,
    idempotencyKey: String
  ) async throws -> AutomationSchedule {
    guard Self.validKey(idempotencyKey), Self.validID(current.id, prefix: "sch_"),
      current.revision > 0, Self.valid(draft)
    else { throw PhaseOneClientError.invalidRequest }
    return try decodeScheduleEnvelope(try await request(
      method: "PUT", path: "/v1/automation/schedules/\(current.id)",
      headers: ["Idempotency-Key": idempotencyKey],
      body: ScheduleMutationWire(draft, revision: current.revision), success: [200]).data)
  }

  public func setScheduleEnabled(
    _ current: AutomationSchedule, enabled: Bool, idempotencyKey: String
  ) async throws -> AutomationSchedule {
    guard Self.validKey(idempotencyKey), Self.validID(current.id, prefix: "sch_"),
      current.revision > 0 else { throw PhaseOneClientError.invalidRequest }
    let action = enabled ? "enable" : "disable"
    return try decodeScheduleEnvelope(try await request(
      method: "POST", path: "/v1/automation/schedules/\(current.id)/\(action)",
      headers: ["Idempotency-Key": idempotencyKey],
      body: RevisionWire(expectedRevision: current.revision), success: [200]).data)
  }

  public func deleteSchedule(_ current: AutomationSchedule, idempotencyKey: String) async throws
    -> AutomationSchedule
  {
    guard Self.validKey(idempotencyKey), Self.validID(current.id, prefix: "sch_"),
      current.revision > 0 else { throw PhaseOneClientError.invalidRequest }
    return try decodeScheduleEnvelope(try await request(
      method: "DELETE", path: "/v1/automation/schedules/\(current.id)",
      headers: ["Idempotency-Key": idempotencyKey],
      body: RevisionWire(expectedRevision: current.revision), success: [200]).data)
  }

  public func principals() async throws -> [AutomationPrincipal] {
    let response: PrincipalListWire = try decode(
      try await request(method: "GET", path: "/v1/automation/principals", success: [200]).data)
    guard response.principals.count <= 128 else { throw PhaseOneClientError.invalidResponse }
    var seen = Set<String>()
    let values = try response.principals.map { wire -> AutomationPrincipal in
      let value = try validate(wire)
      guard seen.insert(value.id).inserted else { throw PhaseOneClientError.invalidResponse }
      return value
    }
    return values.sorted { $0.displayName.localizedStandardCompare($1.displayName) == .orderedAscending }
  }

  public func issuePrincipal(_ draft: AutomationPrincipalDraft, idempotencyKey: String) async throws
    -> AutomationPrincipalIssue
  {
    guard Self.validKey(idempotencyKey), Self.valid(draft) else {
      throw PhaseOneClientError.invalidRequest
    }
    let response: PrincipalIssueWire = try decode(try await request(
      method: "POST", path: "/v1/automation/principals",
      headers: ["Idempotency-Key": idempotencyKey], body: PrincipalMutationWire(draft),
      success: [201]).data)
    let principal = try validate(response.principal)
    let validSecret = response.secret.map(Self.lowerHex) == true
    guard (response.secretAvailable && validSecret)
      || (!response.secretAvailable && response.secret?.isEmpty != false)
    else { throw PhaseOneClientError.invalidResponse }
    return AutomationPrincipalIssue(
      principal: principal,
      secret: response.secretAvailable
        ? AutomationPrincipalSecret(validated: response.secret ?? "") : nil,
      secretAvailable: response.secretAvailable)
  }

  public func revokePrincipal(_ current: AutomationPrincipal, idempotencyKey: String) async throws
    -> AutomationPrincipal
  {
    guard Self.validKey(idempotencyKey), Self.validID(current.id, prefix: "ap_"),
      current.revision > 0 else { throw PhaseOneClientError.invalidRequest }
    let response: PrincipalEnvelopeWire = try decode(try await request(
      method: "POST", path: "/v1/automation/principals/\(current.id)/revoke",
      headers: ["Idempotency-Key": idempotencyKey],
      body: RevisionWire(expectedRevision: current.revision), success: [200]).data)
    return try validate(response.principal)
  }

  public func cancelHistory(_ historyID: String) async throws {
    guard Self.validID(historyID, prefix: "hi_") else { throw PhaseOneClientError.invalidRequest }
    _ = try await request(
      method: "POST", path: "/v1/history/\(historyID)/actions/cancel",
      body: EmptyWire(), success: [200])
  }

  private func decodeFeature(_ data: Data) throws -> AutomationFeature {
    let wire: FeatureWire = try decode(data)
    guard AutomationScheduleRules.validTimezone(wire.timezone),
      AutomationScheduleRules.validQuietWindows(wire.quietHours), wire.policyValid,
      wire.revision >= 0, Self.validText(wire.policyVersion, maximum: 64),
      let updatedAt = Self.date(wire.updatedAt)
    else { throw PhaseOneClientError.invalidResponse }
    return AutomationFeature(
      soundboardEnabled: wire.soundboardEnabled,
      automationEnabled: wire.automationEnabled,
      emergencyDisabled: wire.emergencyDisabled,
      timezone: wire.timezone, quietHours: wire.quietHours,
      policyVersion: wire.policyVersion, revision: wire.revision, updatedAt: updatedAt)
  }

  private func decodeScheduleEnvelope(_ data: Data) throws -> AutomationSchedule {
    let envelope: ScheduleEnvelopeWire = try decode(data)
    return try validate(envelope.schedule)
  }

  private func validate(_ wire: ScheduleWire) throws -> AutomationSchedule {
    guard Self.validID(wire.scheduleID, prefix: "sch_"),
      Self.validID(wire.cueID, prefix: "cq_"), Self.validText(wire.displayName, maximum: 128),
      AutomationScheduleRules.validTimezone(wire.timezone),
      AutomationScheduleRules.validWeekdays(wire.weekdays),
      AutomationScheduleRules.validLocalTime(wire.localTime), wire.delivery == "overlay",
      AutomationScheduleRules.validQuietWindows(wire.additionalQuietHours),
      wire.policyRevision > 0, wire.revision > 0,
      Self.validText(wire.policyVersion, maximum: 64),
      let createdAt = Self.date(wire.createdAt), let updatedAt = Self.date(wire.updatedAt)
    else { throw PhaseOneClientError.invalidResponse }
    let digests = wire.audience.targetDigests
    guard (wire.audience.kind == .explicit) == !digests.isEmpty,
      digests.allSatisfy(Self.lowerHex)
    else { throw PhaseOneClientError.invalidResponse }
    return AutomationSchedule(
      id: wire.scheduleID, cueID: wire.cueID, displayName: wire.displayName,
      timezone: wire.timezone, weekdays: wire.weekdays, localTime: wire.localTime,
      audience: wire.audience.kind, targetDigests: digests,
      airID: wire.audience.airID?.isEmpty == false ? wire.audience.airID : nil,
      additionalQuietHours: wire.additionalQuietHours,
      policyVersion: wire.policyVersion, policyRevision: wire.policyRevision,
      enabled: wire.enabled, revision: wire.revision,
      createdAt: createdAt, updatedAt: updatedAt)
  }

  private func validate(_ wire: PrincipalWire) throws -> AutomationPrincipal {
    guard Self.validID(wire.principalID, prefix: "ap_"),
      Self.validText(wire.displayName, maximum: 128), wire.permission == "trigger",
      (1...64).contains(wire.maxTargetCount), wire.revision > 0,
      !wire.allowedCueIDs.isEmpty, !wire.allowedAudienceKinds.isEmpty,
      wire.allowedCueIDs.allSatisfy({ Self.validID($0, prefix: "cq_") }),
      Set(wire.allowedCueIDs).count == wire.allowedCueIDs.count,
      Set(wire.allowedAudienceKinds).count == wire.allowedAudienceKinds.count,
      wire.allowedTargetDigests.allSatisfy(Self.lowerHex),
      wire.allowedAudienceKinds.contains(.explicit) == !wire.allowedTargetDigests.isEmpty,
      let issuedAt = Self.date(wire.issuedAt), let expiresAt = Self.date(wire.expiresAt),
      expiresAt > issuedAt,
      wire.disabledAt.isEmpty || Self.date(wire.disabledAt) != nil,
      wire.revokedAt.isEmpty || Self.date(wire.revokedAt) != nil
    else { throw PhaseOneClientError.invalidResponse }
    return AutomationPrincipal(
      id: wire.principalID, displayName: wire.displayName, permission: wire.permission,
      airID: wire.boundAirID.isEmpty ? nil : wire.boundAirID,
      maxTargetCount: wire.maxTargetCount, allowedCueIDs: wire.allowedCueIDs,
      allowedAudiences: wire.allowedAudienceKinds, targetDigests: wire.allowedTargetDigests,
      issuedAt: issuedAt, expiresAt: expiresAt,
      disabledAt: wire.disabledAt.isEmpty ? nil : Self.date(wire.disabledAt),
      revokedAt: wire.revokedAt.isEmpty ? nil : Self.date(wire.revokedAt),
      revision: wire.revision)
  }

  private func request<Body: Encodable>(
    method: String, path: String, headers: [String: String] = [:], body: Body,
    success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    guard let data = try? encoder.encode(body) else { throw PhaseOneClientError.invalidRequest }
    var headers = headers
    headers["Content-Type"] = "application/json"
    return try await request(
      method: method, path: path, headers: headers, rawBody: data, success: success)
  }

  private func request(
    method: String, path: String, headers: [String: String] = [:],
    rawBody: Data? = nil, success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    guard let url = origin.endpoint(path: path) else { throw PhaseOneClientError.invalidRequest }
    var request = URLRequest(
      url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: 30)
    request.httpMethod = method
    request.httpBody = rawBody
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    headers.forEach { request.setValue($1, forHTTPHeaderField: $0) }
    let response: HTTPTransportResponse
    do {
      response = try await transport.send(request, maximumResponseBytes: Self.maximumResponseBytes)
    } catch let error as OnboardingClientError where error == .responseTooLarge {
      throw PhaseOneClientError.responseTooLarge
    } catch { throw PhaseOneClientError.transport }
    guard response.data.count <= Self.maximumResponseBytes,
      let finalURL = response.response.url,
      let finalOrigin = try? CoordinatorOrigin(finalURL.absoluteString), finalOrigin == origin
    else { throw PhaseOneClientError.redirectRejected }
    if (300..<400).contains(response.response.statusCode) {
      throw PhaseOneClientError.redirectRejected
    }
    guard response.response.value(forHTTPHeaderField: "Content-Type")?
      .split(separator: ";", maxSplits: 1).first?
      .trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "application/json"
    else { throw PhaseOneClientError.invalidResponse }
    guard success.contains(response.response.statusCode) else {
      let envelope = try? JSONDecoder().decode(AutomationErrorEnvelope.self, from: response.data)
      guard let code = envelope?.error.code, !code.isEmpty else {
        throw PhaseOneClientError.invalidResponse
      }
      throw PhaseOneClientError.rejected(
        status: response.response.statusCode, code: code,
        retryAfterSeconds: envelope?.error.retryAfterSeconds)
    }
    return response
  }

  private func decode<Value: Decodable>(_ data: Data) throws -> Value {
    do { return try JSONDecoder().decode(Value.self, from: data) }
    catch { throw PhaseOneClientError.invalidResponse }
  }

  private static func valid(_ draft: AutomationScheduleDraft) -> Bool {
    validID(draft.cueID, prefix: "cq_") && validText(draft.displayName, maximum: 128)
      && AutomationScheduleRules.validTimezone(draft.timezone)
      && AutomationScheduleRules.validWeekdays(draft.weekdays)
      && AutomationScheduleRules.validLocalTime(draft.localTime)
      && AutomationScheduleRules.validQuietWindows(draft.additionalQuietHours)
      && draft.policyRevision > 0
      && (draft.audience == .explicit) == !draft.targetReferences.isEmpty
      && draft.targetReferences.count <= 64
      && Set(draft.targetReferences).count == draft.targetReferences.count
      && draft.targetReferences.allSatisfy(validTargetReference)
  }

  private static func valid(_ draft: AutomationPrincipalDraft) -> Bool {
    validText(draft.displayName, maximum: 128) && !draft.allowedCueIDs.isEmpty
      && !draft.allowedAudiences.isEmpty && (1...64).contains(draft.maxTargetCount)
      && draft.expiresAt > Date(timeIntervalSince1970: 0)
      && draft.allowedCueIDs.allSatisfy { validID($0, prefix: "cq_") }
      && Set(draft.allowedCueIDs).count == draft.allowedCueIDs.count
      && Set(draft.allowedAudiences).count == draft.allowedAudiences.count
      && draft.allowedAudiences.contains(.explicit) == !draft.targetReferences.isEmpty
      && Set(draft.targetReferences).count == draft.targetReferences.count
      && draft.targetReferences.allSatisfy(validTargetReference)
  }

  private static func validText(_ value: String, maximum: Int) -> Bool {
    !value.isEmpty && value.utf8.count <= maximum
      && value == value.trimmingCharacters(in: .whitespacesAndNewlines)
      && value.unicodeScalars.allSatisfy { !CharacterSet.controlCharacters.contains($0) }
  }

  private static func validID(_ value: String, prefix: String) -> Bool {
    guard value.hasPrefix(prefix), value.utf8.count == prefix.utf8.count + 26 else { return false }
    return value.dropFirst(prefix.count).utf8.allSatisfy {
      ($0 >= 48 && $0 <= 57) || ($0 >= 65 && $0 <= 90)
    }
  }

  private static func validTargetReference(_ value: String) -> Bool {
    let parts = value.split(separator: ":", omittingEmptySubsequences: false)
    guard parts.count == 2, ["node", "actor"].contains(parts[0]), !parts[1].isEmpty,
      value.utf8.count <= 128 else { return false }
    return parts[1].utf8.allSatisfy {
      ($0 >= 48 && $0 <= 57) || ($0 >= 65 && $0 <= 90) || ($0 >= 97 && $0 <= 122)
        || $0 == 45 || $0 == 95
    }
  }

  private static func validKey(_ value: String) -> Bool {
    (16...128).contains(value.utf8.count) && value.utf8.allSatisfy {
      ($0 >= 48 && $0 <= 57) || ($0 >= 65 && $0 <= 90) || ($0 >= 97 && $0 <= 122)
        || [46, 58, 95, 45].contains($0)
    }
  }

  private static func lowerHex(_ value: String) -> Bool {
    value.utf8.count == 64 && value.utf8.allSatisfy {
      ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
    }
  }

  private static func date(_ value: String) -> Date? {
    let fractional = ISO8601DateFormatter()
    fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return fractional.date(from: value) ?? ISO8601DateFormatter().date(from: value)
  }
}

private struct FeatureWire: Decodable {
  let soundboardEnabled: Bool
  let automationEnabled: Bool
  let emergencyDisabled: Bool
  let timezone: String
  let quietHours: [AutomationQuietWindow]
  let policyVersion: String
  let revision: Int64
  let policyValid: Bool
  let updatedAt: String
  enum CodingKeys: String, CodingKey {
    case timezone, revision
    case soundboardEnabled = "soundboard_enabled"
    case automationEnabled = "automation_enabled"
    case emergencyDisabled = "emergency_disabled"
    case quietHours = "quiet_hours"
    case policyVersion = "policy_version"
    case policyValid = "policy_valid"
    case updatedAt = "updated_at"
  }
}

private struct FeatureMutationWire: Encodable {
  let soundboardEnabled: Bool
  let automationEnabled: Bool
  let emergencyDisabled: Bool
  let timezone: String
  let quietHours: [AutomationQuietWindow]
  let expectedRevision: Int64
  enum CodingKeys: String, CodingKey {
    case timezone
    case soundboardEnabled = "soundboard_enabled"
    case automationEnabled = "automation_enabled"
    case emergencyDisabled = "emergency_disabled"
    case quietHours = "quiet_hours"
    case expectedRevision = "expected_revision"
  }
}

private struct AudienceWire: Codable {
  let kind: AutomationAudience
  let targetDigests: [String]
  let airID: String?
  enum CodingKeys: String, CodingKey {
    case kind
    case targetDigests = "target_digests"
    case airID = "air_id"
  }
}

private struct ScheduleWire: Decodable {
  let scheduleID: String
  let cueID: String
  let displayName: String
  let timezone: String
  let weekdays: [Int]
  let localTime: String
  let audience: AudienceWire
  let delivery: String
  let additionalQuietHours: [AutomationQuietWindow]
  let policyVersion: String
  let policyRevision: Int64
  let enabled: Bool
  let revision: Int64
  let createdAt: String
  let updatedAt: String
  enum CodingKeys: String, CodingKey {
    case timezone, weekdays, audience, delivery, enabled, revision
    case scheduleID = "schedule_id", cueID = "cue_id", displayName = "display_name"
    case localTime = "local_time", additionalQuietHours = "additional_quiet_hours"
    case policyVersion = "policy_version", policyRevision = "policy_revision"
    case createdAt = "created_at", updatedAt = "updated_at"
  }
}

private struct ScheduleListWire: Decodable { let schedules: [ScheduleWire] }
private struct ScheduleEnvelopeWire: Decodable { let schedule: ScheduleWire }

private struct ScheduleMutationWire: Encodable {
  struct MutationAudience: Encodable {
    let kind: AutomationAudience
    let targetReferences: [String]?
    let airID: String?
    enum CodingKeys: String, CodingKey {
      case kind
      case targetReferences = "target_references"
      case airID = "air_id"
    }
  }
  let cueID: String
  let displayName: String
  let timezone: String
  let weekdays: [Int]
  let localTime: String
  let audience: MutationAudience
  let delivery = "overlay"
  let additionalQuietHours: [AutomationQuietWindow]
  let policyRevision: Int64
  let expectedRevision: Int64?
  enum CodingKeys: String, CodingKey {
    case timezone, weekdays, audience, delivery
    case cueID = "cue_id", displayName = "display_name", localTime = "local_time"
    case additionalQuietHours = "additional_quiet_hours"
    case policyRevision = "policy_revision", expectedRevision = "expected_revision"
  }
  init(_ draft: AutomationScheduleDraft, revision: Int64?) {
    cueID = draft.cueID
    displayName = draft.displayName
    timezone = draft.timezone
    weekdays = draft.weekdays
    localTime = draft.localTime
    audience = MutationAudience(
      kind: draft.audience,
      targetReferences: draft.targetReferences.isEmpty ? nil : draft.targetReferences,
      airID: draft.airID)
    additionalQuietHours = draft.additionalQuietHours
    policyRevision = draft.policyRevision
    expectedRevision = revision
  }
}

private struct PrincipalWire: Decodable {
  let principalID: String
  let displayName: String
  let permission: String
  let boundAirID: String
  let maxTargetCount: Int
  let allowedCueIDs: [String]
  let allowedAudienceKinds: [AutomationAudience]
  let allowedTargetDigests: [String]
  let issuedAt: String
  let expiresAt: String
  let disabledAt: String
  let revokedAt: String
  let revision: Int64
  enum CodingKeys: String, CodingKey {
    case permission, revision
    case principalID = "principal_id", displayName = "display_name"
    case boundAirID = "bound_air_id", maxTargetCount = "max_target_count"
    case allowedCueIDs = "allowed_cue_ids"
    case allowedAudienceKinds = "allowed_audience_kinds"
    case allowedTargetDigests = "allowed_target_digests"
    case issuedAt = "issued_at", expiresAt = "expires_at"
    case disabledAt = "disabled_at", revokedAt = "revoked_at"
  }
}

private struct PrincipalListWire: Decodable { let principals: [PrincipalWire] }
private struct PrincipalEnvelopeWire: Decodable { let principal: PrincipalWire }
private struct PrincipalIssueWire: Decodable {
  let principal: PrincipalWire
  let secret: String?
  let secretAvailable: Bool
  enum CodingKeys: String, CodingKey {
    case principal, secret
    case secretAvailable = "secret_available"
  }
}

private struct PrincipalMutationWire: Encodable {
  let displayName: String
  let allowedCueIDs: [String]
  let allowedAudienceKinds: [AutomationAudience]
  let allowedTargetReferences: [String]
  let boundAirID: String?
  let maxTargetCount: Int
  let expiresAt: String
  enum CodingKeys: String, CodingKey {
    case displayName = "display_name", allowedCueIDs = "allowed_cue_ids"
    case allowedAudienceKinds = "allowed_audience_kinds"
    case allowedTargetReferences = "allowed_target_references"
    case boundAirID = "bound_air_id", maxTargetCount = "max_target_count"
    case expiresAt = "expires_at"
  }
  init(_ draft: AutomationPrincipalDraft) {
    displayName = draft.displayName
    allowedCueIDs = draft.allowedCueIDs
    allowedAudienceKinds = draft.allowedAudiences
    allowedTargetReferences = draft.targetReferences
    boundAirID = draft.airID
    maxTargetCount = draft.maxTargetCount
    expiresAt = ISO8601DateFormatter().string(from: draft.expiresAt)
  }
}

private struct RevisionWire: Encodable {
  let expectedRevision: Int64
  enum CodingKeys: String, CodingKey { case expectedRevision = "expected_revision" }
}
private struct EmptyWire: Encodable {}
private struct AutomationErrorEnvelope: Decodable {
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
