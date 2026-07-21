import Foundation

public enum CredentialRole: String, Codable, CaseIterable, Sendable {
  case primary
  case companion
  case satellite
}

public struct NodeCapability: Codable, Equatable, Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public var orbitId: Int64
  public var slot: String
  public var nodeToken: String
  public var wsUrl: String

  public init(orbitId: Int64, slot: String, nodeToken: String, wsUrl: String) {
    self.orbitId = orbitId
    self.slot = slot
    self.nodeToken = nodeToken
    self.wsUrl = wsUrl
  }

  enum CodingKeys: String, CodingKey {
    case orbitId = "orbit_id"
    case slot
    case nodeToken = "node_token"
    case wsUrl = "ws_url"
  }

  public var description: String {
    "NodeCapability(<redacted>)"
  }
  public var debugDescription: String { description }

  public var legacyView: NodeCredentials {
    NodeCredentials(orbitId: orbitId, slot: slot, token: nodeToken, wsUrl: wsUrl)
  }
}

public enum ControlContextStrength: String, Codable, CaseIterable, Sendable {
  case active
  case limited
}

public struct ControlCapability: Codable, Equatable, Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public var actorId: Int64
  public var orbitId: Int64?
  public var role: CredentialRole?
  public var controlToken: String
  public var contextStrength: ControlContextStrength

  public init(
    actorId: Int64,
    orbitId: Int64?,
    role: CredentialRole?,
    controlToken: String,
    contextStrength: ControlContextStrength = .active
  ) {
    self.actorId = actorId
    self.orbitId = orbitId
    self.role = role
    self.controlToken = controlToken
    self.contextStrength = contextStrength
  }

  enum CodingKeys: String, CodingKey {
    case actorId = "actor_id"
    case orbitId = "orbit_id"
    case role
    case controlToken = "control_token"
    case contextStrength = "context_strength"
  }

  public var description: String { "ControlCapability(<redacted>)" }
  public var debugDescription: String { description }
}

public struct RecoveryMetadata: Codable, Equatable, Sendable {
  public var actorId: Int64
  public var recoveryId: String
  public var explicitBackupAcknowledged: Bool

  public init(actorId: Int64, recoveryId: String, explicitBackupAcknowledged: Bool = false) {
    self.actorId = actorId
    self.recoveryId = recoveryId
    self.explicitBackupAcknowledged = explicitBackupAcknowledged
  }

  enum CodingKeys: String, CodingKey {
    case actorId = "actor_id"
    case recoveryId = "recovery_id"
    case explicitBackupAcknowledged = "explicit_backup_acknowledged"
  }
}

public struct CredentialBundle: Codable, Equatable, Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public static let currentVersion = 2

  public var version: Int
  public var coordinatorOrigin: CoordinatorOrigin?
  public var node: NodeCapability?
  public var control: ControlCapability?
  public var recovery: RecoveryMetadata?

  public init(
    version: Int = CredentialBundle.currentVersion,
    coordinatorOrigin: CoordinatorOrigin? = nil,
    node: NodeCapability? = nil,
    control: ControlCapability? = nil,
    recovery: RecoveryMetadata? = nil
  ) {
    self.version = version
    self.coordinatorOrigin = coordinatorOrigin
    self.node = node
    self.control = control
    self.recovery = recovery
  }

  enum CodingKeys: String, CodingKey {
    case version
    case coordinatorOrigin = "coordinator_origin"
    case node, control, recovery
  }

  public var description: String {
    "CredentialBundle(version: \(version), node: \(node == nil ? "none" : "present"), control: \(control == nil ? "none" : "present"), secrets: <redacted>)"
  }
  public var debugDescription: String { description }

  public var nodeCredentials: NodeCredentials? { node?.legacyView }

  /// A freshly created Barycenter remains intentionally unusable until its
  /// one-time recovery material has been exported and acknowledged. Joined
  /// installations and legacy node-only bundles have no recovery gate.
  public var activationEligibleNodeCredentials: NodeCredentials? {
    guard recovery?.explicitBackupAcknowledged != false else { return nil }
    return nodeCredentials
  }

  public static func legacy(_ credentials: NodeCredentials) -> CredentialBundle {
    CredentialBundle(
      node: NodeCapability(
        orbitId: credentials.orbitId,
        slot: credentials.slot,
        nodeToken: credentials.token,
        wsUrl: credentials.wsUrl
      ))
  }
}

/// One-time plaintext recovery material. Deliberately not Codable and not
/// CustomStringConvertible. The value can leave memory only through an
/// explicit export operation.
public final class RecoverySecret: @unchecked Sendable {
  private let bytes: Data

  public init(validated value: String) throws {
    guard let canonicalValue = CredentialSyntax.recoverySecret(value) else {
      throw OnboardingClientError.invalidRequest
    }
    bytes = Data(canonicalValue.utf8)
  }

  func reveal() -> String { String(decoding: bytes, as: UTF8.self) }
}

public struct OneTimeRecoveryMaterial: Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  public let actorId: Int64
  public let recoveryId: String
  public let secret: RecoverySecret

  public init(actorId: Int64, recoveryId: String, secret: RecoverySecret) {
    self.actorId = actorId
    self.recoveryId = recoveryId
    self.secret = secret
  }

  public var description: String {
    "OneTimeRecoveryMaterial(actor: \(actorId), recovery: present, secret: <redacted>)"
  }
  public var debugDescription: String { description }
}

public enum RecoveryWarningCopy {
  public static let unrecoverableEnglish =
    "Loss of the sole installation plus an unsaved recovery secret is unrecoverable."
  public static let unrecoverableRussian =
    "Потеря единственной установки вместе с несохранённым секретом восстановления необратима."
  public static let dismissedUnsavedEnglish =
    "The one-time recovery secret was not saved and is now gone. " + unrecoverableEnglish
  public static let dismissedUnsavedRussian =
    "Одноразовый секрет восстановления не был сохранён и теперь утрачен. " + unrecoverableRussian
  public static let destructiveAbandonEnglish =
    "If the server accepted this token from a prior attempt, deleting it means permanent loss of access."
  public static let destructiveAbandonRussian =
    "Если сервер принял этот токен в предыдущей попытке, его удаление навсегда лишит вас доступа."
}

enum CredentialSyntax {
  static func lowerHexToken(_ value: String) -> Bool {
    value.utf8.count == 64
      && value.utf8.allSatisfy {
        ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
      }
  }

  static func recoveryID(_ value: String) -> Bool {
    guard value.hasPrefix("rec_"), value.utf8.count == 36 else { return false }
    return value.dropFirst(4).utf8.allSatisfy {
      ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
    }
  }

  static func recoverySecret(_ value: String) -> String? {
    guard value.utf8.count <= 40 else { return nil }
    var canonicalBytes: [UInt8] = []
    for byte in value.utf8 {
      switch byte {
      case 0x09...0x0D, 0x20, 0x2D:
        continue
      case 0x61...0x7A:
        canonicalBytes.append(byte - 0x20)
      case 0x41...0x5A, 0x30...0x39:
        canonicalBytes.append(byte)
      default:
        // Normalization is deliberately ASCII-only. Unicode case expansion
        // must never turn an otherwise invalid secret into an accepted one.
        return nil
      }
    }
    let canonical = String(decoding: canonicalBytes, as: UTF8.self)
    let alphabet = Set("ABCDEFGHJKMNPQRSTVWXYZ23456789")
    guard canonical.count == 27, canonical.allSatisfy({ alphabet.contains($0) }) else { return nil }
    return canonical
  }

  static func humanCode(_ value: String) -> String? { recoverySecret(value) }

  static func canonicalHumanCode(_ value: String) -> Bool {
    recoverySecret(value) == value
  }

  static func slot(_ value: String) -> Bool {
    value.utf8.count == 1 && value.utf8.allSatisfy { $0 >= 0x61 && $0 <= 0x7A }
  }

  static func canonicalWebSocketURL(_ value: String, origin: CoordinatorOrigin? = nil) -> Bool {
    guard let url = URL(string: value), url.absoluteString == value,
      url.user == nil, url.password == nil, url.query == nil, url.fragment == nil,
      url.path == "/ws", let scheme = url.scheme?.lowercased(), scheme == "ws" || scheme == "wss"
    else { return false }
    let mapped = (scheme == "wss" ? "https" : "http") + value.dropFirst(scheme.count)
    guard let mappedOrigin = try? CoordinatorOrigin(mapped), mappedOrigin.isSecureForCredentials,
      mappedOrigin.webSocketURL?.absoluteString == value
    else { return false }
    return origin == nil || mappedOrigin == origin
  }
}
