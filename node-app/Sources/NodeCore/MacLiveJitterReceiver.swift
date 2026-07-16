import AVFAudio
import AudioToolbox
import Foundation

enum MacLiveDecodeError: Error, Equatable {
    case invalidPacket
    case fecUnavailable
    case conversionFailed
}

/// Decoder work is owned by the receiver queue and never enters the render callback.
protocol MacLiveOpusDecoding: AnyObject {
    func decode(
        packet: Data,
        fec: Bool,
        into output: UnsafeMutableBufferPointer<Float>
    ) throws -> Int
    func reset()
}

/// Self-contained macOS Opus path. AudioConverter accepts raw Opus access units
/// without staging a dylib. It does not expose Opus decode_fec, so callers must
/// use PLC or inject a reviewed FEC-capable backend for a missing packet.
final class MacAVAudioOpusDecoder: MacLiveOpusDecoding {
    static let framesPerPacket = 960

    private let converter: AVAudioConverter
    private let compressed: AVAudioCompressedBuffer
    private let pcm: AVAudioPCMBuffer

    init?() {
        var description = AudioStreamBasicDescription(
            mSampleRate: 48_000,
            mFormatID: kAudioFormatOpus,
            mFormatFlags: 0,
            mBytesPerPacket: 0,
            mFramesPerPacket: UInt32(Self.framesPerPacket),
            mBytesPerFrame: 0,
            mChannelsPerFrame: 1,
            mBitsPerChannel: 0,
            mReserved: 0)
        var propertySize = UInt32(MemoryLayout<AudioStreamBasicDescription>.size)
        guard AudioFormatGetProperty(
            kAudioFormatProperty_FormatInfo,
            0,
            nil,
            &propertySize,
            &description) == noErr,
            let inputFormat = AVAudioFormat(streamDescription: &description),
            let outputFormat = AVAudioFormat(
                commonFormat: .pcmFormatFloat32,
                sampleRate: 48_000,
                channels: 1,
                interleaved: false),
            let converter = AVAudioConverter(from: inputFormat, to: outputFormat),
            let pcm = AVAudioPCMBuffer(
                pcmFormat: outputFormat,
                frameCapacity: AVAudioFrameCount(Self.framesPerPacket))
        else { return nil }

        self.converter = converter
        self.compressed = AVAudioCompressedBuffer(
            format: inputFormat,
            packetCapacity: 1,
            maximumPacketSize: LivePTTConstants.maxPayloadBytes)
        self.pcm = pcm
        converter.primeMethod = .none
    }

    func decode(
        packet: Data,
        fec: Bool,
        into output: UnsafeMutableBufferPointer<Float>
    ) throws -> Int {
        guard !fec else { throw MacLiveDecodeError.fecUnavailable }
        guard !packet.isEmpty,
              packet.count <= LivePTTConstants.maxPayloadBytes,
              output.count >= Self.framesPerPacket
        else { throw MacLiveDecodeError.invalidPacket }

        packet.copyBytes(
            to: compressed.data.assumingMemoryBound(to: UInt8.self),
            count: packet.count)
        compressed.byteLength = UInt32(packet.count)
        compressed.packetCount = 1
        compressed.packetDescriptions?.pointee = AudioStreamPacketDescription(
            mStartOffset: 0,
            mVariableFramesInPacket: UInt32(Self.framesPerPacket),
            mDataByteSize: UInt32(packet.count))
        pcm.frameLength = 0

        var supplied = false
        var conversionError: NSError?
        let status = converter.convert(to: pcm, error: &conversionError) {
            [compressed] _, inputStatus in
            guard !supplied else {
                inputStatus.pointee = .noDataNow
                return nil
            }
            supplied = true
            inputStatus.pointee = .haveData
            return compressed
        }
        guard conversionError == nil,
              status != .error,
              pcm.frameLength > 0,
              Int(pcm.frameLength) <= Self.framesPerPacket,
              let source = pcm.floatChannelData?[0]
        else { throw MacLiveDecodeError.conversionFailed }

        let produced = Int(pcm.frameLength)
        let prefix = Self.framesPerPacket - produced
        output.baseAddress!.update(repeating: 0, count: output.count)
        (output.baseAddress! + prefix).update(from: source, count: produced)
        return Self.framesPerPacket
    }

    func reset() { converter.reset() }
}

protocol MacLiveAudioRouting: AnyObject {
    var livePCMCapacityFrames: Int { get }
    var livePCMBufferedFrames: Int { get }
    var livePCMUnderrunCallbacks: Int64 { get }
    func prepareLivePCM() -> Int64
    func activateLivePCM(generation: Int64)
    func writeLivePCM(
        generation: Int64,
        samples: UnsafePointer<Float>,
        count: Int
    ) -> Int
    func stopLivePCM(generation: Int64, discard: Bool)
}

extension AudioEngine: MacLiveAudioRouting {}

struct MacLiveJitterSnapshot: Equatable {
    enum Phase: String { case idle, buffering, playing, draining }

    var phase: Phase
    var sessionId: String?
    var generation: Int64?
    var expectedSequence: UInt32
    var highestSequence: UInt32
    var encodedFrames: Int
    var encodedBytes: Int
    var pcmFrames: Int
    var pcmCapacityFrames: Int
    var receivedFrames: Int
    var decodedFrames: Int
    var duplicateFrames: Int
    var lateFrames: Int
    var fecFrames: Int
    var plcFrames: Int
    var failedFrames: Int
    var underrunCallbacks: Int64
}

/// Per-session bounded jitter, concealment and PCM production path. All packet,
/// decoder, timer and event work is serialized off the audio render callback.
final class MacLiveJitterReceiver {
    private static let packetWindow = Int(LivePTTConstants.maxGapFrames) + 1
    private static let frameSamples = 960
    private static let maxConsecutiveConcealments = 8

    private struct Session {
        var start: LivePTTStartPayload
        var sessionBytes: [UInt8]
        var routeGeneration: Int64
        var phase: MacLiveJitterSnapshot.Phase = .buffering
        var packets: [UInt32: LivePTTBinaryFrame] = [:]
        var expectedSequence: UInt32 = 1
        var highestSequence: UInt32 = 0
        var captureBaseUs: UInt64?
        var lastCommandSequence: Int64 = 0
        var endSequence: UInt32?
        var drainDeadlineMs: Int64?
        var eventSequence: Int64 = 1
        var receivedFrames = 0
        var decodedFrames = 0
        var duplicateFrames = 0
        var lateFrames = 0
        var fecFrames = 0
        var plcFrames = 0
        var failedFrames = 0
        var consecutiveConcealments = 0
    }

    private let route: MacLiveAudioRouting
    private let decoder: MacLiveOpusDecoding
    private let send: (Message) -> Void
    private let coordinatorNowMs: () -> Int64
    private let queue: DispatchQueue
    private let automaticTick: Bool
    private var timer: DispatchSourceTimer?
    private var session: Session?
    private var highestGeneration: Int64 = 0
    private let decodeScratch: UnsafeMutableBufferPointer<Float>
    private let lastPCM: UnsafeMutableBufferPointer<Float>
    private var hasLastPCM = false

    init(
        route: MacLiveAudioRouting,
        decoder: MacLiveOpusDecoding,
        automaticTick: Bool = true,
        coordinatorNowMs: @escaping () -> Int64,
        send: @escaping (Message) -> Void
    ) {
        self.route = route
        self.decoder = decoder
        self.automaticTick = automaticTick
        self.coordinatorNowMs = coordinatorNowMs
        self.send = send
        self.queue = DispatchQueue(label: "duet.mac-live-jitter")
        let scratch = UnsafeMutablePointer<Float>.allocate(capacity: Self.frameSamples)
        scratch.initialize(repeating: 0, count: Self.frameSamples)
        self.decodeScratch = UnsafeMutableBufferPointer(
            start: scratch, count: Self.frameSamples)
        let prior = UnsafeMutablePointer<Float>.allocate(capacity: Self.frameSamples)
        prior.initialize(repeating: 0, count: Self.frameSamples)
        self.lastPCM = UnsafeMutableBufferPointer(start: prior, count: Self.frameSamples)
    }

    convenience init?(
        route: MacLiveAudioRouting,
        automaticTick: Bool = true,
        coordinatorNowMs: @escaping () -> Int64,
        send: @escaping (Message) -> Void
    ) {
        guard let decoder = MacAVAudioOpusDecoder() else { return nil }
        self.init(
            route: route,
            decoder: decoder,
            automaticTick: automaticTick,
            coordinatorNowMs: coordinatorNowMs,
            send: send)
    }

    deinit {
        timer?.cancel()
        decodeScratch.baseAddress?.deinitialize(count: Self.frameSamples)
        decodeScratch.baseAddress?.deallocate()
        lastPCM.baseAddress?.deinitialize(count: Self.frameSamples)
        lastPCM.baseAddress?.deallocate()
    }

    @discardableResult
    func start(_ payload: LivePTTStartPayload, authorized: Bool) -> Bool {
        queue.sync { startLocked(payload, authorized: authorized) }
    }

    @discardableResult
    func receive(_ frame: LivePTTBinaryFrame) -> LivePTTFrameDecision {
        queue.sync { receiveLocked(frame) }
    }

    func tick() { queue.sync { tickLocked(nowMs: coordinatorNowMs()) } }

    func end(_ payload: LivePTTEndPayload) {
        queue.sync {
            guard var active = session,
                  payload.sessionId == active.start.sessionId,
                  payload.generation == active.start.generation,
                  payload.commandSequence > active.lastCommandSequence,
                  payload.lastSequence >= active.expectedSequence - 1,
                  payload.drainDeadlineCoordMs == payload.endedAtCoordMs
                    + LivePTTConstants.drainTimeoutMs,
                  (try? LivePTTValidation.validate(.livePTTEnd(payload))) != nil
            else { return }
            active.lastCommandSequence = payload.commandSequence
            active.endSequence = payload.lastSequence
            active.drainDeadlineMs = payload.drainDeadlineCoordMs
            active.phase = .draining
            session = active
            tickLocked(nowMs: coordinatorNowMs())
        }
    }

    func cancel(_ payload: LivePTTCancelPayload) {
        queue.sync {
            guard let active = session,
                  payload.sessionId == active.start.sessionId,
                  payload.generation == active.start.generation,
                  payload.commandSequence > active.lastCommandSequence,
                  (try? LivePTTValidation.validate(.livePTTCancel(payload))) != nil
            else { return }
            terminateLocked(state: "cancelled", discard: true)
        }
    }

    func revoke(reason: String) {
        queue.sync {
            guard session != nil else { return }
            terminateLocked(state: "cancelled", discard: true)
        }
    }

    func snapshot() -> MacLiveJitterSnapshot {
        queue.sync {
            guard let active = session else {
                return MacLiveJitterSnapshot(
                    phase: .idle, sessionId: nil, generation: nil,
                    expectedSequence: 0, highestSequence: 0,
                    encodedFrames: 0, encodedBytes: 0,
                    pcmFrames: route.livePCMBufferedFrames,
                    pcmCapacityFrames: route.livePCMCapacityFrames,
                    receivedFrames: 0, decodedFrames: 0, duplicateFrames: 0,
                    lateFrames: 0, fecFrames: 0, plcFrames: 0, failedFrames: 0,
                    underrunCallbacks: route.livePCMUnderrunCallbacks)
            }
            return MacLiveJitterSnapshot(
                phase: active.phase,
                sessionId: active.start.sessionId,
                generation: active.start.generation,
                expectedSequence: active.expectedSequence,
                highestSequence: active.highestSequence,
                encodedFrames: active.packets.count,
                encodedBytes: active.packets.values.reduce(0) { $0 + $1.payload.count },
                pcmFrames: route.livePCMBufferedFrames,
                pcmCapacityFrames: route.livePCMCapacityFrames,
                receivedFrames: active.receivedFrames,
                decodedFrames: active.decodedFrames,
                duplicateFrames: active.duplicateFrames,
                lateFrames: active.lateFrames,
                fecFrames: active.fecFrames,
                plcFrames: active.plcFrames,
                failedFrames: active.failedFrames,
                underrunCallbacks: route.livePCMUnderrunCallbacks)
        }
    }

    private func startLocked(_ payload: LivePTTStartPayload, authorized: Bool) -> Bool {
        let now = coordinatorNowMs()
        let message = Message.livePTTStart(payload)
        let valid = (try? LivePTTValidation.validate(message)) != nil
        let rejectCode: String?
        if !valid { rejectCode = "unsupported" }
        else if !authorized { rejectCode = "unauthorized" }
        else if session != nil { rejectCode = "busy" }
        else if payload.generation <= highestGeneration { rejectCode = "expired" }
        else if now > payload.acceptDeadlineCoordMs { rejectCode = "expired" }
        else if route.livePCMCapacityFrames < Self.frameSamples * 4 {
            rejectCode = "unsupported"
        } else { rejectCode = nil }

        if let rejectCode {
            send(.livePTTReject(LivePTTRejectPayload(
                sessionId: payload.sessionId,
                generation: payload.generation,
                eventSequence: 1,
                code: rejectCode,
                rejectedAtCoordMs: max(1, now))))
            return false
        }

        guard let sessionBytes = Self.decodeSessionId(payload.sessionId) else {
            return false
        }
        highestGeneration = payload.generation
        decoder.reset()
        hasLastPCM = false
        let routeGeneration = route.prepareLivePCM()
        session = Session(
            start: payload,
            sessionBytes: sessionBytes,
            routeGeneration: routeGeneration)
        send(.livePTTAccept(LivePTTAcceptPayload(
            sessionId: payload.sessionId,
            generation: payload.generation,
            eventSequence: 1,
            acceptedAtCoordMs: max(1, now),
            liveEdgeSequence: 1,
            bufferFrames: 3)))
        armTimerLocked()
        return true
    }

    private func receiveLocked(_ frame: LivePTTBinaryFrame) -> LivePTTFrameDecision {
        guard var active = session, frame.sessionId == active.sessionBytes else {
            return .stale
        }
        guard (try? frame.encoded()) != nil else { return .invalid }
        if frame.sequence < active.expectedSequence {
            active.lateFrames += 1
            session = active
            return .stale
        }
        if let existing = active.packets[frame.sequence] {
            if existing == frame {
                active.duplicateFrames += 1
                session = active
                return .duplicate
            }
            active.lateFrames += 1
            session = active
            return .stale
        }
        guard frame.sequence <= 15_000,
              Int(frame.sequence - active.expectedSequence) < Self.packetWindow,
              active.packets.count < Self.packetWindow
        else { return .invalid }

        if frame.sequence == 1 {
            guard active.captureBaseUs == nil else { return .invalid }
            active.captureBaseUs = frame.captureMonotonicUs
        }
        guard let base = active.captureBaseUs else { return .invalid }
        let expectedCapture = base + UInt64(frame.sequence - 1) * 20_000
        guard frame.captureMonotonicUs == expectedCapture else { return .invalid }

        active.packets[frame.sequence] = frame
        active.highestSequence = max(active.highestSequence, frame.sequence)
        active.receivedFrames += 1
        session = active
        if active.phase == .buffering, active.highestSequence >= 3 {
            prebufferLocked()
        }
        return .apply
    }

    private func prebufferLocked() {
        guard session?.phase == .buffering else { return }
        for _ in 0..<3 {
            guard decodeExpectedLocked() else { return }
        }
        guard let active = session else { return }
        route.activateLivePCM(generation: active.routeGeneration)
        session?.phase = .playing
        session?.eventSequence += 1
        if let started = session {
            send(.livePTTReceipt(LivePTTReceiptPayload(
                sessionId: started.start.sessionId,
                generation: started.start.generation,
                eventSequence: started.eventSequence,
                state: "audible_started",
                lastSequence: started.expectedSequence - 1,
                observedAtCoordMs: max(1, coordinatorNowMs()))))
        }
    }

    private func tickLocked(nowMs: Int64) {
        guard let active = session else { return }
        if nowMs > active.start.startedAtCoordMs + active.start.maxDurationMs {
            failLocked(stage: "render", code: "max_duration")
            return
        }
        if active.phase == .playing || active.phase == .draining {
            if active.endSequence == nil || active.expectedSequence <= active.endSequence! {
                _ = decodeExpectedLocked()
            }
        }
        guard let current = session, current.phase == .draining else { return }
        if let deadline = current.drainDeadlineMs, nowMs >= deadline {
            terminateLocked(state: "ended", discard: true)
        } else if let end = current.endSequence,
                  current.expectedSequence > end,
                  route.livePCMBufferedFrames == 0 {
            terminateLocked(state: "ended", discard: false)
        }
    }

    @discardableResult
    private func decodeExpectedLocked() -> Bool {
        guard var active = session else { return false }
        let sequence = active.expectedSequence
        do {
            if let packet = active.packets.removeValue(forKey: sequence) {
                let count = try decoder.decode(
                    packet: packet.payload, fec: false, into: decodeScratch)
                guard count == Self.frameSamples else {
                    throw MacLiveDecodeError.conversionFailed
                }
                active.consecutiveConcealments = 0
            } else if let next = active.packets[sequence + 1] {
                do {
                    let count = try decoder.decode(
                        packet: next.payload, fec: true, into: decodeScratch)
                    guard count == Self.frameSamples else {
                        throw MacLiveDecodeError.conversionFailed
                    }
                    active.fecFrames += 1
                    active.consecutiveConcealments += 1
                } catch MacLiveDecodeError.fecUnavailable {
                    concealLocked(into: decodeScratch,
                                  consecutive: active.consecutiveConcealments)
                    active.plcFrames += 1
                    active.consecutiveConcealments += 1
                }
            } else {
                guard active.highestSequence >= sequence || active.endSequence != nil else {
                    session = active
                    return false
                }
                concealLocked(into: decodeScratch,
                              consecutive: active.consecutiveConcealments)
                active.plcFrames += 1
                active.consecutiveConcealments += 1
            }

            guard active.consecutiveConcealments <= Self.maxConsecutiveConcealments else {
                active.failedFrames += 1
                session = active
                failLocked(stage: "jitter", code: "concealment_exhausted")
                return false
            }
            let written = route.writeLivePCM(
                generation: active.routeGeneration,
                samples: decodeScratch.baseAddress!,
                count: Self.frameSamples)
            guard written == Self.frameSamples else {
                active.failedFrames += 1
                session = active
                failLocked(stage: "render", code: "buffer_full")
                return false
            }
            lastPCM.baseAddress!.update(
                from: decodeScratch.baseAddress!, count: Self.frameSamples)
            hasLastPCM = true
            active.decodedFrames += 1
            active.expectedSequence += 1
            session = active
            return true
        } catch {
            active.failedFrames += 1
            session = active
            failLocked(stage: "decode", code: "decode_failed")
            return false
        }
    }

    private func concealLocked(
        into output: UnsafeMutableBufferPointer<Float>,
        consecutive: Int
    ) {
        guard hasLastPCM else {
            output.baseAddress!.update(repeating: 0, count: output.count)
            return
        }
        let attenuation = max(0.2, powf(0.86, Float(consecutive + 1)))
        for index in 0..<output.count {
            let edge = min(Float(index) / 96, Float(output.count - index) / 96, 1)
            output[index] = lastPCM[index] * attenuation * edge
        }
    }

    private func failLocked(stage: String, code: String) {
        guard var active = session else { return }
        active.eventSequence += 1
        send(.livePTTFailed(LivePTTFailedPayload(
            sessionId: active.start.sessionId,
            generation: active.start.generation,
            eventSequence: active.eventSequence,
            stage: stage,
            code: code,
            failedAtCoordMs: max(1, coordinatorNowMs()))))
        session = active
        terminateLocked(state: "failed", discard: true)
    }

    private func terminateLocked(state: String, discard: Bool) {
        guard var active = session else { return }
        active.eventSequence += 1
        route.stopLivePCM(
            generation: active.routeGeneration,
            discard: discard)
        send(.livePTTReceipt(LivePTTReceiptPayload(
            sessionId: active.start.sessionId,
            generation: active.start.generation,
            eventSequence: active.eventSequence,
            state: state,
            lastSequence: active.expectedSequence > 1
                ? active.expectedSequence - 1 : nil,
            observedAtCoordMs: max(1, coordinatorNowMs()))))
        session = nil
        timer?.cancel()
        timer = nil
        decoder.reset()
        hasLastPCM = false
    }

    private func armTimerLocked() {
        guard automaticTick else { return }
        timer?.cancel()
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(
            deadline: .now() + .milliseconds(LivePTTConstants.frameMs),
            repeating: .milliseconds(LivePTTConstants.frameMs),
            leeway: .milliseconds(2))
        timer.setEventHandler { [weak self] in
            guard let self else { return }
            self.tickLocked(nowMs: self.coordinatorNowMs())
        }
        self.timer = timer
        timer.resume()
    }

    private static func decodeSessionId(_ value: String) -> [UInt8]? {
        guard value.count == 32 else { return nil }
        var bytes: [UInt8] = []
        bytes.reserveCapacity(16)
        var index = value.startIndex
        while index < value.endIndex {
            let next = value.index(index, offsetBy: 2)
            guard let byte = UInt8(value[index..<next], radix: 16) else { return nil }
            bytes.append(byte)
            index = next
        }
        return bytes
    }
}
