import Foundation

public struct HTTPTransportResponse: @unchecked Sendable {
  public let data: Data
  public let response: HTTPURLResponse

  public init(data: Data, response: HTTPURLResponse) {
    self.data = data
    self.response = response
  }
}

public protocol OnboardingHTTPTransport: Sendable {
  func send(_ request: URLRequest, maximumResponseBytes: Int) async throws -> HTTPTransportResponse
}

private final class BoundedResponseDelegate: NSObject, URLSessionDataDelegate,
  @unchecked Sendable
{
  private let lock = NSLock()
  private let maximumResponseBytes: Int
  private var data = Data()
  private var response: HTTPURLResponse?
  private var task: URLSessionDataTask?
  private var continuation: CheckedContinuation<HTTPTransportResponse, Error>?
  private var cancellationRequested = false

  init(maximumResponseBytes: Int) {
    self.maximumResponseBytes = maximumResponseBytes
    data.reserveCapacity(min(maximumResponseBytes, 4_096))
  }

  func perform(_ request: URLRequest, session: URLSession) async throws -> HTTPTransportResponse {
    try await withTaskCancellationHandler {
      try Task.checkCancellation()
      return try await withCheckedThrowingContinuation { continuation in
        lock.lock()
        if cancellationRequested {
          lock.unlock()
          continuation.resume(throwing: OnboardingClientError.cancelled)
          return
        }
        self.continuation = continuation
        let task = session.dataTask(with: request)
        self.task = task
        lock.unlock()
        task.resume()
      }
    } onCancel: {
      self.cancel()
    }
  }

  func urlSession(
    _ session: URLSession,
    task: URLSessionTask,
    willPerformHTTPRedirection response: HTTPURLResponse,
    newRequest request: URLRequest,
    completionHandler: @escaping (URLRequest?) -> Void
  ) {
    completionHandler(nil)
  }

  func urlSession(
    _ session: URLSession,
    dataTask: URLSessionDataTask,
    didReceive response: URLResponse,
    completionHandler: @escaping (URLSession.ResponseDisposition) -> Void
  ) {
    guard let http = response as? HTTPURLResponse else {
      completionHandler(.cancel)
      finish(throwing: OnboardingClientError.invalidResponse)
      return
    }
    if http.expectedContentLength > Int64(maximumResponseBytes) {
      completionHandler(.cancel)
      finish(throwing: OnboardingClientError.responseTooLarge)
      return
    }
    lock.lock()
    self.response = http
    lock.unlock()
    completionHandler(.allow)
  }

  func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive chunk: Data) {
    lock.lock()
    let exceedsLimit = chunk.count > maximumResponseBytes - data.count
    if !exceedsLimit { data.append(chunk) }
    lock.unlock()
    if exceedsLimit {
      dataTask.cancel()
      finish(throwing: OnboardingClientError.responseTooLarge)
    }
  }

  func urlSession(
    _ session: URLSession,
    task: URLSessionTask,
    didCompleteWithError error: (any Error)?
  ) {
    if let error {
      let nsError = error as NSError
      if nsError.domain == NSURLErrorDomain && nsError.code == URLError.cancelled.rawValue {
        finish(throwing: OnboardingClientError.cancelled)
      } else {
        finish(throwing: error)
      }
      return
    }
    lock.lock()
    let response = self.response
    let data = self.data
    lock.unlock()
    guard let response else {
      finish(throwing: OnboardingClientError.invalidResponse)
      return
    }
    finish(returning: HTTPTransportResponse(data: data, response: response))
  }

  private func cancel() {
    lock.lock()
    cancellationRequested = true
    let task = self.task
    let continuation = self.continuation
    self.continuation = nil
    lock.unlock()
    task?.cancel()
    continuation?.resume(throwing: OnboardingClientError.cancelled)
  }

  private func finish(returning value: HTTPTransportResponse) {
    lock.lock()
    let continuation = self.continuation
    self.continuation = nil
    lock.unlock()
    continuation?.resume(returning: value)
  }

  private func finish(throwing error: any Error) {
    lock.lock()
    let continuation = self.continuation
    self.continuation = nil
    lock.unlock()
    continuation?.resume(throwing: error)
  }
}

public final class URLSessionOnboardingTransport: OnboardingHTTPTransport, @unchecked Sendable {
  private let lock = NSLock()
  private let configuration: URLSessionConfiguration

  public init(configuration: URLSessionConfiguration = .ephemeral) {
    configuration.urlCache = nil
    configuration.requestCachePolicy = .reloadIgnoringLocalAndRemoteCacheData
    configuration.httpCookieStorage = nil
    configuration.httpShouldSetCookies = false
    self.configuration = configuration.copy() as! URLSessionConfiguration
  }

  public func send(_ request: URLRequest, maximumResponseBytes: Int) async throws
    -> HTTPTransportResponse
  {
    guard (1...OnboardingHTTPClient.hardMaximumResponseBytes).contains(maximumResponseBytes) else {
      throw OnboardingClientError.invalidRequest
    }
    let configuration = configurationCopy()
    let delegate = BoundedResponseDelegate(maximumResponseBytes: maximumResponseBytes)
    let queue = OperationQueue()
    queue.maxConcurrentOperationCount = 1
    let session = URLSession(configuration: configuration, delegate: delegate, delegateQueue: queue)
    do {
      let result = try await delegate.perform(request, session: session)
      session.finishTasksAndInvalidate()
      return result
    } catch {
      session.invalidateAndCancel()
      throw error
    }
  }

  private func configurationCopy() -> URLSessionConfiguration {
    lock.lock()
    defer { lock.unlock() }
    return configuration.copy() as! URLSessionConfiguration
  }
}

public enum OnboardingAPIErrorCode: String, Equatable, Sendable {
  case invalidRequest = "invalid_request"
  case unauthorized
  case insufficientCapability = "insufficient_capability"
  case credentialInvalid = "credential_invalid"
  case tooManyAttempts = "too_many_attempts"
  case internalError = "internal_error"
}

public enum OnboardingClientError: Error, Equatable, LocalizedError, Sendable {
  case invalidOrigin
  case insecureTransport
  case invalidRequest
  case transport
  case cancelled
  case responseTooLarge
  case redirectRejected
  case invalidResponse
  case api(status: Int, code: OnboardingAPIErrorCode, retryAfterSeconds: Int?)
  case storage

  public var errorDescription: String? {
    switch self {
    case .invalidOrigin, .insecureTransport: return "The coordinator address is not permitted."
    case .invalidRequest: return "The request contains invalid values."
    case .transport: return "The coordinator could not be reached."
    case .cancelled: return "The operation was cancelled."
    case .api(let status, let code, let retry):
      if code == .tooManyAttempts, let retry {
        return "Try again in \(retry) seconds."
      }
      return "The coordinator rejected the operation (\(status), \(code.rawValue))."
    case .storage: return "Protected credentials could not be saved safely."
    default: return "The coordinator returned an invalid response."
    }
  }
}

public struct CreatedOrbit: Sendable, CustomStringConvertible, CustomDebugStringConvertible {
  public let title: String
  public let bundle: CredentialBundle
  public let recovery: OneTimeRecoveryMaterial
  public var description: String { "CreatedOrbit(credentials: <redacted>, recovery: <redacted>)" }
  public var debugDescription: String { description }
}

public struct JoinedOrbit: Sendable, CustomStringConvertible, CustomDebugStringConvertible {
  public let title: String
  public let bundle: CredentialBundle
  public var description: String { "JoinedOrbit(credentials: <redacted>)" }
  public var debugDescription: String { description }
}

public struct DeviceInvite: Sendable, CustomStringConvertible, CustomDebugStringConvertible {
  public let code: String
  public let intendedRole: CredentialRole
  public let expiresAt: Date
  public var description: String {
    "DeviceInvite(code: <redacted>, role: \(intendedRole.rawValue))"
  }
  public var debugDescription: String { description }
}

public struct TelegramLinkCode: Sendable, CustomStringConvertible, CustomDebugStringConvertible {
  public let code: String
  public let desiredRole: CredentialRole
  public let expiresAt: Date
  public let botUsername: String
  public var description: String { "TelegramLinkCode(<redacted>)" }
  public var debugDescription: String { description }
}

public struct ActorCredentialContext: Equatable, Sendable {
  public let orbitId: Int64
  public let actorId: Int64
  public let role: CredentialRole
}

public enum ActorContextProbe: Equatable, Sendable {
  case active(ActorCredentialContext)
  case limited
  case unauthorized
  case rateLimited(Int)
}

public final class OnboardingHTTPClient: @unchecked Sendable {
  public static let hardMaximumResponseBytes = 64 * 1_024
  public let origin: CoordinatorOrigin
  private let transport: any OnboardingHTTPTransport
  private let maximumResponseBytes: Int

  public init(
    coordinator: String,
    transport: any OnboardingHTTPTransport = URLSessionOnboardingTransport(),
    maximumResponseBytes: Int = 64 * 1024
  ) throws {
    do { origin = try CoordinatorOrigin(coordinator) } catch {
      throw OnboardingClientError.invalidOrigin
    }
    guard origin.isSecureForCredentials else { throw OnboardingClientError.insecureTransport }
    guard (1...Self.hardMaximumResponseBytes).contains(maximumResponseBytes) else {
      throw OnboardingClientError.invalidRequest
    }
    self.transport = transport
    self.maximumResponseBytes = maximumResponseBytes
  }

  public func createOrbit(title: String, installationAttemptID: String) async throws -> CreatedOrbit
  {
    let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !cleanTitle.isEmpty, cleanTitle.utf8.count <= 120,
      Self.validAttemptID(installationAttemptID)
    else {
      throw OnboardingClientError.invalidRequest
    }
    let object = try await request(
      method: "POST", path: "/v1/onboarding/orbits",
      body: CreateRequest(title: cleanTitle, installationAttemptID: installationAttemptID),
      successStatus: 201, allowed403: []
    )
    try object.exactKeys([
      "orbit_id", "title", "actor_id", "role", "slot", "node_token",
      "control_token", "recovery_id", "recovery_secret", "shown_once",
    ])
    guard let orbitID = object["orbit_id"]?.int64, orbitID > 0,
      let responseTitle = object["title"]?.string,
      responseTitle.utf8.elementsEqual(cleanTitle.utf8),
      let actorID = object["actor_id"]?.int64, actorID > 0,
      let role = Self.role(object["role"]), role == .primary,
      let slot = object["slot"]?.string, CredentialSyntax.slot(slot),
      let nodeToken = object["node_token"]?.string, CredentialSyntax.lowerHexToken(nodeToken),
      let controlToken = object["control_token"]?.string,
      CredentialSyntax.lowerHexToken(controlToken),
      let recoveryID = object["recovery_id"]?.string, CredentialSyntax.recoveryID(recoveryID),
      let recoveryRaw = object["recovery_secret"]?.string,
      CredentialSyntax.recoverySecret(recoveryRaw) == recoveryRaw,
      object["shown_once"]?.bool == true,
      let ws = origin.webSocketURL?.absoluteString
    else {
      throw OnboardingClientError.invalidResponse
    }
    let secret = try RecoverySecret(validated: recoveryRaw)
    let bundle = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(orbitId: orbitID, slot: slot, nodeToken: nodeToken, wsUrl: ws),
      control: ControlCapability(
        actorId: actorID, orbitId: orbitID, role: role, controlToken: controlToken),
      recovery: RecoveryMetadata(actorId: actorID, recoveryId: recoveryID)
    )
    return CreatedOrbit(
      title: responseTitle,
      bundle: bundle,
      recovery: OneTimeRecoveryMaterial(actorId: actorID, recoveryId: recoveryID, secret: secret)
    )
  }

  func issueDeviceInvite(
    control: ControlCapability,
    intendedRole: CredentialRole = .companion
  ) async throws -> DeviceInvite {
    guard intendedRole == .companion || intendedRole == .satellite,
      CredentialSyntax.lowerHexToken(control.controlToken)
    else {
      throw OnboardingClientError.invalidRequest
    }
    let object = try await request(
      method: "POST", path: "/v1/device-invites", bearer: control.controlToken,
      body: RoleRequest(role: intendedRole.rawValue, key: "intended_role"),
      successStatus: 201, allowed403: [.insufficientCapability]
    )
    try object.exactKeys(["invite_code", "intended_role", "expires_at"])
    guard let code = object["invite_code"]?.string, CredentialSyntax.canonicalHumanCode(code),
      let role = Self.role(object["intended_role"]), role == intendedRole,
      let expires = Self.date(object["expires_at"])
    else {
      throw OnboardingClientError.invalidResponse
    }
    return DeviceInvite(code: code, intendedRole: role, expiresAt: expires)
  }

  public func consumeDeviceInvite(_ enteredCode: String) async throws -> JoinedOrbit {
    guard let code = CredentialSyntax.humanCode(enteredCode) else {
      throw OnboardingClientError.invalidRequest
    }
    let object = try await request(
      method: "POST", path: "/v1/device-invites/consume",
      body: InviteConsumeRequest(inviteCode: code), successStatus: 200,
      allowed403: [.credentialInvalid]
    )
    try object.exactKeys([
      "orbit_id", "title", "actor_id", "role", "slot", "node_token", "control_token",
    ])
    guard let orbitID = object["orbit_id"]?.int64, orbitID > 0,
      let title = object["title"]?.string,
      !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
      title.utf8.count <= 120,
      let actorID = object["actor_id"]?.int64, actorID > 0,
      let role = Self.role(object["role"]), role == .companion || role == .satellite,
      let slot = object["slot"]?.string, CredentialSyntax.slot(slot),
      let nodeToken = object["node_token"]?.string, CredentialSyntax.lowerHexToken(nodeToken),
      let controlToken = object["control_token"]?.string,
      CredentialSyntax.lowerHexToken(controlToken),
      let ws = origin.webSocketURL?.absoluteString
    else {
      throw OnboardingClientError.invalidResponse
    }
    let bundle = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(orbitId: orbitID, slot: slot, nodeToken: nodeToken, wsUrl: ws),
      control: ControlCapability(
        actorId: actorID, orbitId: orbitID, role: role, controlToken: controlToken)
    )
    return JoinedOrbit(title: title, bundle: bundle)
  }

  func probe(token: String) async throws -> ActorContextProbe {
    guard CredentialSyntax.lowerHexToken(token) else { throw OnboardingClientError.invalidRequest }
    do {
      let object = try await request(
        method: "GET", path: "/v1/actor/context", bearer: token,
        body: Optional<EmptyRequest>.none, successStatus: 200,
        allowed403: [.insufficientCapability]
      )
      return .active(try Self.context(object))
    } catch OnboardingClientError.api(let status, let code, let retry) {
      if status == 401 && code == .unauthorized { return .unauthorized }
      if status == 403 && code == .insufficientCapability { return .limited }
      if status == 429 && code == .tooManyAttempts, let retry { return .rateLimited(retry) }
      throw OnboardingClientError.api(status: status, code: code, retryAfterSeconds: retry)
    }
  }

  func consumeRecovery(
    recoveryID: String,
    secret: RecoverySecret,
    replacementControlToken: String
  ) async throws -> ActorCredentialContext {
    guard CredentialSyntax.recoveryID(recoveryID),
      CredentialSyntax.lowerHexToken(replacementControlToken)
    else {
      throw OnboardingClientError.invalidRequest
    }
    let object = try await request(
      method: "POST", path: "/v1/recovery/consume",
      body: RecoveryConsumeRequest(
        recoveryID: recoveryID,
        recoverySecret: secret.reveal(),
        replacementControlToken: replacementControlToken
      ),
      successStatus: 200, allowed403: [.credentialInvalid]
    )
    return try Self.context(object)
  }

  func rotateRecovery(control: ControlCapability) async throws -> OneTimeRecoveryMaterial {
    guard CredentialSyntax.lowerHexToken(control.controlToken) else {
      throw OnboardingClientError.invalidRequest
    }
    let object = try await request(
      method: "POST", path: "/v1/recovery/rotate", bearer: control.controlToken,
      body: EmptyRequest(), successStatus: 200, allowed403: [.insufficientCapability]
    )
    try object.exactKeys(["actor_id", "recovery_id", "recovery_secret", "shown_once"])
    guard let actorID = object["actor_id"]?.int64, actorID == control.actorId,
      let recoveryID = object["recovery_id"]?.string, CredentialSyntax.recoveryID(recoveryID),
      let raw = object["recovery_secret"]?.string,
      CredentialSyntax.recoverySecret(raw) == raw,
      object["shown_once"]?.bool == true
    else {
      throw OnboardingClientError.invalidResponse
    }
    return OneTimeRecoveryMaterial(
      actorId: actorID, recoveryId: recoveryID,
      secret: try RecoverySecret(validated: raw)
    )
  }

  func issueTelegramLink(
    control: ControlCapability,
    desiredRole: CredentialRole = .companion
  ) async throws -> TelegramLinkCode {
    guard desiredRole == .companion || desiredRole == .satellite,
      CredentialSyntax.lowerHexToken(control.controlToken)
    else {
      throw OnboardingClientError.invalidRequest
    }
    let object = try await request(
      method: "POST", path: "/v1/telegram-links", bearer: control.controlToken,
      body: RoleRequest(role: desiredRole.rawValue, key: "desired_role"),
      successStatus: 201, allowed403: [.insufficientCapability]
    )
    try object.exactKeys(["link_code", "desired_role", "expires_at", "bot_username"])
    guard let code = object["link_code"]?.string, CredentialSyntax.canonicalHumanCode(code),
      let role = Self.role(object["desired_role"]), role == desiredRole,
      let expires = Self.date(object["expires_at"]),
      let username = object["bot_username"]?.string, !username.hasPrefix("@"),
      Self.validBotUsername(username)
    else {
      throw OnboardingClientError.invalidResponse
    }
    return TelegramLinkCode(
      code: code, desiredRole: role, expiresAt: expires, botUsername: username)
  }

  private func request<Body: Encodable>(
    method: String,
    path: String,
    bearer: String? = nil,
    body: Body?,
    successStatus: Int,
    allowed403: Set<OnboardingAPIErrorCode>
  ) async throws -> [String: StrictJSONValue] {
    guard let url = origin.endpoint(path: path) else { throw OnboardingClientError.invalidRequest }
    var request = URLRequest(
      url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData, timeoutInterval: 15)
    request.httpMethod = method
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    if let bearer {
      guard CredentialSyntax.lowerHexToken(bearer) else {
        throw OnboardingClientError.invalidRequest
      }
      request.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization")
    }
    if let body {
      request.setValue("application/json", forHTTPHeaderField: "Content-Type")
      do {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        request.httpBody = try encoder.encode(body)
      } catch {
        throw OnboardingClientError.invalidRequest
      }
    }

    let received: HTTPTransportResponse
    do {
      received = try await transport.send(request, maximumResponseBytes: maximumResponseBytes)
    } catch is CancellationError {
      throw OnboardingClientError.cancelled
    } catch let error as URLError where error.code == .cancelled {
      throw OnboardingClientError.cancelled
    } catch let error as OnboardingClientError {
      throw error
    } catch {
      throw OnboardingClientError.transport
    }
    guard received.data.count <= maximumResponseBytes else {
      throw OnboardingClientError.responseTooLarge
    }
    guard let finalURL = received.response.url,
      let finalOrigin = try? CoordinatorOrigin(finalURL.absoluteString),
      finalOrigin == origin
    else {
      throw OnboardingClientError.redirectRejected
    }
    if (300..<400).contains(received.response.statusCode) {
      throw OnboardingClientError.redirectRejected
    }
    let mediaType = received.response.value(forHTTPHeaderField: "Content-Type")?
      .split(separator: ";", maxSplits: 1).first?
      .trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    guard mediaType == "application/json" else {
      throw OnboardingClientError.invalidResponse
    }

    var parser = StrictJSONParser(received.data)
    let parsed: StrictJSONValue
    do { parsed = try parser.parse() } catch { throw OnboardingClientError.invalidResponse }
    guard let object = parsed.object else { throw OnboardingClientError.invalidResponse }
    if received.response.statusCode == successStatus { return object }
    throw try decodeAPIError(
      status: received.response.statusCode, object: object,
      retryHeader: received.response.value(forHTTPHeaderField: "Retry-After"),
      allowed403: allowed403
    )
  }

  private func decodeAPIError(
    status: Int,
    object: [String: StrictJSONValue],
    retryHeader: String?,
    allowed403: Set<OnboardingAPIErrorCode>
  ) throws -> OnboardingClientError {
    try object.exactKeys(["error"])
    guard let inner = object["error"]?.object else { throw OnboardingClientError.invalidResponse }
    try inner.exactKeys(["code", "message", "retry_after_seconds"])
    guard let rawCode = inner["code"]?.string,
      let code = OnboardingAPIErrorCode(rawValue: rawCode),
      let message = inner["message"]?.string,
      message == Self.message(for: code)
    else {
      throw OnboardingClientError.invalidResponse
    }
    guard let retryValue = inner["retry_after_seconds"] else {
      throw OnboardingClientError.invalidResponse
    }
    let retry: Int?
    let retryIsNull: Bool
    if case .null = retryValue {
      retry = nil
      retryIsNull = true
    } else {
      guard case .number(let literal) = retryValue,
        let seconds = retryValue.positiveInt,
        literal == String(seconds)
      else {
        throw OnboardingClientError.invalidResponse
      }
      retry = seconds
      retryIsNull = false
    }

    let valid: Bool
    switch (status, code) {
    case (400, .invalidRequest), (401, .unauthorized):
      valid = retryIsNull && retryHeader == nil
    case (403, _):
      valid = allowed403.contains(code) && retryIsNull && retryHeader == nil
    case (429, .tooManyAttempts):
      valid = !retryIsNull && retryHeader == retry.map(String.init)
    case (500...599, .internalError):
      valid = retryIsNull && retryHeader == nil
    default: valid = false
    }
    guard valid else { throw OnboardingClientError.invalidResponse }
    return .api(status: status, code: code, retryAfterSeconds: retry)
  }

  private static func context(_ object: [String: StrictJSONValue]) throws -> ActorCredentialContext
  {
    try object.exactKeys(["orbit_id", "actor_id", "role"])
    guard let orbitID = object["orbit_id"]?.int64, orbitID > 0,
      let actorID = object["actor_id"]?.int64, actorID > 0,
      let role = role(object["role"])
    else { throw OnboardingClientError.invalidResponse }
    return ActorCredentialContext(orbitId: orbitID, actorId: actorID, role: role)
  }

  private static func role(_ value: StrictJSONValue?) -> CredentialRole? {
    value?.string.flatMap(CredentialRole.init(rawValue:))
  }

  private static func date(_ value: StrictJSONValue?) -> Date? {
    guard let string = value?.string else { return nil }
    return ISO8601DateFormatter().date(from: string)
  }

  private static func validAttemptID(_ value: String) -> Bool {
    (16...128).contains(value.utf8.count)
      && value.utf8.allSatisfy {
        ($0 >= 65 && $0 <= 90) || ($0 >= 97 && $0 <= 122) || ($0 >= 48 && $0 <= 57) || $0 == 95
          || $0 == 45
      }
  }

  private static func validBotUsername(_ value: String) -> Bool {
    let clean = value.hasPrefix("@") ? String(value.dropFirst()) : value
    return (5...32).contains(clean.count)
      && clean.utf8.allSatisfy {
        ($0 >= 65 && $0 <= 90) || ($0 >= 97 && $0 <= 122) || ($0 >= 48 && $0 <= 57) || $0 == 95
      }
  }

  private static func message(for code: OnboardingAPIErrorCode) -> String {
    switch code {
    case .invalidRequest: return "The request is malformed or contains invalid parameters."
    case .unauthorized: return "Authentication is required."
    case .insufficientCapability: return "This token does not have the required capability."
    case .credentialInvalid: return "The provided credential is not valid."
    case .tooManyAttempts: return "Too many attempts. Please wait before retrying."
    case .internalError: return "An internal error occurred."
    }
  }
}

private struct CreateRequest: Encodable {
  let title: String
  let installationAttemptID: String
  enum CodingKeys: String, CodingKey {
    case title
    case installationAttemptID = "installation_attempt_id"
  }
}

private struct InviteConsumeRequest: Encodable {
  let inviteCode: String
  enum CodingKeys: String, CodingKey { case inviteCode = "invite_code" }
}

private struct RecoveryConsumeRequest: Encodable {
  let recoveryID: String
  let recoverySecret: String
  let replacementControlToken: String
  enum CodingKeys: String, CodingKey {
    case recoveryID = "recovery_id"
    case recoverySecret = "recovery_secret"
    case replacementControlToken = "replacement_control_token"
  }
}

private struct EmptyRequest: Encodable {}

private struct RoleRequest: Encodable {
  let role: String
  let key: String
  func encode(to encoder: Encoder) throws {
    var container = encoder.container(keyedBy: DynamicKey.self)
    try container.encode(role, forKey: DynamicKey(stringValue: key)!)
  }
  private struct DynamicKey: CodingKey {
    let stringValue: String
    let intValue: Int? = nil
    init?(stringValue: String) { self.stringValue = stringValue }
    init?(intValue: Int) { return nil }
  }
}
