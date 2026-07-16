import Foundation

public struct TargetsInboxWireLabel: Equatable, Sendable {
  public let key: String
  public let en: String
  public let ru: String
}

public struct TargetsInboxWireAction: Equatable, Sendable {
  public let action: String
  public let label: TargetsInboxWireLabel
}

public struct TargetsInboxTarget: Equatable, Sendable {
  public let reference: String
  public let kind: String
  public let expiresAt: Date
  public let capabilityState: String
  public let capabilities: [String]
  public let label: TargetsInboxWireLabel
}

public struct TargetsInboxItem: Equatable, Sendable {
  public let id: String
  public let historyItemID: String
  public let title: String
  public let expiresAt: Date
  public let availability: String
  public let sender: TargetsInboxWireLabel
  public let source: TargetsInboxWireLabel
  public let requestedDelivery: TargetsInboxWireLabel
  public let effectiveDelivery: TargetsInboxWireLabel
  public let receipt: TargetsInboxWireLabel
  public let actions: [TargetsInboxWireAction]
}

public struct TargetsInboxPage: Equatable, Sendable {
  public let items: [TargetsInboxItem]
  public let nextCursor: String?
}

public struct TargetsInboxHistoryReceipt: Equatable, Sendable {
  public let targetLabel: String
  public let status: TargetsInboxWireLabel
}

public struct TargetsInboxReceiptPage: Equatable, Sendable {
  public let historyItemID: String
  public let items: [TargetsInboxHistoryReceipt]
  public let nextCursor: String?
}

public struct TargetsInboxHistoryItem: Equatable, Sendable {
  public let id: String
  public let title: String
  public let status: TargetsInboxWireLabel
  public let actions: [TargetsInboxWireAction]
  public let playedCount: Int
  public let otherCount: Int
}

public struct TargetsInboxHistoryPage: Equatable, Sendable {
  public let items: [TargetsInboxHistoryItem]
  public let nextCursor: String?
}

public struct TargetsInboxProjection: Equatable, Sendable {
  public let targets: [TargetsInboxTarget]
  public let inbox: TargetsInboxPage
  public let history: TargetsInboxHistoryPage
  public let contentPolicyState: String
}

public enum TargetsInboxClientError: Error, Equatable, Sendable {
  case invalidConfiguration
  case invalidRequest
  case transport
  case redirectRejected
  case responseTooLarge
  case invalidResponse
  case rejected(status: Int, code: String, retryAfterSeconds: Int?)
}

public protocol TargetsInboxAppServicing: Sendable {
  func projection() async throws -> TargetsInboxProjection
  func inbox(cursor: String?) async throws -> TargetsInboxPage
  func history(cursor: String?) async throws -> TargetsInboxHistoryPage
  func receipts(historyItemID: String, cursor: String?) async throws -> TargetsInboxReceiptPage
  func dismissInbox(_ inboxID: String) async throws -> String
  func replayInbox(_ inboxID: String, delivery: String, idempotencyKey: String) async throws -> String
  func deleteHistory(_ historyItemID: String) async throws -> String
  func reportHistory(_ historyItemID: String, reason: String, details: String) async throws -> String
  func muteHistorySender(_ historyItemID: String, idempotencyKey: String) async throws -> String
}

public final class TargetsInboxAppClient: TargetsInboxAppServicing, @unchecked Sendable {
  private static let maximumResponseBytes = 64 * 1_024
  private let origin: CoordinatorOrigin
  private let controlToken: String
  private let transport: any OnboardingHTTPTransport

  public init(
    bundle: CredentialBundle,
    transport: any OnboardingHTTPTransport = URLSessionOnboardingTransport()
  ) throws {
    guard let origin = bundle.coordinatorOrigin,
      origin.isSecureForCredentials,
      let control = bundle.control,
      control.contextStrength == .active,
      control.orbitId != nil,
      CredentialSyntax.lowerHexToken(control.controlToken)
    else { throw TargetsInboxClientError.invalidConfiguration }
    self.origin = origin
    self.controlToken = control.controlToken
    self.transport = transport
  }

  public func projection() async throws -> TargetsInboxProjection {
    async let targetsValue = targets()
    async let inboxValue = inbox(cursor: nil)
    async let historyValue = history(cursor: nil)
    async let policyValue = contentPolicyState()
    return try await TargetsInboxProjection(
      targets: targetsValue, inbox: inboxValue,
      history: historyValue, contentPolicyState: policyValue)
  }

  public func inbox(cursor: String? = nil) async throws -> TargetsInboxPage {
    guard validCursor(cursor, prefix: "ic_") else {
      throw TargetsInboxClientError.invalidRequest
    }
    let response = try await request(
      method: "GET", path: pagePath("/v1/inbox", limit: 20, cursor: cursor),
      allowQuery: true, success: [200])
    let value: InboxResponse = try decode(response.data)
    guard value.contract == "p2-targets-inbox-parity.v1",
      validCursor(value.nextCursor, prefix: "ic_"), unique(value.items.map(\.id))
    else { throw TargetsInboxClientError.invalidResponse }
    return TargetsInboxPage(items: try value.items.map(inboxItem), nextCursor: value.nextCursor)
  }

  public func history(cursor: String? = nil) async throws -> TargetsInboxHistoryPage {
    guard validCursor(cursor, prefix: "hc_") else {
      throw TargetsInboxClientError.invalidRequest
    }
    let response = try await request(
      method: "GET", path: pagePath("/v1/history", limit: 30, cursor: cursor),
      allowQuery: true, success: [200])
    let value: HistoryResponse = try decode(response.data)
    guard value.contract == "p1-history-presence-telegram-v1",
      validCursor(value.nextCursor, prefix: "hc_"), unique(value.items.map(\.historyItemID))
    else { throw TargetsInboxClientError.invalidResponse }
    return TargetsInboxHistoryPage(items: try value.items.map(historyItem), nextCursor: value.nextCursor)
  }

  public func receipts(
    historyItemID: String,
    cursor: String? = nil
  ) async throws -> TargetsInboxReceiptPage {
    guard validPublicID(historyItemID, prefix: "hi_"), validCursor(cursor, prefix: "rc_") else {
      throw TargetsInboxClientError.invalidRequest
    }
    let path = pagePath("/v1/history/\(historyItemID)/receipts", limit: 20, cursor: cursor)
    let response = try await request(method: "GET", path: path, allowQuery: true, success: [200])
    let value: ReceiptResponse = try decode(response.data)
    guard value.contract == "p2-targets-inbox-parity.v1",
      value.historyItemID == historyItemID,
      validCursor(value.nextCursor, prefix: "rc_"), unique(value.items.map(\.targetLabel))
    else { throw TargetsInboxClientError.invalidResponse }
    let items = try value.items.map { item -> TargetsInboxHistoryReceipt in
      guard validHumanText(item.targetLabel) else { throw TargetsInboxClientError.invalidResponse }
      return .init(targetLabel: item.targetLabel, status: try wireLabel(item.presentation.status))
    }
    return .init(historyItemID: historyItemID, items: items, nextCursor: value.nextCursor)
  }

  public func dismissInbox(_ inboxID: String) async throws -> String {
    guard validPublicID(inboxID, prefix: "ib_") else { throw TargetsInboxClientError.invalidRequest }
    let response = try await request(
      method: "DELETE", path: "/v1/inbox/\(inboxID)", success: [200])
    let value: InboxMutationResponse = try decode(response.data)
    guard value.contract == "p2-targets-inbox-parity.v1", value.item.id == inboxID,
      value.item.availability == "dismissed"
    else { throw TargetsInboxClientError.invalidResponse }
    return "inbox_dismissed"
  }

  public func replayInbox(
    _ inboxID: String,
    delivery: String,
    idempotencyKey: String
  ) async throws -> String {
    guard validPublicID(inboxID, prefix: "ib_"),
      ["overlay", "interrupt", "after_current"].contains(delivery),
      validIdempotencyKey(idempotencyKey)
    else { throw TargetsInboxClientError.invalidRequest }
    let response = try await request(
      method: "POST", path: "/v1/inbox/\(inboxID)/replays",
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: ReplayBody(delivery: delivery), success: [200, 201])
    let value: ReplayResponse = try decode(response.data)
    guard value.contract == "p2-targets-inbox-parity.v1",
      validPublicID(value.historyItemID, prefix: "hi_"),
      ["overlay", "interrupt", "after_current"].contains(value.requestedDelivery),
      ["overlay", "interrupt", "after_current"].contains(value.effectiveDelivery)
    else { throw TargetsInboxClientError.invalidResponse }
    return value.reused ? "replay_already_accepted" : "replay_accepted"
  }

  public func deleteHistory(_ historyItemID: String) async throws -> String {
    guard validPublicID(historyItemID, prefix: "hi_") else {
      throw TargetsInboxClientError.invalidRequest
    }
    let response = try await request(
      method: "POST", path: "/v1/history/\(historyItemID)/actions/delete",
      jsonBody: EmptyBody(), success: [200])
    let value: DeleteResponse = try decode(response.data)
    guard value.historyItemID == historyItemID, value.deleted else {
      throw TargetsInboxClientError.invalidResponse
    }
    return "media_deleted"
  }

  public func reportHistory(
    _ historyItemID: String,
    reason: String,
    details: String
  ) async throws -> String {
    guard validPublicID(historyItemID, prefix: "hi_"), validEnum(reason),
      details == details.trimmingCharacters(in: .whitespacesAndNewlines),
      details.utf8.count <= 2_000
    else { throw TargetsInboxClientError.invalidRequest }
    let response = try await request(
      method: "POST", path: "/v1/history/\(historyItemID)/actions/report",
      jsonBody: ReportBody(reason: reason, details: details), success: [200, 201])
    let value: ReportResponse = try decode(response.data)
    guard value.historyItemID == historyItemID else { throw TargetsInboxClientError.invalidResponse }
    return value.reused ? "report_already_received" : "report_received"
  }

  public func muteHistorySender(
    _ historyItemID: String,
    idempotencyKey: String
  ) async throws -> String {
    guard validPublicID(historyItemID, prefix: "hi_"), validIdempotencyKey(idempotencyKey) else {
      throw TargetsInboxClientError.invalidRequest
    }
    let response = try await request(
      method: "POST", path: "/v1/history/\(historyItemID)/actions/block_actor",
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: EmptyBody(), success: [200, 201])
    let value: BlockResponse = try decode(response.data)
    guard validPublicID(value.blockID, prefix: "bl_") else {
      throw TargetsInboxClientError.invalidResponse
    }
    return value.reused ? "sender_already_blocked" : "sender_blocked"
  }

  private func targets() async throws -> [TargetsInboxTarget] {
    let response = try await request(
      method: "GET", path: "/v1/transmission-targets", success: [200])
    let value: TargetResponse = try decode(response.data)
    guard value.contract == "p2-targets-inbox-parity.v1",
      unique(value.targets.map(\.reference))
    else {
      throw TargetsInboxClientError.invalidResponse
    }
    return try value.targets.map { target in
      guard validTargetReference(target.reference), ["barycenter", "pulsar"].contains(target.kind),
        ["known", "mixed", "unknown"].contains(target.capabilityState),
        let expiresAt = parseDate(target.expiresAt), target.expiresAt == formatDate(expiresAt),
        target.presentation.capabilities == target.capabilities
      else { throw TargetsInboxClientError.invalidResponse }
      return TargetsInboxTarget(
        reference: target.reference, kind: target.kind, expiresAt: expiresAt,
        capabilityState: target.capabilityState, capabilities: target.capabilities,
        label: try wireLabel(target.presentation.label))
    }
  }

  private func contentPolicyState() async throws -> String {
    do {
      let response = try await request(
        method: "GET", path: "/v1/content-policy/acceptance", success: [200])
      let value: PolicyResponse = try decode(response.data)
      guard value.contract == "p2-content-policy-consent.v1" else {
        throw TargetsInboxClientError.invalidResponse
      }
      return value.current && value.termsAccepted ? "current" : "stale"
    } catch TargetsInboxClientError.rejected(let status, let code, _)
      where status == 428 && code == "content_policy_acceptance_required"
    {
      return "required"
    }
  }

  private func inboxItem(_ item: InboxResponse.Item) throws -> TargetsInboxItem {
    guard validPublicID(item.id, prefix: "ib_"), validPublicID(item.historyItemID, prefix: "hi_"),
      validHumanText(item.media.title), let expiresAt = parseDate(item.expiresAt),
      ["available", "dismissed", "replayed", "unavailable", "expired"].contains(item.availability)
    else { throw TargetsInboxClientError.invalidResponse }
    return .init(
      id: item.id, historyItemID: item.historyItemID, title: item.media.title,
      expiresAt: expiresAt, availability: item.availability,
      sender: try wireLabel(item.presentation.sender), source: try wireLabel(item.presentation.source),
      requestedDelivery: try wireLabel(item.presentation.requestedDelivery),
      effectiveDelivery: try wireLabel(item.presentation.effectiveDelivery),
      receipt: try wireLabel(item.presentation.receipt),
      actions: try wireActions(item.presentation.actions, exact: item.actions))
  }

  private func historyItem(_ item: HistoryResponse.Item) throws -> TargetsInboxHistoryItem {
    guard validPublicID(item.historyItemID, prefix: "hi_"), validHumanText(item.media.title) else {
      throw TargetsInboxClientError.invalidResponse
    }
    return .init(
      id: item.historyItemID, title: item.media.title,
      status: try wireLabel(item.presentation.status),
      actions: try wireActions(item.presentation.actions, exact: item.actions),
      playedCount: max(0, item.targetCounts?.played ?? 0),
      otherCount: max(0, item.targetCounts?.other ?? 0))
  }

  private func wireActions(
    _ values: [WireAction], exact: [String]
  ) throws -> [TargetsInboxWireAction] {
    guard values.map(\.action) == exact, Set(exact).count == exact.count,
      exact.allSatisfy(validEnum)
    else { throw TargetsInboxClientError.invalidResponse }
    return try values.map { .init(action: $0.action, label: try wireLabel($0.label)) }
  }

  private func wireLabel(_ value: WireLabel) throws -> TargetsInboxWireLabel {
    guard validEnumKey(value.key), validHumanText(value.en), validHumanText(value.ru) else {
      throw TargetsInboxClientError.invalidResponse
    }
    return .init(key: value.key, en: value.en, ru: value.ru)
  }

  private func pagePath(_ base: String, limit: Int, cursor: String?) -> String {
    var components = URLComponents()
    components.queryItems = [URLQueryItem(name: "limit", value: String(limit))]
    if let cursor { components.queryItems?.append(.init(name: "cursor", value: cursor)) }
    return base + "?" + (components.percentEncodedQuery ?? "")
  }

  private func request<Body: Encodable>(
    method: String, path: String, headers: [String: String] = [:],
    jsonBody: Body, success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    let body: Data
    do { body = try encoder.encode(jsonBody) }
    catch { throw TargetsInboxClientError.invalidRequest }
    var allHeaders = headers
    allHeaders["Content-Type"] = "application/json"
    return try await request(
      method: method, path: path, headers: allHeaders, rawBody: body, success: success)
  }

  private func request(
    method: String, path: String, headers: [String: String] = [:],
    rawBody: Data? = nil, allowQuery: Bool = false, success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    let url: URL?
    if allowQuery, let marker = path.firstIndex(of: "?") {
      let endpointPath = String(path[..<marker])
      let query = String(path[path.index(after: marker)...])
      if let base = origin.endpoint(path: endpointPath) {
        var components = URLComponents(url: base, resolvingAgainstBaseURL: false)
        components?.percentEncodedQuery = query
        url = components?.url
      } else { url = nil }
    } else { url = origin.endpoint(path: path) }
    guard let url else { throw TargetsInboxClientError.invalidRequest }
    var request = URLRequest(
      url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: 30)
    request.httpMethod = method
    request.httpBody = rawBody
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    request.setValue("Bearer \(controlToken)", forHTTPHeaderField: "Authorization")
    for (name, value) in headers { request.setValue(value, forHTTPHeaderField: name) }
    let received: HTTPTransportResponse
    do {
      received = try await transport.send(request, maximumResponseBytes: Self.maximumResponseBytes)
    } catch let error as OnboardingClientError where error == .responseTooLarge {
      throw TargetsInboxClientError.responseTooLarge
    } catch { throw TargetsInboxClientError.transport }
    guard received.data.count <= Self.maximumResponseBytes,
      let finalURL = received.response.url,
      let finalOrigin = try? CoordinatorOrigin(finalURL.absoluteString), finalOrigin == origin
    else { throw TargetsInboxClientError.redirectRejected }
    if (300..<400).contains(received.response.statusCode) {
      throw TargetsInboxClientError.redirectRejected
    }
    guard received.response.value(forHTTPHeaderField: "Content-Type")?
      .split(separator: ";", maxSplits: 1).first?
      .trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "application/json"
    else { throw TargetsInboxClientError.invalidResponse }
    guard success.contains(received.response.statusCode) else {
      let envelope: ErrorEnvelope
      do { envelope = try JSONDecoder().decode(ErrorEnvelope.self, from: received.data) }
      catch { throw TargetsInboxClientError.invalidResponse }
      throw TargetsInboxClientError.rejected(
        status: received.response.statusCode, code: envelope.error.code,
        retryAfterSeconds: envelope.error.retryAfterSeconds)
    }
    return received
  }

  private func decode<Value: Decodable>(_ data: Data) throws -> Value {
    do { return try JSONDecoder().decode(Value.self, from: data) }
    catch { throw TargetsInboxClientError.invalidResponse }
  }

  private func parseDate(_ value: String) -> Date? {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter.date(from: value) ?? ISO8601DateFormatter().date(from: value)
  }

  private func formatDate(_ value: Date) -> String {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter.string(from: value)
  }

  private func validTargetReference(_ value: String) -> Bool {
    guard value.hasPrefix("trf_") else { return false }
    let suffix = value.dropFirst(4)
    return suffix.utf8.count == 43 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-")
        .contains($0)
    }
  }

  private func validCursor(_ value: String?, prefix: String) -> Bool {
    guard let value else { return true }
    guard value.hasPrefix(prefix) else { return false }
    let suffix = value.dropFirst(prefix.count)
    return suffix.utf8.count == 64 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "0123456789abcdef").contains($0)
    }
  }

  private func validPublicID(_ value: String, prefix: String) -> Bool {
    guard value.hasPrefix(prefix) else { return false }
    let suffix = value.dropFirst(prefix.count)
    return suffix.count == 26 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "0123456789ABCDEFGHJKMNPQRSTVWXYZ").contains($0)
    }
  }

  private func validIdempotencyKey(_ value: String) -> Bool {
    guard (16...128).contains(value.utf8.count),
      value.first?.isASCII == true, value.first?.isLetter == true || value.first?.isNumber == true
    else { return false }
    return value.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:-")
        .contains($0)
    }
  }

  private func unique(_ values: [String]) -> Bool {
    Set(values).count == values.count
  }

  private func validEnum(_ value: String) -> Bool {
    guard (1...64).contains(value.utf8.count), value.first?.isLetter == true else { return false }
    return value.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyz0123456789_").contains($0)
    }
  }

  private func validEnumKey(_ value: String) -> Bool {
    !value.isEmpty && value.utf8.count <= 128 && value.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyz0123456789_.").contains($0)
    }
  }

  private func validHumanText(_ value: String) -> Bool {
    !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && value.utf8.count <= 512
  }
}

private struct EmptyBody: Encodable {}
private struct ReplayBody: Encodable { let delivery: String }
private struct ReportBody: Encodable { let reason: String; let details: String }
private struct WireLabel: Decodable { let key: String; let en: String; let ru: String }
private struct WireAction: Decodable { let action: String; let label: WireLabel }

private struct TargetResponse: Decodable {
  struct Target: Decodable {
    struct Presentation: Decodable {
      let label: WireLabel
      let capabilityState: WireLabel
      let capabilities: [String]
      enum CodingKeys: String, CodingKey {
        case label, capabilities
        case capabilityState = "capability_state"
      }
    }
    let reference: String
    let kind: String
    let capabilityState: String
    let capabilities: [String]
    let expiresAt: String
    let presentation: Presentation
    enum CodingKeys: String, CodingKey {
      case reference, kind, capabilities, presentation
      case capabilityState = "capability_state"
      case expiresAt = "expires_at"
    }
  }
  let contract: String
  let targets: [Target]
}

private struct InboxResponse: Decodable {
  struct Item: Decodable {
    struct Media: Decodable { let title: String }
    struct Presentation: Decodable {
      let sender: WireLabel
      let source: WireLabel
      let requestedDelivery: WireLabel
      let effectiveDelivery: WireLabel
      let receipt: WireLabel
      let actions: [WireAction]
      enum CodingKeys: String, CodingKey {
        case sender, source, receipt, actions
        case requestedDelivery = "requested_delivery"
        case effectiveDelivery = "effective_delivery"
      }
    }
    let id: String
    let historyItemID: String
    let media: Media
    let availability: String
    let expiresAt: String
    let actions: [String]
    let presentation: Presentation
    enum CodingKeys: String, CodingKey {
      case id, media, availability, actions, presentation
      case historyItemID = "history_item_id"
      case expiresAt = "expires_at"
    }
  }
  let contract: String
  let items: [Item]
  let nextCursor: String?
  enum CodingKeys: String, CodingKey {
    case contract, items
    case nextCursor = "next_cursor"
  }
}

private struct InboxMutationResponse: Decodable {
  struct Item: Decodable { let id: String; let availability: String }
  let contract: String
  let item: Item
}

private struct HistoryResponse: Decodable {
  struct Item: Decodable {
    struct Media: Decodable { let title: String }
    struct Counts: Decodable { let played: Int; let other: Int }
    struct Presentation: Decodable { let status: WireLabel; let actions: [WireAction] }
    let historyItemID: String
    let media: Media
    let targetCounts: Counts?
    let actions: [String]
    let presentation: Presentation
    enum CodingKeys: String, CodingKey {
      case media, actions, presentation
      case historyItemID = "history_item_id"
      case targetCounts = "target_counts"
    }
  }
  let contract: String
  let items: [Item]
  let nextCursor: String?
  enum CodingKeys: String, CodingKey {
    case contract, items
    case nextCursor = "next_cursor"
  }
}

private struct ReceiptResponse: Decodable {
  struct Item: Decodable {
    struct Presentation: Decodable { let status: WireLabel }
    let targetLabel: String
    let presentation: Presentation
    enum CodingKeys: String, CodingKey {
      case presentation
      case targetLabel = "target_label"
    }
  }
  let contract: String
  let historyItemID: String
  let items: [Item]
  let nextCursor: String?
  enum CodingKeys: String, CodingKey {
    case contract, items
    case historyItemID = "history_item_id"
    case nextCursor = "next_cursor"
  }
}

private struct PolicyResponse: Decodable {
  let contract: String
  let current: Bool
  let termsAccepted: Bool
  enum CodingKeys: String, CodingKey {
    case contract, current
    case termsAccepted = "terms_accepted"
  }
}

private struct ReplayResponse: Decodable {
  let contract: String
  let historyItemID: String
  let requestedDelivery: String
  let effectiveDelivery: String
  let reused: Bool
  enum CodingKeys: String, CodingKey {
    case contract, reused
    case historyItemID = "history_item_id"
    case requestedDelivery = "requested_delivery"
    case effectiveDelivery = "effective_delivery"
  }
}

private struct DeleteResponse: Decodable {
  let historyItemID: String
  let deleted: Bool
  enum CodingKeys: String, CodingKey { case historyItemID = "history_item_id"; case deleted }
}
private struct ReportResponse: Decodable {
  let historyItemID: String
  let reused: Bool
  enum CodingKeys: String, CodingKey { case historyItemID = "history_item_id"; case reused }
}
private struct BlockResponse: Decodable {
  let blockID: String
  let reused: Bool
  enum CodingKeys: String, CodingKey { case blockID = "block_id"; case reused }
}
private struct ErrorEnvelope: Decodable {
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
