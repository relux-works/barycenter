import AppKit
import Foundation
import NodeAppUI
import NodeCore

enum MacCaptureAppCompositionError: Error {
    case cueUnavailable
}

/// Production binding between the platform-neutral shell and the reviewed
/// macOS capture components. This object owns every callback and hook so a
/// runtime replacement or quit has one idempotent teardown boundary.
@MainActor
final class MacCaptureAppComposition {
    private static let selectedInputKey = "captureInputDevice.v1"

    private let model: PulsarShellModel
    private let workflow: MacCaptureWorkflowController
    let mediaStore: CaptureMediaStore
    let recoveredDrafts: [CaptureMediaHandle]
    private let shortcutStore: MacRecordingShortcutStore
    private let shortcutController: MacRecordingShortcutController
    private let shortcutLifecycle: MacRecordingShortcutLifecycle
    private let defaults: UserDefaults
    private var selectedDeviceID: String?
    private var selfTestDraftAvailable = false
    private var stopped = false
    var onNormalDraft: ((CaptureMediaHandle) -> Void)?

    init(
        audio: AudioEngine,
        log: Logger,
        supportRoot: URL,
        model: PulsarShellModel,
        defaults: UserDefaults = .standard,
        bundle: Bundle = .main
    ) throws {
        guard let cueURL = bundle.url(
            forResource: "pulsar-recording-cue",
            withExtension: "wav",
            subdirectory: "Audio") else {
            throw MacCaptureAppCompositionError.cueUnavailable
        }
        _ = try BuiltinRecordingCue.load(from: cueURL)

        let store = CaptureMediaStore(
            root: supportRoot.appendingPathComponent("CaptureMedia", isDirectory: true))
        let recovery = try store.recover()
        mediaStore = store
        recoveredDrafts = recovery.retainedDrafts
        let inspector = MacShortAudioInspector()
        let intake = MacShortAudioIntake(inspector: inspector, store: store)
        let capture = MacMicrophoneCaptureEngine(
            permission: SystemMicrophonePermissionAuthorizer(),
            backend: MacAVAudioCaptureBackend(),
            mediaStore: store,
            ducker: AudioEngineCaptureDucker(audio: audio))
        let output = MacProductionLocalClipOutput(audio: audio, log: log)
        workflow = try MacCaptureWorkflowController(
            capture: capture,
            output: output,
            store: store,
            intake: intake,
            cueURL: cueURL)
        self.model = model
        self.defaults = defaults
        selectedDeviceID = defaults.string(forKey: Self.selectedInputKey)

        shortcutStore = MacRecordingShortcutStore(defaults: defaults)
        let initialShortcut = shortcutStore.load()
        shortcutController = MacRecordingShortcutController(
            registrar: CarbonGlobalShortcutRegistrar(),
            shortcut: initialShortcut,
            onToggleRecording: { [weak workflow] in workflow?.toggleNormalRecording() })
        shortcutLifecycle = MacRecordingShortcutLifecycle(
            controller: shortcutController,
            cancelRecording: { [weak workflow] in workflow?.cancelNormalRecording() })

        workflow.onEvent = { [weak self] event in self?.consume(event) }
        shortcutController.onStateChange = { [weak self] state in self?.consume(state) }
    }

    func start() {
        guard !stopped else { return }
        model.setSelfTestAvailable(true)
        model.setRecording(.idle, available: true)
        workflow.selectDevice(selectedDeviceID)
        workflow.start()
        shortcutLifecycle.start()
    }

    func toggleRecording() { workflow.toggleNormalRecording() }
    func cancelRecording() { workflow.cancelNormalRecording() }
    func playBuiltinCue() { workflow.playBuiltinCue() }
    func recordFiveSeconds() { workflow.recordFiveSeconds() }
    func reviewFile(_ url: URL) { workflow.reviewFile(url) }
    func acceptFile(_ url: URL) { workflow.acceptFile(url) }
    func closeSelfTest() { workflow.closeSelfTest() }

    func deleteLocalDraft() {
        // This action is owned by the local self-test view. Finalized normal
        // recordings are deleted only through PhaseOneDraftOutbox so its
        // durable operation record cannot be orphaned.
        workflow.deleteSelfTestDraft()
        selfTestDraftAvailable = false
        updateDraftAvailability()
    }

    func selectDevice(_ id: String?) {
        selectedDeviceID = id
        if let id {
            defaults.set(id, forKey: Self.selectedInputKey)
        } else {
            defaults.removeObject(forKey: Self.selectedInputKey)
        }
        workflow.selectDevice(id)
    }

    func setShortcut(_ choice: PulsarRecordingShortcutChoice) {
        let shortcut = Self.platformShortcut(choice)
        shortcutStore.save(shortcut)
        shortcutController.reconfigure(shortcut)
        model.setRecordingShortcut(choice, state: Self.shellShortcutState(shortcutController.state))
    }

    var currentShortcut: MacRecordingShortcut { shortcutController.shortcut }

    func shutdown() {
        guard !stopped else { return }
        stopped = true
        shortcutLifecycle.stop()
        workflow.shutdown()
        model.setRecording(.unavailable, available: false)
        model.setSelfTestAvailable(false)
        model.setRecordingShortcut(
            model.snapshot.recordingShortcut,
            state: .inactive)
    }

    private func consume(_ event: MacCaptureWorkflowEvent) {
        switch event {
        case .recording(let state):
            model.setRecording(Self.shellRecordingState(state), available: true)
        case .recordingMeter(let value):
            model.setRecordingMeter(value)
        case .devices(let devices):
            if let selectedDeviceID,
               !devices.contains(where: { $0.id == selectedDeviceID }) {
                self.selectedDeviceID = nil
                defaults.removeObject(forKey: Self.selectedInputKey)
            }
            model.setCaptureDevices(
                devices.map {
                    PulsarCaptureDevice(id: $0.id, name: $0.name, isDefault: $0.isDefault)
                },
                selectedDeviceID: selectedDeviceID)
        case .normalDraft(let handle):
            onNormalDraft?(handle)
        case .normalDraftDeleted:
            break
        case .selfTest(let event):
            consume(event)
        }
    }

    private func consume(_ event: MacLocalSelfTestEvent) {
        switch event {
        case .phase(let phase):
            model.updateSelfTest(state: Self.shellSelfTestState(phase))
        case .meter(let value):
            model.updateSelfTest(state: model.snapshot.selfTestState, meter: value)
        case .fileReview(let review):
            model.setLocalFileReview(PulsarLocalFileReview(
                filename: review.filename,
                format: review.format?.rawValue,
                durationMs: review.durationMs,
                sizeBytes: review.sizeBytes,
                audience: review.audience,
                deliveryModes: review.deliveryModes,
                rightsReminder: review.rightsReminder,
                serverValidationRequired: review.serverValidationRequired,
                rejection: review.rejection?.rawValue))
        case .draft:
            selfTestDraftAvailable = true
            updateDraftAvailability()
        case .failure:
            model.updateSelfTest(state: .failed)
        }
    }

    private func consume(_ state: MacRecordingShortcutState) {
        model.setRecordingShortcut(
            Self.shellShortcutChoice(shortcutController.shortcut),
            state: Self.shellShortcutState(state))
    }

    private func updateDraftAvailability() {
        model.updateSelfTest(
            state: model.snapshot.selfTestState,
            draftAvailable: selfTestDraftAvailable)
    }

    private static func shellRecordingState(
        _ state: MacCaptureWorkflowRecordingState
    ) -> PulsarRecordingState {
        switch state {
        case .idle: .idle
        case .processing: .processing
        case .recording: .recording
        case .failed(let code): .failed(code)
        }
    }

    private static func shellSelfTestState(_ phase: MacLocalSelfTestPhase) -> PulsarSelfTestState {
        switch phase {
        case .idle: .idle
        case .playingBuiltinCue: .playingBuiltinCue
        case .requestingPermission: .requestingPermission
        case .recording: .recording
        case .playingStopCue: .playingStopCue
        case .playingRecording: .playingRecording
        case .reviewingDraft: .reviewingDraft
        case .failed: .failed
        }
    }

    private static func platformShortcut(
        _ choice: PulsarRecordingShortcutChoice
    ) -> MacRecordingShortcut {
        switch choice {
        case .controlShiftSpace:
            MacRecordingShortcut(key: .space, modifiers: [.control, .shift])!
        case .commandShiftSpace:
            MacRecordingShortcut(key: .space, modifiers: [.command, .shift])!
        case .controlOptionSpace:
            MacRecordingShortcut(key: .space, modifiers: [.control, .option])!
        case .controlShiftR:
            MacRecordingShortcut(key: .r, modifiers: [.control, .shift])!
        }
    }

    private static func shellShortcutChoice(
        _ shortcut: MacRecordingShortcut
    ) -> PulsarRecordingShortcutChoice {
        switch (shortcut.key, shortcut.modifiers) {
        case (.space, [.command, .shift]): .commandShiftSpace
        case (.space, [.control, .option]): .controlOptionSpace
        case (.r, [.control, .shift]): .controlShiftR
        default: .controlShiftSpace
        }
    }

    private static func shellShortcutState(
        _ state: MacRecordingShortcutState
    ) -> PulsarRecordingShortcutState {
        switch state {
        case .inactive: .inactive
        case .registered: .registered
        case .conflict: .conflict
        case .unavailable: .unavailable
        case .suspended: .suspended
        }
    }
}
