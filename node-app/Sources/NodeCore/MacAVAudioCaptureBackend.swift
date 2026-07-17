import AVFAudio
import AudioToolbox
import CoreAudio
import Foundation

/// Fixed-storage handoff from the Core Audio tap to the backend worker. The
/// tap only downmixes into this bounded ring and uses nonblocking signalling;
/// resampling, quality processing and client callbacks stay off the realtime
/// boundary.
private final class MacAVCaptureSampleMailbox: @unchecked Sendable {
    enum Offer { case accepted, scheduleDrain, overflow, terminal }

    private let lock = NSLock()
    private let storage: UnsafeMutablePointer<Float>
    private let capacity: Int
    private var head = 0
    private var tail = 0
    private var count = 0
    private var drainScheduled = false
    private var terminal = false

    init(capacity: Int = 16_384) {
        self.capacity = capacity
        storage = .allocate(capacity: capacity)
        storage.initialize(repeating: 0, count: capacity)
    }

    deinit {
        storage.deinitialize(count: capacity)
        storage.deallocate()
    }

    func offer(_ buffer: AVAudioPCMBuffer) -> Offer {
        guard lock.try() else { return .overflow }
        defer { lock.unlock() }
        guard !terminal else { return .terminal }
        guard let channels = buffer.floatChannelData else {
            terminal = true
            return .overflow
        }
        let frames = Int(buffer.frameLength)
        let channelCount = Int(buffer.format.channelCount)
        guard frames > 0, (1...8).contains(channelCount), frames <= capacity - count else {
            terminal = true
            return .overflow
        }
        let scale = Float(1.0 / Double(channelCount))
        for frame in 0..<frames {
            var mono: Float = 0
            for channel in 0..<channelCount { mono += channels[channel][frame] * scale }
            storage[head] = mono
            head = (head + 1) % capacity
        }
        count += frames
        if !drainScheduled {
            drainScheduled = true
            return .scheduleDrain
        }
        return .accepted
    }

    func pop(maxCount: Int = 4_096) -> [Float]? {
        lock.lock()
        defer { lock.unlock() }
        guard !terminal, count > 0 else { return nil }
        let amount = min(count, maxCount)
        var result = [Float](repeating: 0, count: amount)
        for index in 0..<amount {
            result[index] = storage[tail]
            tail = (tail + 1) % capacity
        }
        count -= amount
        return result
    }

    /// Called only by the worker after a nil pop. A producer racing the drain
    /// either leaves data for the current worker or observes a cleared signal
    /// and schedules the next one; no wakeup can be lost.
    func finishDrain() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        guard !terminal else {
            drainScheduled = false
            return false
        }
        if count > 0 { return true }
        drainScheduled = false
        return false
    }

    func reset() {
        lock.lock()
        defer { lock.unlock() }
        head = 0
        tail = 0
        count = 0
        drainScheduled = false
        terminal = false
    }
}

/// AVAudioEngine/CoreAudio implementation. The tap is normalized to mono
/// float samples and linearly resampled to the shared 48 kHz writer contract.
/// It owns no storage, cue, upload, or lifecycle policy.
public final class MacAVAudioCaptureBackend:
    MacMicrophoneCaptureBackend, MacCaptureQualityBackendConfiguring, @unchecked Sendable
{
    private let queue = DispatchQueue(label: "works.relux.pulsar.mac-capture-backend")
    private let routeResolver: MacCaptureOutputRouteResolving
    private let safetyProcessor = MacCaptureInputSafetyProcessor()
    private var engine: AVAudioEngine?
    private var configurationObserver: NSObjectProtocol?
    private var active = false
    private var sourceRate: Double = 48_000
    private var resamplePosition = 0.0
    private var previousSample: Float?
    private var qualityWorkflow = "recorded_clip"
    private var qualityRequest: MacCaptureQualityRequest = .legacyUnprocessed
    private var qualitySession: MacCaptureQualitySession?
    private var qualityStateHandler: (@Sendable (CaptureQualityState?) -> Void)?
    private var sampleMailbox: MacAVCaptureSampleMailbox?
    private var drainSignal: DispatchSourceUserDataAdd?
    private var failureSignal: DispatchSourceUserDataAdd?

    public init(
        routeResolver: MacCaptureOutputRouteResolving = SystemMacCaptureOutputRouteResolver()
    ) {
        self.routeResolver = routeResolver
    }

    deinit { stop() }

    public func availableDevices() -> [MacCaptureDevice] {
        Self.inputDevices()
    }

    public func configureCaptureQuality(
        workflow: String,
        request: MacCaptureQualityRequest,
        onState: @escaping @Sendable (CaptureQualityState?) -> Void
    ) {
        queue.sync {
            guard !active else { return }
            qualityWorkflow = workflow
            qualityRequest = request
            qualityStateHandler = onState
        }
    }

    public func start(
        selectedDeviceID: String?,
        onSamples: @escaping @Sendable ([Float]) -> Void,
        onFailure: @escaping @Sendable () -> Void
    ) throws {
        try queue.sync {
            guard !active else { throw MacCaptureEngineError.alreadyActive }
            let devices = Self.inputDevices()
            guard !devices.isEmpty else { throw MacCaptureEngineError.noInputDevice }
            let selected: AudioDeviceID?
            if let selectedDeviceID {
                guard let parsed = UInt32(selectedDeviceID),
                      devices.contains(where: { $0.id == selectedDeviceID }) else {
                    throw MacCaptureEngineError.selectedDeviceUnavailable
                }
                selected = AudioDeviceID(parsed)
            } else {
                selected = nil
            }

            let engine = AVAudioEngine()
            let input = engine.inputNode
            if let selected, let unit = input.audioUnit {
                var device = selected
                let status = AudioUnitSetProperty(
                    unit,
                    kAudioOutputUnitProperty_CurrentDevice,
                    kAudioUnitScope_Global,
                    0,
                    &device,
                    UInt32(MemoryLayout<AudioDeviceID>.size))
                guard status == noErr else { throw MacCaptureEngineError.backendUnavailable }
            }
            let resolvedMode = routeResolver.resolvedMode()
            var voiceProcessingEnabled = false
            if qualityRequest.processingRequested {
                do {
                    try input.setVoiceProcessingEnabled(true)
                    input.isVoiceProcessingBypassed = false
                    input.isVoiceProcessingAGCEnabled = true
                    _ = engine.outputNode
                    voiceProcessingEnabled = input.isVoiceProcessingEnabled
                } catch {
                    voiceProcessingEnabled = false
                }
            }
            let session = try makeQualitySession(
                resolvedMode: resolvedMode,
                voiceProcessingEnabled: voiceProcessingEnabled)
            let native = input.outputFormat(forBus: 0)
            guard native.sampleRate > 0, native.channelCount > 0,
                  let tapFormat = AVAudioFormat(
                    commonFormat: .pcmFormatFloat32,
                    sampleRate: native.sampleRate,
                    channels: native.channelCount,
                    interleaved: false) else {
                throw MacCaptureEngineError.backendUnavailable
            }
            qualitySession = session
            safetyProcessor.reset()
            sourceRate = native.sampleRate
            resamplePosition = 0
            previousSample = nil
            let mailbox = MacAVCaptureSampleMailbox()
            let drainSignal = DispatchSource.makeUserDataAddSource(queue: queue)
            let failureSignal = DispatchSource.makeUserDataAddSource(queue: queue)
            drainSignal.setEventHandler { [weak self, mailbox] in
                self?.drain(mailbox: mailbox, onSamples: onSamples)
            }
            failureSignal.setEventHandler { [weak self] in
                guard let self, self.active else { return }
                if let session = self.qualitySession {
                    self.emitQuality(CaptureQualityState(
                        generation: session.generation,
                        workflow: session.workflow,
                        requestedMode: session.request.mode.rawValue,
                        resolvedMode: session.resolvedMode,
                        lifecycle: "failed",
                        quality: "degraded",
                        aec: session.aec,
                        ns: session.ns,
                        agc: session.agc,
                        inputHealth: "processor_overrun",
                        reason: "processor_overrun",
                        updatedMonotonicMs: Self.monotonicMs(),
                        processorOverruns: 1))
                }
                onFailure()
            }
            drainSignal.resume()
            failureSignal.resume()
            input.installTap(onBus: 0, bufferSize: 1_024, format: tapFormat) {
                buffer, _ in
                // BEGIN MAC AV CAPTURE CALLBACK
                switch mailbox.offer(buffer) {
                case .scheduleDrain:
                    drainSignal.add(data: 1)
                case .overflow:
                    failureSignal.add(data: 1)
                case .accepted, .terminal:
                    break
                }
                // END MAC AV CAPTURE CALLBACK
            }
            emitQuality(session.state(lifecycle: "preparing", nowMs: Self.monotonicMs()))
            engine.prepare()
            do {
                try engine.start()
            } catch {
                input.removeTap(onBus: 0)
                drainSignal.cancel()
                failureSignal.cancel()
                mailbox.reset()
                qualitySession = nil
                safetyProcessor.reset()
                emitQuality(nil)
                throw MacCaptureEngineError.backendUnavailable
            }
            sampleMailbox = mailbox
            self.drainSignal = drainSignal
            self.failureSignal = failureSignal
            configurationObserver = NotificationCenter.default.addObserver(
                forName: .AVAudioEngineConfigurationChange,
                object: engine,
                queue: nil
            ) { [weak self] _ in
                guard let self else { return }
                self.queue.async {
                    guard self.active else { return }
                    if let session = self.qualitySession {
                        self.emitQuality(session.state(
                            lifecycle: "reconfiguring", nowMs: Self.monotonicMs()))
                    }
                    onFailure()
                }
            }
            self.engine = engine
            active = true
            emitQuality(session.state(lifecycle: "capturing", nowMs: Self.monotonicMs()))
        }
    }

    public func stop() {
        queue.sync {
            guard active || engine != nil else { return }
            if let qualitySession {
                emitQuality(qualitySession.state(
                    lifecycle: "stopping", nowMs: Self.monotonicMs()))
            }
            if let observer = configurationObserver {
                NotificationCenter.default.removeObserver(observer)
            }
            configurationObserver = nil
            drainSignal?.cancel()
            failureSignal?.cancel()
            drainSignal = nil
            failureSignal = nil
            sampleMailbox?.reset()
            sampleMailbox = nil
            if let engine {
                engine.inputNode.removeTap(onBus: 0)
                engine.stop()
                engine.reset()
            }
            self.engine = nil
            active = false
            qualitySession = nil
            safetyProcessor.reset()
            previousSample = nil
            resamplePosition = 0
            emitQuality(nil)
        }
    }

    private func makeQualitySession(
        resolvedMode: String,
        voiceProcessingEnabled: Bool
    ) throws -> MacCaptureQualitySession {
        let request = qualityRequest
        let decision = MacCaptureQualityDecision.evaluate(
            request: request,
            resolvedMode: resolvedMode,
            voiceProcessingEnabled: voiceProcessingEnabled)
        if request.processingRequested && decision.quality != "accepted"
            && !request.degradedConsent
        {
            throw MacCaptureEngineError.captureQualityUnsupported
        }
        return MacCaptureQualitySession(
            generation: MacCaptureQualityGeneration.next(),
            workflow: qualityWorkflow,
            request: request,
            resolvedMode: resolvedMode,
            quality: decision.quality,
            aec: decision.aec,
            ns: decision.ns,
            agc: decision.agc,
            reason: decision.reason)
    }

    private func emitQuality(_ state: CaptureQualityState?) {
        qualityStateHandler?(state)
    }

    private func drain(
        mailbox: MacAVCaptureSampleMailbox,
        onSamples: @escaping @Sendable ([Float]) -> Void
    ) {
        guard active else { return }
        while active {
            if let mono = mailbox.pop() {
                var normalized = resampleTo48k(mono)
                if qualityRequest.processingRequested {
                    _ = safetyProcessor.process(&normalized)
                }
                if !normalized.isEmpty { onSamples(normalized) }
                continue
            }
            if !mailbox.finishDrain() { break }
        }
    }

    private static func monotonicMs() -> Int64 {
        Int64((ProcessInfo.processInfo.systemUptime * 1_000).rounded())
    }

    private func resampleTo48k(_ input: [Float]) -> [Float] {
        guard sourceRate != 48_000 else {
            previousSample = input.last
            return input
        }
        var source = input
        if let previousSample { source.insert(previousSample, at: 0) }
        guard source.count >= 2 else {
            previousSample = source.last
            return []
        }
        let step = sourceRate / 48_000
        var output: [Float] = []
        output.reserveCapacity(Int(Double(source.count) / step) + 2)
        var position = resamplePosition
        while position + 1 < Double(source.count) {
            let lower = Int(position)
            let fraction = Float(position - Double(lower))
            output.append(source[lower] + (source[lower + 1] - source[lower]) * fraction)
            position += step
        }
        resamplePosition = position - Double(source.count - 1)
        previousSample = source.last
        return output
    }

    private static func inputDevices() -> [MacCaptureDevice] {
        let defaultID = defaultInputDevice()
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDevices,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var size: UInt32 = 0
        let system = AudioObjectID(kAudioObjectSystemObject)
        guard AudioObjectGetPropertyDataSize(system, &address, 0, nil, &size) == noErr else {
            return []
        }
        var ids = [AudioDeviceID](
            repeating: 0,
            count: Int(size) / MemoryLayout<AudioDeviceID>.size)
        guard AudioObjectGetPropertyData(system, &address, 0, nil, &size, &ids) == noErr else {
            return []
        }
        return ids.compactMap { id in
            guard hasInputStreams(id), let name = deviceName(id) else { return nil }
            return MacCaptureDevice(id: String(id), name: name, isDefault: id == defaultID)
        }.sorted {
            if $0.isDefault != $1.isDefault { return $0.isDefault }
            return $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending
        }
    }

    private static func defaultInputDevice() -> AudioDeviceID? {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDefaultInputDevice,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var id = AudioDeviceID(0)
        var size = UInt32(MemoryLayout<AudioDeviceID>.size)
        guard AudioObjectGetPropertyData(
            AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size, &id) == noErr,
              id != 0 else { return nil }
        return id
    }

    private static func hasInputStreams(_ id: AudioDeviceID) -> Bool {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioDevicePropertyStreams,
            mScope: kAudioDevicePropertyScopeInput,
            mElement: kAudioObjectPropertyElementMain)
        var size: UInt32 = 0
        return AudioObjectGetPropertyDataSize(id, &address, 0, nil, &size) == noErr && size > 0
    }

    private static func deviceName(_ id: AudioDeviceID) -> String? {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioObjectPropertyName,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var name: Unmanaged<CFString>?
        var size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(id, &address, 0, nil, &size, &name) == noErr,
              let value = name?.takeUnretainedValue() else { return nil }
        return value as String
    }
}
