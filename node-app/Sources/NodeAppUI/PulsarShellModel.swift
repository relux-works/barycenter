import Foundation
import Observation

public enum PulsarShellLocale: String, CaseIterable, Identifiable, Sendable {
    case en
    case ru

    public var id: Self { self }

    public static func preferred(from locale: Locale = .current) -> Self {
        locale.language.languageCode == .russian ? .ru : .en
    }
}

public enum PulsarShellSection: String, CaseIterable, Identifiable, Sendable {
    case home
    case airs
    case inbox
    case create
    case join
    case tryLocally
    case history
    case settings

    public var id: Self { self }
}

public enum PulsarConnectionState: Equatable, Sendable {
    case unpaired
    case reconnecting
    case online
    case degraded(String)

    public var isPaired: Bool {
        if case .unpaired = self { return false }
        return true
    }
}

public enum PulsarRecordingState: Equatable, Sendable {
    case unavailable
    case idle
    case recording
    case processing
    case failed(String)
}

public enum PulsarRecordingShortcutChoice: String, CaseIterable, Identifiable, Sendable {
    case controlShiftSpace = "control_shift_space"
    case commandShiftSpace = "command_shift_space"
    case controlOptionSpace = "control_option_space"
    case controlShiftR = "control_shift_r"

    public var id: Self { self }

    public var displayValue: String {
        switch self {
        case .controlShiftSpace: "⌃⇧Space"
        case .commandShiftSpace: "⌘⇧Space"
        case .controlOptionSpace: "⌃⌥Space"
        case .controlShiftR: "⌃⇧R"
        }
    }
}

public enum PulsarRecordingShortcutState: Equatable, Sendable {
    case inactive
    case registered
    case conflict
    case unavailable
    case suspended
}

public struct PulsarCaptureDevice: Equatable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let isDefault: Bool

    public init(id: String, name: String, isDefault: Bool) {
        self.id = id
        self.name = name
        self.isDefault = isDefault
    }
}

public enum PulsarSelfTestState: String, Equatable, Sendable {
    case idle
    case playingBuiltinCue
    case requestingPermission
    case recording
    case playingStopCue
    case playingRecording
    case reviewingDraft
    case failed
}

public struct PulsarLocalFileReview: Equatable, Sendable {
    public let filename: String
    public let format: String?
    public let durationMs: Int64?
    public let sizeBytes: Int64
    public let audience: [String]
    public let deliveryModes: [String]
    public let rightsReminder: String
    public let serverValidationRequired: Bool
    public let rejection: String?

    public init(
        filename: String,
        format: String?,
        durationMs: Int64?,
        sizeBytes: Int64,
        audience: [String],
        deliveryModes: [String],
        rightsReminder: String,
        serverValidationRequired: Bool,
        rejection: String?
    ) {
        self.filename = filename
        self.format = format
        self.durationMs = durationMs
        self.sizeBytes = sizeBytes
        self.audience = audience
        self.deliveryModes = deliveryModes
        self.rightsReminder = rightsReminder
        self.serverValidationRequired = serverValidationRequired
        self.rejection = rejection
    }

    public var isEligible: Bool { rejection == nil }
}

public enum PulsarDNDMode: String, CaseIterable, Identifiable, Sendable {
    case allowAll = "allow_all"
    case messagesOnly = "messages_only"
    case mutedUntil = "muted_until"

    public var id: Self { self }
}

public enum PulsarRouteTarget: String, CaseIterable, Identifiable, Sendable {
    case thisPulsar = "this_pulsar"
    case ownBarycenter = "own_barycenter"
    case currentAir = "current_air"

    public var id: Self { self }
}

public enum PulsarDeliveryMode: String, CaseIterable, Identifiable, Sendable {
    case overlay
    case interrupt
    case afterCurrent = "after_current"

    public var id: Self { self }
}

public enum PulsarHistoryAction: String, Equatable, Sendable {
    case delete
    case replay
    case report
    case blockActor = "block_actor"
}

public enum PulsarModerationReason: String, CaseIterable, Identifiable, Sendable {
    case spam
    case harassment
    case illegal
    case sexualContent = "sexual_content"
    case violence
    case other

    public var id: Self { self }
}

public struct PulsarHistoryActionRequest: Equatable, Sendable {
    public let action: PulsarHistoryAction
    public let reason: PulsarModerationReason?
    public let details: String

    public init(
        action: PulsarHistoryAction,
        reason: PulsarModerationReason? = nil,
        details: String = ""
    ) {
        self.action = action
        self.reason = reason
        self.details = details
    }
}

public enum PulsarOutgoingDraftState: String, Equatable, Sendable {
    case retained, uploading, uploaded, transmitting, accepted
    case retryableFailure = "retryable_failure"
}

public enum PulsarIdentityOperationState: Equatable, Sendable {
    case idle
    case busy
    case succeeded(String)
    case recoveryExportRequired(String)
    case failed(String)
}

public struct PulsarOutgoingDraft: Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let state: PulsarOutgoingDraftState
    public let route: PulsarRouteTarget?
    public let requestedDelivery: PulsarDeliveryMode?
    public let effectiveDelivery: PulsarDeliveryMode?
    public let downgradeReason: String?
    public let status: String?
    public let failureCode: String?
    public let localBytesRetained: Bool
    public let explicitTargetCount: Int?

    public init(
        id: String,
        title: String,
        state: PulsarOutgoingDraftState,
        route: PulsarRouteTarget? = nil,
        requestedDelivery: PulsarDeliveryMode? = nil,
        effectiveDelivery: PulsarDeliveryMode? = nil,
        downgradeReason: String? = nil,
        status: String? = nil,
        failureCode: String? = nil,
        localBytesRetained: Bool = true,
        explicitTargetCount: Int? = nil
    ) {
        self.id = id
        self.title = title
        self.state = state
        self.route = route
        self.requestedDelivery = requestedDelivery
        self.effectiveDelivery = effectiveDelivery
        self.downgradeReason = downgradeReason
        self.status = status
        self.failureCode = failureCode
        self.localBytesRetained = localBytesRetained
        self.explicitTargetCount = explicitTargetCount
    }
}

public struct PulsarHistoryItem: Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let detail: String
    public let occurredAt: Date
    public let direction: String
    public let senderName: String?
    public let status: String
    public let requestedDelivery: String?
    public let effectiveDelivery: String?
    public let downgradeReason: String?
    public let allowedActions: [PulsarHistoryAction]

    public init(
        id: String,
        title: String,
        detail: String,
        occurredAt: Date,
        direction: String = "",
        senderName: String? = nil,
        status: String = "",
        requestedDelivery: String? = nil,
        effectiveDelivery: String? = nil,
        downgradeReason: String? = nil,
        allowedActions: [PulsarHistoryAction] = []
    ) {
        self.id = id
        self.title = title
        self.detail = detail
        self.occurredAt = occurredAt
        self.direction = direction
        self.senderName = senderName
        self.status = status
        self.requestedDelivery = requestedDelivery
        self.effectiveDelivery = effectiveDelivery
        self.downgradeReason = downgradeReason
        self.allowedActions = allowedActions
    }
}

public struct PulsarShellSnapshot: Equatable, Sendable {
    public var connection: PulsarConnectionState
    public var connectionIdentity: String?
    public var presenceSummary: String?
    public var routeName: String?
    public var nowPlaying: String?
    public var playbackState: String
    public var history: [PulsarHistoryItem]
    public var outgoingDrafts: [PulsarOutgoingDraft]
    public var phaseOneActionOutcome: String?
    public var phaseOneFailure: String?
    public var identityOperation: PulsarIdentityOperationState
    public var airs: PulsarAirState
    public var dndMode: PulsarDNDMode
    public var recording: PulsarRecordingState
    public var recordingAvailable: Bool
    public var recordingMeter: Float
    public var captureDevices: [PulsarCaptureDevice]
    public var selectedCaptureDeviceID: String?
    public var recordingShortcut: PulsarRecordingShortcutChoice
    public var recordingShortcutState: PulsarRecordingShortcutState
    public var selfTestAvailable: Bool
    public var selfTestState: PulsarSelfTestState
    public var selfTestMeter: Float
    public var localFileReview: PulsarLocalFileReview?
    public var localDraftAvailable: Bool
    public var volume: Int

    public init(
        connection: PulsarConnectionState = .unpaired,
        connectionIdentity: String? = nil,
        presenceSummary: String? = nil,
        routeName: String? = nil,
        nowPlaying: String? = nil,
        playbackState: String = "stopped",
        history: [PulsarHistoryItem] = [],
        outgoingDrafts: [PulsarOutgoingDraft] = [],
        phaseOneActionOutcome: String? = nil,
        phaseOneFailure: String? = nil,
        identityOperation: PulsarIdentityOperationState = .idle,
        airs: PulsarAirState = .init(),
        dndMode: PulsarDNDMode = .allowAll,
        recording: PulsarRecordingState = .unavailable,
        recordingAvailable: Bool = false,
        recordingMeter: Float = 0,
        captureDevices: [PulsarCaptureDevice] = [],
        selectedCaptureDeviceID: String? = nil,
        recordingShortcut: PulsarRecordingShortcutChoice = .controlShiftSpace,
        recordingShortcutState: PulsarRecordingShortcutState = .inactive,
        selfTestAvailable: Bool = false,
        selfTestState: PulsarSelfTestState = .idle,
        selfTestMeter: Float = 0,
        localFileReview: PulsarLocalFileReview? = nil,
        localDraftAvailable: Bool = false,
        volume: Int = 80
    ) {
        self.connection = connection
        self.connectionIdentity = connectionIdentity
        self.presenceSummary = presenceSummary
        self.routeName = routeName
        self.nowPlaying = nowPlaying
        self.playbackState = playbackState
        self.history = history
        self.outgoingDrafts = outgoingDrafts
        self.phaseOneActionOutcome = phaseOneActionOutcome
        self.phaseOneFailure = phaseOneFailure
        self.identityOperation = identityOperation
        self.airs = airs
        self.dndMode = dndMode
        self.recording = recording
        self.recordingAvailable = recordingAvailable
        self.recordingMeter = min(max(recordingMeter, 0), 1)
        self.captureDevices = captureDevices
        self.selectedCaptureDeviceID = selectedCaptureDeviceID
        self.recordingShortcut = recordingShortcut
        self.recordingShortcutState = recordingShortcutState
        self.selfTestAvailable = selfTestAvailable
        self.selfTestState = selfTestState
        self.selfTestMeter = min(max(selfTestMeter, 0), 1)
        self.localFileReview = localFileReview
        self.localDraftAvailable = localDraftAvailable
        self.volume = min(max(volume, 0), 100)
    }
}

@MainActor
@Observable
public final class PulsarShellModel {
    public var locale: PulsarShellLocale
    public var selectedSection: PulsarShellSection
    public private(set) var snapshot: PulsarShellSnapshot

    public init(
        locale: PulsarShellLocale = .preferred(),
        selectedSection: PulsarShellSection = .home,
        snapshot: PulsarShellSnapshot = .init()
    ) {
        self.locale = locale
        self.selectedSection = selectedSection
        self.snapshot = snapshot
    }

    public func replaceSnapshot(_ snapshot: PulsarShellSnapshot) {
        self.snapshot = snapshot
    }

    public func updateConnection(_ connection: PulsarConnectionState, identity: String? = nil) {
        snapshot.connection = connection
        snapshot.connectionIdentity = identity
    }

    public func updateRuntime(
        routeName: String?,
        nowPlaying: String?,
        playbackState: String,
        dndMode: PulsarDNDMode,
        volume: Int
    ) {
        snapshot.routeName = routeName
        snapshot.nowPlaying = nowPlaying
        snapshot.playbackState = playbackState
        snapshot.dndMode = dndMode
        snapshot.volume = min(max(volume, 0), 100)
    }

    public func setHistory(_ items: [PulsarHistoryItem]) {
        snapshot.history = items
    }

    public func setPhaseOneData(
        presenceSummary: String?,
        history: [PulsarHistoryItem]? = nil,
        outgoingDrafts: [PulsarOutgoingDraft]? = nil,
        failure: String?
    ) {
        snapshot.presenceSummary = presenceSummary
        if let history { snapshot.history = history }
        if let outgoingDrafts { snapshot.outgoingDrafts = outgoingDrafts }
        snapshot.phaseOneFailure = failure
    }

    public func setPhaseOneActionState(outcome: String?, failure: String?) {
        snapshot.phaseOneActionOutcome = outcome
        snapshot.phaseOneFailure = failure
    }

    public func setIdentityOperation(_ state: PulsarIdentityOperationState) {
        snapshot.identityOperation = state
    }

    public func setAirState(_ state: PulsarAirState) {
        snapshot.airs = state
    }

    public func updateAirState(
        saved: [PulsarAirItem]? = nil,
        pendingJoin: PulsarPendingAirJoin?? = nil,
        inviteSecret: PulsarAirInviteSecret?? = nil,
        busy: Bool? = nil,
        outcome: String?? = nil,
        failure: String?? = nil
    ) {
        if let saved { snapshot.airs.saved = saved }
        if let pendingJoin { snapshot.airs.pendingJoin = pendingJoin }
        if let inviteSecret { snapshot.airs.inviteSecret = inviteSecret }
        if let busy { snapshot.airs.busy = busy }
        if let outcome { snapshot.airs.outcome = outcome }
        if let failure { snapshot.airs.failure = failure }
    }

    public func setRecording(_ state: PulsarRecordingState, available: Bool) {
        snapshot.recording = state
        snapshot.recordingAvailable = available
    }

    public func setRecordingMeter(_ value: Float) {
        snapshot.recordingMeter = min(max(value, 0), 1)
    }

    public func setCaptureDevices(
        _ devices: [PulsarCaptureDevice],
        selectedDeviceID: String?
    ) {
        snapshot.captureDevices = devices
        snapshot.selectedCaptureDeviceID = selectedDeviceID
    }

    public func setRecordingShortcut(
        _ shortcut: PulsarRecordingShortcutChoice,
        state: PulsarRecordingShortcutState
    ) {
        snapshot.recordingShortcut = shortcut
        snapshot.recordingShortcutState = state
    }

    public func setSelfTestAvailable(_ available: Bool) {
        snapshot.selfTestAvailable = available
    }

    public func updateSelfTest(
        state: PulsarSelfTestState,
        meter: Float? = nil,
        draftAvailable: Bool? = nil
    ) {
        snapshot.selfTestState = state
        if let meter { snapshot.selfTestMeter = min(max(meter, 0), 1) }
        if let draftAvailable { snapshot.localDraftAvailable = draftAvailable }
    }

    public func setLocalFileReview(_ review: PulsarLocalFileReview?) {
        snapshot.localFileReview = review
    }

    public func setDNDMode(_ mode: PulsarDNDMode) {
        snapshot.dndMode = mode
    }

    public func setVolume(_ volume: Int) {
        snapshot.volume = min(max(volume, 0), 100)
    }
}

@MainActor
public final class PulsarShellActions {
    private let onCreateOrbit: () -> Void
    private let onJoinOrbit: () -> Void
    private let onTryLocally: () -> Void
    private let onSetDND: (PulsarDNDMode) -> Void
    private let onSetVolume: (Int) -> Void
    private let onToggleRecording: () -> Void
    private let onCancelRecording: () -> Void
    private let onSetCaptureDevice: (String?) -> Void
    private let onSetRecordingShortcut: (PulsarRecordingShortcutChoice) -> Void
    private let onPlayBuiltinCue: () -> Void
    private let onRecordFiveSeconds: () -> Void
    private let onReviewLocalFile: (URL) -> Void
    private let onAcceptLocalFile: (URL) -> Void
    private let onDeleteLocalDraft: () -> Void
    private let onCloseSelfTest: () -> Void
    private let onSendDraft: (String, PulsarRouteTarget, PulsarDeliveryMode, Bool) -> Void
    private let onSendTargetedDraft: (String, PulsarDeliveryMode, Bool) -> Void
    private let onDeleteOutgoingDraft: (String) -> Void
    private let onRefreshPhaseOneData: () -> Void
    private let onHistoryAction: (String, PulsarHistoryActionRequest) -> Void
    private let onSubmitCreateOrbit: (String) -> Void
    private let onSubmitJoinOrbit: (String) -> Void
    private let onExportRecovery: () -> Void
    private let onRefreshAirs: () -> Void
    private let onCreateAir: (String) -> Void
    private let onConsumeAirInvite: (String) -> Void
    private let onConfirmAirJoin: (String, Bool) -> Void
    private let onDeclineAirJoin: (String) -> Void
    private let onIssueAirInvite: (String, PulsarAirRole) -> Void
    private let onWithdrawAirInvite: () -> Void
    private let onHideAirInvite: () -> Void
    private let onActivateAir: (String) -> Void
    private let onDeactivateAir: (String) -> Void
    private let onLeaveAir: (String) -> Void
    private let onDissolveAir: (String) -> Void
    private let onReplaceAirPolicy: (String, PulsarAirPolicy) -> Void

    public init(
        createOrbit: @escaping @MainActor () -> Void = {},
        joinOrbit: @escaping @MainActor () -> Void = {},
        tryLocally: @escaping @MainActor () -> Void = {},
        setDND: @escaping @MainActor (PulsarDNDMode) -> Void = { _ in },
        setVolume: @escaping @MainActor (Int) -> Void = { _ in },
        toggleRecording: @escaping @MainActor () -> Void = {},
        cancelRecording: @escaping @MainActor () -> Void = {},
        setCaptureDevice: @escaping @MainActor (String?) -> Void = { _ in },
        setRecordingShortcut: @escaping @MainActor (PulsarRecordingShortcutChoice) -> Void = { _ in },
        playBuiltinCue: @escaping @MainActor () -> Void = {},
        recordFiveSeconds: @escaping @MainActor () -> Void = {},
        reviewLocalFile: @escaping @MainActor (URL) -> Void = { _ in },
        acceptLocalFile: @escaping @MainActor (URL) -> Void = { _ in },
        deleteLocalDraft: @escaping @MainActor () -> Void = {},
        closeSelfTest: @escaping @MainActor () -> Void = {},
        sendDraft: @escaping @MainActor (String, PulsarRouteTarget, PulsarDeliveryMode, Bool) -> Void = { _, _, _, _ in },
        sendTargetedDraft: @escaping @MainActor (String, PulsarDeliveryMode, Bool) -> Void = { _, _, _ in },
        deleteOutgoingDraft: @escaping @MainActor (String) -> Void = { _ in },
        refreshPhaseOneData: @escaping @MainActor () -> Void = {},
        historyAction: @escaping @MainActor (String, PulsarHistoryActionRequest) -> Void = { _, _ in },
        submitCreateOrbit: @escaping @MainActor (String) -> Void = { _ in },
        submitJoinOrbit: @escaping @MainActor (String) -> Void = { _ in },
        exportRecovery: @escaping @MainActor () -> Void = {},
        refreshAirs: @escaping @MainActor () -> Void = {},
        createAir: @escaping @MainActor (String) -> Void = { _ in },
        consumeAirInvite: @escaping @MainActor (String) -> Void = { _ in },
        confirmAirJoin: @escaping @MainActor (String, Bool) -> Void = { _, _ in },
        declineAirJoin: @escaping @MainActor (String) -> Void = { _ in },
        issueAirInvite: @escaping @MainActor (String, PulsarAirRole) -> Void = { _, _ in },
        withdrawAirInvite: @escaping @MainActor () -> Void = {},
        hideAirInvite: @escaping @MainActor () -> Void = {},
        activateAir: @escaping @MainActor (String) -> Void = { _ in },
        deactivateAir: @escaping @MainActor (String) -> Void = { _ in },
        leaveAir: @escaping @MainActor (String) -> Void = { _ in },
        dissolveAir: @escaping @MainActor (String) -> Void = { _ in },
        replaceAirPolicy: @escaping @MainActor (String, PulsarAirPolicy) -> Void = { _, _ in }
    ) {
        self.onCreateOrbit = createOrbit
        self.onJoinOrbit = joinOrbit
        self.onTryLocally = tryLocally
        self.onSetDND = setDND
        self.onSetVolume = setVolume
        self.onToggleRecording = toggleRecording
        self.onCancelRecording = cancelRecording
        self.onSetCaptureDevice = setCaptureDevice
        self.onSetRecordingShortcut = setRecordingShortcut
        self.onPlayBuiltinCue = playBuiltinCue
        self.onRecordFiveSeconds = recordFiveSeconds
        self.onReviewLocalFile = reviewLocalFile
        self.onAcceptLocalFile = acceptLocalFile
        self.onDeleteLocalDraft = deleteLocalDraft
        self.onCloseSelfTest = closeSelfTest
        self.onSendDraft = sendDraft
        self.onSendTargetedDraft = sendTargetedDraft
        self.onDeleteOutgoingDraft = deleteOutgoingDraft
        self.onRefreshPhaseOneData = refreshPhaseOneData
        self.onHistoryAction = historyAction
        self.onSubmitCreateOrbit = submitCreateOrbit
        self.onSubmitJoinOrbit = submitJoinOrbit
        self.onExportRecovery = exportRecovery
        self.onRefreshAirs = refreshAirs
        self.onCreateAir = createAir
        self.onConsumeAirInvite = consumeAirInvite
        self.onConfirmAirJoin = confirmAirJoin
        self.onDeclineAirJoin = declineAirJoin
        self.onIssueAirInvite = issueAirInvite
        self.onWithdrawAirInvite = withdrawAirInvite
        self.onHideAirInvite = hideAirInvite
        self.onActivateAir = activateAir
        self.onDeactivateAir = deactivateAir
        self.onLeaveAir = leaveAir
        self.onDissolveAir = dissolveAir
        self.onReplaceAirPolicy = replaceAirPolicy
    }

    public func createOrbit() { onCreateOrbit() }
    public func joinOrbit() { onJoinOrbit() }
    public func tryLocally() { onTryLocally() }
    public func setDND(_ mode: PulsarDNDMode) { onSetDND(mode) }
    public func setVolume(_ volume: Int) { onSetVolume(volume) }
    public func toggleRecording() { onToggleRecording() }
    public func cancelRecording() { onCancelRecording() }
    public func setCaptureDevice(_ id: String?) { onSetCaptureDevice(id) }
    public func setRecordingShortcut(_ shortcut: PulsarRecordingShortcutChoice) {
        onSetRecordingShortcut(shortcut)
    }
    public func playBuiltinCue() { onPlayBuiltinCue() }
    public func recordFiveSeconds() { onRecordFiveSeconds() }
    public func reviewLocalFile(_ url: URL) { onReviewLocalFile(url) }
    public func acceptLocalFile(_ url: URL) { onAcceptLocalFile(url) }
    public func deleteLocalDraft() { onDeleteLocalDraft() }
    public func closeSelfTest() { onCloseSelfTest() }
    public func sendDraft(
        _ id: String,
        route: PulsarRouteTarget,
        delivery: PulsarDeliveryMode,
        rightsAcknowledged: Bool
    ) { onSendDraft(id, route, delivery, rightsAcknowledged) }
    public func sendTargetedDraft(
        _ id: String,
        delivery: PulsarDeliveryMode,
        rightsAcknowledged: Bool
    ) { onSendTargetedDraft(id, delivery, rightsAcknowledged) }
    public func deleteOutgoingDraft(_ id: String) { onDeleteOutgoingDraft(id) }
    public func refreshPhaseOneData() { onRefreshPhaseOneData() }
    public func performHistoryAction(_ id: String, action: PulsarHistoryAction) {
        onHistoryAction(id, .init(action: action))
    }
    public func performHistoryAction(_ id: String, request: PulsarHistoryActionRequest) {
        onHistoryAction(id, request)
    }
    public func submitCreateOrbit(title: String) { onSubmitCreateOrbit(title) }
    public func submitJoinOrbit(code: String) { onSubmitJoinOrbit(code) }
    public func exportRecovery() { onExportRecovery() }
    public func refreshAirs() { onRefreshAirs() }
    public func createAir(title: String) { onCreateAir(title) }
    public func consumeAirInvite(code: String) { onConsumeAirInvite(code) }
    public func confirmAirJoin(_ airID: String, activate: Bool) {
        onConfirmAirJoin(airID, activate)
    }
    public func declineAirJoin(_ airID: String) { onDeclineAirJoin(airID) }
    public func issueAirInvite(_ airID: String, role: PulsarAirRole) {
        onIssueAirInvite(airID, role)
    }
    public func withdrawAirInvite() { onWithdrawAirInvite() }
    public func hideAirInvite() { onHideAirInvite() }
    public func activateAir(_ airID: String) { onActivateAir(airID) }
    public func deactivateAir(_ airID: String) { onDeactivateAir(airID) }
    public func leaveAir(_ airID: String) { onLeaveAir(airID) }
    public func dissolveAir(_ airID: String) { onDissolveAir(airID) }
    public func replaceAirPolicy(_ airID: String, policy: PulsarAirPolicy) {
        onReplaceAirPolicy(airID, policy)
    }
}

public enum PulsarShellText: String, CaseIterable, Sendable {
    case appName, home, airs, inbox, create, join, tryLocally, history, settings
    case openMainWindow, primaryActions, status, presence, routing, nowPlaying
    case localControls, noHistory, noRoute, silence, volume, dnd, recording
    case startRecording, stopRecording, recordingUnavailable, selfTestUnavailable
    case cancelRecording, recordingShortcut, shortcutRegistered, shortcutConflict
    case shortcutUnavailable, shortcutSuspended, shortcutInactive, shortcutFallback
    case inputDevice, defaultInput, playBuiltinCue, recordFiveSeconds, chooseAudioFile, dropAudioFile
    case fileReview, filename, format, duration, size, audience, deliveryModes
    case rightsReminder, serverWillRecheck, acceptDraft, deleteDraft, p2FileGuidance
    case selfTestIdle, selfTestCue, selfTestPermission, selfTestRecording
    case selfTestStopCue, selfTestPlayback, selfTestReview, selfTestFailed
    case createTitle, createBody, createAction, joinTitle, joinBody, joinAction
    case tryTitle, tryBody, tryAction, historyTitle, settingsTitle, language
    case integrations, spotifyOptional, telegramOptional
    case connectionUnpaired, connectionReconnecting, connectionOnline, connectionDegraded
    case dndAllowAll, dndMessagesOnly, dndMutedUntil
    case recordingIdle, recordingActive, recordingProcessing, recordingFailed
    case unpairedHelp, degradedHelp, recordingHelp, quit
    case outgoingDrafts, routeTarget, deliveryMode, uploadRightsConfirm, send, retry, refresh
    case selectedRecipients, sendSelectedRecipients
    case thisPulsar, ownBarycenter, currentAir, overlay, interrupt, afterCurrent
    case requestedDelivery, effectiveDelivery, coordinatorFailure, blockSender, replay, deleteHistory
    case report, reportReason, reportDetails, submitReport, cancel, confirmDelete, confirmBlock
    case orbitTitle, inviteCode, createWithAPI, joinWithAPI, identityBusy
    case identitySucceeded, identityFailed, recoveryRequired, exportRecovery
}

public struct PulsarShellCopy: Sendable {
    public let locale: PulsarShellLocale

    public init(locale: PulsarShellLocale) { self.locale = locale }

    public func text(_ key: PulsarShellText) -> String {
        switch locale {
        case .en: Self.en[key]!
        case .ru: Self.ru[key]!
        }
    }

    public func title(for section: PulsarShellSection) -> String {
        switch section {
        case .home: text(.home)
        case .airs: text(.airs)
        case .inbox: text(.inbox)
        case .create: text(.create)
        case .join: text(.join)
        case .tryLocally: text(.tryLocally)
        case .history: text(.history)
        case .settings: text(.settings)
        }
    }

    public func connectionLabel(_ state: PulsarConnectionState) -> String {
        switch state {
        case .unpaired: text(.connectionUnpaired)
        case .reconnecting: text(.connectionReconnecting)
        case .online: text(.connectionOnline)
        case .degraded(let reason): "\(text(.connectionDegraded)): \(reason)"
        }
    }

    public func connectionSymbol(_ state: PulsarConnectionState) -> String {
        switch state {
        case .unpaired: "person.crop.circle.badge.questionmark"
        case .reconnecting: "arrow.triangle.2.circlepath"
        case .online: "checkmark.circle.fill"
        case .degraded: "exclamationmark.triangle.fill"
        }
    }

    public func dndLabel(_ mode: PulsarDNDMode) -> String {
        switch mode {
        case .allowAll: text(.dndAllowAll)
        case .messagesOnly: text(.dndMessagesOnly)
        case .mutedUntil: text(.dndMutedUntil)
        }
    }

    public func recordingLabel(_ state: PulsarRecordingState) -> String {
        switch state {
        case .unavailable: text(.recordingUnavailable)
        case .idle: text(.recordingIdle)
        case .recording: text(.recordingActive)
        case .processing: text(.recordingProcessing)
        case .failed(let reason): "\(text(.recordingFailed)): \(reason)"
        }
    }

    public func recordingSymbol(_ state: PulsarRecordingState) -> String {
        switch state {
        case .unavailable: "mic.slash"
        case .idle: "mic"
        case .recording: "record.circle.fill"
        case .processing: "waveform"
        case .failed: "exclamationmark.triangle.fill"
        }
    }

    public func selfTestLabel(_ state: PulsarSelfTestState) -> String {
        switch state {
        case .idle: text(.selfTestIdle)
        case .playingBuiltinCue: text(.selfTestCue)
        case .requestingPermission: text(.selfTestPermission)
        case .recording: text(.selfTestRecording)
        case .playingStopCue: text(.selfTestStopCue)
        case .playingRecording: text(.selfTestPlayback)
        case .reviewingDraft: text(.selfTestReview)
        case .failed: text(.selfTestFailed)
        }
    }

    public func recordingShortcutLabel(_ state: PulsarRecordingShortcutState) -> String {
        switch state {
        case .inactive: text(.shortcutInactive)
        case .registered: text(.shortcutRegistered)
        case .conflict: text(.shortcutConflict)
        case .unavailable: text(.shortcutUnavailable)
        case .suspended: text(.shortcutSuspended)
        }
    }

    public func routeLabel(_ route: PulsarRouteTarget) -> String {
        switch route {
        case .thisPulsar: text(.thisPulsar)
        case .ownBarycenter: text(.ownBarycenter)
        case .currentAir: text(.currentAir)
        }
    }

    public func deliveryLabel(_ delivery: PulsarDeliveryMode) -> String {
        switch delivery {
        case .overlay: text(.overlay)
        case .interrupt: text(.interrupt)
        case .afterCurrent: text(.afterCurrent)
        }
    }

    public func draftStateLabel(_ state: PulsarOutgoingDraftState) -> String {
        switch (locale, state) {
        case (.en, .retained): "Ready to send"
        case (.en, .uploading): "Uploading"
        case (.en, .uploaded): "Upload confirmed"
        case (.en, .transmitting): "Requesting delivery"
        case (.en, .accepted): "Accepted"
        case (.en, .retryableFailure): "Retry available"
        case (.ru, .retained): "Готово к отправке"
        case (.ru, .uploading): "Загружается"
        case (.ru, .uploaded): "Загрузка подтверждена"
        case (.ru, .transmitting): "Запрашиваю доставку"
        case (.ru, .accepted): "Принято"
        case (.ru, .retryableFailure): "Можно повторить"
        }
    }

    public func moderationReasonLabel(_ reason: PulsarModerationReason) -> String {
        switch (locale, reason) {
        case (.en, .spam): "Spam"
        case (.en, .harassment): "Harassment"
        case (.en, .illegal): "Illegal content"
        case (.en, .sexualContent): "Sexual content"
        case (.en, .violence): "Violence"
        case (.en, .other): "Other"
        case (.ru, .spam): "Спам"
        case (.ru, .harassment): "Преследование"
        case (.ru, .illegal): "Незаконный контент"
        case (.ru, .sexualContent): "Сексуальный контент"
        case (.ru, .violence): "Насилие"
        case (.ru, .other): "Другое"
        }
    }

    public func historyActionMessage(_ code: String) -> String {
        let en = [
            "media_deleted": "Media deleted. It can no longer be replayed.",
            "report_received": "Report received for moderation.",
            "report_already_received": "This item was already reported; the existing report remains active.",
            "sender_blocked": "Sender blocked. New deliveries from this sender are stopped.",
            "sender_already_blocked": "Sender was already blocked.",
            "replay_accepted": "Replay accepted.",
            "replay_already_accepted": "Replay was already accepted.",
            "action_not_allowed": "This action is not available for the selected item.",
            "history_action_unavailable": "The item changed and this action is no longer available.",
            "coordinator_unavailable": "Cannot reach the coordinator. Check the connection and try again.",
            "unauthorized": "Your current account is not allowed to perform this action.",
            "forbidden": "Your current account is not allowed to perform this action.",
            "insufficient_capability": "Your current account is not allowed to perform this action.",
            "invalid_request": "Check the report details and try again.",
        ]
        let ru = [
            "media_deleted": "Медиа удалено. Его больше нельзя повторно воспроизвести.",
            "report_received": "Жалоба принята на модерацию.",
            "report_already_received": "На этот материал уже подана жалоба; существующая жалоба остаётся активной.",
            "sender_blocked": "Отправитель заблокирован. Новые доставки от него остановлены.",
            "sender_already_blocked": "Отправитель уже был заблокирован.",
            "replay_accepted": "Повтор принят.",
            "replay_already_accepted": "Повтор уже был принят.",
            "action_not_allowed": "Это действие недоступно для выбранного материала.",
            "history_action_unavailable": "Материал изменился, и действие больше недоступно.",
            "coordinator_unavailable": "Нет связи с координатором. Проверьте подключение и повторите попытку.",
            "unauthorized": "Текущей учётной записи это действие недоступно.",
            "forbidden": "Текущей учётной записи это действие недоступно.",
            "insufficient_capability": "Текущей учётной записи это действие недоступно.",
            "invalid_request": "Проверьте сведения жалобы и повторите попытку.",
        ]
        switch locale {
        case .en: return en[code] ?? "The action failed. Try again."
        case .ru: return ru[code] ?? "Не удалось выполнить действие. Повторите попытку."
        }
    }

    private static let en: [PulsarShellText: String] = [
        .appName: "Pulsar", .home: "Home", .airs: "Airs", .inbox: "Inbox & targets",
        .create: "Create", .join: "Join",
        .tryLocally: "Try locally", .history: "History", .settings: "Settings",
        .openMainWindow: "Open Pulsar", .primaryActions: "Primary actions",
        .status: "Status", .presence: "Presence", .routing: "Routing",
        .nowPlaying: "Now playing", .localControls: "Local controls",
        .noHistory: "No recent activity", .noRoute: "No output route",
        .silence: "Nothing is playing", .volume: "Volume", .dnd: "Do Not Disturb",
        .recording: "Recording", .startRecording: "Start recording",
        .stopRecording: "Stop recording", .recordingUnavailable: "Recording is not configured yet",
        .selfTestUnavailable: "Local self-test is not configured yet",
        .cancelRecording: "Cancel recording", .recordingShortcut: "Recording shortcut",
        .shortcutRegistered: "Global shortcut is active",
        .shortcutConflict: "Shortcut is used by another app",
        .shortcutUnavailable: "Global shortcuts are unavailable on this Mac",
        .shortcutSuspended: "Shortcut is paused while the session is inactive",
        .shortcutInactive: "Global shortcut is not active",
        .shortcutFallback: "Window and menu-bar recording remain available.",
        .inputDevice: "Microphone", .defaultInput: "System default",
        .playBuiltinCue: "Play reviewed cue", .recordFiveSeconds: "Record 5 seconds",
        .chooseAudioFile: "Choose short audio file", .dropAudioFile: "or drop an audio file here",
        .fileReview: "Local file review", .filename: "Filename", .format: "Format",
        .duration: "Duration", .size: "Size", .audience: "Audience",
        .deliveryModes: "Eligible delivery modes", .rightsReminder: "Rights",
        .serverWillRecheck: "The server will re-check format, duration, size and policy before delivery.",
        .acceptDraft: "Use as local draft", .deleteDraft: "Delete local draft",
        .p2FileGuidance: "This file is outside P1 limits. Streaming support is planned for P2; it has not been accepted.",
        .selfTestIdle: "Ready — no audio leaves this Mac", .selfTestCue: "Playing reviewed cue",
        .selfTestPermission: "Waiting for microphone permission", .selfTestRecording: "Recording locally",
        .selfTestStopCue: "Finishing recording", .selfTestPlayback: "Playing local recording",
        .selfTestReview: "Local draft is ready", .selfTestFailed: "Local self-test failed",
        .createTitle: "Create an air", .createBody: "Create a shared audio space directly with Barycenter. You will save a one-time recovery file before it becomes active.",
        .createAction: "Create securely", .joinTitle: "Join an air",
        .joinBody: "Enter the invitation code issued for this installation.",
        .joinAction: "Join securely", .tryTitle: "Try Pulsar locally",
        .tryBody: "Record five seconds and play them only on this Mac before sending anything.",
        .tryAction: "Run local self-test", .historyTitle: "Recent activity",
        .settingsTitle: "Pulsar settings", .language: "Language",
        .integrations: "Optional integrations",
        .spotifyOptional: "Spotify is an optional music source; Pulsar audio and local review work without it.",
        .telegramOptional: "Telegram is an optional companion control; Create, Join, routing, history, and reports remain available in Pulsar.",
        .connectionUnpaired: "Not paired", .connectionReconnecting: "Reconnecting",
        .connectionOnline: "Connected", .connectionDegraded: "Needs attention",
        .dndAllowAll: "Allow all audio", .dndMessagesOnly: "Messages only",
        .dndMutedUntil: "Muted", .recordingIdle: "Not recording",
        .recordingActive: "Recording — press Stop to finish",
        .recordingProcessing: "Preparing recording", .recordingFailed: "Recording failed",
        .unpairedHelp: "Create or join an air, try local audio, or open settings. Pairing is not required for those paths.",
        .degradedHelp: "Local controls and settings remain available while Pulsar reconnects.",
        .recordingHelp: "Recording is active. The Stop control remains available in this window and the menu bar.",
        .quit: "Quit Pulsar",
        .outgoingDrafts: "Ready to send", .routeTarget: "Send to",
        .selectedRecipients: "Selected recipients", .sendSelectedRecipients: "Send to selected recipients",
        .uploadRightsConfirm: "I created this content or have the rights, permissions, and recording consents to send it to every selected recipient.",
        .deliveryMode: "Delivery", .send: "Send", .retry: "Retry", .refresh: "Refresh",
        .thisPulsar: "This Pulsar", .ownBarycenter: "My Barycenter",
        .currentAir: "Current air", .overlay: "Play over current audio",
        .interrupt: "Pause and play", .afterCurrent: "Play after current audio",
        .requestedDelivery: "Requested", .effectiveDelivery: "Effective",
        .coordinatorFailure: "Coordinator data is temporarily unavailable",
        .blockSender: "Block sender", .replay: "Replay", .deleteHistory: "Delete permanently",
        .report: "Report", .reportReason: "Report reason", .reportDetails: "Details (optional)",
        .submitReport: "Submit report", .cancel: "Cancel",
        .confirmDelete: "Delete this media permanently? It can no longer be replayed.",
        .confirmBlock: "Block this sender? New deliveries from this sender will stop.",
        .orbitTitle: "Air name", .inviteCode: "Invitation code",
        .createWithAPI: "Create securely", .joinWithAPI: "Join securely",
        .identityBusy: "Contacting Barycenter…", .identitySucceeded: "Credentials saved",
        .identityFailed: "Identity operation failed",
        .recoveryRequired: "Save the one-time recovery file before continuing.",
        .exportRecovery: "Save recovery file",
    ]

    private static let ru: [PulsarShellText: String] = [
        .appName: "Пульсар", .home: "Главная", .airs: "Эфиры", .inbox: "Входящие и адресаты",
        .create: "Создать", .join: "Присоединиться",
        .tryLocally: "Попробовать локально", .history: "История", .settings: "Настройки",
        .openMainWindow: "Открыть Пульсар", .primaryActions: "Основные действия",
        .status: "Статус", .presence: "Присутствие", .routing: "Маршрут звука",
        .nowPlaying: "Сейчас играет", .localControls: "Локальные настройки",
        .noHistory: "Недавних событий нет", .noRoute: "Выход звука не выбран",
        .silence: "Сейчас ничего не играет", .volume: "Громкость", .dnd: "Не беспокоить",
        .recording: "Запись", .startRecording: "Начать запись",
        .stopRecording: "Остановить запись", .recordingUnavailable: "Запись пока не настроена",
        .selfTestUnavailable: "Локальная самопроверка пока не настроена",
        .cancelRecording: "Отменить запись", .recordingShortcut: "Комбинация для записи",
        .shortcutRegistered: "Глобальная комбинация активна",
        .shortcutConflict: "Комбинация занята другим приложением",
        .shortcutUnavailable: "Глобальные комбинации недоступны на этом маке",
        .shortcutSuspended: "Комбинация приостановлена, пока сессия неактивна",
        .shortcutInactive: "Глобальная комбинация не активна",
        .shortcutFallback: "Запись из окна и строки меню остаётся доступна.",
        .inputDevice: "Микрофон", .defaultInput: "Системный по умолчанию",
        .playBuiltinCue: "Воспроизвести проверенный сигнал", .recordFiveSeconds: "Записать 5 секунд",
        .chooseAudioFile: "Выбрать короткий аудиофайл", .dropAudioFile: "или перетащи аудиофайл сюда",
        .fileReview: "Проверка локального файла", .filename: "Имя файла", .format: "Формат",
        .duration: "Длительность", .size: "Размер", .audience: "Аудитория",
        .deliveryModes: "Доступные режимы доставки", .rightsReminder: "Права",
        .serverWillRecheck: "Перед доставкой сервер повторно проверит формат, длительность, размер и правила.",
        .acceptDraft: "Создать локальный черновик", .deleteDraft: "Удалить локальный черновик",
        .p2FileGuidance: "Файл выходит за пределы P1. Потоковая поддержка запланирована на P2; файл не принят.",
        .selfTestIdle: "Готово — звук не покидает этот мак", .selfTestCue: "Воспроизвожу проверенный сигнал",
        .selfTestPermission: "Жду разрешения на микрофон", .selfTestRecording: "Записываю локально",
        .selfTestStopCue: "Завершаю запись", .selfTestPlayback: "Воспроизвожу локальную запись",
        .selfTestReview: "Локальный черновик готов", .selfTestFailed: "Ошибка локальной самопроверки",
        .createTitle: "Создать эфир", .createBody: "Создай общее аудиопространство напрямую в Барицентре. Перед активацией нужно сохранить одноразовый файл восстановления.",
        .createAction: "Создать безопасно", .joinTitle: "Присоединиться к эфиру",
        .joinBody: "Введи код приглашения, выпущенный для этой установки.",
        .joinAction: "Присоединиться безопасно", .tryTitle: "Проверить Пульсар локально",
        .tryBody: "Запиши пять секунд и воспроизведи их только на этом маке до любой отправки.",
        .tryAction: "Запустить самопроверку", .historyTitle: "Недавние события",
        .settingsTitle: "Настройки Пульсара", .language: "Язык",
        .integrations: "Необязательные интеграции",
        .spotifyOptional: "Spotify — необязательный источник музыки; звук Пульсара и локальная проверка работают без него.",
        .telegramOptional: "Telegram — необязательный пульт; создание, присоединение, маршрутизация, история и жалобы доступны в Пульсаре.",
        .connectionUnpaired: "Не подключён", .connectionReconnecting: "Переподключение",
        .connectionOnline: "Подключён", .connectionDegraded: "Нужно внимание",
        .dndAllowAll: "Разрешить весь звук", .dndMessagesOnly: "Только сообщения",
        .dndMutedUntil: "Звук выключен", .recordingIdle: "Запись не идёт",
        .recordingActive: "Идёт запись — нажми «Остановить», чтобы закончить",
        .recordingProcessing: "Подготавливаю запись", .recordingFailed: "Ошибка записи",
        .unpairedHelp: "Создай эфир, присоединись, проверь локальный звук или открой настройки — для этих путей подключение не требуется.",
        .degradedHelp: "Локальные настройки остаются доступны, пока Пульсар переподключается.",
        .recordingHelp: "Запись активна. Кнопка остановки остаётся доступна в этом окне и в строке меню.",
        .quit: "Выйти из Пульсара",
        .outgoingDrafts: "Готово к отправке", .routeTarget: "Отправить в",
        .selectedRecipients: "Выбранные адресаты", .sendSelectedRecipients: "Отправить выбранным адресатам",
        .uploadRightsConfirm: "Я создал(а) этот материал либо имею права, разрешения и согласия на запись, чтобы отправить его каждому выбранному получателю.",
        .deliveryMode: "Доставка", .send: "Отправить", .retry: "Повторить", .refresh: "Обновить",
        .thisPulsar: "Этот Пульсар", .ownBarycenter: "Мой Барицентр",
        .currentAir: "Текущий эфир", .overlay: "Поверх текущего звука",
        .interrupt: "Приостановить и воспроизвести", .afterCurrent: "После текущего звука",
        .requestedDelivery: "Запрошено", .effectiveDelivery: "Фактически",
        .coordinatorFailure: "Данные координатора временно недоступны",
        .blockSender: "Заблокировать отправителя", .replay: "Повторить", .deleteHistory: "Удалить навсегда",
        .report: "Пожаловаться", .reportReason: "Причина жалобы", .reportDetails: "Детали (необязательно)",
        .submitReport: "Отправить жалобу", .cancel: "Отмена",
        .confirmDelete: "Удалить это медиа навсегда? Его больше нельзя будет повторить.",
        .confirmBlock: "Заблокировать отправителя? Новые доставки от него остановятся.",
        .orbitTitle: "Название эфира", .inviteCode: "Код приглашения",
        .createWithAPI: "Создать безопасно", .joinWithAPI: "Присоединиться безопасно",
        .identityBusy: "Связываюсь с Барицентром…", .identitySucceeded: "Данные доступа сохранены",
        .identityFailed: "Не удалось выполнить действие с доступом",
        .recoveryRequired: "Сохрани одноразовый файл восстановления перед продолжением.",
        .exportRecovery: "Сохранить файл восстановления",
    ]
}
