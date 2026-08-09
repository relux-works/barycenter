import AVFAudio
import AudioToolbox
import CoreAudio
import Foundation

struct MacCaptureInputDeviceRecord: Equatable, Sendable {
    let audioDeviceID: AudioDeviceID
    let device: MacCaptureDevice
}

protocol MacCaptureInputDeviceDiscovering: AnyObject {
    func inputDevices() -> [MacCaptureInputDeviceRecord]
}

protocol MacInputOnlyAudioSession: AnyObject {
    var configurationChangeObject: AnyObject { get }

    func selectInputDevice(_ id: AudioDeviceID) throws
    func inputFormat() -> AVAudioFormat
    func installTap(
        format: AVAudioFormat,
        handler: @escaping @Sendable (AVAudioPCMBuffer) -> Void
    )
    func removeTap()
    func prepare()
    func start() throws
    func stop()
    func reset()
}

final class SystemMacCaptureInputDeviceDiscovery:
    MacCaptureInputDeviceDiscovering, @unchecked Sendable
{
    func inputDevices() -> [MacCaptureInputDeviceRecord] {
        let defaultID = Self.defaultInputDevice()
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
            guard Self.hasInputStreams(id),
                let uid = Self.deviceUID(id),
                let name = Self.deviceName(id)
            else {
                return nil
            }
            return MacCaptureInputDeviceRecord(
                audioDeviceID: id,
                device: MacCaptureDevice(
                    id: uid,
                    name: name,
                    isDefault: id == defaultID))
        }.sorted {
            if $0.device.isDefault != $1.device.isDefault {
                return $0.device.isDefault
            }
            return $0.device.name.localizedCaseInsensitiveCompare($1.device.name)
                == .orderedAscending
        }
    }

    private static func defaultInputDevice() -> AudioDeviceID? {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDefaultInputDevice,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var id = AudioDeviceID(0)
        var size = UInt32(MemoryLayout<AudioDeviceID>.size)
        guard
            AudioObjectGetPropertyData(
                AudioObjectID(kAudioObjectSystemObject),
                &address,
                0,
                nil,
                &size,
                &id) == noErr,
            id != 0
        else {
            return nil
        }
        return id
    }

    private static func hasInputStreams(_ id: AudioDeviceID) -> Bool {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioDevicePropertyStreams,
            mScope: kAudioDevicePropertyScopeInput,
            mElement: kAudioObjectPropertyElementMain)
        var size: UInt32 = 0
        return AudioObjectGetPropertyDataSize(id, &address, 0, nil, &size) == noErr
            && size > 0
    }

    private static func deviceUID(_ id: AudioDeviceID) -> String? {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioDevicePropertyDeviceUID,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var uid: Unmanaged<CFString>?
        var size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(id, &address, 0, nil, &size, &uid) == noErr,
            let value = uid?.takeUnretainedValue()
        else {
            return nil
        }
        return value as String
    }

    private static func deviceName(_ id: AudioDeviceID) -> String? {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioObjectPropertyName,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var name: Unmanaged<CFString>?
        var size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(id, &address, 0, nil, &size, &name) == noErr,
            let value = name?.takeUnretainedValue()
        else {
            return nil
        }
        return value as String
    }
}

private final class SystemMacInputOnlyAudioSession:
    MacInputOnlyAudioSession, @unchecked Sendable
{
    private let engine = AVAudioEngine()

    var configurationChangeObject: AnyObject { engine }

    func selectInputDevice(_ id: AudioDeviceID) throws {
        let input = engine.inputNode
        guard let unit = input.audioUnit else {
            throw NSError(
                domain: "works.relux.pulsar.input-only",
                code: 1)
        }
        var selected = id
        let status = AudioUnitSetProperty(
            unit,
            kAudioOutputUnitProperty_CurrentDevice,
            kAudioUnitScope_Global,
            0,
            &selected,
            UInt32(MemoryLayout<AudioDeviceID>.size))
        guard status == noErr else {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status))
        }
    }

    func inputFormat() -> AVAudioFormat {
        engine.inputNode.outputFormat(forBus: 0)
    }

    func installTap(
        format: AVAudioFormat,
        handler: @escaping @Sendable (AVAudioPCMBuffer) -> Void
    ) {
        engine.inputNode.installTap(
            onBus: 0,
            bufferSize: 1_024,
            format: format
        ) { buffer, _ in
            handler(buffer)
        }
    }

    func removeTap() {
        engine.inputNode.removeTap(onBus: 0)
    }

    func prepare() {
        engine.prepare()
    }

    func start() throws {
        try engine.start()
    }

    func stop() {
        engine.stop()
    }

    func reset() {
        engine.reset()
    }
}

private final class MacInputCaptureSampleMailbox: @unchecked Sendable {
    enum Offer {
        case accepted
        case scheduleDrain
        case overflow
        case terminal
    }

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
            for channel in 0..<channelCount {
                mono += channels[channel][frame] * scale
            }
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

/// Microphone-only AVAudioEngine backend for ordinary recorded clips and the
/// local recording self-test. Its audio-session interface exposes input
/// operations only, so this path cannot configure voice processing or inspect,
/// validate, or gate startup on an output route.
public final class MacAVAudioCaptureBackend:
    MacMicrophoneCaptureBackend, @unchecked Sendable
{
    private let queue = DispatchQueue(label: "works.relux.pulsar.mac-input-capture")
    private let deviceDiscovery: MacCaptureInputDeviceDiscovering
    private let makeSession: () -> MacInputOnlyAudioSession
    private let log: Logger?
    private let safetyProcessor = MacCaptureInputSafetyProcessor()
    private var session: MacInputOnlyAudioSession?
    private var configurationObserver: NSObjectProtocol?
    private var active = false
    private var tapInstalled = false
    private var sourceRate: Double = 48_000
    private var resamplePosition = 0.0
    private var previousSample: Float?
    private var sampleMailbox: MacInputCaptureSampleMailbox?
    private var drainSignal: DispatchSourceUserDataAdd?
    private var failureSignal: DispatchSourceUserDataAdd?

    public convenience init(log: Logger? = nil) {
        self.init(
            deviceDiscovery: SystemMacCaptureInputDeviceDiscovery(),
            makeSession: { SystemMacInputOnlyAudioSession() },
            log: log)
    }

    init(
        deviceDiscovery: MacCaptureInputDeviceDiscovering,
        makeSession: @escaping () -> MacInputOnlyAudioSession,
        log: Logger? = nil
    ) {
        self.deviceDiscovery = deviceDiscovery
        self.makeSession = makeSession
        self.log = log
    }

    deinit {
        stop()
    }

    public func availableDevices() -> [MacCaptureDevice] {
        deviceDiscovery.inputDevices().map(\.device)
    }

    public func start(
        selectedDeviceID: String?,
        onSamples: @escaping @Sendable ([Float]) -> Void,
        onFailure: @escaping @Sendable () -> Void
    ) throws {
        try queue.sync {
            guard !active else {
                throw MacCaptureEngineError.alreadyActive
            }
            let records = deviceDiscovery.inputDevices()
            guard !records.isEmpty else {
                throw MacCaptureEngineError.noInputDevice
            }
            let selectedRecord: MacCaptureInputDeviceRecord?
            if let selectedDeviceID {
                guard let record = records.first(where: { $0.device.id == selectedDeviceID }) else {
                    throw MacCaptureEngineError.selectedDeviceUnavailable
                }
                selectedRecord = record
            } else {
                selectedRecord = nil
            }

            let session = makeSession()
            if let selectedRecord {
                do {
                    try session.selectInputDevice(selectedRecord.audioDeviceID)
                } catch {
                    release(session: session, tapInstalled: false, mailbox: nil)
                    let diagnostic = Self.diagnostic(
                        stage: .inputSelection,
                        error: error)
                    logStartupDiagnostic(diagnostic)
                    throw MacCaptureEngineError.selectedDeviceUnavailable
                }
            }

            let native = session.inputFormat()
            guard native.sampleRate > 0, native.channelCount > 0,
                let tapFormat = AVAudioFormat(
                    commonFormat: .pcmFormatFloat32,
                    sampleRate: native.sampleRate,
                    channels: native.channelCount,
                    interleaved: false)
            else {
                release(session: session, tapInstalled: false, mailbox: nil)
                let diagnostic = MacCaptureStartupDiagnostic(
                    stage: .inputOnly,
                    attempt: 1,
                    elapsedMilliseconds: 0,
                    cause: .invalidInputFormat(
                        sampleRate: native.sampleRate,
                        channels: native.channelCount))
                logStartupDiagnostic(diagnostic)
                throw MacCaptureEngineError.backendStartupFailed(diagnostic)
            }

            safetyProcessor.reset()
            resamplePosition = 0
            previousSample = nil
            let mailbox = MacInputCaptureSampleMailbox()
            let drainSignal = DispatchSource.makeUserDataAddSource(queue: queue)
            let failureSignal = DispatchSource.makeUserDataAddSource(queue: queue)
            drainSignal.setEventHandler { [weak self, mailbox] in
                self?.drain(mailbox: mailbox, onSamples: onSamples)
            }
            failureSignal.setEventHandler { [weak self] in
                guard let self, self.active else { return }
                onFailure()
            }
            drainSignal.resume()
            failureSignal.resume()

            session.installTap(format: tapFormat) { buffer in
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
            var startedSourceRate = native.sampleRate
            do {
                session.prepare()
                try session.start()
                let observed = session.inputFormat()
                guard observed.sampleRate > 0, observed.channelCount > 0 else {
                    throw MacCaptureStartupDiagnostic(
                        stage: .inputOnly,
                        attempt: 1,
                        elapsedMilliseconds: 0,
                        cause: .invalidInputFormat(
                            sampleRate: observed.sampleRate,
                            channels: observed.channelCount))
                }
                startedSourceRate = observed.sampleRate
            } catch {
                release(session: session, tapInstalled: true, mailbox: mailbox)
                drainSignal.cancel()
                failureSignal.cancel()
                safetyProcessor.reset()
                let diagnostic: MacCaptureStartupDiagnostic
                if let typed = error as? MacCaptureStartupDiagnostic {
                    diagnostic = typed
                } else {
                    diagnostic = Self.diagnostic(stage: .inputOnly, error: error)
                }
                logStartupDiagnostic(diagnostic)
                throw MacCaptureEngineError.backendStartupFailed(diagnostic)
            }

            sourceRate = startedSourceRate
            sampleMailbox = mailbox
            self.drainSignal = drainSignal
            self.failureSignal = failureSignal
            self.session = session
            tapInstalled = true
            configurationObserver = NotificationCenter.default.addObserver(
                forName: .AVAudioEngineConfigurationChange,
                object: session.configurationChangeObject,
                queue: nil
            ) { [weak self] _ in
                guard let self else { return }
                self.queue.async {
                    guard self.active else { return }
                    onFailure()
                }
            }
            active = true
        }
    }

    public func stop() {
        queue.sync {
            guard active || session != nil else { return }
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
            if let session {
                release(
                    session: session,
                    tapInstalled: tapInstalled,
                    mailbox: nil)
            }
            self.session = nil
            tapInstalled = false
            active = false
            safetyProcessor.reset()
            previousSample = nil
            resamplePosition = 0
        }
    }

    private func release(
        session: MacInputOnlyAudioSession,
        tapInstalled: Bool,
        mailbox: MacInputCaptureSampleMailbox?
    ) {
        session.stop()
        if tapInstalled {
            session.removeTap()
        }
        session.reset()
        mailbox?.reset()
    }

    private func drain(
        mailbox: MacInputCaptureSampleMailbox,
        onSamples: @escaping @Sendable ([Float]) -> Void
    ) {
        guard active else { return }
        while active {
            if let mono = mailbox.pop() {
                var normalized = resampleTo48k(mono)
                _ = safetyProcessor.process(&normalized)
                if !normalized.isEmpty {
                    onSamples(normalized)
                }
                continue
            }
            if !mailbox.finishDrain() {
                break
            }
        }
    }

    private func logStartupDiagnostic(_ diagnostic: MacCaptureStartupDiagnostic) {
        log?.warn(
            "mac input-only capture startup failed",
            diagnostic.redactedLogFields(decision: .fail))
    }

    private static func diagnostic(
        stage: MacCaptureStartupStage,
        error: Error
    ) -> MacCaptureStartupDiagnostic {
        let cocoa = error as NSError
        let cause: MacCaptureStartupCause
        if let status = coreAudioStatus(from: cocoa) {
            cause = .coreAudio(status: status)
        } else {
            cause = .engine(domain: cocoa.domain, code: cocoa.code)
        }
        return MacCaptureStartupDiagnostic(
            stage: stage,
            attempt: 1,
            elapsedMilliseconds: 0,
            cause: cause)
    }

    private static func coreAudioStatus(
        from error: NSError,
        depth: Int = 0
    ) -> Int32? {
        guard depth < 4 else { return nil }
        if error.domain == NSOSStatusErrorDomain,
            let status = Int32(exactly: error.code)
        {
            return status
        }
        if let underlying = error.userInfo[NSUnderlyingErrorKey] as? NSError {
            return coreAudioStatus(from: underlying, depth: depth + 1)
        }
        return nil
    }

    private func resampleTo48k(_ input: [Float]) -> [Float] {
        guard sourceRate != 48_000 else {
            previousSample = input.last
            return input
        }
        var source = input
        if let previousSample {
            source.insert(previousSample, at: 0)
        }
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
            output.append(
                source[lower] + (source[lower + 1] - source[lower]) * fraction)
            position += step
        }
        resamplePosition = position - Double(source.count - 1)
        previousSample = source.last
        return output
    }
}
