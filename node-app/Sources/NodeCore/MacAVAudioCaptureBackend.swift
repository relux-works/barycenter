import AVFAudio
import AudioToolbox
import CoreAudio
import Foundation

/// AVAudioEngine/CoreAudio implementation. The tap is normalized to mono
/// float samples and linearly resampled to the shared 48 kHz writer contract.
/// It owns no storage, cue, upload, or lifecycle policy.
public final class MacAVAudioCaptureBackend: MacMicrophoneCaptureBackend, @unchecked Sendable {
    private let queue = DispatchQueue(label: "works.relux.pulsar.mac-capture-backend")
    private var engine: AVAudioEngine?
    private var configurationObserver: NSObjectProtocol?
    private var active = false
    private var sourceRate: Double = 48_000
    private var resamplePosition = 0.0
    private var previousSample: Float?

    public init() {}

    deinit { stop() }

    public func availableDevices() -> [MacCaptureDevice] {
        Self.inputDevices()
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
            let native = input.outputFormat(forBus: 0)
            guard native.sampleRate > 0, native.channelCount > 0,
                  let tapFormat = AVAudioFormat(
                    commonFormat: .pcmFormatFloat32,
                    sampleRate: native.sampleRate,
                    channels: native.channelCount,
                    interleaved: false) else {
                throw MacCaptureEngineError.backendUnavailable
            }
            sourceRate = native.sampleRate
            resamplePosition = 0
            previousSample = nil
            input.installTap(onBus: 0, bufferSize: 1_024, format: tapFormat) {
                [weak self] buffer, _ in
                guard let self else { return }
                let mono = Self.downmix(buffer)
                guard !mono.isEmpty else { return }
                let normalized = self.resampleTo48k(mono)
                if !normalized.isEmpty { onSamples(normalized) }
            }
            engine.prepare()
            do {
                try engine.start()
            } catch {
                input.removeTap(onBus: 0)
                throw MacCaptureEngineError.backendUnavailable
            }
            configurationObserver = NotificationCenter.default.addObserver(
                forName: .AVAudioEngineConfigurationChange,
                object: engine,
                queue: nil
            ) { [weak self] _ in
                guard let self else { return }
                self.queue.async {
                    guard self.active else { return }
                    onFailure()
                }
            }
            self.engine = engine
            active = true
        }
    }

    public func stop() {
        queue.sync {
            guard active || engine != nil else { return }
            if let observer = configurationObserver {
                NotificationCenter.default.removeObserver(observer)
            }
            configurationObserver = nil
            if let engine {
                engine.inputNode.removeTap(onBus: 0)
                engine.stop()
                engine.reset()
            }
            self.engine = nil
            active = false
            previousSample = nil
            resamplePosition = 0
        }
    }

    private static func downmix(_ buffer: AVAudioPCMBuffer) -> [Float] {
        guard let channels = buffer.floatChannelData else { return [] }
        let frameCount = Int(buffer.frameLength)
        let channelCount = Int(buffer.format.channelCount)
        guard frameCount > 0, channelCount > 0 else { return [] }
        var mono = [Float](repeating: 0, count: frameCount)
        let scale = Float(1.0 / Double(channelCount))
        for channel in 0..<channelCount {
            let source = channels[channel]
            for frame in 0..<frameCount { mono[frame] += source[frame] * scale }
        }
        return mono
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
