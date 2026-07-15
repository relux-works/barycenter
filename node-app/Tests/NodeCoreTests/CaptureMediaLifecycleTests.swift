import Foundation
import Testing
@testable import NodeCore

@Suite("Capture media lifecycle v1")
struct CaptureMediaLifecycleTests {
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

    private var contractURL: URL {
        repositoryRoot.appendingPathComponent("protocol/capture-media-lifecycle-v1.json")
    }

    @Test("Canonical cue bytes and metadata are exact")
    func cueBytesAreCanonical() throws {
        let data = try Data(contentsOf: cueURL)
        #expect(BuiltinRecordingCue.validate(data))
        #expect(try BuiltinRecordingCue.load(from: cueURL) == data)
        #expect(data.count == BuiltinRecordingCue.byteCount)

        var corrupt = data
        corrupt[100] ^= 0xff
        #expect(!BuiltinRecordingCue.validate(corrupt))

        let metadata = try JSONSerialization.jsonObject(with: Data(contentsOf:
            repositoryRoot.appendingPathComponent("assets/audio/pulsar-recording-cue.json"))) as? [String: Any]
        #expect(metadata?["sha256"] as? String == BuiltinRecordingCue.sha256)
        #expect(metadata?["bytes"] as? Int == BuiltinRecordingCue.byteCount)

        let missing = temporaryRoot().appendingPathComponent("missing.wav")
        #expect(throws: CaptureMediaStoreError.cueUnavailable) {
            try BuiltinRecordingCue.load(from: missing)
        }
    }

    @Test("Swift transition table implements every shared JSON transition")
    func transitionTableMatchesSharedContract() throws {
        struct Contract: Decodable { let transitions: [Transition] }
        struct Transition: Decodable {
            let `class`: String
            let from: String
            let action: String
            let to: String
        }
        let contract = try JSONDecoder().decode(Contract.self, from: Data(contentsOf: contractURL))
        #expect(contract.transitions.count == 17)
        for transition in contract.transitions {
            let storageClass = try #require(CaptureMediaClass(rawValue: transition.class))
            let from = try #require(CaptureMediaState(rawValue: transition.from))
            let action = try #require(CaptureMediaAction(rawValue: transition.action))
            let expected = try #require(CaptureMediaState(rawValue: transition.to))
            #expect(CaptureMediaLifecycle.transition(
                storageClass: storageClass, from: from, action: action) == expected)
        }
        #expect(CaptureMediaLifecycle.transition(
            storageClass: .selfTest, from: .selfTestLocal, action: .beginUpload) == nil)
        #expect(CaptureMediaLifecycle.transition(
            storageClass: .userRecording, from: .capturingPartial,
            action: .uploadConfirmed) == nil)
    }

    @Test("Self-test is disposable and never enters the upload state machine")
    func selfTestDeletesOnCloseAndRecovery() throws {
        let root = temporaryRoot()
        let ids = IDSequence()
        let store = CaptureMediaStore(root: root, idProvider: { ids.next() })
        let cue = try Data(contentsOf: cueURL)

        let active = try store.begin(.selfTest)
        #expect(active.fileURL.lastPathComponent == ids.first + ".partial.wav")
        try cue.write(to: active.fileURL)
        let local = try store.finalize(store.stop(active))
        #expect(local.state == .selfTestLocal)
        #expect(FileManager.default.fileExists(atPath: local.fileURL.path))
        #expect(CaptureMediaLifecycle.transition(
            storageClass: .selfTest, from: local.state, action: .beginUpload) == nil)
        try store.closeSelfTest(local)
        #expect(!FileManager.default.fileExists(atPath: local.fileURL.path))

        let abandoned = try store.begin(.selfTest)
        try cue.write(to: abandoned.fileURL)
        let abandonedLocal = try store.finalize(store.stop(abandoned))
        let recovered = try store.recover()
        #expect(recovered.deletedSelfTestCount == 1)
        #expect(recovered.retainedDrafts.isEmpty)
        #expect(!FileManager.default.fileExists(atPath: abandonedLocal.fileURL.path))
    }

    @Test("Finalized user draft survives restart until confirmation or delete")
    func durableDraftSurvivesRecovery() throws {
        let root = temporaryRoot()
        let ids = IDSequence()
        let cue = try Data(contentsOf: cueURL)
        let firstStore = CaptureMediaStore(root: root, idProvider: { ids.next() })
        let active = try firstStore.begin(.userRecording)
        try cue.write(to: active.fileURL)
        let draft = try firstStore.finalize(firstStore.stop(active))
        #expect(draft.state == .durableUnsent)
        #expect(draft.fileURL.lastPathComponent == ids.first + ".draft.wav")

        let restarted = CaptureMediaStore(root: root, idProvider: { ids.next() })
        let recovery = try restarted.recover()
        #expect(recovery.retainedDrafts == [draft])
        #expect(FileManager.default.fileExists(atPath: draft.fileURL.path))

        #expect(CaptureMediaLifecycle.transition(
            storageClass: .userRecording, from: .durableUnsent,
            action: .beginUpload) == .uploading)
        #expect(CaptureMediaLifecycle.transition(
            storageClass: .userRecording, from: .uploading,
            action: .uploadFailedOrInterrupted) == .durableUnsent)
        try restarted.confirmUploadAndDelete(draft)
        #expect(!FileManager.default.fileExists(atPath: draft.fileURL.path))
    }

    @Test("Picker intake copies bytes without retaining a source name or access")
    func pickerIntakeCopiesIntoOpaquePrivateDraft() throws {
        let root = temporaryRoot()
        let sourceDirectory = temporaryRoot()
        try FileManager.default.createDirectory(at: sourceDirectory, withIntermediateDirectories: true)
        let source = sourceDirectory.appendingPathComponent("family-voice-message.wav")
        let cue = try Data(contentsOf: cueURL)
        try cue.write(to: source)
        let ids = IDSequence()
        let store = CaptureMediaStore(root: root, idProvider: { ids.next() })

        let draft = try store.importUserDraft(from: source, useSecurityScopedAccess: false)
        #expect(draft.state == .durableUnsent)
        #expect(!draft.fileURL.path.contains("family-voice-message"))
        try FileManager.default.removeItem(at: source)
        #expect(try Data(contentsOf: draft.fileURL) == cue)
        #expect(try store.recover().retainedDrafts == [draft])
    }

    @Test("Cancel, invalid finalization and startup recovery remove partial media")
    func partialsNeverBecomeSendable() throws {
        let root = temporaryRoot()
        let ids = IDSequence()
        let store = CaptureMediaStore(root: root, idProvider: { ids.next() })

        let cancelled = try store.begin(.userRecording)
        try Data("partial microphone bytes".utf8).write(to: cancelled.fileURL)
        try store.cancel(cancelled)
        #expect(!FileManager.default.fileExists(atPath: cancelled.fileURL.path))

        let invalid = try store.begin(.userRecording)
        try Data("not a complete wav".utf8).write(to: invalid.fileURL)
        #expect(throws: CaptureMediaStoreError.invalidWAV) {
            try store.finalize(store.stop(invalid))
        }
        #expect(!FileManager.default.fileExists(atPath: invalid.fileURL.path))

        let crashed = try store.begin(.userRecording)
        try Data("RIFF incomplete".utf8).write(to: crashed.fileURL)
        let recovery = try CaptureMediaStore(root: root).recover()
        #expect(recovery.deletedPartialCount == 1)
        #expect(recovery.retainedDrafts.isEmpty)
    }

    @Test("Recovery rejects invalid draft names and truncated durable files")
    func recoveryRejectsInvalidDrafts() throws {
        let root = temporaryRoot()
        let draftDirectory = root.appendingPathComponent("drafts", isDirectory: true)
        try FileManager.default.createDirectory(at: draftDirectory, withIntermediateDirectories: true)
        try Data("RIFF truncated".utf8).write(
            to: draftDirectory.appendingPathComponent(String(repeating: "a", count: 32) + ".draft.wav"))
        try Data("source filename canary".utf8).write(
            to: draftDirectory.appendingPathComponent("family-voice-message.wav"))

        let recovery = try CaptureMediaStore(root: root).recover()
        #expect(recovery.deletedInvalidDraftCount == 2)
        #expect(recovery.retainedDrafts.isEmpty)
        #expect((try FileManager.default.contentsOfDirectory(atPath: draftDirectory.path)).isEmpty)
    }

    @Test("Storage errors are fixed codes and never expose private paths")
    func storageErrorsDoNotExposePaths() throws {
        let root = temporaryRoot().appendingPathComponent("private-family-recording-canary")
        try FileManager.default.createDirectory(
            at: root.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("not a directory".utf8).write(to: root)
        #expect(throws: CaptureMediaStoreError.storage) {
            try CaptureMediaStore(root: root).begin(.userRecording)
        }
    }

    @Test("Cue sequencing excludes both cues from committed microphone samples")
    func cuesStayOutsideCapture() {
        var sequencer = RecordingCueSequencer()
        #expect(!sequencer.mayCommitMicrophoneSamples)
        #expect(sequencer.begin() == .playStartCue)
        #expect(!sequencer.mayCommitMicrophoneSamples)
        #expect(sequencer.stopRequested() == nil)
        #expect(sequencer.startCueCompleted() == .enableMicrophoneCommit)
        #expect(sequencer.mayCommitMicrophoneSamples)
        #expect(sequencer.stopRequested() == .disableMicrophoneCommitAndCloseCapture)
        #expect(!sequencer.mayCommitMicrophoneSamples)
        #expect(sequencer.captureClosed() == .playStopCue)
        #expect(!sequencer.mayCommitMicrophoneSamples)
        #expect(sequencer.stopCueCompleted() == .complete)
        #expect(sequencer.phase == .complete)
    }

    private func temporaryRoot() -> URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent("capture-media-tests-\(UUID().uuidString)", isDirectory: true)
    }
}

private final class IDSequence: @unchecked Sendable {
    let first = "00000000000000000000000000000001"
    private var value = 0
    private let lock = NSLock()

    func next() -> String {
        lock.lock()
        defer { lock.unlock() }
        value += 1
        return String(format: "%032x", value)
    }
}
