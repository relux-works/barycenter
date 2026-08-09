import Foundation
import Testing

@testable import NodeCore

@Suite("macOS microphone capture engine")
struct MacMicrophoneCaptureEngineTests {
    @Test("Permission is requested only by explicit Record and denial is typed")
    func explicitPermissionBoundary() async throws {
        let fixture = Fixture(permission: .denied)
        await #expect(throws: MacCaptureEngineError.explicitUserActionRequired) {
            try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: false)
        }
        #expect(fixture.permission.requestCount == 0)
        #expect(fixture.backend.startCount == 0)

        await #expect(throws: MacCaptureEngineError.permissionDenied) {
            try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        }
        #expect(fixture.permission.requestCount == 0)
        #expect(fixture.backend.startCount == 0)
        #expect(fixture.ducker.values.isEmpty)
    }

    @Test("Undetermined TCC is requested once after explicit Record")
    func permissionRequestAfterRecord() async throws {
        let fixture = Fixture(permission: .notDetermined, requestedPermission: .granted)
        try await fixture.engine.begin(selectedDeviceID: "selected", explicitUserAction: true)
        #expect(fixture.permission.requestCount == 1)
        #expect(fixture.backend.selectedDeviceID == "selected")
        fixture.engine.cancel()
        fixture.waitUntilIdle()
    }

    @Test("Cancelling an open TCC prompt prevents a late grant from starting capture")
    func cancelWhilePermissionIsPending() async throws {
        let fixture = Fixture(
            permission: .notDetermined,
            requestedPermission: .granted,
            deferPermissionRequest: true)
        let attempt = Task {
            try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        }
        fixture.permission.waitUntilRequested()
        fixture.engine.cancel()
        fixture.waitUntilIdle()
        fixture.permission.resolveRequest()

        await #expect(throws: MacCaptureEngineError.invalidState) {
            try await attempt.value
        }
        #expect(fixture.backend.startCount == 0)
        #expect(fixture.ducker.values.isEmpty)
        #expect(try fixture.store.recover().retainedDrafts.isEmpty)
    }

    @Test("Start and stop cues remain outside the one finalized mono draft")
    func normalStopFinalizesExactlyOneCueFreeDraft() async throws {
        let fixture = Fixture(permission: .granted)
        try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        fixture.backend.emit(Array(repeating: 0.9, count: 12))  // start cue window: discarded
        try fixture.engine.startCueCompleted()
        let microphone: [Float] = [0.25, -0.25, 0.5, -0.5]
        fixture.backend.emit(microphone)
        try fixture.engine.stop()
        fixture.flushEvents()

        let finished = fixture.events.compactMap { event -> CaptureMediaHandle? in
            if case .finished(let draft, .userStopped) = event { return draft }
            return nil
        }
        #expect(finished.count == 1)
        let draft = try #require(finished.first)
        let wav = try Data(contentsOf: draft.fileURL)
        #expect(wav.count == 44 + microphone.count * 2)
        #expect(wav[44..<wav.count] != Data(repeating: 0, count: microphone.count * 2))
        #expect(fixture.events.firstIndex(of: .playStartCue) != nil)
        #expect(fixture.events.firstIndex(of: .playStopCue) != nil)
        #expect(fixture.ducker.values == [true, false])
        #expect(try fixture.store.recover().retainedDrafts == [draft])
        #expect(throws: MacCaptureEngineError.invalidState) { try fixture.engine.stop() }
    }

    @Test(
        "Duration and byte hard limits auto-stop with explicit reasons",
        arguments: [
            (
                MacCaptureLimits(maxDurationSeconds: 1, maxBytes: 1_000_000), 48_100,
                MacCaptureTerminalReason.durationLimit
            ),
            (MacCaptureLimits(maxDurationSeconds: 180, maxBytes: 52), 10, MacCaptureTerminalReason.byteLimit),
        ])
    func hardLimits(limits: MacCaptureLimits, sampleCount: Int, reason: MacCaptureTerminalReason) async throws {
        let fixture = Fixture(permission: .granted, limits: limits)
        try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        try fixture.engine.startCueCompleted()
        fixture.backend.emit(Array(repeating: 0.4, count: sampleCount))
        fixture.waitUntilIdle()
        fixture.flushEvents()
        let reasons = fixture.events.compactMap { event -> MacCaptureTerminalReason? in
            if case .finished(_, let value) = event { return value }
            return nil
        }
        #expect(reasons == [reason])
        #expect(fixture.ducker.values == [true, false])
    }

    @Test(
        "Every unsafe terminal path closes capture and removes the partial",
        arguments: [
            MacCaptureTerminalReason.userCancelled,
            .deviceLost,
            .permissionRevoked,
            .systemSleep,
            .sessionLocked,
            .appQuit,
            .backendFailure,
        ])
    func unsafeTerminalPaths(reason: MacCaptureTerminalReason) async throws {
        let fixture = Fixture(permission: .granted)
        try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        try fixture.engine.startCueCompleted()
        fixture.backend.emit([0.2, 0.3, 0.4])
        switch reason {
        case .userCancelled: fixture.engine.cancel()
        case .deviceLost: fixture.engine.handleDeviceLoss()
        case .permissionRevoked:
            fixture.permission.set(.denied)
            fixture.engine.recheckPermission()
        case .systemSleep: fixture.engine.handleSystemSleep()
        case .sessionLocked: fixture.engine.handleSessionLock()
        case .appQuit: fixture.engine.shutdown()
        case .backendFailure: fixture.engine.handleBackendFailure()
        default: Issue.record("unexpected fixture reason")
        }
        fixture.waitUntilIdle()
        fixture.flushEvents()
        #expect(fixture.events.contains(.cancelled(reason)))
        #expect(
            fixture.events.allSatisfy {
                if case .finished = $0 { return false }
                return true
            })
        let recovery = try fixture.store.recover()
        #expect(recovery.retainedDrafts.isEmpty)
        #expect(recovery.deletedPartialCount == 0, "terminal path removed the partial before restart")
        #expect(fixture.backend.stopCount == 1)
        #expect(fixture.ducker.values == [true, false])
    }

    @Test("Missing and stale selected devices fail before capture")
    func deviceValidation() async throws {
        let none = Fixture(permission: .granted, devices: [])
        await #expect(throws: MacCaptureEngineError.noInputDevice) {
            try await none.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        }
        let stale = Fixture(permission: .granted)
        await #expect(throws: MacCaptureEngineError.selectedDeviceUnavailable) {
            try await stale.engine.begin(selectedDeviceID: "removed", explicitUserAction: true)
        }
        #expect(none.backend.startCount == 0)
        #expect(stale.backend.startCount == 0)
    }

    @Test("Runtime input failure is actionable and never retains a partial draft")
    func runtimeInputFailureIsTyped() async throws {
        let cases: [(String?, MacCaptureEngineError)] = [
            (nil, .backendUnavailable),
            ("selected", .selectedDeviceUnavailable),
        ]
        for (selection, expected) in cases {
            let fixture = Fixture(permission: .granted)
            try await fixture.engine.begin(
                selectedDeviceID: selection,
                explicitUserAction: true)
            try fixture.engine.startCueCompleted()
            fixture.backend.emit([0.2, 0.3, 0.4])
            fixture.backend.fail()
            fixture.waitUntilIdle()
            fixture.flushEvents()

            #expect(fixture.events.contains(.failed(expected)))
            #expect(fixture.events.contains(.cancelled(.deviceLost)))
            #expect(
                fixture.events.allSatisfy {
                    if case .finished = $0 { return false }
                    return true
                })
            #expect(try fixture.store.recover().retainedDrafts.isEmpty)
        }
    }

    @Test("Input-only !dev startup failure removes the partial and preserves typed diagnostics")
    func inputOnlyDeviceFailureHasNoPartialDraft() async throws {
        let fixture = Fixture(permission: .granted)
        let diagnostic = MacCaptureStartupDiagnostic(
            stage: .inputOnly,
            attempt: 1,
            elapsedMilliseconds: 0,
            cause: .coreAudio(status: 560_227_702))
        fixture.backend.startError = .backendStartupFailed(diagnostic)

        await #expect(throws: MacCaptureEngineError.backendStartupFailed(diagnostic)) {
            try await fixture.engine.begin(
                selectedDeviceID: nil,
                explicitUserAction: true)
        }
        fixture.flushEvents()

        #expect(fixture.events.contains(.failed(.backendStartupFailed(diagnostic))))
        #expect(
            fixture.events.allSatisfy {
                if case .finished = $0 { return false }
                return true
            })
        let recovery = try fixture.store.recover()
        #expect(recovery.retainedDrafts.isEmpty)
        #expect(recovery.deletedPartialCount == 0)
        #expect(fixture.engine.currentPhase() == .idle)
    }

    @Test("Meter is local and bounded")
    func meterIsBounded() async throws {
        let fixture = Fixture(permission: .granted)
        try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        try fixture.engine.startCueCompleted()
        fixture.backend.emit([2, -2, 2, -2])
        try fixture.engine.stop()
        fixture.flushEvents()
        let meters = fixture.events.compactMap { event -> Float? in
            if case .meter(let value) = event { return value }
            return nil
        }
        #expect(meters.count == 1)
        #expect(meters.allSatisfy { $0 >= 0 && $0 <= 1 })
    }

    @Test("quality request and exact workflow reach the shared backend before capture")
    func qualityConfigurationAndState() async throws {
        let fixture = Fixture(permission: .granted)
        let request = MacCaptureQualityRequest(
            mode: .headphone, processingRequested: true, degradedConsent: false)
        fixture.engine.setCaptureQualityRequest(request)
        fixture.engine.selectCaptureQualityWorkflow("local_self_test")
        try await fixture.engine.begin(selectedDeviceID: nil, explicitUserAction: true)
        #expect(fixture.backend.qualityWorkflow == "local_self_test")
        #expect(fixture.backend.qualityRequest == request)

        let state = MacCaptureQualitySession(
            generation: 7, workflow: "local_self_test", request: request,
            resolvedMode: "headphone", quality: "accepted", aec: "active",
            ns: "active", agc: "active", reason: "none"
        ).state(lifecycle: "capturing", nowMs: 1)
        fixture.backend.emitQuality(state)
        fixture.flushEvents()
        #expect(fixture.events.contains(.quality(state)))
        fixture.engine.cancel()
        fixture.waitUntilIdle()
    }
}

private final class Fixture: @unchecked Sendable {
    let permission: FakePermission
    let backend: FakeCaptureBackend
    let ducker = FakeDucker()
    let store: CaptureMediaStore
    let engine: MacMicrophoneCaptureEngine
    private let eventQueue = DispatchQueue(label: "capture-test-events")
    private let eventLock = NSLock()
    private var storedEvents: [MacCaptureEvent] = []
    var events: [MacCaptureEvent] { eventLock.withLock { storedEvents } }

    init(
        permission: MacMicrophonePermission,
        requestedPermission: MacMicrophonePermission? = nil,
        devices: [MacCaptureDevice] = [
            MacCaptureDevice(id: "default", name: "Default mic", isDefault: true),
            MacCaptureDevice(id: "selected", name: "Selected mic", isDefault: false),
        ],
        limits: MacCaptureLimits = MacCaptureLimits(),
        deferPermissionRequest: Bool = false
    ) {
        self.permission = FakePermission(
            value: permission,
            requested: requestedPermission ?? permission,
            deferred: deferPermissionRequest)
        backend = FakeCaptureBackend(devices: devices)
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("mac-capture-tests-\(UUID().uuidString)", isDirectory: true)
        store = CaptureMediaStore(
            root: root,
            idProvider: { "00000000000000000000000000000001" })
        engine = MacMicrophoneCaptureEngine(
            permission: self.permission,
            backend: backend,
            mediaStore: store,
            ducker: ducker,
            limits: limits,
            eventQueue: eventQueue,
            observeLifecycle: false)
        engine.onEvent = { [weak self] event in
            self?.eventLock.withLock { self?.storedEvents.append(event) }
        }
    }

    func flushEvents() { eventQueue.sync {} }

    func waitUntilIdle() {
        for _ in 0..<1_000 {
            if engine.currentPhase() == .idle { return }
            Thread.sleep(forTimeInterval: 0.001)
        }
        Issue.record("capture engine did not become idle")
    }
}

private final class FakePermission: MacMicrophonePermissionAuthorizing, @unchecked Sendable {
    private let lock = NSLock()
    private var value: MacMicrophonePermission
    let requested: MacMicrophonePermission
    private let deferred: Bool
    private var storedRequestCount = 0
    private var continuation: CheckedContinuation<MacMicrophonePermission, Never>?
    var requestCount: Int { lock.withLock { storedRequestCount } }

    init(
        value: MacMicrophonePermission,
        requested: MacMicrophonePermission,
        deferred: Bool
    ) {
        self.value = value
        self.requested = requested
        self.deferred = deferred
    }

    func currentPermission() -> MacMicrophonePermission { lock.withLock { value } }

    func requestPermission() async -> MacMicrophonePermission {
        if !deferred {
            return lock.withLock {
                storedRequestCount += 1
                value = requested
                return requested
            }
        }
        return await withCheckedContinuation { continuation in
            lock.withLock {
                storedRequestCount += 1
                self.continuation = continuation
            }
        }
    }

    func set(_ value: MacMicrophonePermission) {
        lock.withLock { self.value = value }
    }

    func waitUntilRequested() {
        for _ in 0..<1_000 {
            if requestCount == 1 { return }
            Thread.sleep(forTimeInterval: 0.001)
        }
        Issue.record("permission request did not start")
    }

    func resolveRequest() {
        let pending = lock.withLock { () -> CheckedContinuation<MacMicrophonePermission, Never>? in
            value = requested
            defer { continuation = nil }
            return continuation
        }
        pending?.resume(returning: requested)
    }
}

private final class FakeCaptureBackend:
    MacMicrophoneCaptureBackend, MacCaptureQualityBackendConfiguring, @unchecked Sendable
{
    let devices: [MacCaptureDevice]
    private let lock = NSLock()
    private var samples: (@Sendable ([Float]) -> Void)?
    private var failure: (@Sendable () -> Void)?
    private var qualityState: (@Sendable (CaptureQualityState?) -> Void)?
    private var storedStartError: MacCaptureEngineError?
    private(set) var startCount = 0
    private(set) var stopCount = 0
    private(set) var selectedDeviceID: String?
    private(set) var qualityWorkflow: String?
    private(set) var qualityRequest: MacCaptureQualityRequest?
    var startError: MacCaptureEngineError? {
        get { lock.withLock { storedStartError } }
        set { lock.withLock { storedStartError = newValue } }
    }

    init(devices: [MacCaptureDevice]) { self.devices = devices }
    func availableDevices() -> [MacCaptureDevice] { devices }

    func configureCaptureQuality(
        workflow: String,
        request: MacCaptureQualityRequest,
        onState: @escaping @Sendable (CaptureQualityState?) -> Void
    ) {
        lock.withLock {
            qualityWorkflow = workflow
            qualityRequest = request
            qualityState = onState
        }
    }

    func start(
        selectedDeviceID: String?,
        onSamples: @escaping @Sendable ([Float]) -> Void,
        onFailure: @escaping @Sendable () -> Void
    ) throws {
        let error = lock.withLock { () -> MacCaptureEngineError? in
            startCount += 1
            self.selectedDeviceID = selectedDeviceID
            guard storedStartError == nil else { return storedStartError }
            samples = onSamples
            failure = onFailure
            return nil
        }
        if let error { throw error }
    }

    func stop() {
        lock.withLock {
            stopCount += 1
            samples = nil
            failure = nil
        }
    }

    func emit(_ values: [Float]) { lock.withLock { samples }?(values) }
    func emitQuality(_ state: CaptureQualityState?) { lock.withLock { qualityState }?(state) }
    func fail() { lock.withLock { failure }?() }
}

private final class FakeDucker: MacCaptureProgramDucking, @unchecked Sendable {
    private let lock = NSLock()
    private var stored: [Bool] = []
    var values: [Bool] { lock.withLock { stored } }
    func setCaptureDucking(active: Bool) { lock.withLock { stored.append(active) } }
}
