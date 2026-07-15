import Foundation
import Testing
@testable import NodeCore

@MainActor
@Suite("macOS integrated capture workflow")
struct MacCaptureWorkflowControllerTests {
    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    private var cueURL: URL {
        repositoryRoot.appendingPathComponent("assets/audio/pulsar-recording-cue.wav")
    }

    @Test("Normal capture serializes cues and publishes a durable draft only after the stop cue")
    func normalCaptureSequence() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.root) }
        var events: [MacCaptureWorkflowEvent] = []
        fixture.controller.onEvent = { events.append($0) }

        fixture.controller.selectDevice("external")
        fixture.controller.start()
        fixture.controller.toggleNormalRecording()
        try await waitUntil { fixture.output.playedURLs.count == 1 }
        #expect(fixture.capture.selectedDeviceID == "external")
        #expect(fixture.output.playedURLs == [cueURL])

        fixture.output.completeNext(.success(()))
        try await waitUntil { fixture.capture.startCueCompletedCount == 1 }
        #expect(events.contains(.recording(.recording)))

        fixture.controller.toggleNormalRecording()
        try await waitUntil { fixture.output.playedURLs.count == 2 }
        #expect(!events.contains(.normalDraft(fixture.draft)))
        fixture.output.completeNext(.success(()))
        try await waitUntil { events.contains(.normalDraft(fixture.draft)) }
        #expect(events.last == .recording(.idle))

        fixture.controller.deleteLocalDraft()
        #expect(!FileManager.default.fileExists(atPath: fixture.draft.fileURL.path))
        #expect(events.contains(.normalDraftDeleted))
    }

    @Test("Self-test owns the shared capture gate and shutdown is idempotent")
    func mutualExclusionAndShutdown() async throws {
        let fixture = try makeFixture()
        defer { try? FileManager.default.removeItem(at: fixture.root) }
        var events: [MacCaptureWorkflowEvent] = []
        fixture.controller.onEvent = { events.append($0) }

        fixture.controller.recordFiveSeconds()
        try await waitUntil { fixture.output.playedURLs.count == 1 }
        fixture.controller.toggleNormalRecording()
        #expect(fixture.capture.beginCount == 1)
        #expect(!events.contains { event in
            if case .recording(.failed) = event { return true }
            return false
        })

        fixture.controller.shutdown()
        fixture.controller.shutdown()
        #expect(fixture.capture.shutdownCount == 1)
        #expect(fixture.capture.cancelCount >= 1)
        #expect(fixture.output.cancelCount >= 1)
    }

    @Test("Application capture composition owns no coordinator, upload or network client")
    func applicationBoundaryIsLocal() throws {
        let source = try String(contentsOf: repositoryRoot.appendingPathComponent(
            "node-app/Sources/NodeApp/MacCaptureAppComposition.swift"), encoding: .utf8)
        for forbidden in ["CoordinatorClient", "URLSession", "beginUpload", "MediaClipClient"] {
            #expect(!source.contains(forbidden))
        }
        #expect(source.contains("MacProductionLocalClipOutput"))
        #expect(source.contains("MacRecordingShortcutLifecycle"))
    }

    @Test("Application data compositions preserve the self-test and recovery boundaries")
    func applicationDataBoundaries() throws {
        let phaseOne = try String(contentsOf: repositoryRoot.appendingPathComponent(
            "node-app/Sources/NodeApp/MacPhaseOneAppComposition.swift"), encoding: .utf8)
        #expect(phaseOne.contains("PhaseOneDraftOutbox"))
        #expect(phaseOne.contains("onNormalDraft"))
        #expect(!phaseOne.contains("MacLocalSelfTest"))
        #expect(!phaseOne.contains("selfTest"))

        let identity = try String(contentsOf: repositoryRoot.appendingPathComponent(
            "node-app/Sources/NodeApp/MacIdentityAppComposition.swift"), encoding: .utf8)
        #expect(identity.contains("OnboardingService"))
        #expect(identity.contains("RecoveryExportHelper.save"))
        #expect(identity.contains("acknowledgeRecoveryBackup"))
        #expect(identity.range(of: "onCredentialsActivated()", options: .backwards)!.lowerBound >
                identity.range(of: "acknowledgeRecoveryBackup")!.lowerBound)
    }

    private func makeFixture() throws -> WorkflowFixture {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(
            "mac-capture-workflow-\(UUID().uuidString)", isDirectory: true)
        let store = CaptureMediaStore(root: root.appendingPathComponent("store"))
        let draft = try store.importUserDraft(bytes: Data(contentsOf: cueURL))
        let capture = WorkflowCapture(draft: draft)
        let output = WorkflowOutput()
        let controller = try MacCaptureWorkflowController(
            capture: capture,
            output: output,
            store: store,
            intake: MacShortAudioIntake(inspector: .init(), store: store),
            cueURL: cueURL)
        return WorkflowFixture(
            root: root,
            draft: draft,
            capture: capture,
            output: output,
            controller: controller)
    }

    private func waitUntil(
        timeout: Duration = .seconds(5),
        _ condition: @escaping @MainActor () -> Bool
    ) async throws {
        let clock = ContinuousClock()
        let deadline = clock.now.advanced(by: timeout)
        while !condition(), clock.now < deadline {
            try await Task.sleep(for: .milliseconds(2))
        }
        if !condition() { throw WorkflowWaitError.timeout }
    }
}

private struct WorkflowFixture {
    let root: URL
    let draft: CaptureMediaHandle
    let capture: WorkflowCapture
    let output: WorkflowOutput
    let controller: MacCaptureWorkflowController
}

private final class WorkflowCapture: MacCaptureWorkflowCapturing, @unchecked Sendable {
    var onEvent: (@Sendable (MacCaptureEvent) -> Void)?
    let draft: CaptureMediaHandle
    private(set) var selectedDeviceID: String?
    private(set) var beginCount = 0
    private(set) var startCueCompletedCount = 0
    private(set) var cancelCount = 0
    private(set) var shutdownCount = 0

    init(draft: CaptureMediaHandle) { self.draft = draft }

    func availableDevices() -> [MacCaptureDevice] {
        [
            .init(id: "default", name: "Default", isDefault: true),
            .init(id: "external", name: "External", isDefault: false),
        ]
    }

    func begin(selectedDeviceID: String?, explicitUserAction: Bool) async throws {
        #expect(explicitUserAction)
        beginCount += 1
        self.selectedDeviceID = selectedDeviceID
        onEvent?(.phase(.requestingPermission))
        onEvent?(.phase(.playingStartCue))
        onEvent?(.playStartCue)
    }

    func startCueCompleted() throws {
        startCueCompletedCount += 1
        onEvent?(.phase(.recording))
    }

    func stop() throws {
        onEvent?(.phase(.finalizing))
        onEvent?(.playStopCue)
        onEvent?(.finished(draft, .userStopped))
        onEvent?(.phase(.idle))
    }

    func cancel() { cancelCount += 1 }
    func shutdown() { shutdownCount += 1 }
}

private final class WorkflowOutput: MacLocalClipPlaying, @unchecked Sendable {
    private let lock = NSLock()
    private var urls: [URL] = []
    private var completions: [@Sendable (Result<Void, MacLocalClipOutputError>) -> Void] = []
    private var cancels = 0

    var playedURLs: [URL] { lock.withLock { urls } }
    var cancelCount: Int { lock.withLock { cancels } }

    func play(
        fileURL: URL,
        completion: @escaping @Sendable (Result<Void, MacLocalClipOutputError>) -> Void
    ) {
        lock.withLock {
            urls.append(fileURL)
            completions.append(completion)
        }
    }

    func completeNext(_ result: Result<Void, MacLocalClipOutputError>) {
        let completion = lock.withLock { completions.removeFirst() }
        completion(result)
    }

    func cancel() { lock.withLock { cancels += 1 } }
}

private enum WorkflowWaitError: Error { case timeout }
