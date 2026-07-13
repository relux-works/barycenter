import Foundation

/// The source-compatible node view used by the existing pairing startup.
/// New saves are folded into ``CredentialBundle`` in Keychain. The Codable
/// shape remains unchanged so legacy `node-credentials.json` and Keychain
/// items can be migrated without changing a token, slot, or WebSocket URL.
public struct NodeCredentials: Codable, Equatable, Hashable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public var orbitId: Int64
  public var slot: String
  public var token: String
  public var wsUrl: String

  enum CodingKeys: String, CodingKey {
    case orbitId = "orbit_id"
    case slot
    case token = "token"
    case wsUrl = "ws_url"
  }

  public static func fileURL(besideConfig configPath: String) -> URL {
    URL(fileURLWithPath: configPath).deletingLastPathComponent()
      .appendingPathComponent("node-credentials.json")
  }

  public static func load(besideConfig configPath: String) -> NodeCredentials? {
    let url = fileURL(besideConfig: configPath)
    guard let data = try? Data(contentsOf: url) else { return nil }
    return try? JSONDecoder().decode(NodeCredentials.self, from: data)
  }

  /// Retained for source compatibility. New saves go to Keychain; only the
  /// corresponding loader may read pre-upgrade plaintext for migration.
  public func save(besideConfig configPath: String) throws {
    _ = configPath
    try CredentialsStore.save(self)
  }

  public var description: String {
    "NodeCredentials(orbit: \(orbitId), token: <redacted>, endpoint: <redacted>)"
  }
  public var debugDescription: String { description }
}

public enum PairingError: Error, CustomStringConvertible {
  case http(Int)
  case transport
  case badResponse

  public var description: String {
    switch self {
    case .http(let code):
      if code == 403 { return "код не подошёл или истёк — попроси новый: /pair у бота" }
      if code == 409 { return "в орбите нет свободных мест" }
      return "сервер ответил с ошибкой \(code)"
    case .transport: return "не удалось связаться с координатором"
    case .badResponse: return "непонятный ответ координатора"
    }
  }
}

public struct PairingClient: Sendable {
  public static let maximumResponseBytes = 16 * 1_024
  private let origin: CoordinatorOrigin
  private let transport: any OnboardingHTTPTransport

  public init(
    coordinatorBase: String,
    transport: any OnboardingHTTPTransport = URLSessionOnboardingTransport()
  ) throws {
    origin = try CoordinatorOrigin(coordinatorBase)
    guard origin.isSecureForCredentials else { throw PairingError.transport }
    self.transport = transport
  }

  public func pair(code: String) async -> Result<NodeCredentials, PairingError> {
    let alphabet = Set("ABCDEFGHJKMNPQRSTVWXYZ23456789")
    guard code.count == 8, code.allSatisfy({ alphabet.contains($0) }),
      let url = origin.endpoint(path: "/pair")
    else { return .failure(.badResponse) }
    var request = URLRequest(url: url, timeoutInterval: 15)
    request.httpMethod = "POST"
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    request.setValue("application/json", forHTTPHeaderField: "Content-Type")
    request.httpBody = try? JSONSerialization.data(withJSONObject: ["code": code])

    let received: HTTPTransportResponse
    do {
      received = try await transport.send(
        request, maximumResponseBytes: Self.maximumResponseBytes)
    } catch {
      return .failure(.transport)
    }
    guard received.data.count <= Self.maximumResponseBytes,
      let finalURL = received.response.url,
      let finalOrigin = try? CoordinatorOrigin(finalURL.absoluteString),
      finalOrigin == origin,
      !(300..<400).contains(received.response.statusCode)
    else { return .failure(.transport) }
    guard received.response.statusCode == 200 else {
      return .failure(.http(received.response.statusCode))
    }
    let mediaType = received.response.value(forHTTPHeaderField: "Content-Type")?
      .split(separator: ";", maxSplits: 1).first?
      .trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    guard mediaType == "application/json" else { return .failure(.badResponse) }

    var parser = StrictJSONParser(received.data)
    guard let parsed = try? parser.parse(), let object = parsed.object,
      (try? object.exactKeys(["orbit_id", "slot", "token", "ws_url"])) != nil,
      let orbitID = object["orbit_id"]?.int64, orbitID > 0,
      let slot = object["slot"]?.string, CredentialSyntax.slot(slot),
      let token = object["token"]?.string, CredentialSyntax.lowerHexToken(token),
      let wsURL = object["ws_url"]?.string,
      CredentialSyntax.canonicalWebSocketURL(wsURL, origin: origin)
    else { return .failure(.badResponse) }
    return .success(
      NodeCredentials(orbitId: orbitID, slot: slot, token: token, wsUrl: wsURL))
  }
}

private final class PairingResultBox: @unchecked Sendable {
  private let lock = NSLock()
  private var result: Result<NodeCredentials, PairingError> = .failure(.badResponse)

  func store(_ result: Result<NodeCredentials, PairingError>) {
    lock.lock()
    self.result = result
    lock.unlock()
  }

  func load() -> Result<NodeCredentials, PairingError> {
    lock.lock()
    defer { lock.unlock() }
    return result
  }
}

/// Exchanges a pairing code for credentials: POST {coordinator}/pair.
/// Synchronous by design — it runs from the CLI before anything starts.
public func pairNode(code: String, coordinatorBase: String) -> Result<NodeCredentials, PairingError>
{
  guard let client = try? PairingClient(coordinatorBase: coordinatorBase) else {
    return .failure(.transport)
  }
  let semaphore = DispatchSemaphore(value: 0)
  let box = PairingResultBox()
  Task {
    box.store(await client.pair(code: code))
    semaphore.signal()
  }
  semaphore.wait()
  return box.load()
}
