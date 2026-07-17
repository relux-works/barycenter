import Foundation

public struct SoundboardCue: Equatable, Identifiable, Sendable {
  public let id: String
  public let title: String
  public let sourceKind: String
  public let mediaID: String?
  public let builtinAssetID: String?
  public let sourceSHA256: String
  public let sourceBytes: Int64
  public let sourceDurationMS: Int64
  public let revision: Int64
  public let sourceGeneration: Int64
  public let position: Int
}

public struct SoundboardCueList: Equatable, Sendable {
  public let orderRevision: Int64
  public let cues: [SoundboardCue]
}

public struct SoundboardFallbackConfirmation: Equatable, Sendable {
  public let token: String
  public let delivery: PhaseOneDelivery

  public init(token: String, delivery: PhaseOneDelivery) {
    self.token = token
    self.delivery = delivery
  }
}

public struct SoundboardTriggerIntent: Equatable, Sendable {
  public let route: PhaseOneRoute
  public let delivery: PhaseOneDelivery
  public let includeOrigin: Bool
  public let fallback: SoundboardFallbackConfirmation?

  public init(
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    includeOrigin: Bool,
    fallback: SoundboardFallbackConfirmation? = nil
  ) {
    self.route = route
    self.delivery = delivery
    self.includeOrigin = includeOrigin
    self.fallback = fallback
  }
}

public struct SoundboardTriggerReceipt: Equatable, Sendable {
  public let executionID: String
  public let transmission: PhaseOneTransmissionReceipt
}

public struct PhaseOneConfirmationChallenge: Error, Equatable, Sendable {
  public let token: String
  public let alternatives: [PhaseOneDelivery]
}

public protocol SoundboardAppServicing: Sendable {
  func soundboardCues() async throws -> SoundboardCueList
  func createSoundboardMediaCue(
    title: String, mediaID: String, idempotencyKey: String
  ) async throws -> SoundboardCueList
  func renameSoundboardCue(
    _ cueID: String, title: String, revision: Int64, idempotencyKey: String
  ) async throws -> SoundboardCueList
  func deleteSoundboardCue(
    _ cueID: String, revision: Int64, idempotencyKey: String
  ) async throws -> SoundboardCueList
  func reorderSoundboardCues(
    _ cueIDs: [String], revision: Int64, idempotencyKey: String
  ) async throws -> SoundboardCueList
  func triggerSoundboardCue(
    _ cueID: String, intent: SoundboardTriggerIntent, idempotencyKey: String
  ) async throws -> SoundboardTriggerReceipt
}

public enum MacSoundboardShortcutStatus: String, Codable, Equatable, Sendable {
  case inactive, registered, conflict, unavailable, suspended
}

public struct MacSoundboardShortcutBinding: Codable, Equatable, Sendable {
  public let cueID: String
  public let shortcut: MacRecordingShortcut

  public init(cueID: String, shortcut: MacRecordingShortcut) {
    self.cueID = cueID
    self.shortcut = shortcut
  }
}

public struct MacSoundboardShortcutState: Equatable, Sendable {
  public let cueID: String
  public let shortcut: MacRecordingShortcut
  public let status: MacSoundboardShortcutStatus
}

public struct MacSoundboardPreferences: Codable, Equatable, Sendable {
  public static let maximumBindings = 16
  public var version = 1
  public var selectedCueID: String?
  public var route: PhaseOneRoute = .ownBarycenter
  public var delivery: PhaseOneDelivery = .overlay
  public var includeOrigin = true
  public var bindings: [MacSoundboardShortcutBinding] = []

  public init() {}

  public func validated() -> MacSoundboardPreferences? {
    guard version == 1, bindings.count <= Self.maximumBindings,
      selectedCueID.map(Self.validCueID) != false
    else { return nil }
    var cueIDs = Set<String>()
    var shortcuts = Set<String>()
    for binding in bindings {
      let key = "\(binding.shortcut.key.rawValue):\(binding.shortcut.modifiers.rawValue)"
      guard Self.validCueID(binding.cueID), cueIDs.insert(binding.cueID).inserted,
        shortcuts.insert(key).inserted
      else { return nil }
    }
    return self
  }

  private static func validCueID(_ value: String) -> Bool {
    value.hasPrefix("cq_") && value.count == 29
  }
}

public final class MacSoundboardPreferenceStore: @unchecked Sendable {
  private let defaults: UserDefaults
  private let key: String

  public init(defaults: UserDefaults = .standard, key: String = "soundboardPreferences.v1") {
    self.defaults = defaults
    self.key = key
  }

  public func load() -> MacSoundboardPreferences {
    guard let data = defaults.data(forKey: key), data.count <= 16_384,
      let decoded = try? JSONDecoder().decode(MacSoundboardPreferences.self, from: data),
      let valid = decoded.validated()
    else { return MacSoundboardPreferences() }
    return valid
  }

  public func save(_ value: MacSoundboardPreferences) throws {
    guard let valid = value.validated() else { throw PhaseOneClientError.invalidRequest }
    let data = try JSONEncoder().encode(valid)
    guard data.count <= 16_384 else { throw PhaseOneClientError.invalidRequest }
    defaults.set(data, forKey: key)
  }
}

@MainActor
public final class MacSoundboardShortcutController {
  private let registrar: MacGlobalShortcutRegistering
  private let recordingShortcut: () -> MacRecordingShortcut
  private var trigger: (String) -> Void
  private var registrations: [String: MacGlobalShortcutRegistration] = [:]
  private var bindings: [MacSoundboardShortcutBinding] = []
  private var suspended = false
  public private(set) var states: [MacSoundboardShortcutState] = []
  public var onChange: (([MacSoundboardShortcutState]) -> Void)?

  public init(
    registrar: MacGlobalShortcutRegistering,
    recordingShortcut: @escaping () -> MacRecordingShortcut,
    trigger: @escaping (String) -> Void
  ) {
    self.registrar = registrar
    self.recordingShortcut = recordingShortcut
    self.trigger = trigger
  }

  public func configure(_ bindings: [MacSoundboardShortcutBinding]) {
    unregisterAll()
    self.bindings = Array(bindings.prefix(MacSoundboardPreferences.maximumBindings))
    registerAll()
  }

  public func setTrigger(_ trigger: @escaping (String) -> Void) { self.trigger = trigger }

  public func suspend() {
    guard !suspended else { return }
    suspended = true
    unregisterAll()
    publish(bindings.map { .init(cueID: $0.cueID, shortcut: $0.shortcut, status: .suspended) })
  }

  public func resume() {
    guard suspended else { return }
    suspended = false
    registerAll()
  }

  public func stop() {
    suspended = false
    unregisterAll()
    publish(bindings.map { .init(cueID: $0.cueID, shortcut: $0.shortcut, status: .inactive) })
  }

  private func registerAll() {
    guard !suspended else { return }
    var result: [MacSoundboardShortcutState] = []
    for binding in bindings {
      guard binding.shortcut != recordingShortcut() else {
        result.append(.init(cueID: binding.cueID, shortcut: binding.shortcut, status: .conflict))
        continue
      }
      switch registrar.register(binding.shortcut, handler: { [weak self] in
        guard self?.registrations[binding.cueID] != nil else { return }
        self?.trigger(binding.cueID)
      }) {
      case .success(let registration):
        registrations[binding.cueID] = registration
        result.append(.init(cueID: binding.cueID, shortcut: binding.shortcut, status: .registered))
      case .failure(.conflict):
        result.append(.init(cueID: binding.cueID, shortcut: binding.shortcut, status: .conflict))
      case .failure(.unavailable):
        result.append(.init(cueID: binding.cueID, shortcut: binding.shortcut, status: .unavailable))
      }
    }
    publish(result)
  }

  private func unregisterAll() {
    registrations.values.forEach(registrar.unregister)
    registrations.removeAll()
  }

  private func publish(_ value: [MacSoundboardShortcutState]) {
    states = value
    onChange?(value)
  }
}
