import CryptoKit
import Darwin
import Foundation

public enum CaptureMediaClass: String, Codable, CaseIterable, Sendable {
    case selfTest = "self_test"
    case userRecording = "user_recording"
}

public enum CaptureMediaState: String, Codable, CaseIterable, Sendable {
    case absent
    case capturingPartial = "capturing_partial"
    case finalizing
    case selfTestLocal = "self_test_local"
    case durableUnsent = "durable_unsent"
    case uploading
    case uploadedConfirmed = "uploaded_confirmed"
    case deleted
}

public enum CaptureMediaAction: String, Codable, CaseIterable, Sendable {
    case begin
    case stop
    case finalizeSucceeded = "finalize_succeeded"
    case finalizeFailed = "finalize_failed"
    case cancel
    case close
    case explicitDelete = "explicit_delete"
    case beginUpload = "begin_upload"
    case uploadFailedOrInterrupted = "upload_failed_or_interrupted"
    case uploadConfirmed = "upload_confirmed"
    case cleanup
}

public enum CaptureMediaLifecycle {
    public static func transition(
        storageClass: CaptureMediaClass,
        from state: CaptureMediaState,
        action: CaptureMediaAction
    ) -> CaptureMediaState? {
        switch (storageClass, state, action) {
        case (_, .absent, .begin): .capturingPartial
        case (_, .capturingPartial, .stop): .finalizing
        case (.selfTest, .finalizing, .finalizeSucceeded): .selfTestLocal
        case (.userRecording, .finalizing, .finalizeSucceeded): .durableUnsent
        case (_, .capturingPartial, .cancel), (_, .finalizing, .finalizeFailed): .deleted
        case (.selfTest, .selfTestLocal, .close),
             (.selfTest, .selfTestLocal, .explicitDelete): .deleted
        case (.userRecording, .durableUnsent, .beginUpload): .uploading
        case (.userRecording, .uploading, .uploadFailedOrInterrupted): .durableUnsent
        case (.userRecording, .uploading, .uploadConfirmed): .uploadedConfirmed
        case (.userRecording, .uploadedConfirmed, .cleanup),
             (.userRecording, .durableUnsent, .explicitDelete): .deleted
        default: nil
        }
    }
}

public enum RecordingCuePhase: String, Equatable, Sendable {
    case idle
    case playingStartCue = "playing_start_cue"
    case capturing
    case closingCapture = "closing_capture"
    case playingStopCue = "playing_stop_cue"
    case complete
    case cancelled
}

public enum RecordingCueCommand: Equatable, Sendable {
    case playStartCue
    case enableMicrophoneCommit
    case disableMicrophoneCommitAndCloseCapture
    case playStopCue
    case complete
}

public struct RecordingCueSequencer: Equatable, Sendable {
    public private(set) var phase: RecordingCuePhase = .idle
    public var mayCommitMicrophoneSamples: Bool { phase == .capturing }

    public init() {}

    public mutating func begin() -> RecordingCueCommand? {
        guard phase == .idle else { return nil }
        phase = .playingStartCue
        return .playStartCue
    }

    public mutating func startCueCompleted() -> RecordingCueCommand? {
        guard phase == .playingStartCue else { return nil }
        phase = .capturing
        return .enableMicrophoneCommit
    }

    public mutating func stopRequested() -> RecordingCueCommand? {
        guard phase == .capturing else { return nil }
        phase = .closingCapture
        return .disableMicrophoneCommitAndCloseCapture
    }

    public mutating func captureClosed() -> RecordingCueCommand? {
        guard phase == .closingCapture else { return nil }
        phase = .playingStopCue
        return .playStopCue
    }

    public mutating func stopCueCompleted() -> RecordingCueCommand? {
        guard phase == .playingStopCue else { return nil }
        phase = .complete
        return .complete
    }

    public mutating func cancel() {
        phase = .cancelled
    }
}

public enum BuiltinRecordingCue {
    public static let assetID = "pulsar.recording-cue.v1"
    public static let filename = "pulsar-recording-cue.wav"
    public static let sha256 = "479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
    public static let byteCount = 15_404
    public static let sampleRate = 48_000
    public static let channels = 1
    public static let bitsPerSample = 16
    public static let frames = 7_680

    public static func load(from bundle: Bundle = .main) throws -> Data {
        guard let url = bundle.url(
            forResource: "pulsar-recording-cue",
            withExtension: "wav",
            subdirectory: "Audio"
        ) else { throw CaptureMediaStoreError.cueUnavailable }
        return try load(from: url)
    }

    public static func load(from url: URL) throws -> Data {
        guard let data = try? Data(contentsOf: url, options: .mappedIfSafe),
              validate(data) else { throw CaptureMediaStoreError.cueUnavailable }
        return data
    }

    public static func validate(_ data: Data) -> Bool {
        guard data.count == byteCount,
              data.prefix(4) == Data("RIFF".utf8),
              data[8..<12] == Data("WAVE".utf8),
              littleEndianUInt16(data, at: 20) == 1,
              littleEndianUInt16(data, at: 22) == channels,
              littleEndianUInt32(data, at: 24) == sampleRate,
              littleEndianUInt16(data, at: 34) == bitsPerSample,
              data[36..<40] == Data("data".utf8),
              littleEndianUInt32(data, at: 40) == data.count - 44 else { return false }
        return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined() == sha256
    }

    private static func littleEndianUInt16(_ data: Data, at offset: Int) -> Int {
        Int(data[offset]) | Int(data[offset + 1]) << 8
    }

    private static func littleEndianUInt32(_ data: Data, at offset: Int) -> Int {
        Int(data[offset]) | Int(data[offset + 1]) << 8 |
            Int(data[offset + 2]) << 16 | Int(data[offset + 3]) << 24
    }
}

public struct CaptureMediaHandle: Equatable, Sendable {
    public let id: String
    public let storageClass: CaptureMediaClass
    public let state: CaptureMediaState
    public let fileURL: URL
}

public enum CaptureMediaStoreError: Error, Equatable, Sendable {
    case cueUnavailable
    case invalidIdentifier
    case invalidState
    case invalidWAV
    case storage
}

public struct CaptureMediaRecovery: Equatable, Sendable {
    public let retainedDrafts: [CaptureMediaHandle]
    public let deletedPartialCount: Int
    public let deletedSelfTestCount: Int
    public let deletedInvalidDraftCount: Int
}

/// App-private filesystem contract shared by future capture and intake tasks.
/// It never logs paths and maps all filesystem failures to content-free codes.
public final class CaptureMediaStore: @unchecked Sendable {
    private let root: URL
    private let fileManager: FileManager
    private let idProvider: @Sendable () -> String
    private let queue = DispatchQueue(label: "works.relux.pulsar.capture-media-store")

    public init(
        root: URL,
        fileManager: FileManager = .default,
        idProvider: @escaping @Sendable () -> String = {
            UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased()
        }
    ) {
        // Canonicalize the nearest existing prefix (notably macOS /var ->
        // /private/var) even when the app-specific leaf does not exist yet.
        // Handles returned before and after recovery then compare identically,
        // and ownership checks cannot be bypassed by path aliases.
        self.root = Self.canonicalRoot(root, fileManager: fileManager)
        self.fileManager = fileManager
        self.idProvider = idProvider
    }

    public func begin(_ storageClass: CaptureMediaClass) throws -> CaptureMediaHandle {
        try queue.sync {
            try prepareDirectories()
            let id = idProvider()
            guard Self.isIdentifier(id) else { throw CaptureMediaStoreError.invalidIdentifier }
            let url = partialDirectory.appendingPathComponent(id + ".partial.wav")
            guard fileManager.createFile(atPath: url.path, contents: nil,
                                         attributes: [.posixPermissions: 0o600]) else {
                throw CaptureMediaStoreError.storage
            }
            return CaptureMediaHandle(id: id, storageClass: storageClass,
                                      state: .capturingPartial, fileURL: url)
        }
    }

    public func stop(_ handle: CaptureMediaHandle) throws -> CaptureMediaHandle {
        guard handle.state == .capturingPartial, Self.isIdentifier(handle.id),
              handle.fileURL == expectedURL(for: handle) else {
            throw CaptureMediaStoreError.invalidState
        }
        return CaptureMediaHandle(id: handle.id, storageClass: handle.storageClass,
                                  state: .finalizing, fileURL: handle.fileURL)
    }

    /// Copies picker-approved bytes into app-private storage and closes any
    /// security-scoped access before finalization makes the draft sendable.
    public func importUserDraft(from sourceURL: URL, useSecurityScopedAccess: Bool = true) throws
        -> CaptureMediaHandle {
        let accessGranted = !useSecurityScopedAccess || sourceURL.startAccessingSecurityScopedResource()
        guard accessGranted else { throw CaptureMediaStoreError.storage }
        defer {
            if useSecurityScopedAccess { sourceURL.stopAccessingSecurityScopedResource() }
        }
        let data: Data
        do {
            data = try Data(contentsOf: sourceURL, options: .mappedIfSafe)
        } catch {
            throw CaptureMediaStoreError.storage
        }
        return try importUserDraft(bytes: data)
    }

    public func importUserDraft(bytes: Data) throws -> CaptureMediaHandle {
        let partial = try begin(.userRecording)
        do {
            let file = try FileHandle(forWritingTo: partial.fileURL)
            try file.write(contentsOf: bytes)
            try file.close()
            return try finalize(stop(partial))
        } catch let error as CaptureMediaStoreError {
            try? cancel(partial)
            throw error
        } catch {
            try? cancel(partial)
            throw CaptureMediaStoreError.storage
        }
    }

    public func finalize(_ handle: CaptureMediaHandle) throws -> CaptureMediaHandle {
        try queue.sync {
            guard handle.state == .finalizing, Self.isIdentifier(handle.id),
                  handle.fileURL == partialDirectory.appendingPathComponent(handle.id + ".partial.wav") else {
                throw CaptureMediaStoreError.invalidState
            }
            var movedDestination: URL?
            do {
                let data = try Data(contentsOf: handle.fileURL, options: .mappedIfSafe)
                guard Self.isStructurallyCompleteWAV(data) else {
                    try? fileManager.removeItem(at: handle.fileURL)
                    throw CaptureMediaStoreError.invalidWAV
                }
                let file = try FileHandle(forWritingTo: handle.fileURL)
                try file.synchronize()
                try file.close()
                let state: CaptureMediaState
                let destination: URL
                switch handle.storageClass {
                case .selfTest:
                    state = .selfTestLocal
                    destination = selfTestDirectory.appendingPathComponent(handle.id + ".selftest.wav")
                case .userRecording:
                    state = .durableUnsent
                    destination = draftDirectory.appendingPathComponent(handle.id + ".draft.wav")
                }
                try fileManager.moveItem(at: handle.fileURL, to: destination)
                movedDestination = destination
                try setOwnerOnly(destination, isDirectory: false)
                try syncDirectory(destination.deletingLastPathComponent())
                return CaptureMediaHandle(id: handle.id, storageClass: handle.storageClass,
                                          state: state, fileURL: destination)
            } catch let error as CaptureMediaStoreError {
                if let movedDestination { try? fileManager.removeItem(at: movedDestination) }
                throw error
            } catch {
                if let movedDestination { try? fileManager.removeItem(at: movedDestination) }
                try? fileManager.removeItem(at: handle.fileURL)
                throw CaptureMediaStoreError.storage
            }
        }
    }

    public func cancel(_ handle: CaptureMediaHandle) throws {
        try deleteOwned(handle, allowed: [.capturingPartial, .finalizing])
    }

    public func closeSelfTest(_ handle: CaptureMediaHandle) throws {
        guard handle.storageClass == .selfTest else { throw CaptureMediaStoreError.invalidState }
        try deleteOwned(handle, allowed: [.selfTestLocal])
    }

    public func confirmUploadAndDelete(_ handle: CaptureMediaHandle) throws {
        guard handle.storageClass == .userRecording else { throw CaptureMediaStoreError.invalidState }
        try deleteOwned(handle, allowed: [.durableUnsent, .uploadedConfirmed])
    }

    public func explicitlyDelete(_ handle: CaptureMediaHandle) throws {
        try deleteOwned(handle, allowed: [.selfTestLocal, .durableUnsent])
    }

    public func recover() throws -> CaptureMediaRecovery {
        try queue.sync {
            try prepareDirectories()
            let partials = try removeAllFiles(in: partialDirectory)
            let selfTests = try removeAllFiles(in: selfTestDirectory)
            var retained: [CaptureMediaHandle] = []
            var invalid = 0
            for url in try fileManager.contentsOfDirectory(
                at: draftDirectory, includingPropertiesForKeys: nil,
                options: []) {
                let name = url.lastPathComponent
                guard name.hasSuffix(".draft.wav") else {
                    try removeAndVerify(url)
                    invalid += 1
                    continue
                }
                let id = String(name.dropLast(".draft.wav".count))
                guard Self.isIdentifier(id),
                      let data = try? Data(contentsOf: url, options: .mappedIfSafe),
                      Self.isStructurallyCompleteWAV(data) else {
                    try removeAndVerify(url)
                    invalid += 1
                    continue
                }
                retained.append(CaptureMediaHandle(
                    id: id, storageClass: .userRecording,
                    state: .durableUnsent, fileURL: url))
            }
            retained.sort { $0.id < $1.id }
            return CaptureMediaRecovery(
                retainedDrafts: retained,
                deletedPartialCount: partials,
                deletedSelfTestCount: selfTests,
                deletedInvalidDraftCount: invalid)
        }
    }

    private func deleteOwned(
        _ handle: CaptureMediaHandle,
        allowed: Set<CaptureMediaState>
    ) throws {
        try queue.sync {
            guard allowed.contains(handle.state), Self.isIdentifier(handle.id),
                  expectedURL(for: handle) == handle.fileURL else {
                throw CaptureMediaStoreError.invalidState
            }
            try removeAndVerify(handle.fileURL)
        }
    }

    private func expectedURL(for handle: CaptureMediaHandle) -> URL {
        switch handle.state {
        case .capturingPartial, .finalizing:
            partialDirectory.appendingPathComponent(handle.id + ".partial.wav")
        case .selfTestLocal:
            selfTestDirectory.appendingPathComponent(handle.id + ".selftest.wav")
        case .durableUnsent, .uploadedConfirmed, .uploading:
            draftDirectory.appendingPathComponent(handle.id + ".draft.wav")
        default:
            root.appendingPathComponent("invalid")
        }
    }

    private var partialDirectory: URL { root.appendingPathComponent("partials", isDirectory: true) }
    private var selfTestDirectory: URL { root.appendingPathComponent("self-tests", isDirectory: true) }
    private var draftDirectory: URL { root.appendingPathComponent("drafts", isDirectory: true) }

    private static func canonicalRoot(_ input: URL, fileManager: FileManager) -> URL {
        var ancestor = input.standardizedFileURL
        var missingComponents: [String] = []
        while !fileManager.fileExists(atPath: ancestor.path),
              ancestor.path != "/" {
            missingComponents.append(ancestor.lastPathComponent)
            ancestor.deleteLastPathComponent()
        }
        let resolvedAncestor: URL
        if let pointer = realpath(ancestor.path, nil) {
            defer { free(pointer) }
            resolvedAncestor = URL(
                fileURLWithPath: String(cString: pointer),
                isDirectory: true)
        } else {
            resolvedAncestor = ancestor
        }
        var result = resolvedAncestor
        for component in missingComponents.reversed() {
            result.appendPathComponent(component, isDirectory: true)
        }
        return URL(fileURLWithPath: result.standardizedFileURL.path, isDirectory: true)
    }

    private func prepareDirectories() throws {
        do {
            for directory in [root, partialDirectory, selfTestDirectory, draftDirectory] {
                try fileManager.createDirectory(
                    at: directory, withIntermediateDirectories: true,
                    attributes: [.posixPermissions: 0o700])
                try setOwnerOnly(directory, isDirectory: true)
            }
        } catch {
            throw CaptureMediaStoreError.storage
        }
    }

    private func setOwnerOnly(_ url: URL, isDirectory: Bool) throws {
        try fileManager.setAttributes(
            [.posixPermissions: NSNumber(value: isDirectory ? 0o700 : 0o600)],
            ofItemAtPath: url.path)
    }

    private func syncDirectory(_ url: URL) throws {
        let descriptor = open(url.path, O_RDONLY | O_DIRECTORY)
        guard descriptor >= 0 else { throw CaptureMediaStoreError.storage }
        defer { close(descriptor) }
        guard fsync(descriptor) == 0 else { throw CaptureMediaStoreError.storage }
    }

    private func removeAllFiles(in directory: URL) throws -> Int {
        var count = 0
        for url in try fileManager.contentsOfDirectory(
            at: directory, includingPropertiesForKeys: nil,
            options: []) {
            try removeAndVerify(url)
            count += 1
        }
        return count
    }

    private func removeAndVerify(_ url: URL) throws {
        do {
            if fileManager.fileExists(atPath: url.path) { try fileManager.removeItem(at: url) }
            guard !fileManager.fileExists(atPath: url.path) else {
                throw CaptureMediaStoreError.storage
            }
        } catch let error as CaptureMediaStoreError {
            throw error
        } catch {
            throw CaptureMediaStoreError.storage
        }
    }

    static func isIdentifier(_ value: String) -> Bool {
        value.utf8.count == 32 && value.utf8.allSatisfy {
            ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
        }
    }

    static func isStructurallyCompleteWAV(_ data: Data) -> Bool {
        guard data.count >= 44,
              data.prefix(4) == Data("RIFF".utf8),
              data[8..<12] == Data("WAVE".utf8),
              data[36..<40] == Data("data".utf8) else { return false }
        let riffBytes = Int(data[4]) | Int(data[5]) << 8 |
            Int(data[6]) << 16 | Int(data[7]) << 24
        let payloadBytes = Int(data[40]) | Int(data[41]) << 8 |
            Int(data[42]) << 16 | Int(data[43]) << 24
        return riffBytes == data.count - 8 && payloadBytes == data.count - 44
    }
}
