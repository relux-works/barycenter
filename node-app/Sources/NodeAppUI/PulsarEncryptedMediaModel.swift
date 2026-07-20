import Foundation
import Observation

public enum PulsarEncryptedMediaPath: String, CaseIterable, Equatable, Identifiable, Sendable {
  case plaintext
  case protectedClip = "protected_clip"
  case protectedTrack = "protected_track"
  case protectedLive = "protected_live"

  public var id: String { rawValue }

  public var isProtected: Bool { self != .plaintext }
}

public enum PulsarEncryptedMediaVerification: String, CaseIterable, Equatable, Sendable {
  case verified
  case unverified
  case revoked
}

public enum PulsarEncryptedMediaMembership: String, CaseIterable, Equatable, Sendable {
  case current
  case rotationRequired = "rotation_required"
  case removed
  case forked
}

public enum PulsarEncryptedMediaRuntimeOwnership: String, CaseIterable, Equatable, Sendable {
  case unattested
  case singleApplicationInstance = "single_application_instance"
  case crossProcessSerialized = "cross_process_serialized"

  public var isSafe: Bool { self != .unattested }
}

public enum PulsarEncryptedMediaAvailability: String, CaseIterable, Equatable, Sendable {
  case plaintext
  case encrypted
  case blocked
}

public enum PulsarEncryptedMediaGrantMode: String, CaseIterable, Equatable, Sendable {
  case oneTime = "one_time"
  case timeBound = "time_bound"
}

public enum PulsarEncryptedMediaGrantStatus: String, CaseIterable, Equatable, Sendable {
  case active
  case expired
  case revoked
}

public enum PulsarEncryptedMediaRecoveryMode: String, CaseIterable, Equatable, Sendable {
  case deviceTransfer = "device_transfer"
  case userHeldRecovery = "user_held_recovery"
}

public struct PulsarEncryptedMediaComponents: Equatable, Sendable {
  public var keyStateReady: Bool
  public var protectedSendReady: Bool
  public var protectedPlaybackReady: Bool
  public var protectedLiveReady: Bool
  public var sameRepositoryWitness: Bool

  public init(
    keyStateReady: Bool = false,
    protectedSendReady: Bool = false,
    protectedPlaybackReady: Bool = false,
    protectedLiveReady: Bool = false,
    sameRepositoryWitness: Bool = false
  ) {
    self.keyStateReady = keyStateReady
    self.protectedSendReady = protectedSendReady
    self.protectedPlaybackReady = protectedPlaybackReady
    self.protectedLiveReady = protectedLiveReady
    self.sameRepositoryWitness = sameRepositoryWitness
  }
}

public struct PulsarEncryptedMediaDevice: Equatable, Identifiable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let id: String
  public let label: PulsarLocalizedLabel
  public let verification: PulsarEncryptedMediaVerification
  public let currentMember: Bool
  public let isThisDevice: Bool
  public let canRevoke: Bool
  public let revision: UInt64

  public init(
    id: String,
    label: PulsarLocalizedLabel,
    verification: PulsarEncryptedMediaVerification,
    currentMember: Bool,
    isThisDevice: Bool,
    canRevoke: Bool,
    revision: UInt64
  ) {
    self.id = id
    self.label = label
    self.verification = verification
    self.currentMember = currentMember
    self.isThisDevice = isThisDevice
    self.canRevoke = canRevoke
    self.revision = revision
  }

  public var description: String { "PulsarEncryptedMediaDevice(<opaque>)" }
  public var debugDescription: String { description }
}

public struct PulsarEncryptedMediaUnsupportedRecipient: Equatable, Identifiable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let id: String
  public let label: PulsarLocalizedLabel

  public init(id: String, label: PulsarLocalizedLabel) {
    self.id = id
    self.label = label
  }

  public var description: String { "PulsarEncryptedMediaUnsupportedRecipient(<opaque>)" }
  public var debugDescription: String { description }
}

public struct PulsarEncryptedMediaHistoryGrant: Equatable, Identifiable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let id: String
  public let title: String
  public let objectID: String
  public let recipientDeviceID: String
  public let firstEpoch: UInt64
  public let lastEpoch: UInt64
  public let mode: PulsarEncryptedMediaGrantMode
  public let expiresAt: Date
  public let status: PulsarEncryptedMediaGrantStatus

  public init(
    id: String,
    title: String,
    objectID: String,
    recipientDeviceID: String,
    firstEpoch: UInt64,
    lastEpoch: UInt64,
    mode: PulsarEncryptedMediaGrantMode,
    expiresAt: Date,
    status: PulsarEncryptedMediaGrantStatus
  ) {
    self.id = id
    self.title = title
    self.objectID = objectID
    self.recipientDeviceID = recipientDeviceID
    self.firstEpoch = firstEpoch
    self.lastEpoch = lastEpoch
    self.mode = mode
    self.expiresAt = expiresAt
    self.status = status
  }

  public var description: String { "PulsarEncryptedMediaHistoryGrant(<opaque>)" }
  public var debugDescription: String { description }
}

public struct PulsarEncryptedMediaHistoryGrantDraft: Equatable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let objectID: String
  public let title: String
  public let recipientDeviceID: String
  public let firstEpoch: UInt64
  public let lastEpoch: UInt64
  public let mode: PulsarEncryptedMediaGrantMode
  public let expiresAt: Date

  public init(
    objectID: String,
    title: String,
    recipientDeviceID: String,
    firstEpoch: UInt64,
    lastEpoch: UInt64,
    mode: PulsarEncryptedMediaGrantMode,
    expiresAt: Date
  ) {
    self.objectID = objectID
    self.title = title
    self.recipientDeviceID = recipientDeviceID
    self.firstEpoch = firstEpoch
    self.lastEpoch = lastEpoch
    self.mode = mode
    self.expiresAt = expiresAt
  }

  public var description: String { "PulsarEncryptedMediaHistoryGrantDraft(<opaque>)" }
  public var debugDescription: String { description }
}

public struct PulsarEncryptedMediaReportTarget: Equatable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public let objectID: String
  public let title: String
  public let canReportMetadata: Bool
  public let canExportDecryptedEvidence: Bool
  public let decryptedEvidenceReady: Bool
  public let consentVersion: String

  public init(
    objectID: String,
    title: String,
    canReportMetadata: Bool,
    canExportDecryptedEvidence: Bool,
    decryptedEvidenceReady: Bool,
    consentVersion: String
  ) {
    self.objectID = objectID
    self.title = title
    self.canReportMetadata = canReportMetadata
    self.canExportDecryptedEvidence = canExportDecryptedEvidence
    self.decryptedEvidenceReady = decryptedEvidenceReady
    self.consentVersion = consentVersion
  }

  public var description: String { "PulsarEncryptedMediaReportTarget(<opaque>)" }
  public var debugDescription: String { description }
}

public struct PulsarEncryptedMediaSnapshot: Equatable, Sendable,
  CustomStringConvertible, CustomDebugStringConvertible
{
  public var state: PulsarTargetsInboxSurfaceState
  public var selectedPath: PulsarEncryptedMediaPath
  public var capabilityAdvertised: Bool
  public var reviewedSuiteSelected: Bool
  public var runtimeWiringApproved: Bool
  public var ownership: PulsarEncryptedMediaRuntimeOwnership
  public var components: PulsarEncryptedMediaComponents
  public var thisDeviceVerification: PulsarEncryptedMediaVerification
  public var membership: PulsarEncryptedMediaMembership
  public var epoch: UInt64
  public var devices: [PulsarEncryptedMediaDevice]
  public var unsupportedRecipients: [PulsarEncryptedMediaUnsupportedRecipient]
  public var unsupportedExclusionConfirmed: Bool
  public var recoveryModes: [PulsarEncryptedMediaRecoveryMode]
  public var recoveryTargetDeviceID: String?
  public var historyRecoverable: Bool
  public var historyGrants: [PulsarEncryptedMediaHistoryGrant]
  public var historyGrantDraft: PulsarEncryptedMediaHistoryGrantDraft?
  public var reportTarget: PulsarEncryptedMediaReportTarget?
  public var actions: [PulsarActionCapability]
  public var actionOutcome: PulsarLocalizedLabel?
  public var actionFailure: PulsarLocalizedLabel?
  public var commandInFlight: Bool

  public init(
    state: PulsarTargetsInboxSurfaceState = .loading,
    selectedPath: PulsarEncryptedMediaPath = .plaintext,
    capabilityAdvertised: Bool = false,
    reviewedSuiteSelected: Bool = false,
    runtimeWiringApproved: Bool = false,
    ownership: PulsarEncryptedMediaRuntimeOwnership = .unattested,
    components: PulsarEncryptedMediaComponents = .init(),
    thisDeviceVerification: PulsarEncryptedMediaVerification = .unverified,
    membership: PulsarEncryptedMediaMembership = .removed,
    epoch: UInt64 = 0,
    devices: [PulsarEncryptedMediaDevice] = [],
    unsupportedRecipients: [PulsarEncryptedMediaUnsupportedRecipient] = [],
    unsupportedExclusionConfirmed: Bool = false,
    recoveryModes: [PulsarEncryptedMediaRecoveryMode] = [],
    recoveryTargetDeviceID: String? = nil,
    historyRecoverable: Bool = true,
    historyGrants: [PulsarEncryptedMediaHistoryGrant] = [],
    historyGrantDraft: PulsarEncryptedMediaHistoryGrantDraft? = nil,
    reportTarget: PulsarEncryptedMediaReportTarget? = nil,
    actions: [PulsarActionCapability] = [],
    actionOutcome: PulsarLocalizedLabel? = nil,
    actionFailure: PulsarLocalizedLabel? = nil,
    commandInFlight: Bool = false
  ) {
    self.state = state
    self.selectedPath = selectedPath
    self.capabilityAdvertised = capabilityAdvertised
    self.reviewedSuiteSelected = reviewedSuiteSelected
    self.runtimeWiringApproved = runtimeWiringApproved
    self.ownership = ownership
    self.components = components
    self.thisDeviceVerification = thisDeviceVerification
    self.membership = membership
    self.epoch = epoch
    self.devices = devices
    self.unsupportedRecipients = unsupportedRecipients
    self.unsupportedExclusionConfirmed = unsupportedExclusionConfirmed
    self.recoveryModes = recoveryModes
    self.recoveryTargetDeviceID = recoveryTargetDeviceID
    self.historyRecoverable = historyRecoverable
    self.historyGrants = historyGrants
    self.historyGrantDraft = historyGrantDraft
    self.reportTarget = reportTarget
    self.actions = actions
    self.actionOutcome = actionOutcome
    self.actionFailure = actionFailure
    self.commandInFlight = commandInFlight
  }

  public var description: String { "PulsarEncryptedMediaSnapshot(<opaque>)" }
  public var debugDescription: String { description }
}

public enum PulsarEncryptedMediaCommand: Equatable, Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  case refresh
  case selectPath(PulsarEncryptedMediaPath)
  case verifyDevice(String)
  case revokeDevice(String)
  case beginDeviceTransfer(String)
  case beginUserHeldRecovery
  case confirmUnsupportedExclusion([String])
  case createHistoryGrant(
    objectID: String,
    recipientDeviceID: String,
    firstEpoch: UInt64,
    lastEpoch: UInt64,
    mode: PulsarEncryptedMediaGrantMode,
    expiresAt: Date)
  case revokeHistoryGrant(String)
  case reportMetadata(String)
  case exportDecryptedEvidence(objectID: String, consentVersion: String)

  public var description: String {
    switch self {
    case .refresh: "PulsarEncryptedMediaCommand(refresh)"
    case .selectPath(let path): "PulsarEncryptedMediaCommand(select_path,\(path.rawValue))"
    case .verifyDevice: "PulsarEncryptedMediaCommand(verify_device,<opaque>)"
    case .revokeDevice: "PulsarEncryptedMediaCommand(revoke_device,<opaque>)"
    case .beginDeviceTransfer: "PulsarEncryptedMediaCommand(device_transfer,<opaque>)"
    case .beginUserHeldRecovery: "PulsarEncryptedMediaCommand(user_held_recovery)"
    case .confirmUnsupportedExclusion:
      "PulsarEncryptedMediaCommand(confirm_unsupported_exclusion,<opaque>)"
    case .createHistoryGrant:
      "PulsarEncryptedMediaCommand(create_history_grant,<opaque>)"
    case .revokeHistoryGrant:
      "PulsarEncryptedMediaCommand(revoke_history_grant,<opaque>)"
    case .reportMetadata: "PulsarEncryptedMediaCommand(report_metadata,<opaque>)"
    case .exportDecryptedEvidence:
      "PulsarEncryptedMediaCommand(export_decrypted_evidence,<opaque>)"
    }
  }

  public var debugDescription: String { description }
}

@MainActor
public final class PulsarEncryptedMediaActions {
  private let onPerform: (PulsarEncryptedMediaCommand) -> Void

  public init(
    perform: @escaping @MainActor (PulsarEncryptedMediaCommand) -> Void = { _ in }
  ) {
    onPerform = perform
  }

  public func perform(_ command: PulsarEncryptedMediaCommand?) {
    guard let command else { return }
    onPerform(command)
  }
}

@MainActor
@Observable
public final class PulsarEncryptedMediaModel {
  public static let maximumDevices = 64
  public static let maximumHistoryGrants = 100
  public static let maximumHistoryGrantDays = 30.0

  public private(set) var snapshot: PulsarEncryptedMediaSnapshot

  public init(snapshot: PulsarEncryptedMediaSnapshot = .init()) {
    self.snapshot = .init()
    replace(snapshot)
  }

  public func replace(_ replacement: PulsarEncryptedMediaSnapshot, now: Date = .now) {
    var normalized = replacement
    normalized.devices = Self.uniqueValidDevices(replacement.devices)
    normalized.unsupportedRecipients = Self.uniqueValidRecipients(
      replacement.unsupportedRecipients)
    normalized.recoveryModes = Array(Set(replacement.recoveryModes)).sorted {
      $0.rawValue < $1.rawValue
    }
    normalized.historyGrants = Self.validGrants(replacement.historyGrants, now: now)
    normalized.actions = Self.uniqueActions(replacement.actions)

    if normalized.epoch == 0 || normalized.thisDeviceVerification != .verified
      || normalized.membership != .current
    {
      normalized.unsupportedExclusionConfirmed = false
    }
    if !normalized.unsupportedRecipients.isEmpty && !replacement.unsupportedExclusionConfirmed {
      normalized.unsupportedExclusionConfirmed = false
    }
    if !normalized.ownership.isSafe || !normalized.components.sameRepositoryWitness {
      normalized.runtimeWiringApproved = false
      normalized.capabilityAdvertised = false
    }
    if !normalized.runtimeWiringApproved || !normalized.reviewedSuiteSelected {
      normalized.capabilityAdvertised = false
    }
    if normalized.state != .ready {
      normalized.commandInFlight = false
    }
    if let target = normalized.recoveryTargetDeviceID,
      !normalized.devices.contains(where: { $0.id == target })
    {
      normalized.recoveryTargetDeviceID = nil
    }
    if let draft = normalized.historyGrantDraft,
      !Self.validHistoryDraft(draft, devices: normalized.devices, now: now)
    {
      normalized.historyGrantDraft = nil
    }
    if let report = normalized.reportTarget,
      !Self.validIdentifier(report.objectID) || !Self.validConsentVersion(report.consentVersion)
    {
      normalized.reportTarget = nil
    }
    snapshot = normalized
  }

  public func availability(for path: PulsarEncryptedMediaPath) -> PulsarEncryptedMediaAvailability {
    guard path.isProtected else { return .plaintext }
    guard protectedFoundationReady else { return .blocked }
    switch path {
    case .plaintext:
      return .plaintext
    case .protectedClip, .protectedTrack:
      return snapshot.components.protectedSendReady
        && snapshot.components.protectedPlaybackReady ? .encrypted : .blocked
    case .protectedLive:
      return snapshot.components.protectedLiveReady ? .encrypted : .blocked
    }
  }

  public func pathFailure(_ path: PulsarEncryptedMediaPath) -> String? {
    guard path.isProtected else { return nil }
    if snapshot.state != .ready { return "surface_not_ready" }
    if !snapshot.runtimeWiringApproved { return "runtime_disabled" }
    if !snapshot.ownership.isSafe || !snapshot.components.sameRepositoryWitness {
      return "ownership_unattested"
    }
    if !snapshot.reviewedSuiteSelected || !snapshot.capabilityAdvertised {
      return "capability_unavailable"
    }
    if !snapshot.components.keyStateReady { return "secure_key_state_unavailable" }
    if snapshot.thisDeviceVerification != .verified { return "device_unverified" }
    if snapshot.membership != .current || snapshot.epoch == 0 { return "membership_stale" }
    if !snapshot.unsupportedRecipients.isEmpty && !snapshot.unsupportedExclusionConfirmed {
      return "unsupported_recipients_require_choice"
    }
    switch path {
    case .protectedClip, .protectedTrack:
      if !snapshot.components.protectedSendReady { return "protected_send_unavailable" }
      if !snapshot.components.protectedPlaybackReady { return "protected_playback_unavailable" }
    case .protectedLive:
      if !snapshot.components.protectedLiveReady { return "protected_live_unavailable" }
    case .plaintext:
      return nil
    }
    return nil
  }

  public func refreshCommand() -> PulsarEncryptedMediaCommand? {
    action("refresh") && !snapshot.commandInFlight ? .refresh : nil
  }

  public func selectPathCommand(_ path: PulsarEncryptedMediaPath) -> PulsarEncryptedMediaCommand? {
    guard readyForCommand, action("select_path") else { return nil }
    if path.isProtected && availability(for: path) != .encrypted { return nil }
    return .selectPath(path)
  }

  public func verifyDeviceCommand(_ id: String) -> PulsarEncryptedMediaCommand? {
    guard readyForCommand, action("verify_device"),
      let device = snapshot.devices.first(where: { $0.id == id }),
      device.verification == .unverified, device.currentMember
    else { return nil }
    return .verifyDevice(id)
  }

  public func revokeDeviceCommand(_ id: String, confirmed: Bool) -> PulsarEncryptedMediaCommand? {
    guard confirmed, readyForCommand, action("revoke_device"),
      let device = snapshot.devices.first(where: { $0.id == id }),
      device.verification == .verified, device.canRevoke
    else { return nil }
    return .revokeDevice(id)
  }

  public func deviceTransferCommand() -> PulsarEncryptedMediaCommand? {
    guard protectedCryptographyReady, action("device_transfer"),
      snapshot.recoveryModes.contains(.deviceTransfer),
      let id = snapshot.recoveryTargetDeviceID,
      let target = snapshot.devices.first(where: { $0.id == id }),
      target.verification == .verified, target.currentMember, !target.isThisDevice
    else { return nil }
    return .beginDeviceTransfer(id)
  }

  public func userHeldRecoveryCommand(confirmed: Bool) -> PulsarEncryptedMediaCommand? {
    guard confirmed, recoveryFoundationReady, action("user_held_recovery"),
      snapshot.recoveryModes.contains(.userHeldRecovery)
    else { return nil }
    return .beginUserHeldRecovery
  }

  public func unsupportedExclusionCommand(confirmed: Bool) -> PulsarEncryptedMediaCommand? {
    guard confirmed, readyForCommand, action("confirm_unsupported_exclusion"),
      !snapshot.unsupportedRecipients.isEmpty
    else { return nil }
    return .confirmUnsupportedExclusion(snapshot.unsupportedRecipients.map(\.id))
  }

  public func createHistoryGrantCommand(
    confirmed: Bool, now: Date = .now
  ) -> PulsarEncryptedMediaCommand? {
    guard confirmed, protectedCryptographyReady, action("create_history_grant"),
      snapshot.historyRecoverable, let draft = snapshot.historyGrantDraft,
      Self.validHistoryDraft(draft, devices: snapshot.devices, now: now)
    else { return nil }
    return .createHistoryGrant(
      objectID: draft.objectID,
      recipientDeviceID: draft.recipientDeviceID,
      firstEpoch: draft.firstEpoch,
      lastEpoch: draft.lastEpoch,
      mode: draft.mode,
      expiresAt: draft.expiresAt)
  }

  public func revokeHistoryGrantCommand(
    _ id: String, confirmed: Bool
  ) -> PulsarEncryptedMediaCommand? {
    guard confirmed, readyForCommand, action("revoke_history_grant"),
      snapshot.historyGrants.contains(where: { $0.id == id && $0.status == .active })
    else { return nil }
    return .revokeHistoryGrant(id)
  }

  public func metadataReportCommand() -> PulsarEncryptedMediaCommand? {
    guard readyForCommand, action("report_metadata"),
      let target = snapshot.reportTarget, target.canReportMetadata
    else { return nil }
    return .reportMetadata(target.objectID)
  }

  public func decryptedEvidenceExportCommand(
    confirmed: Bool
  ) -> PulsarEncryptedMediaCommand? {
    guard confirmed, protectedCryptographyReady, action("export_decrypted_evidence"),
      let target = snapshot.reportTarget,
      target.canExportDecryptedEvidence, target.decryptedEvidenceReady
    else { return nil }
    return .exportDecryptedEvidence(
      objectID: target.objectID, consentVersion: target.consentVersion)
  }

  private var readyForCommand: Bool {
    snapshot.state == .ready && !snapshot.commandInFlight
  }

  private var readyForProtectedCommand: Bool {
    readyForCommand && snapshot.components.keyStateReady
      && snapshot.components.sameRepositoryWitness && snapshot.ownership.isSafe
      && snapshot.thisDeviceVerification == .verified
      && snapshot.membership == .current && snapshot.epoch > 0
  }

  private var protectedFoundationReady: Bool {
    protectedCryptographyReady
      && (snapshot.unsupportedRecipients.isEmpty || snapshot.unsupportedExclusionConfirmed)
  }

  private var protectedCryptographyReady: Bool {
    readyForProtectedCommand && recoveryFoundationReady
  }

  private var recoveryFoundationReady: Bool {
    readyForCommand && snapshot.runtimeWiringApproved
      && snapshot.reviewedSuiteSelected && snapshot.capabilityAdvertised
      && snapshot.components.keyStateReady && snapshot.components.sameRepositoryWitness
      && snapshot.ownership.isSafe
  }

  private func action(_ name: String) -> Bool {
    snapshot.actions.contains { $0.action == name }
  }

  private static func uniqueValidDevices(
    _ devices: [PulsarEncryptedMediaDevice]
  ) -> [PulsarEncryptedMediaDevice] {
    var seen = Set<String>()
    return devices.filter {
      validIdentifier($0.id) && $0.revision > 0 && seen.insert($0.id).inserted
    }.prefix(maximumDevices).map { $0 }
  }

  private static func uniqueValidRecipients(
    _ recipients: [PulsarEncryptedMediaUnsupportedRecipient]
  ) -> [PulsarEncryptedMediaUnsupportedRecipient] {
    var seen = Set<String>()
    return recipients.filter {
      validIdentifier($0.id) && seen.insert($0.id).inserted
    }.prefix(maximumDevices).map { $0 }
  }

  private static func validGrants(
    _ grants: [PulsarEncryptedMediaHistoryGrant], now: Date
  ) -> [PulsarEncryptedMediaHistoryGrant] {
    var seen = Set<String>()
    return grants.filter {
      validIdentifier($0.id) && validIdentifier($0.objectID)
        && validIdentifier($0.recipientDeviceID) && $0.firstEpoch > 0
        && $0.lastEpoch >= $0.firstEpoch && seen.insert($0.id).inserted
        && ($0.status != .active || $0.expiresAt > now)
    }.prefix(maximumHistoryGrants).map { $0 }
  }

  private static func uniqueActions(
    _ actions: [PulsarActionCapability]
  ) -> [PulsarActionCapability] {
    var seen = Set<String>()
    return actions.filter {
      validAction($0.action) && seen.insert($0.action).inserted
    }
  }

  private static func validHistoryDraft(
    _ draft: PulsarEncryptedMediaHistoryGrantDraft,
    devices: [PulsarEncryptedMediaDevice],
    now: Date
  ) -> Bool {
    guard validIdentifier(draft.objectID), validIdentifier(draft.recipientDeviceID),
      draft.firstEpoch > 0, draft.lastEpoch >= draft.firstEpoch,
      draft.expiresAt > now,
      draft.expiresAt.timeIntervalSince(now) <= maximumHistoryGrantDays * 86_400,
      let device = devices.first(where: { $0.id == draft.recipientDeviceID }),
      device.verification == .verified, device.currentMember
    else { return false }
    return true
  }

  private static func validIdentifier(_ value: String) -> Bool {
    let bytes = value.utf8
    guard (8...128).contains(bytes.count) else { return false }
    return bytes.allSatisfy {
      ($0 >= 48 && $0 <= 57) || ($0 >= 65 && $0 <= 90)
        || ($0 >= 97 && $0 <= 122) || $0 == 45 || $0 == 95
    }
  }

  private static func validAction(_ value: String) -> Bool {
    let bytes = value.utf8
    return (1...64).contains(bytes.count)
      && bytes.allSatisfy {
        ($0 >= 97 && $0 <= 122) || $0 == 95
      }
  }

  private static func validConsentVersion(_ value: String) -> Bool {
    let bytes = value.utf8
    return (1...64).contains(bytes.count)
      && bytes.allSatisfy {
        ($0 >= 48 && $0 <= 57) || ($0 >= 65 && $0 <= 90)
          || ($0 >= 97 && $0 <= 122) || $0 == 45 || $0 == 46 || $0 == 95
      }
  }
}
