import Foundation

public protocol MacCaptureWorkflowCapturing: MacLocalSelfTestCapturing {
    func availableDevices() -> [MacCaptureDevice]
    func shutdown()
}

extension MacMicrophoneCaptureEngine: MacCaptureWorkflowCapturing {}

public enum MacCaptureWorkflowRecordingFailure: Equatable, Sendable {
    case explicitUserActionRequired
    case permissionDenied
    case permissionRestricted
    case noInputDevice
    case selectedDeviceUnavailable
    case alreadyActive
    case invalidState
    case backendUnavailable
    case backendStartupFailed(MacCaptureStartupDiagnostic)
    case captureQualityUnsupported
    case storage
    case cuePlaybackFailed
    case startCueFailed
}

public enum MacCaptureWorkflowRecordingState: Equatable, Sendable {
    case idle
    case processing
    case recording
    case failed(MacCaptureWorkflowRecordingFailure)
}

public enum MacCaptureWorkflowEvent: Equatable, Sendable {
    case recording(MacCaptureWorkflowRecordingState)
    case recordingMeter(Float)
    case devices([MacCaptureDevice])
    case captureQuality(CaptureQualityState?)
    case normalDraft(CaptureMediaHandle)
    case normalDraftDeleted
    case selfTest(MacLocalSelfTestEvent)
}

/// Main-actor composition of the already reviewed capture, cue, local-output,
/// self-test and file-intake components. It is the single operation gate for
/// the microphone and production local mixer; callback threads only enqueue
/// bounded state events and never perform UI, file or network work.
@MainActor
public final class MacCaptureWorkflowController {
    private enum Owner: Equatable {
        case idle
        case normal
        case selfTest
    }

    private let capture: MacCaptureWorkflowCapturing
    private let output: MacLocalClipPlaying
    private let store: CaptureMediaStore
    private let selfTest: MacLocalSelfTestService
    private let cueURL: URL
    private var owner: Owner = .idle
    private var normalStopCuePlaying = false
    private var normalCanStop = false
    private var pendingNormalDraft: CaptureMediaHandle?
    private var latestNormalDraft: CaptureMediaHandle?
    private var selectedDeviceID: String?
    private var stopped = false

    public var onEvent: ((MacCaptureWorkflowEvent) -> Void)?

    public init(
        capture: MacCaptureWorkflowCapturing,
        output: MacLocalClipPlaying,
        store: CaptureMediaStore,
        intake: MacShortAudioIntake,
        cueURL: URL,
        eventQueue: DispatchQueue = .main
    ) throws {
        self.capture = capture
        self.output = output
        self.store = store
        self.cueURL = cueURL
        self.selfTest = try MacLocalSelfTestService(
            capture: capture,
            output: output,
            store: store,
            intake: intake,
            cueURL: cueURL,
            eventQueue: eventQueue,
            bindCaptureEvents: false)
        capture.onEvent = { [weak self] event in
            Task { @MainActor [weak self] in self?.consumeCaptureEvent(event) }
        }
        selfTest.onEvent = { [weak self] event in
            Task { @MainActor [weak self] in self?.consumeSelfTestEvent(event) }
        }
    }

    public func start() {
        guard !stopped else { return }
        let devices = capture.availableDevices()
        if let selectedDeviceID,
            !devices.contains(where: { $0.id == selectedDeviceID })
        {
            self.selectedDeviceID = nil
        }
        onEvent?(.devices(devices))
        onEvent?(.recording(.idle))
        onEvent?(.selfTest(.phase(.idle)))
    }

    public func selectDevice(_ id: String?) {
        guard !stopped, owner == .idle else { return }
        let devices = capture.availableDevices()
        selectedDeviceID = id.flatMap { candidate in
            devices.contains(where: { $0.id == candidate }) ? candidate : nil
        }
        onEvent?(.devices(devices))
    }

    @discardableResult
    public func setCaptureQualityRequest(_ request: MacCaptureQualityRequest) -> Bool {
        guard !stopped, owner == .idle,
            let capture = capture as? MacCaptureQualityWorkflowSelecting
        else {
            return false
        }
        capture.setCaptureQualityRequest(request)
        return true
    }

    public func toggleNormalRecording() {
        guard !stopped else { return }
        switch owner {
        case .idle:
            owner = .normal
            (capture as? MacCaptureQualityWorkflowSelecting)?
                .selectCaptureQualityWorkflow("recorded_clip")
            normalCanStop = false
            onEvent?(.recording(.processing))
            Task { @MainActor [weak self] in
                guard let self else { return }
                do {
                    try await self.capture.begin(
                        selectedDeviceID: self.selectedDeviceID,
                        explicitUserAction: true)
                } catch {
                    self.finishNormalFailure(Self.recordingFailure(from: error))
                }
            }
        case .normal:
            guard normalCanStop else { return }
            normalCanStop = false
            do {
                try capture.stop()
                onEvent?(.recording(.processing))
            } catch {
                finishNormalFailure(Self.recordingFailure(from: error))
            }
        case .selfTest:
            return
        }
    }

    public func cancelNormalRecording() {
        guard !stopped, owner == .normal else { return }
        capture.cancel()
    }

    public func playBuiltinCue() {
        guard !stopped, owner == .idle else { return }
        owner = .selfTest
        selfTest.playBuiltinCue()
    }

    public func recordFiveSeconds() {
        guard !stopped, owner == .idle else { return }
        owner = .selfTest
        (capture as? MacCaptureQualityWorkflowSelecting)?
            .selectCaptureQualityWorkflow("local_self_test")
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await self.selfTest.recordFiveSeconds(
                    selectedDeviceID: self.selectedDeviceID)
            } catch {
                self.owner = .idle
                self.onEvent?(.selfTest(.failure("capture_failed")))
                self.onEvent?(.selfTest(.phase(.failed)))
            }
        }
    }

    public func reviewFile(_ url: URL) { selfTest.reviewFile(url) }
    public func acceptFile(_ url: URL) { selfTest.acceptFile(url) }
    public func deleteSelfTestDraft() { selfTest.deleteDraft() }
    public func deleteLocalDraft() {
        selfTest.deleteDraft()
        if let latestNormalDraft {
            try? store.explicitlyDelete(latestNormalDraft)
            self.latestNormalDraft = nil
            onEvent?(.normalDraftDeleted)
        }
    }

    public func closeSelfTest() {
        guard !stopped, owner != .normal else { return }
        selfTest.close()
        owner = .idle
    }

    public func shutdown() {
        guard !stopped else { return }
        stopped = true
        selfTest.close()
        capture.shutdown()
        output.cancel()
        owner = .idle
        pendingNormalDraft = nil
        normalStopCuePlaying = false
        normalCanStop = false
    }

    private func consumeCaptureEvent(_ event: MacCaptureEvent) {
        guard !stopped else { return }
        if case .quality(let state) = event {
            onEvent?(.captureQuality(state))
            return
        }
        switch owner {
        case .selfTest:
            selfTest.consumeCaptureEvent(event)
        case .normal:
            consumeNormalCaptureEvent(event)
        case .idle:
            if case .devices(let devices) = event { onEvent?(.devices(devices)) }
        }
    }

    private func consumeNormalCaptureEvent(_ event: MacCaptureEvent) {
        switch event {
        case .phase(.requestingPermission), .phase(.playingStartCue), .phase(.finalizing):
            onEvent?(.recording(.processing))
        case .phase(.recording):
            normalCanStop = true
            onEvent?(.recording(.recording))
        case .phase(.idle):
            break
        case .devices(let devices):
            onEvent?(.devices(devices))
        case .meter(let value):
            onEvent?(.recordingMeter(value))
        case .quality:
            break
        case .playStartCue:
            output.play(fileURL: cueURL) { [weak self] result in
                Task { @MainActor [weak self] in
                    guard let self, self.owner == .normal else { return }
                    switch result {
                    case .success:
                        do { try self.capture.startCueCompleted() } catch { self.finishNormalFailure(.startCueFailed) }
                    case .failure:
                        self.capture.cancel()
                        self.finishNormalFailure(.cuePlaybackFailed)
                    }
                }
            }
        case .playStopCue:
            normalStopCuePlaying = true
            onEvent?(.recording(.processing))
            output.play(fileURL: cueURL) { [weak self] result in
                Task { @MainActor [weak self] in
                    guard let self, self.owner == .normal else { return }
                    self.normalStopCuePlaying = false
                    switch result {
                    case .success: self.finishNormalIfReady()
                    case .failure: self.finishNormalFailure(.cuePlaybackFailed)
                    }
                }
            }
        case .finished(let handle, _):
            pendingNormalDraft = handle
            finishNormalIfReady()
        case .cancelled:
            output.cancel()
            owner = .idle
            pendingNormalDraft = nil
            normalStopCuePlaying = false
            normalCanStop = false
            onEvent?(.recordingMeter(0))
            onEvent?(.recording(.idle))
        case .failed(let error):
            finishNormalFailure(Self.recordingFailure(from: error))
        }
    }

    private func finishNormalIfReady() {
        guard !normalStopCuePlaying, let draft = pendingNormalDraft else { return }
        pendingNormalDraft = nil
        latestNormalDraft = draft
        owner = .idle
        normalCanStop = false
        onEvent?(.recordingMeter(0))
        onEvent?(.normalDraft(draft))
        onEvent?(.recording(.idle))
    }

    private func finishNormalFailure(_ failure: MacCaptureWorkflowRecordingFailure) {
        guard owner == .normal else { return }
        output.cancel()
        capture.cancel()
        pendingNormalDraft = nil
        normalStopCuePlaying = false
        owner = .idle
        normalCanStop = false
        onEvent?(.recordingMeter(0))
        onEvent?(.recording(.failed(failure)))
    }

    private static func recordingFailure(
        from error: Error
    ) -> MacCaptureWorkflowRecordingFailure {
        guard let error = error as? MacCaptureEngineError else {
            return .backendUnavailable
        }
        switch error {
        case .explicitUserActionRequired: return .explicitUserActionRequired
        case .permissionDenied: return .permissionDenied
        case .permissionRestricted: return .permissionRestricted
        case .noInputDevice: return .noInputDevice
        case .selectedDeviceUnavailable: return .selectedDeviceUnavailable
        case .alreadyActive: return .alreadyActive
        case .invalidState: return .invalidState
        case .backendUnavailable: return .backendUnavailable
        case .backendStartupFailed(let diagnostic):
            return .backendStartupFailed(diagnostic)
        case .captureQualityUnsupported: return .captureQualityUnsupported
        case .storage: return .storage
        }
    }

    private func consumeSelfTestEvent(_ event: MacLocalSelfTestEvent) {
        guard !stopped else { return }
        onEvent?(.selfTest(event))
        if case .phase(let phase) = event,
            phase == .idle || phase == .reviewingDraft || phase == .failed
        {
            owner = .idle
        }
    }
}
