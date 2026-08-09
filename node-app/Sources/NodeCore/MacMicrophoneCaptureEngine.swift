import AVFAudio
import AVFoundation
import AppKit
import AudioToolbox
import CoreAudio
import Foundation

public struct MacCaptureDevice: Equatable, Sendable {
    public let id: String
    public let name: String
    public let isDefault: Bool

    public init(id: String, name: String, isDefault: Bool) {
        self.id = id
        self.name = name
        self.isDefault = isDefault
    }
}

public enum MacMicrophonePermission: String, Equatable, Sendable {
    case notDetermined = "not_determined"
    case granted
    case denied
    case restricted
}

public enum MacCaptureTerminalReason: String, Equatable, Sendable {
    case userStopped = "user_stopped"
    case durationLimit = "duration_limit"
    case byteLimit = "byte_limit"
    case userCancelled = "user_cancelled"
    case deviceLost = "device_lost"
    case permissionRevoked = "permission_revoked"
    case systemSleep = "system_sleep"
    case sessionLocked = "session_locked"
    case appQuit = "app_quit"
    case backendFailure = "backend_failure"
}

public enum MacCapturePhase: String, Equatable, Sendable {
    case idle
    case requestingPermission = "requesting_permission"
    case playingStartCue = "playing_start_cue"
    case recording
    case finalizing
}

public enum MacCaptureEngineError: Error, Equatable, Sendable {
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
}

public enum MacCaptureEvent: Equatable, Sendable {
    case phase(MacCapturePhase)
    case devices([MacCaptureDevice])
    case meter(Float)
    case quality(CaptureQualityState?)
    case playStartCue
    case playStopCue
    case finished(CaptureMediaHandle, MacCaptureTerminalReason)
    case cancelled(MacCaptureTerminalReason)
    case failed(MacCaptureEngineError)
}

public struct MacCaptureLimits: Equatable, Sendable {
    public let maxFrames: Int64
    public let maxPCMBytes: Int64

    public init(maxDurationSeconds: Int = 180, maxBytes: Int64 = 50 * 1_024 * 1_024) {
        maxFrames = Int64(max(1, maxDurationSeconds)) * 48_000
        // The user-facing byte limit applies to the complete WAV, including
        // its fixed 44-byte header. Keep at least one PCM16 frame for smaller
        // test limits while holding the production draft to exactly 50 MiB.
        maxPCMBytes = max(2, maxBytes - 44)
    }
}

public protocol MacMicrophonePermissionAuthorizing: AnyObject {
    func currentPermission() -> MacMicrophonePermission
    func requestPermission() async -> MacMicrophonePermission
}

public protocol MacMicrophoneCaptureBackend: AnyObject {
    func availableDevices() -> [MacCaptureDevice]
    func start(
        selectedDeviceID: String?,
        onSamples: @escaping @Sendable ([Float]) -> Void,
        onFailure: @escaping @Sendable () -> Void
    ) throws
    func stop()
}

public protocol MacCaptureProgramDucking: AnyObject {
    func setCaptureDucking(active: Bool)
}

public final class AudioEngineCaptureDucker: MacCaptureProgramDucking {
    private let audio: AudioEngine
    private let duckGain = Float(pow(10, -12.0 / 20.0))

    public init(audio: AudioEngine) { self.audio = audio }

    public func setCaptureDucking(active: Bool) {
        audio.setMusicGain(active ? duckGain : 1, fadeMs: active ? 100 : 160)
    }
}

public final class SystemMicrophonePermissionAuthorizer: MacMicrophonePermissionAuthorizing {
    public init() {}

    public func currentPermission() -> MacMicrophonePermission {
        Self.map(AVCaptureDevice.authorizationStatus(for: .audio))
    }

    public func requestPermission() async -> MacMicrophonePermission {
        guard currentPermission() == .notDetermined else { return currentPermission() }
        let granted = await AVCaptureDevice.requestAccess(for: .audio)
        return granted ? .granted : currentPermission()
    }

    private static func map(_ status: AVAuthorizationStatus) -> MacMicrophonePermission {
        switch status {
        case .notDetermined: .notDetermined
        case .authorized: .granted
        case .denied: .denied
        case .restricted: .restricted
        @unknown default: .restricted
        }
    }
}

/// UI-independent capture owner. Its serial queue is the only lifecycle
/// writer; backend callbacks never touch disk or state directly.
public final class MacMicrophoneCaptureEngine: MacCaptureQualityWorkflowSelecting, @unchecked Sendable {
    public static let sampleRate = 48_000

    private let permission: MacMicrophonePermissionAuthorizing
    private let backend: MacMicrophoneCaptureBackend
    private let mediaStore: CaptureMediaStore
    private let ducker: MacCaptureProgramDucking
    private let limits: MacCaptureLimits
    private let queue = DispatchQueue(label: "works.relux.pulsar.mac-capture")
    private let eventQueue: DispatchQueue
    private var qualityRequest: MacCaptureQualityRequest
    private var qualityWorkflow = "recorded_clip"

    private var phase: MacCapturePhase = .idle
    private var writer: MacCaptureWAVWriter?
    private var partial: CaptureMediaHandle?
    private var commitSamples = false
    private var terminalClaimed = false
    private var generation: UInt64 = 0
    private var runtimeActive = false
    private var permissionTimer: DispatchSourceTimer?
    private var observers: [NSObjectProtocol] = []

    public var onEvent: (@Sendable (MacCaptureEvent) -> Void)?

    public init(
        permission: MacMicrophonePermissionAuthorizing,
        backend: MacMicrophoneCaptureBackend,
        mediaStore: CaptureMediaStore,
        ducker: MacCaptureProgramDucking,
        limits: MacCaptureLimits = MacCaptureLimits(),
        qualityRequest: MacCaptureQualityRequest = .legacyUnprocessed,
        eventQueue: DispatchQueue = .main,
        observeLifecycle: Bool = true
    ) {
        self.permission = permission
        self.backend = backend
        self.mediaStore = mediaStore
        self.ducker = ducker
        self.limits = limits
        self.qualityRequest = qualityRequest
        self.eventQueue = eventQueue
        if observeLifecycle { installLifecycleObservers() }
    }

    deinit {
        permissionTimer?.cancel()
        let center = NSWorkspace.shared.notificationCenter
        observers.forEach(center.removeObserver)
        backend.stop()
    }

    public func availableDevices() -> [MacCaptureDevice] {
        backend.availableDevices()
    }

    public func currentPhase() -> MacCapturePhase { queue.sync { phase } }

    public func selectCaptureQualityWorkflow(_ workflow: String) {
        queue.sync {
            guard phase == .idle,
                ["recorded_clip", "local_self_test", "live_ptt"].contains(workflow)
            else { return }
            qualityWorkflow = workflow
        }
    }

    public func setCaptureQualityRequest(_ request: MacCaptureQualityRequest) {
        queue.sync {
            guard phase == .idle else { return }
            qualityRequest = request
        }
    }

    /// Must be called only from a visible, explicit Record action.
    public func begin(selectedDeviceID: String?, explicitUserAction: Bool) async throws {
        guard explicitUserAction else { throw MacCaptureEngineError.explicitUserActionRequired }
        let attempt = queue.sync { () -> UInt64? in
            guard phase == .idle else { return nil }
            generation &+= 1
            terminalClaimed = false
            phase = .requestingPermission
            emit(.phase(.requestingPermission))
            return generation
        }
        guard let attempt else { throw MacCaptureEngineError.alreadyActive }

        var status = permission.currentPermission()
        if status == .notDetermined { status = await permission.requestPermission() }
        switch status {
        case .granted:
            break
        case .denied, .notDetermined:
            resetAfterPermissionFailure(.permissionDenied, attempt: attempt)
            throw MacCaptureEngineError.permissionDenied
        case .restricted:
            resetAfterPermissionFailure(.permissionRestricted, attempt: attempt)
            throw MacCaptureEngineError.permissionRestricted
        }

        try queue.sync {
            guard generation == attempt, phase == .requestingPermission,
                !terminalClaimed
            else { throw MacCaptureEngineError.invalidState }
            let devices = backend.availableDevices()
            guard !devices.isEmpty else {
                resetLocked(error: .noInputDevice)
                throw MacCaptureEngineError.noInputDevice
            }
            if let selectedDeviceID,
                !devices.contains(where: { $0.id == selectedDeviceID })
            {
                resetLocked(error: .selectedDeviceUnavailable)
                throw MacCaptureEngineError.selectedDeviceUnavailable
            }
            do {
                let partial = try mediaStore.begin(.userRecording)
                let writer = try MacCaptureWAVWriter(url: partial.fileURL)
                self.partial = partial
                self.writer = writer
                terminalClaimed = false
                commitSamples = false
                if let qualityBackend = backend as? MacCaptureQualityBackendConfiguring {
                    qualityBackend.configureCaptureQuality(
                        workflow: qualityWorkflow,
                        request: qualityRequest,
                        onState: { [weak self] state in self?.emit(.quality(state)) })
                }
                try backend.start(
                    selectedDeviceID: selectedDeviceID,
                    onSamples: { [weak self] samples in
                        self?.queue.async { self?.consumeLocked(samples) }
                    },
                    onFailure: { [weak self] in
                        self?.queue.async {
                            self?.cancelLocked(
                                reason: .deviceLost,
                                error: selectedDeviceID == nil
                                    ? .backendUnavailable
                                    : .selectedDeviceUnavailable)
                        }
                    })
                ducker.setCaptureDucking(active: true)
                runtimeActive = true
                phase = .playingStartCue
                emit(.devices(devices))
                emit(.phase(.playingStartCue))
                emit(.playStartCue)
                startPermissionMonitorLocked()
            } catch let error as MacCaptureEngineError {
                resetLocked(error: error)
                throw error
            } catch is CaptureMediaStoreError {
                resetLocked(error: .storage)
                throw MacCaptureEngineError.storage
            } catch {
                resetLocked(error: .backendUnavailable)
                throw MacCaptureEngineError.backendUnavailable
            }
        }
    }

    public func startCueCompleted() throws {
        try queue.sync {
            guard phase == .playingStartCue, !terminalClaimed else {
                throw MacCaptureEngineError.invalidState
            }
            commitSamples = true
            phase = .recording
            emit(.phase(.recording))
        }
    }

    public func stop() throws {
        try queue.sync {
            guard phase == .recording, !terminalClaimed else {
                throw MacCaptureEngineError.invalidState
            }
            try finishLocked(reason: .userStopped)
        }
    }

    public func cancel() { queue.async { self.cancelLocked(reason: .userCancelled) } }
    public func shutdown() { queue.sync { cancelLocked(reason: .appQuit) } }
    public func handleSystemSleep() { queue.async { self.cancelLocked(reason: .systemSleep) } }
    public func handleSessionLock() { queue.async { self.cancelLocked(reason: .sessionLocked) } }
    public func handleDeviceLoss() { queue.async { self.cancelLocked(reason: .deviceLost) } }
    public func handleBackendFailure() {
        queue.async { self.cancelLocked(reason: .backendFailure, error: .backendUnavailable) }
    }

    /// Explicit test/UI seam for immediately observed TCC changes; the active
    /// engine also polls once per second because macOS exposes no revoke event.
    public func recheckPermission() {
        queue.async {
            if self.phase != .idle, self.permission.currentPermission() != .granted {
                self.cancelLocked(reason: .permissionRevoked)
            }
        }
    }

    private func consumeLocked(_ samples: [Float]) {
        guard phase == .recording, commitSamples, !terminalClaimed,
            let writer
        else { return }
        do {
            let remainingFrames = limits.maxFrames - writer.framesWritten
            let remainingBytes = limits.maxPCMBytes - writer.pcmBytesWritten
            let allowed = min(Int64(samples.count), remainingFrames, remainingBytes / 2)
            if allowed > 0 {
                let committed = Array(samples.prefix(Int(allowed)))
                try writer.append(committed)
                emit(.meter(Self.meter(for: committed)))
            }
            if writer.framesWritten >= limits.maxFrames {
                try finishLocked(reason: .durationLimit)
            } else if writer.pcmBytesWritten >= limits.maxPCMBytes {
                try finishLocked(reason: .byteLimit)
            }
        } catch {
            cancelLocked(reason: .backendFailure, error: .storage)
        }
    }

    private func finishLocked(reason: MacCaptureTerminalReason) throws {
        guard !terminalClaimed, let partial, let writer else {
            throw MacCaptureEngineError.invalidState
        }
        terminalClaimed = true
        generation &+= 1
        commitSamples = false
        phase = .finalizing
        emit(.phase(.finalizing))
        stopRuntimeLocked()
        do {
            try writer.finish()
            let draft = try mediaStore.finalize(mediaStore.stop(partial))
            self.writer = nil
            self.partial = nil
            phase = .idle
            emit(.playStopCue)
            emit(.finished(draft, reason))
            emit(.phase(.idle))
        } catch {
            try? mediaStore.cancel(partial)
            self.writer = nil
            self.partial = nil
            phase = .idle
            emit(.failed(.storage))
            emit(.phase(.idle))
            throw MacCaptureEngineError.storage
        }
    }

    private func cancelLocked(
        reason: MacCaptureTerminalReason,
        error: MacCaptureEngineError? = nil
    ) {
        guard phase != .idle, !terminalClaimed else { return }
        terminalClaimed = true
        generation &+= 1
        commitSamples = false
        stopRuntimeLocked()
        writer?.discard()
        if let partial { try? mediaStore.cancel(partial) }
        writer = nil
        partial = nil
        phase = .idle
        if let error { emit(.failed(error)) }
        emit(.cancelled(reason))
        emit(.phase(.idle))
    }

    private func stopRuntimeLocked() {
        permissionTimer?.cancel()
        permissionTimer = nil
        guard runtimeActive else { return }
        backend.stop()
        ducker.setCaptureDucking(active: false)
        runtimeActive = false
    }

    private func resetAfterPermissionFailure(_ error: MacCaptureEngineError, attempt: UInt64) {
        queue.sync {
            guard generation == attempt, phase == .requestingPermission else { return }
            resetLocked(error: error)
        }
    }

    private func resetLocked(error: MacCaptureEngineError) {
        stopRuntimeLocked()
        writer?.discard()
        if let partial { try? mediaStore.cancel(partial) }
        writer = nil
        partial = nil
        terminalClaimed = false
        generation &+= 1
        commitSamples = false
        phase = .idle
        emit(.failed(error))
        emit(.phase(.idle))
    }

    private func startPermissionMonitorLocked() {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now() + .seconds(1), repeating: .seconds(1))
        timer.setEventHandler { [weak self] in
            guard let self, self.phase != .idle else { return }
            if self.permission.currentPermission() != .granted {
                self.cancelLocked(reason: .permissionRevoked)
            }
        }
        timer.resume()
        permissionTimer = timer
    }

    private func installLifecycleObservers() {
        let center = NSWorkspace.shared.notificationCenter
        observers.append(
            center.addObserver(
                forName: NSWorkspace.willSleepNotification, object: nil, queue: nil
            ) { [weak self] _ in self?.handleSystemSleep() })
        observers.append(
            center.addObserver(
                forName: NSWorkspace.sessionDidResignActiveNotification, object: nil, queue: nil
            ) { [weak self] _ in self?.handleSessionLock() })
    }

    private func emit(_ event: MacCaptureEvent) {
        guard let onEvent else { return }
        eventQueue.async { onEvent(event) }
    }

    static func meter(for samples: [Float]) -> Float {
        guard !samples.isEmpty else { return 0 }
        let sum = samples.reduce(0.0) { $0 + Double($1 * $1) }
        return Float(min(1, max(0, sqrt(sum / Double(samples.count)))))
    }
}

private final class MacCaptureWAVWriter {
    let url: URL
    private var handle: FileHandle?
    private(set) var framesWritten: Int64 = 0
    var pcmBytesWritten: Int64 { framesWritten * 2 }

    init(url: URL) throws {
        self.url = url
        guard let handle = try? FileHandle(forWritingTo: url) else {
            throw MacCaptureEngineError.storage
        }
        self.handle = handle
        try handle.truncate(atOffset: 0)
        try handle.write(contentsOf: Data(repeating: 0, count: 44))
    }

    func append(_ samples: [Float]) throws {
        guard let handle, !samples.isEmpty else { return }
        var bytes = Data(count: samples.count * 2)
        bytes.withUnsafeMutableBytes { raw in
            let output = raw.bindMemory(to: Int16.self)
            for index in samples.indices {
                let clamped = min(1, max(-1, samples[index]))
                output[index] = Int16((clamped * Float(Int16.max)).rounded()).littleEndian
            }
        }
        try handle.write(contentsOf: bytes)
        framesWritten += Int64(samples.count)
    }

    func finish() throws {
        guard let handle else { throw MacCaptureEngineError.storage }
        let dataBytes = UInt32(pcmBytesWritten)
        var header = Data(count: 44)
        header.replaceSubrange(0..<4, with: Data("RIFF".utf8))
        header.writeLE(dataBytes + 36, at: 4)
        header.replaceSubrange(8..<12, with: Data("WAVE".utf8))
        header.replaceSubrange(12..<16, with: Data("fmt ".utf8))
        header.writeLE(UInt32(16), at: 16)
        header.writeLE(UInt16(1), at: 20)
        header.writeLE(UInt16(1), at: 22)
        header.writeLE(UInt32(MacMicrophoneCaptureEngine.sampleRate), at: 24)
        header.writeLE(UInt32(MacMicrophoneCaptureEngine.sampleRate * 2), at: 28)
        header.writeLE(UInt16(2), at: 32)
        header.writeLE(UInt16(16), at: 34)
        header.replaceSubrange(36..<40, with: Data("data".utf8))
        header.writeLE(dataBytes, at: 40)
        try handle.seek(toOffset: 0)
        try handle.write(contentsOf: header)
        try handle.synchronize()
        try handle.close()
        self.handle = nil
    }

    func discard() {
        try? handle?.close()
        handle = nil
    }
}

extension Data {
    fileprivate mutating func writeLE<T: FixedWidthInteger>(_ value: T, at offset: Int) {
        var little = value.littleEndian
        Swift.withUnsafeBytes(of: &little) {
            replaceSubrange(offset..<(offset + $0.count), with: $0)
        }
    }
}
