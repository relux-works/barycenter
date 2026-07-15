import AVFAudio
import AudioToolbox
import Foundation

public enum MacLocalClipOutputError: Error, Equatable, Sendable {
    case busy
    case decodeFailed
    case playbackFailed
}

public protocol MacLocalClipPlaying: AnyObject {
    func play(
        fileURL: URL,
        completion: @escaping @Sendable (Result<Void, MacLocalClipOutputError>) -> Void
    )
    func cancel()
}

public protocol MacLocalSelfTestCapturing: AnyObject {
    var onEvent: (@Sendable (MacCaptureEvent) -> Void)? { get set }
    func begin(selectedDeviceID: String?, explicitUserAction: Bool) async throws
    func startCueCompleted() throws
    func stop() throws
    func cancel()
}

extension MacMicrophoneCaptureEngine: MacLocalSelfTestCapturing {}

/// Local-only facade over the exact production clip mixer. Its synthetic
/// schedule never leaves the process and enables no fetch, coordinator
/// callback or mixer telemetry.
public final class MacProductionLocalClipOutput: MacLocalClipPlaying, @unchecked Sendable {
    private let mixer: MacOverlayMediaClipMixer
    private let lock = NSLock()
    private var active: (clip: PreparedMediaClip, generation: Int64)?
    private var reservedGeneration: Int64?
    private var nextGeneration: Int64 = 0

    public convenience init(audio: AudioEngine, log: Logger) {
        self.init(mixer: MacOverlayMediaClipMixer(audio: audio, log: log))
    }

    init(mixer: MacOverlayMediaClipMixer) { self.mixer = mixer }

    public func play(
        fileURL: URL,
        completion: @escaping @Sendable (Result<Void, MacLocalClipOutputError>) -> Void
    ) {
        let generation: Int64 = lock.withLock {
            guard active == nil, reservedGeneration == nil else { return 0 }
            nextGeneration += 1
            reservedGeneration = nextGeneration
            return nextGeneration
        }
        guard generation > 0 else {
            completion(.failure(.busy))
            return
        }
        let clip: PreparedMediaClip
        do {
            clip = try mixer.prepare(localURL: fileURL, delivery: "overlay")
        } catch {
            clearReservation(generation)
            completion(.failure(.decodeFailed))
            return
        }
        let claimed: Bool = lock.withLock {
            guard reservedGeneration == generation, active == nil else { return false }
            reservedGeneration = nil
            active = (clip, generation)
            return true
        }
        guard claimed else {
            mixer.dispose(clip)
            completion(.failure(.playbackFailed))
            return
        }
        let start = Int64((Date().timeIntervalSince1970 * 1_000).rounded()) + 30
        let payload = PlayMediaAtPayload(
            transmissionId: "local-self-test-\(generation)",
            generation: generation,
            tCoordMs: start,
            startDeadlineCoordMs: start + 100,
            delivery: "overlay",
            duckDb: 0,
            attackMs: 0,
            releaseMs: 0,
            fadeOutMs: nil,
            fadeInMs: nil)
        guard let control = MixerControlParameters(payload) else {
            clear(clip: clip, generation: generation)
            mixer.dispose(clip)
            completion(.failure(.playbackFailed))
            return
        }
        let plan = MediaClipPlayPlan(
            payload: payload,
            localStartMs: start,
            localStartDeadlineMs: start + 100,
            control: control,
            collectTelemetry: false)
        do {
            try mixer.arm(
                clip,
                plan: plan,
                onStarted: { _ in },
                onEnded: { [weak self] _ in
                    self?.clear(clip: clip, generation: generation)
                    completion(.success(()))
                },
                onFailed: { [weak self] _ in
                    self?.clear(clip: clip, generation: generation)
                    completion(.failure(.playbackFailed))
                })
        } catch {
            clear(clip: clip, generation: generation)
            mixer.dispose(clip)
            completion(.failure(.playbackFailed))
        }
    }

    public func cancel() {
        let current = lock.withLock {
            reservedGeneration = nil
            return active
        }
        guard let current else { return }
        mixer.cancel(
            current.clip,
            command: CancelMediaPayload(
                transmissionId: "local-self-test-\(current.generation)",
                generation: current.generation,
                reason: "local_close",
                action: "cancel",
                resumeMain: false,
                fadeMs: 40)
        ) { [weak self] _ in
            self?.clear(clip: current.clip, generation: current.generation)
        }
    }

    private func clear(clip: PreparedMediaClip, generation: Int64) {
        lock.withLock {
            guard active?.clip === clip, active?.generation == generation else { return }
            active = nil
        }
    }

    private func clearReservation(_ generation: Int64) {
        lock.withLock {
            if reservedGeneration == generation { reservedGeneration = nil }
        }
    }
}

public enum MacShortAudioFormat: String, CaseIterable, Sendable {
    case wav, mp3, m4a, aac, ogg, flac
}

public enum MacShortAudioRejection: String, Equatable, Sendable {
    case unsupportedFormat = "unsupported_format"
    case empty = "empty"
    case sizeLimit = "size_limit"
    case durationLimit = "duration_limit"
    case unreadable = "unreadable"
}

public struct MacShortAudioReview: Equatable, Sendable {
    public let filename: String
    public let format: MacShortAudioFormat?
    public let durationMs: Int64?
    public let sizeBytes: Int64
    public let audience: [String]
    public let deliveryModes: [String]
    public let rightsReminder: String
    public let serverValidationRequired: Bool
    public let rejection: MacShortAudioRejection?

    public var isEligible: Bool { rejection == nil }
}

public struct MacShortAudioLimits: Equatable, Sendable {
    public let maximumBytes: Int64
    public let maximumDurationMs: Int64
    public let maximumOverlayDurationMs: Int64

    public init(
        maximumBytes: Int64 = 50 << 20,
        maximumDurationMs: Int64 = 180_000,
        maximumOverlayDurationMs: Int64 = 60_000
    ) {
        self.maximumBytes = maximumBytes
        self.maximumDurationMs = maximumDurationMs
        self.maximumOverlayDurationMs = maximumOverlayDurationMs
    }
}

public final class MacShortAudioInspector {
    private let limits: MacShortAudioLimits

    public init(limits: MacShortAudioLimits = MacShortAudioLimits()) {
        self.limits = limits
    }

    public func inspect(_ url: URL) -> MacShortAudioReview {
        let filename = url.lastPathComponent
        let values = try? url.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey])
        let bytes = Int64(values?.fileSize ?? 0)
        guard values?.isRegularFile == true, bytes > 0 else {
            return review(filename, nil, nil, bytes, .empty)
        }
        let headerFormat = Self.headerFormat(for: url)
        guard bytes <= limits.maximumBytes else {
            return review(filename, headerFormat, nil, bytes, .sizeLimit)
        }
        do {
            let file = try AVAudioFile(forReading: url)
            guard let format = headerFormat ?? Self.codecFormat(for: file, sourceURL: url) else {
                return review(filename, nil, nil, bytes, .unsupportedFormat)
            }
            guard file.length > 0, file.processingFormat.sampleRate > 0 else {
                return review(filename, format, nil, bytes, .unreadable)
            }
            let duration = Int64(
                (Double(file.length) / file.processingFormat.sampleRate * 1_000).rounded(.up))
            guard duration > 0 else {
                return review(filename, format, nil, bytes, .unreadable)
            }
            guard duration <= limits.maximumDurationMs else {
                return review(filename, format, duration, bytes, .durationLimit)
            }
            return review(filename, format, duration, bytes, nil)
        } catch {
            return review(
                filename,
                headerFormat,
                nil,
                bytes,
                headerFormat == nil ? .unsupportedFormat : .unreadable)
        }
    }

    private func review(
        _ filename: String,
        _ format: MacShortAudioFormat?,
        _ duration: Int64?,
        _ bytes: Int64,
        _ rejection: MacShortAudioRejection?
    ) -> MacShortAudioReview {
        var modes = ["interrupt", "after_current"]
        if let duration, duration <= limits.maximumOverlayDurationMs {
            modes.insert("overlay", at: 0)
        }
        return MacShortAudioReview(
            filename: filename,
            format: format,
            durationMs: duration,
            sizeBytes: bytes,
            audience: ["this_pulsar", "own_barycenter", "current_approach"],
            deliveryModes: rejection == nil ? modes : [],
            rightsReminder: "Upload only audio you recorded or have the right to share.",
            serverValidationRequired: rejection == nil,
            rejection: rejection)
    }

    private static func headerFormat(for url: URL) -> MacShortAudioFormat? {
        guard let file = try? FileHandle(forReadingFrom: url) else { return nil }
        defer { try? file.close() }
        guard let prefix = try? file.read(upToCount: 12) else { return nil }
        if prefix.count >= 12,
           prefix[0..<4] == Data("RIFF".utf8),
           prefix[8..<12] == Data("WAVE".utf8) { return .wav }
        if prefix.starts(with: Data("fLaC".utf8)) { return .flac }
        if prefix.starts(with: Data("OggS".utf8)) { return .ogg }
        if prefix.starts(with: Data("ID3".utf8)) { return .mp3 }
        if prefix.count >= 8, prefix[4..<8] == Data("ftyp".utf8) { return .m4a }
        if prefix.count >= 2, prefix[0] == 0xff {
            let second = prefix[1]
            if second & 0xf6 == 0xf0 { return .aac }
            if second & 0xe0 == 0xe0, second & 0x06 != 0 { return .mp3 }
        }
        return nil
    }

    private static func codecFormat(
        for file: AVAudioFile,
        sourceURL: URL
    ) -> MacShortAudioFormat? {
        guard let value = file.fileFormat.settings[AVFormatIDKey] as? NSNumber else { return nil }
        switch AudioFormatID(value.uint32Value) {
        case kAudioFormatMPEGLayer3:
            return .mp3
        case kAudioFormatMPEG4AAC, kAudioFormatMPEG4AAC_HE, kAudioFormatMPEG4AAC_HE_V2:
            return sourceURL.pathExtension.lowercased() == "aac" ? .aac : .m4a
        case kAudioFormatOpus:
            return .ogg
        case kAudioFormatFLAC:
            return .flac
        default:
            return nil
        }
    }
}

public enum MacShortAudioIntakeError: Error, Equatable, Sendable {
    case rejected(MacShortAudioRejection)
    case accessDenied
    case conversionFailed
    case storage
}

public final class MacShortAudioIntake {
    private let inspector: MacShortAudioInspector
    private let store: CaptureMediaStore

    public init(inspector: MacShortAudioInspector, store: CaptureMediaStore) {
        self.inspector = inspector
        self.store = store
    }

    public func review(_ url: URL) -> MacShortAudioReview { inspector.inspect(url) }

    public func accept(_ url: URL, useSecurityScopedAccess: Bool = true) throws
        -> (MacShortAudioReview, CaptureMediaHandle) {
        let access = !useSecurityScopedAccess || url.startAccessingSecurityScopedResource()
        guard access else { throw MacShortAudioIntakeError.accessDenied }
        defer {
            if useSecurityScopedAccess { url.stopAccessingSecurityScopedResource() }
        }
        let review = inspector.inspect(url)
        if let rejection = review.rejection {
            throw MacShortAudioIntakeError.rejected(rejection)
        }
        let partial: CaptureMediaHandle
        do {
            partial = try store.begin(.userRecording)
        } catch {
            throw MacShortAudioIntakeError.storage
        }
        do {
            try Self.canonicalize(source: url, destination: partial.fileURL)
            let draft = try store.finalize(store.stop(partial))
            return (review, draft)
        } catch let error as MacShortAudioIntakeError {
            try? store.cancel(partial)
            throw error
        } catch {
            try? store.cancel(partial)
            throw MacShortAudioIntakeError.conversionFailed
        }
    }

    private static func canonicalize(source: URL, destination: URL) throws {
        var input: ExtAudioFileRef?
        guard ExtAudioFileOpenURL(source as CFURL, &input) == noErr,
              let input else {
            throw MacShortAudioIntakeError.conversionFailed
        }
        defer { ExtAudioFileDispose(input) }
        var target = AudioStreamBasicDescription(
            mSampleRate: 44_100,
            mFormatID: kAudioFormatLinearPCM,
            mFormatFlags: kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked,
            mBytesPerPacket: 4,
            mFramesPerPacket: 1,
            mBytesPerFrame: 4,
            mChannelsPerFrame: 2,
            mBitsPerChannel: 16,
            mReserved: 0)
        guard ExtAudioFileSetProperty(
            input,
            kExtAudioFileProperty_ClientDataFormat,
            UInt32(MemoryLayout<AudioStreamBasicDescription>.size),
            &target) == noErr else {
            throw MacShortAudioIntakeError.conversionFailed
        }
        let handle = try FileHandle(forWritingTo: destination)
        defer { try? handle.close() }
        try handle.truncate(atOffset: 0)
        try handle.write(contentsOf: Data(repeating: 0, count: 44))
        let frameCapacity: UInt32 = 4_096
        var buffer = Data(count: Int(frameCapacity) * Int(target.mBytesPerFrame))
        var pcmBytes: UInt32 = 0
        while true {
            var frames = frameCapacity
            let status: OSStatus = buffer.withUnsafeMutableBytes { raw in
                var list = AudioBufferList(
                    mNumberBuffers: 1,
                    mBuffers: AudioBuffer(
                        mNumberChannels: target.mChannelsPerFrame,
                        mDataByteSize: UInt32(raw.count),
                        mData: raw.baseAddress))
                return ExtAudioFileRead(input, &frames, &list)
            }
            guard status == noErr else {
                throw MacShortAudioIntakeError.conversionFailed
            }
            guard frames > 0 else { break }
            let byteCount = frames.multipliedReportingOverflow(by: target.mBytesPerFrame)
            let addition = pcmBytes.addingReportingOverflow(byteCount.partialValue)
            guard !byteCount.overflow, !addition.overflow else {
                throw MacShortAudioIntakeError.conversionFailed
            }
            try handle.write(contentsOf: buffer.prefix(Int(byteCount.partialValue)))
            pcmBytes = addition.partialValue
        }
        var header = Data(count: 44)
        header.replaceSubrange(0..<4, with: Data("RIFF".utf8))
        header.writeLocalLE(pcmBytes + 36, at: 4)
        header.replaceSubrange(8..<12, with: Data("WAVE".utf8))
        header.replaceSubrange(12..<16, with: Data("fmt ".utf8))
        header.writeLocalLE(UInt32(16), at: 16)
        header.writeLocalLE(UInt16(1), at: 20)
        header.writeLocalLE(UInt16(2), at: 22)
        header.writeLocalLE(UInt32(44_100), at: 24)
        header.writeLocalLE(UInt32(44_100 * 4), at: 28)
        header.writeLocalLE(UInt16(4), at: 32)
        header.writeLocalLE(UInt16(16), at: 34)
        header.replaceSubrange(36..<40, with: Data("data".utf8))
        header.writeLocalLE(pcmBytes, at: 40)
        try handle.seek(toOffset: 0)
        try handle.write(contentsOf: header)
        try handle.synchronize()
    }
}

public enum MacLocalSelfTestPhase: String, Equatable, Sendable {
    case idle
    case playingBuiltinCue = "playing_builtin_cue"
    case requestingPermission = "requesting_permission"
    case recording
    case playingStopCue = "playing_stop_cue"
    case playingRecording = "playing_recording"
    case reviewingDraft = "reviewing_draft"
    case failed
}

public enum MacLocalSelfTestEvent: Equatable, Sendable {
    case phase(MacLocalSelfTestPhase)
    case meter(Float)
    case fileReview(MacShortAudioReview)
    case draft(CaptureMediaHandle)
    case failure(String)
}

public final class MacLocalSelfTestService: @unchecked Sendable {
    public static let exactRecordingSeconds: TimeInterval = 5

    private let capture: MacLocalSelfTestCapturing
    private let output: MacLocalClipPlaying
    private let store: CaptureMediaStore
    private let intake: MacShortAudioIntake
    private let cueURL: URL
    private let recordingDuration: TimeInterval
    private let queue = DispatchQueue(label: "works.relux.pulsar.local-self-test")
    private let eventQueue: DispatchQueue
    private var phase: MacLocalSelfTestPhase = .idle
    private var draft: CaptureMediaHandle?
    private var pendingPlaybackDraft: CaptureMediaHandle?
    private var stopCuePlaying = false
    private var stopTimer: DispatchSourceTimer?

    public var onEvent: (@Sendable (MacLocalSelfTestEvent) -> Void)?

    public init(
        capture: MacLocalSelfTestCapturing,
        output: MacLocalClipPlaying,
        store: CaptureMediaStore,
        intake: MacShortAudioIntake,
        cueURL: URL,
        recordingDuration: TimeInterval = MacLocalSelfTestService.exactRecordingSeconds,
        eventQueue: DispatchQueue = .main
    ) throws {
        _ = try BuiltinRecordingCue.load(from: cueURL)
        precondition(recordingDuration > 0)
        self.capture = capture
        self.output = output
        self.store = store
        self.intake = intake
        self.cueURL = cueURL
        self.recordingDuration = recordingDuration
        self.eventQueue = eventQueue
        capture.onEvent = { [weak self] event in self?.handleCapture(event) }
    }

    public func playBuiltinCue() {
        queue.async {
            guard self.phase == .idle || self.phase == .reviewingDraft else { return }
            self.setPhase(.playingBuiltinCue)
            self.output.play(fileURL: self.cueURL) { [weak self] result in
                self?.queue.async {
                    switch result {
                    case .success: self?.setPhase(self?.draft == nil ? .idle : .reviewingDraft)
                    case .failure: self?.fail("cue_playback_failed")
                    }
                }
            }
        }
    }

    public func recordFiveSeconds(selectedDeviceID: String? = nil) async throws {
        try await capture.begin(selectedDeviceID: selectedDeviceID, explicitUserAction: true)
    }

    public func reviewFile(_ url: URL, useSecurityScopedAccess: Bool = true) {
        queue.async {
            let access = !useSecurityScopedAccess || url.startAccessingSecurityScopedResource()
            guard access else {
                self.fail("file_access_denied")
                return
            }
            defer {
                if useSecurityScopedAccess { url.stopAccessingSecurityScopedResource() }
            }
            let review = self.intake.review(url)
            self.emit(.fileReview(review))
        }
    }

    public func acceptFile(_ url: URL, useSecurityScopedAccess: Bool = true) {
        queue.async {
            do {
                let previousDraft = self.draft
                let (review, draft) = try self.intake.accept(
                    url, useSecurityScopedAccess: useSecurityScopedAccess)
                if let previousDraft { try? self.store.explicitlyDelete(previousDraft) }
                self.draft = draft
                self.emit(.fileReview(review))
                self.emit(.draft(draft))
                self.setPhase(.reviewingDraft)
            } catch let error as MacShortAudioIntakeError {
                self.fail("file_\(error)")
            } catch {
                self.fail("file_intake_failed")
            }
        }
    }

    public func close() {
        queue.sync {
            stopTimer?.cancel()
            stopTimer = nil
            capture.cancel()
            output.cancel()
            deleteDraftLocked()
            pendingPlaybackDraft = nil
            stopCuePlaying = false
            setPhase(.idle)
        }
    }

    public func deleteDraft() {
        queue.async {
            self.output.cancel()
            self.deleteDraftLocked()
            self.setPhase(.idle)
        }
    }

    private func handleCapture(_ event: MacCaptureEvent) {
        queue.async {
            switch event {
            case .phase(.requestingPermission):
                self.setPhase(.requestingPermission)
            case .playStartCue:
                self.output.play(fileURL: self.cueURL) { [weak self] result in
                    self?.queue.async {
                        guard let self else { return }
                        switch result {
                        case .success:
                            do {
                                try self.capture.startCueCompleted()
                                self.setPhase(.recording)
                                self.scheduleFiveSecondStop()
                            } catch {
                                self.capture.cancel()
                                self.fail("capture_start_failed")
                            }
                        case .failure:
                            self.capture.cancel()
                            self.fail("cue_playback_failed")
                        }
                    }
                }
            case .meter(let value):
                self.emit(.meter(value))
            case .playStopCue:
                self.stopCuePlaying = true
                self.setPhase(.playingStopCue)
                self.output.play(fileURL: self.cueURL) { [weak self] result in
                    self?.queue.async {
                        guard let self else { return }
                        self.stopCuePlaying = false
                        if case .failure = result {
                            self.fail("cue_playback_failed")
                        } else {
                            self.playPendingRecordingLocked()
                        }
                    }
                }
            case .finished(let handle, _):
                self.stopTimer?.cancel()
                self.stopTimer = nil
                self.deleteDraftLocked()
                self.draft = handle
                self.pendingPlaybackDraft = handle
                self.emit(.draft(handle))
                if !self.stopCuePlaying { self.playPendingRecordingLocked() }
            case .cancelled:
                self.stopTimer?.cancel()
                self.stopTimer = nil
                self.setPhase(.idle)
            case .failed(let error):
                self.fail("capture_\(error)")
            case .phase, .devices:
                break
            }
        }
    }

    private func scheduleFiveSecondStop() {
        stopTimer?.cancel()
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(
            deadline: .now() + recordingDuration,
            leeway: .milliseconds(min(10, max(1, Int(recordingDuration * 100)))))
        timer.setEventHandler { [weak self] in
            guard let self else { return }
            do { try self.capture.stop() } catch { self.fail("capture_stop_failed") }
        }
        timer.resume()
        stopTimer = timer
    }

    private func playPendingRecordingLocked() {
        guard let handle = pendingPlaybackDraft else {
            setPhase(draft == nil ? .idle : .reviewingDraft)
            return
        }
        pendingPlaybackDraft = nil
        setPhase(.playingRecording)
        output.play(fileURL: handle.fileURL) { [weak self] result in
            self?.queue.async {
                switch result {
                case .success: self?.setPhase(.reviewingDraft)
                case .failure: self?.fail("recording_playback_failed")
                }
            }
        }
    }

    private func deleteDraftLocked() {
        guard let draft else { return }
        try? store.explicitlyDelete(draft)
        self.draft = nil
    }

    private func setPhase(_ phase: MacLocalSelfTestPhase) {
        self.phase = phase
        emit(.phase(phase))
    }

    private func fail(_ code: String) {
        setPhase(.failed)
        emit(.failure(code))
    }

    private func emit(_ event: MacLocalSelfTestEvent) {
        guard let onEvent else { return }
        eventQueue.async { onEvent(event) }
    }
}

private extension NSLock {
    func withLock<T>(_ body: () -> T) -> T {
        lock()
        defer { unlock() }
        return body()
    }
}

private extension Data {
    mutating func writeLocalLE<T: FixedWidthInteger>(_ value: T, at offset: Int) {
        var little = value.littleEndian
        Swift.withUnsafeBytes(of: &little) {
            replaceSubrange(offset..<(offset + $0.count), with: $0)
        }
    }
}
