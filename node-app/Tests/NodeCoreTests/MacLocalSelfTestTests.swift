import Foundation
import Testing
@testable import NodeCore

@Suite("macOS local self-test and file intake")
struct MacLocalSelfTestTests {
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

    @Test("Supported local files expose a complete review and become private canonical drafts")
    func supportedFileReviewAndIntake() throws {
        let root = temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let source = root.appendingPathComponent("voice sample.wav")
        try Data(contentsOf: cueURL).write(to: source)
        let store = CaptureMediaStore(root: root.appendingPathComponent("store"))
        let inspector = MacShortAudioInspector()
        let intake = MacShortAudioIntake(inspector: inspector, store: store)

        let review = inspector.inspect(source)
        #expect(review.isEligible)
        #expect(review.filename == "voice sample.wav")
        #expect(review.format == .wav)
        #expect((review.durationMs ?? 0) > 0)
        #expect(review.sizeBytes == Int64(BuiltinRecordingCue.byteCount))
        #expect(review.audience == ["this_pulsar", "own_barycenter", "current_approach"])
        #expect(review.deliveryModes == ["overlay", "interrupt", "after_current"])
        #expect(!review.rightsReminder.isEmpty)
        #expect(review.serverValidationRequired)

        let (_, draft) = try intake.accept(source, useSecurityScopedAccess: false)
        #expect(draft.storageClass == .userRecording)
        #expect(draft.state == .durableUnsent)
        #expect(FileManager.default.fileExists(atPath: draft.fileURL.path))
        let bytes = try Data(contentsOf: draft.fileURL)
        #expect(bytes.prefix(4) == Data("RIFF".utf8))
        #expect(bytes[8..<12] == Data("WAVE".utf8))
        let permissions = try #require(
            FileManager.default.attributesOfItem(atPath: draft.fileURL.path)[.posixPermissions] as? NSNumber)
        #expect(permissions.intValue & 0o777 == 0o600)
    }

    @Test("Unsupported, oversized and over-duration files are never accepted")
    func rejectionIsFailClosed() throws {
        let root = temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let bytes = try Data(contentsOf: cueURL)
        let unsupported = root.appendingPathComponent("voice.txt")
        try Data("not an audio file".utf8).write(to: unsupported)
        let oversized = root.appendingPathComponent("voice.wav")
        try bytes.write(to: oversized)
        let store = CaptureMediaStore(root: root.appendingPathComponent("store"))
        let inspector = MacShortAudioInspector(limits: .init(
            maximumBytes: Int64(bytes.count - 1),
            maximumDurationMs: 1,
            maximumOverlayDurationMs: 1))
        let intake = MacShortAudioIntake(inspector: inspector, store: store)

        #expect(inspector.inspect(unsupported).rejection == .unsupportedFormat)
        #expect(inspector.inspect(oversized).rejection == .sizeLimit)
        #expect(throws: MacShortAudioIntakeError.rejected(.sizeLimit)) {
            try intake.accept(oversized, useSecurityScopedAccess: false)
        }

        let formatInspector = MacShortAudioInspector()
        #expect(formatInspector.inspect(unsupported).rejection == .unsupportedFormat)
        let durationInspector = MacShortAudioInspector(limits: .init(
            maximumBytes: Int64.max,
            maximumDurationMs: 1,
            maximumOverlayDurationMs: 1))
        #expect(durationInspector.inspect(oversized).rejection == .durationLimit)
        #expect(try store.recover().retainedDrafts.isEmpty)
    }

    @Test("Five-second self-test serializes both cues and recording on the production output seam")
    func exactLocalSequenceAndCloseCleanup() async throws {
        let root = temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        let store = CaptureMediaStore(root: root.appendingPathComponent("store"))
        let draft = try store.importUserDraft(bytes: Data(contentsOf: cueURL))
        let capture = FakeSelfTestCapture(finishedDraft: draft)
        let output = FakeLocalClipOutput()
        let intake = MacShortAudioIntake(inspector: .init(), store: store)
        let events = LockedSelfTestEvents()
        let service = try MacLocalSelfTestService(
            capture: capture,
            output: output,
            store: store,
            intake: intake,
            cueURL: cueURL,
            recordingDuration: 0.01,
            eventQueue: DispatchQueue(label: "self-test-events"))
        service.onEvent = { events.append($0) }

        #expect(MacLocalSelfTestService.exactRecordingSeconds == 5)
        try await service.recordFiveSeconds()
        try waitUntil { output.playedURLs.count == 1 }
        #expect(output.playedURLs == [cueURL])
        output.completeNext(.success(()))

        try waitUntil { capture.stopCount == 1 }
        try waitUntil { output.playedURLs.count == 2 }
        #expect(output.playedURLs[1] == cueURL)
        output.completeNext(.success(()))

        try waitUntil { output.playedURLs.count == 3 }
        #expect(output.playedURLs[2] == draft.fileURL)
        output.completeNext(.success(()))
        try waitUntil { events.values.contains(.phase(.reviewingDraft)) }

        service.close()
        #expect(!FileManager.default.fileExists(atPath: draft.fileURL.path))
        #expect(capture.cancelCount == 1)
        #expect(output.cancelCount == 1)
        #expect(events.values.contains(.phase(.recording)))
        #expect(events.values.contains(.phase(.playingStopCue)))
        #expect(events.values.contains(.phase(.playingRecording)))
    }

    @Test("Local self-test source has no coordinator, upload, telemetry or network client")
    func localBoundaryIsArchitectural() throws {
        let source = try String(contentsOf:
            repositoryRoot.appendingPathComponent("node-app/Sources/NodeCore/MacLocalSelfTest.swift"),
            encoding: .utf8)
        for forbidden in ["URLSession", "CoordinatorClient", ".beginUpload", "TelemetryClient"] {
            #expect(!source.contains(forbidden))
        }
        #expect(source.contains("collectTelemetry: false"))
    }

    private func temporaryRoot() -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent("mac-local-self-test-\(UUID().uuidString)", isDirectory: true)
    }
}

private final class FakeSelfTestCapture: MacLocalSelfTestCapturing, @unchecked Sendable {
    var onEvent: (@Sendable (MacCaptureEvent) -> Void)?
    private let lock = NSLock()
    private let finishedDraft: CaptureMediaHandle
    private var stops = 0
    private var cancels = 0

    init(finishedDraft: CaptureMediaHandle) { self.finishedDraft = finishedDraft }

    var stopCount: Int { lock.read { stops } }
    var cancelCount: Int { lock.read { cancels } }

    func begin(selectedDeviceID: String?, explicitUserAction: Bool) async throws {
        #expect(explicitUserAction)
        onEvent?(.phase(.requestingPermission))
        onEvent?(.playStartCue)
    }

    func startCueCompleted() throws {}

    func stop() throws {
        lock.write { stops += 1 }
        onEvent?(.playStopCue)
        onEvent?(.finished(finishedDraft, .userStopped))
    }

    func cancel() { lock.write { cancels += 1 } }
}

private final class FakeLocalClipOutput: MacLocalClipPlaying, @unchecked Sendable {
    private let lock = NSLock()
    private var urls: [URL] = []
    private var completions: [@Sendable (Result<Void, MacLocalClipOutputError>) -> Void] = []
    private var cancels = 0

    var playedURLs: [URL] { lock.read { urls } }
    var cancelCount: Int { lock.read { cancels } }

    func play(
        fileURL: URL,
        completion: @escaping @Sendable (Result<Void, MacLocalClipOutputError>) -> Void
    ) {
        lock.write {
            urls.append(fileURL)
            completions.append(completion)
        }
    }

    func completeNext(_ result: Result<Void, MacLocalClipOutputError>) {
        let completion = lock.readWrite { completions.removeFirst() }
        completion(result)
    }

    func cancel() { lock.write { cancels += 1 } }
}

private final class LockedSelfTestEvents: @unchecked Sendable {
    private let lock = NSLock()
    private var storage: [MacLocalSelfTestEvent] = []
    var values: [MacLocalSelfTestEvent] { lock.read { storage } }
    func append(_ event: MacLocalSelfTestEvent) { lock.write { storage.append(event) } }
}

private extension NSLock {
    func read<T>(_ body: () -> T) -> T {
        lock()
        defer { unlock() }
        return body()
    }

    func write(_ body: () -> Void) {
        lock()
        defer { unlock() }
        body()
    }

    func readWrite<T>(_ body: () -> T) -> T {
        lock()
        defer { unlock() }
        return body()
    }
}

private func waitUntil(
    timeout: TimeInterval = 1,
    _ condition: () -> Bool
) throws {
    let deadline = Date().addingTimeInterval(timeout)
    while !condition(), Date() < deadline {
        Thread.sleep(forTimeInterval: 0.002)
    }
    if !condition() { throw SelfTestWaitError.timeout }
}

private enum SelfTestWaitError: Error { case timeout }
