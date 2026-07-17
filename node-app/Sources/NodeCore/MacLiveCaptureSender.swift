import AVFAudio
import AudioToolbox
import Foundation

enum MacLiveEncodeError: Error, Equatable {
    case unavailable
    case invalidFrame
    case oversizedPacket
}

protocol MacLiveOpusEncoding: AnyObject {
    func encode(
        samples: UnsafeBufferPointer<Float>,
        into output: UnsafeMutableRawBufferPointer
    ) throws -> Int
    func reset()
}

/// Self-contained engineering encoder. It fixes rate, channel count, frame
/// size, bitrate and constrained-VBR where AVAudioConverter exposes them.
/// Apple's API exposes no Opus FEC/complexity control, so this backend must not
/// make `live_ptt_v1` production-advertisable without the reviewed libopus path.
final class MacAVAudioOpusEncoder: MacLiveOpusEncoding {
    private static let frameSamples = 960
    private let converter: AVAudioConverter
    private let pcm: AVAudioPCMBuffer
    private let compressed: AVAudioCompressedBuffer

    init?() {
        guard let input = AVAudioFormat(
            commonFormat: .pcmFormatFloat32, sampleRate: 48_000,
            channels: 1, interleaved: false) else { return nil }
        var description = AudioStreamBasicDescription(
            mSampleRate: 48_000, mFormatID: kAudioFormatOpus, mFormatFlags: 0,
            mBytesPerPacket: 0, mFramesPerPacket: 960, mBytesPerFrame: 0,
            mChannelsPerFrame: 1, mBitsPerChannel: 0, mReserved: 0)
        var size = UInt32(MemoryLayout<AudioStreamBasicDescription>.size)
        guard AudioFormatGetProperty(
            kAudioFormatProperty_FormatInfo, 0, nil, &size, &description) == noErr,
            let output = AVAudioFormat(streamDescription: &description),
            let converter = AVAudioConverter(from: input, to: output),
            let pcm = AVAudioPCMBuffer(
                pcmFormat: input, frameCapacity: AVAudioFrameCount(Self.frameSamples))
        else { return nil }
        self.converter = converter
        self.pcm = pcm
        self.compressed = AVAudioCompressedBuffer(
            format: output, packetCapacity: 1,
            maximumPacketSize: LivePTTConstants.maxPayloadBytes)
        converter.primeMethod = .none
        converter.bitRate = 24_000
        converter.bitRateStrategy = AVAudioBitRateStrategy_VariableConstrained
    }

    func encode(
        samples: UnsafeBufferPointer<Float>,
        into output: UnsafeMutableRawBufferPointer
    ) throws -> Int {
        guard samples.count == Self.frameSamples,
              output.count >= LivePTTConstants.maxPayloadBytes,
              let destination = pcm.floatChannelData?[0]
        else { throw MacLiveEncodeError.invalidFrame }
        destination.update(from: samples.baseAddress!, count: samples.count)
        pcm.frameLength = AVAudioFrameCount(samples.count)
        compressed.byteLength = 0
        compressed.packetCount = 0
        var supplied = false
        var error: NSError?
        let status = converter.convert(to: compressed, error: &error) {
            [pcm] _, inputStatus in
            guard !supplied else {
                inputStatus.pointee = .noDataNow
                return nil
            }
            supplied = true
            inputStatus.pointee = .haveData
            return pcm
        }
        let count = Int(compressed.byteLength)
        guard error == nil, status != .error, compressed.packetCount == 1,
              count > 0 else { throw MacLiveEncodeError.unavailable }
        guard count <= LivePTTConstants.maxPayloadBytes else {
            throw MacLiveEncodeError.oversizedPacket
        }
        output.baseAddress!.copyMemory(from: compressed.data, byteCount: count)
        return count
    }

    func reset() { converter.reset() }
}

enum MacLiveHoldSource: String, Equatable, Sendable { case button, menu, shortcut }
enum MacLiveCaptureStopReason: String, Equatable, Sendable {
    case released, localStop = "local_stop", lostRelease = "lost_release"
    case systemSleep = "system_sleep", sessionLocked = "session_locked"
    case permissionRevoked = "permission_revoked", deviceLost = "device_lost"
    case captureQualityUnsupported = "capture_quality_unsupported"
    case appQuit = "app_quit", disconnected, backpressure, encoderFailure = "encoder_failure"
    case maximumDuration = "maximum_duration", coordinatorCancelled = "coordinator_cancelled"
}
enum MacLiveCapturePhase: String, Equatable, Sendable {
    case idle, awaitingStart = "awaiting_start", requestingPermission = "requesting_permission"
    case capturing, stopping
}
enum MacLiveCaptureEvent: Equatable, Sendable {
    case requestStart(localGeneration: UInt64, source: MacLiveHoldSource)
    case phase(MacLiveCapturePhase)
    case meter(Float)
    case quality(CaptureQualityState?)
    case playStartCue
    case playStopCue
    case fallbackToClip
    case terminal(MacLiveCaptureStopReason)
    case failed(String)
}

/// Fixed sample mailbox. The capture callback uses `try()` and never waits.
/// At most one drain and one overflow notification can be queued at a time.
private final class MacLiveSampleMailbox: @unchecked Sendable {
    enum Offer { case accepted, scheduleDrain, overflow, alreadyTerminal }
    private let lock = NSLock()
    private let storage: UnsafeMutablePointer<Float>
    private let capacity: Int
    private var head = 0, tail = 0, count = 0
    private var drainScheduled = false, terminalScheduled = false

    init(capacity: Int) {
        self.capacity = capacity
        storage = .allocate(capacity: capacity)
        storage.initialize(repeating: 0, count: capacity)
    }
    deinit { storage.deinitialize(count: capacity); storage.deallocate() }

    func offer(_ samples: [Float]) -> Offer {
        guard lock.try() else { return markOverflowWithoutWaiting() }
        defer { lock.unlock() }
        guard !terminalScheduled else { return .alreadyTerminal }
        guard !samples.isEmpty, samples.count <= capacity - count else {
            terminalScheduled = true; return .overflow
        }
        for sample in samples { storage[head] = sample; head = (head + 1) % capacity }
        count += samples.count
        if !drainScheduled { drainScheduled = true; return .scheduleDrain }
        return .accepted
    }

    func popFrame(into output: UnsafeMutablePointer<Float>, count wanted: Int) -> Bool {
        lock.lock(); defer { lock.unlock() }
        guard count >= wanted else { return false }
        for index in 0..<wanted { output[index] = storage[tail]; tail = (tail + 1) % capacity }
        count -= wanted
        return true
    }

    func finishDrain(frameSamples: Int) -> Bool {
        lock.lock(); defer { lock.unlock() }
        if terminalScheduled { drainScheduled = false; return false }
        if count >= frameSamples { return true }
        drainScheduled = false
        return false
    }

    func reset() {
        lock.lock(); defer { lock.unlock() }
        head = 0; tail = 0; count = 0
        drainScheduled = false; terminalScheduled = false
    }

    private func markOverflowWithoutWaiting() -> Offer {
        // A contended callback cannot safely mutate state. Its caller posts one
        // idempotent terminal request; the sender queue owns final teardown.
        .overflow
    }
}

final class MacLiveCaptureSender: @unchecked Sendable {
    private static let frameSamples = 960
    private static let sendQueueLimit = 8
    private let permission: MacMicrophonePermissionAuthorizing
    private let backend: MacMicrophoneCaptureBackend
    private let qualityRequest: MacCaptureQualityRequest
    private let encoder: MacLiveOpusEncoding
    private let coordinatorNowMs: () -> Int64
    private let monotonicUs: () -> UInt64
    private let trySendFrame: (LivePTTBinaryFrame) -> Bool
    private let sendControl: (Message) -> Void
    private let eventQueue: DispatchQueue
    private let queue = DispatchQueue(label: "duet.mac-live-capture-sender")
    private let mailbox = MacLiveSampleMailbox(capacity: 3_840)
    private let overflowPosted = RenderAtomicInt64()
    private let pcm: UnsafeMutablePointer<Float>
    private let packet: UnsafeMutableRawPointer
    private var phase: MacLiveCapturePhase = .idle
    private var localGeneration: UInt64 = 0
    private var lastHeartbeatMs: Int64 = 0
    private var selectedDeviceID: String?
    private var session: LivePTTStartPayload?
    private var sessionBytes: [UInt8] = []
    private var sequence: UInt32 = 0
    private var captureBaseUs: UInt64 = 0
    private var pendingFrame: LivePTTBinaryFrame?
    private var outbound: [LivePTTBinaryFrame] = []
    private var backendActive = false
    private var timer: DispatchSourceTimer?
    var onEvent: (@Sendable (MacLiveCaptureEvent) -> Void)?

    struct Snapshot: Equatable {
        var phase: MacLiveCapturePhase
        var localGeneration: UInt64
        var sequence: UInt32
        var queuedFrames: Int
        var hasPendingFrame: Bool
        var backendActive: Bool
    }

    init(
        permission: MacMicrophonePermissionAuthorizing,
        backend: MacMicrophoneCaptureBackend,
        encoder: MacLiveOpusEncoding,
        qualityRequest: MacCaptureQualityRequest = MacCaptureQualityRequest(mode: .auto),
        eventQueue: DispatchQueue = .main,
        coordinatorNowMs: @escaping () -> Int64,
        monotonicUs: @escaping () -> UInt64,
        trySendFrame: @escaping (LivePTTBinaryFrame) -> Bool,
        sendControl: @escaping (Message) -> Void
    ) {
        self.permission = permission; self.backend = backend; self.encoder = encoder
        self.qualityRequest = qualityRequest
        self.eventQueue = eventQueue; self.coordinatorNowMs = coordinatorNowMs
        self.monotonicUs = monotonicUs; self.trySendFrame = trySendFrame
        self.sendControl = sendControl
        pcm = .allocate(capacity: Self.frameSamples)
        pcm.initialize(repeating: 0, count: Self.frameSamples)
        packet = .allocate(byteCount: LivePTTConstants.maxPayloadBytes, alignment: 16)
    }
    deinit {
        timer?.cancel(); backend.stop()
        pcm.deinitialize(count: Self.frameSamples); pcm.deallocate(); packet.deallocate()
    }

    convenience init?(
        permission: MacMicrophonePermissionAuthorizing,
        backend: MacMicrophoneCaptureBackend,
        qualityRequest: MacCaptureQualityRequest = MacCaptureQualityRequest(mode: .auto),
        eventQueue: DispatchQueue = .main,
        coordinatorNowMs: @escaping () -> Int64,
        monotonicUs: @escaping () -> UInt64,
        trySendFrame: @escaping (LivePTTBinaryFrame) -> Bool,
        sendControl: @escaping (Message) -> Void
    ) {
        guard let encoder = MacAVAudioOpusEncoder() else { return nil }
        self.init(permission: permission, backend: backend, encoder: encoder,
                  qualityRequest: qualityRequest,
                  eventQueue: eventQueue, coordinatorNowMs: coordinatorNowMs,
                  monotonicUs: monotonicUs, trySendFrame: trySendFrame,
                  sendControl: sendControl)
    }

    func currentPhase() -> MacLiveCapturePhase { queue.sync { phase } }
    func snapshot() -> Snapshot { queue.sync { Snapshot(
        phase: phase, localGeneration: localGeneration, sequence: sequence,
        queuedFrames: outbound.count, hasPendingFrame: pendingFrame != nil,
        backendActive: backendActive) } }

    @discardableResult
    func localHoldBegan(
        source: MacLiveHoldSource,
        holdCapabilityAvailable: Bool,
        selectedDeviceID: String?
    ) -> UInt64? {
        queue.sync {
            guard phase == .idle else { return nil }
            guard holdCapabilityAvailable else { emit(.fallbackToClip); return nil }
            localGeneration &+= 1
            self.selectedDeviceID = selectedDeviceID
            lastHeartbeatMs = coordinatorNowMs()
            phase = .awaitingStart
            emit(.phase(.awaitingStart)); emit(.requestStart(
                localGeneration: localGeneration, source: source))
            armTimerLocked()
            return localGeneration
        }
    }

    func localHoldHeartbeat(generation: UInt64) {
        queue.async {
            guard generation == self.localGeneration, self.phase != .idle else { return }
            self.lastHeartbeatMs = self.coordinatorNowMs()
        }
    }

    func acceptStart(
        _ payload: LivePTTStartPayload,
        localGeneration generation: UInt64,
        authorized: Bool
    ) async throws {
        let eligible = queue.sync {
            guard generation == localGeneration, phase == .awaitingStart, authorized,
                  (try? LivePTTValidation.validate(.livePTTStart(payload))) != nil,
                  let bytes = Self.sessionBytes(payload.sessionId) else { return false }
            session = payload; sessionBytes = bytes
            phase = .requestingPermission; emit(.phase(.requestingPermission))
            return true
        }
        guard eligible else { throw MacLiveEncodeError.invalidFrame }
        var status = permission.currentPermission()
        if status == .notDetermined { status = await permission.requestPermission() }
        guard status == .granted else {
            queue.sync { terminateLocked(.permissionRevoked, sendTerminal: true) }
            throw MacCaptureEngineError.permissionDenied
        }
        do { try queue.sync {
            guard generation == localGeneration, phase == .requestingPermission,
                  session?.sessionId == payload.sessionId else {
                throw MacLiveEncodeError.invalidFrame
            }
            let devices = backend.availableDevices()
            guard !devices.isEmpty,
                  selectedDeviceID == nil || devices.contains(where: { $0.id == selectedDeviceID })
            else { throw MacCaptureEngineError.selectedDeviceUnavailable }
            sequence = 0
            captureBaseUs = max(1, monotonicUs()); pendingFrame = nil
            outbound.removeAll(keepingCapacity: true); mailbox.reset()
            overflowPosted.store(0); encoder.reset()
            if let qualityBackend = backend as? MacCaptureQualityBackendConfiguring {
                qualityBackend.configureCaptureQuality(
                    workflow: "live_ptt",
                    request: qualityRequest,
                    onState: { [weak self] state in self?.emit(.quality(state)) })
            }
            backendActive = true
            try backend.start(selectedDeviceID: selectedDeviceID, onSamples: { [weak self] samples in
                self?.captureCallback(samples)
            }, onFailure: { [weak self] in self?.queue.async {
                self?.terminateLocked(.deviceLost, sendTerminal: true)
            }})
            phase = .capturing
            emit(.phase(.capturing)); emit(.playStartCue)
        }} catch {
            let reason: MacLiveCaptureStopReason
            if let captureError = error as? MacCaptureEngineError,
               captureError == .captureQualityUnsupported {
                reason = .captureQualityUnsupported
            } else {
                reason = .deviceLost
            }
            queue.sync { terminateLocked(reason, sendTerminal: true) }
            throw error
        }
    }

    func localHoldEnded(generation: UInt64) { queue.async {
        guard generation == self.localGeneration else { return }
        self.terminateLocked(.released, sendTerminal: true)
    }}
    func localStop() { queue.async { self.terminateLocked(.localStop, sendTerminal: true) } }
    func handleSystemSleep() { queue.async { self.terminateLocked(.systemSleep, sendTerminal: true) } }
    func handleSessionLock() { queue.async { self.terminateLocked(.sessionLocked, sendTerminal: true) } }
    func handleDisconnect() { queue.async { self.terminateLocked(.disconnected, sendTerminal: false) } }
    func handleCoordinatorCancel() { queue.async {
        self.terminateLocked(.coordinatorCancelled, sendTerminal: false)
    }}
    func shutdown() { queue.sync { terminateLocked(.appQuit, sendTerminal: false) } }
    func retryOutbound() { queue.async { self.drainOutboundLocked() } }
    func recheckPermission() { queue.async {
        if self.phase == .capturing && self.permission.currentPermission() != .granted {
            self.terminateLocked(.permissionRevoked, sendTerminal: true)
        }
    }}

    /// Deterministic seam used by lifecycle tests; the production timer calls
    /// the same queue-owned check every 250 ms.
    func runWatchdogCheck() { queue.sync { watchdogLocked() } }

    private func captureCallback(_ samples: [Float]) {
        // BEGIN LIVE CAPTURE CALLBACK (source-inspected: bounded try-offer only)
        switch mailbox.offer(samples) {
        case .scheduleDrain: queue.async { [weak self] in self?.drainSamplesLocked() }
        case .overflow:
            var expected: Int64 = 0
            if overflowPosted.compareExchange(expected: &expected, desired: 1) {
                queue.async { [weak self] in
                    self?.terminateLocked(.backpressure, sendTerminal: true)
                }
            }
        case .accepted, .alreadyTerminal: break
        }
        // END LIVE CAPTURE CALLBACK
    }

    private func drainSamplesLocked() {
        guard phase == .capturing else { mailbox.reset(); return }
        while mailbox.popFrame(into: pcm, count: Self.frameSamples) {
            do {
                let count = try encoder.encode(
                    samples: UnsafeBufferPointer(start: pcm, count: Self.frameSamples),
                    into: UnsafeMutableRawBufferPointer(
                        start: packet, count: LivePTTConstants.maxPayloadBytes))
                sequence += 1
                var flags = LivePTTBinaryFrame.fecFlag
                if sequence == 1 { flags |= LivePTTBinaryFrame.startFlag }
                let frame = LivePTTBinaryFrame(
                    flags: flags, sessionId: sessionBytes, sequence: sequence,
                    captureMonotonicUs: captureBaseUs + UInt64(sequence - 1) * 20_000,
                    payload: Data(bytes: packet, count: count))
                if let pendingFrame {
                    guard enqueueLocked(pendingFrame) else {
                        terminateLocked(.backpressure, sendTerminal: true); return
                    }
                }
                pendingFrame = frame
                emit(.meter(Self.meter(pcm, count: Self.frameSamples)))
                drainOutboundLocked()
            } catch {
                terminateLocked(.encoderFailure, sendTerminal: true); return
            }
        }
        if mailbox.finishDrain(frameSamples: Self.frameSamples) {
            queue.async { [weak self] in self?.drainSamplesLocked() }
        }
    }

    private func enqueueLocked(_ frame: LivePTTBinaryFrame) -> Bool {
        guard outbound.count < Self.sendQueueLimit else { return false }
        outbound.append(frame); return true
    }

    private func drainOutboundLocked() {
        while let first = outbound.first, trySendFrame(first) { outbound.removeFirst() }
    }

    private func terminateLocked(_ reason: MacLiveCaptureStopReason, sendTerminal: Bool) {
        guard phase != .idle else { return }
        phase = .stopping; emit(.phase(.stopping)); localGeneration &+= 1
        timer?.cancel(); timer = nil
        if backendActive { backend.stop(); backendActive = false }
        drainSamplesLocked()
        let graceful = sendTerminal && Self.endReason(reason) != nil
        if graceful, var terminal = pendingFrame {
            terminal.flags |= LivePTTBinaryFrame.endFlag
            pendingFrame = nil
            if outbound.count < Self.sendQueueLimit { outbound.append(terminal) }
            drainOutboundLocked()
        }
        if sendTerminal, let session {
            let message: Message
            if graceful, sequence > 0, let endReason = Self.endReason(reason) {
                let now = max(1, coordinatorNowMs())
                message = .livePTTEnd(LivePTTEndPayload(
                    sessionId: session.sessionId, generation: session.generation,
                    commandSequence: 1, lastSequence: sequence,
                    endedAtCoordMs: now,
                    drainDeadlineCoordMs: now + LivePTTConstants.drainTimeoutMs,
                    reason: endReason))
            } else if let cancelReason = Self.cancelReason(reason) {
                message = .livePTTCancel(LivePTTCancelPayload(
                    sessionId: session.sessionId, generation: session.generation,
                    commandSequence: 1, cancelledAtCoordMs: max(1, coordinatorNowMs()),
                    reason: cancelReason, discardBuffered: true))
            } else {
                message = .livePTTFailed(LivePTTFailedPayload(
                    sessionId: session.sessionId, generation: session.generation,
                    eventSequence: 1, stage: "capture", code: reason.rawValue,
                    failedAtCoordMs: max(1, coordinatorNowMs())))
            }
            if (try? LivePTTValidation.validate(message)) != nil {
                sendControl(message)
            }
        }
        mailbox.reset(); overflowPosted.store(0); encoder.reset()
        outbound.removeAll(keepingCapacity: true)
        pendingFrame = nil; self.session = nil; sessionBytes.removeAll(keepingCapacity: true)
        selectedDeviceID = nil; sequence = 0; phase = .idle
        emit(.playStopCue); emit(.terminal(reason)); emit(.phase(.idle))
    }

    private func armTimerLocked() {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now() + .milliseconds(250), repeating: .milliseconds(250))
        timer.setEventHandler { [weak self] in self?.watchdogLocked() }
        self.timer = timer; timer.resume()
    }

    private func watchdogLocked() {
        guard phase != .idle else { return }
        let now = coordinatorNowMs()
        if now - lastHeartbeatMs > 1_500 {
            terminateLocked(.lostRelease, sendTerminal: true)
        } else if let session,
                  now > session.startedAtCoordMs + session.maxDurationMs {
            terminateLocked(.maximumDuration, sendTerminal: true)
        }
    }

    private func emit(_ event: MacLiveCaptureEvent) {
        guard let onEvent else { return }; eventQueue.async { onEvent(event) }
    }
    private static func meter(_ samples: UnsafePointer<Float>, count: Int) -> Float {
        var sum: Double = 0
        for index in 0..<count { sum += Double(samples[index] * samples[index]) }
        return Float(min(1, sqrt(sum / Double(count))))
    }
    private static func sessionBytes(_ value: String) -> [UInt8]? {
        guard value.count == 32 else { return nil }; var bytes: [UInt8] = []
        var index = value.startIndex
        while index < value.endIndex {
            let next = value.index(index, offsetBy: 2)
            guard let byte = UInt8(value[index..<next], radix: 16) else { return nil }
            bytes.append(byte); index = next
        }
        return bytes
    }

    private static func endReason(_ reason: MacLiveCaptureStopReason) -> String? {
        switch reason {
        case .released: "release"
        case .lostRelease: "lost_release"
        case .systemSleep: "sleep"
        case .sessionLocked: "lock"
        case .permissionRevoked: "permission_revoked"
        case .deviceLost: "device_lost"
        case .appQuit: "quit"
        case .disconnected: "disconnect"
        default: nil
        }
    }

    private static func cancelReason(_ reason: MacLiveCaptureStopReason) -> String? {
        switch reason {
        case .localStop, .released: "user_cancel"
        case .lostRelease: "lost_release"
        case .backpressure: "backpressure"
        case .maximumDuration: "timeout"
        default: nil
        }
    }
}
