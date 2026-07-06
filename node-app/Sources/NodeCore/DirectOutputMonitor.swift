// Direct delivery mode (spec v1.3, 6.2 item 8): NodeApp plays into the macOS
// default output; this monitor keeps that output on the configured device —
// polls the current default, reports degraded when it drifts (e.g. the
// AirPlay/BT speaker dropped and macOS fell back to built-in), and tries to
// switch back with the same backoff discipline as AirfoilBridge.

import CoreAudio
import Foundation

public final class DirectOutputMonitor {
    private let desired: String? // nil = accept whatever output is current
    private let pollS: Int
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.output-monitor")
    private var timer: DispatchSourceTimer?
    private var reconnectAttempt = 0
    private var nextReconnectAt = Date.distantPast

    /// Same channel as AirfoilBridge: speaker states + degraded flag.
    public var onStates: (([SpeakerState], _ degraded: Bool) -> Void)?

    public init(desiredDeviceName: String?, pollS: Int, log: Logger) {
        desired = desiredDeviceName?.isEmpty == false ? desiredDeviceName : nil
        self.pollS = max(1, pollS)
        self.log = log
    }

    public func start() {
        queue.async {
            self.poll()
            let t = DispatchSource.makeTimerSource(queue: self.queue)
            t.schedule(deadline: .now() + .seconds(self.pollS), repeating: .seconds(self.pollS))
            t.setEventHandler { [weak self] in self?.poll() }
            t.resume()
            self.timer = t
        }
    }

    public func stop() {
        queue.sync {
            timer?.cancel()
            timer = nil
        }
    }

    private func poll() {
        let current = Self.currentOutputName()
        guard let desired else {
            onStates?([SpeakerState(name: current ?? "system output", connected: current != nil)], false)
            return
        }
        if current == desired {
            reconnectAttempt = 0
            nextReconnectAt = .distantPast
            onStates?([SpeakerState(name: desired, connected: true)], false)
            return
        }
        // Drifted (device dropped, macOS fell back): spec 4.4 degradation.
        let now = Date()
        if now >= nextReconnectAt {
            log.info("output drifted, reconnecting", [
                "current": current ?? "none", "desired": desired, "attempt": reconnectAttempt,
            ])
            if Self.setDefaultOutput(named: desired) {
                log.info("output restored", ["device": desired])
                reconnectAttempt = 0
                nextReconnectAt = .distantPast
                onStates?([SpeakerState(name: desired, connected: true)], false)
                return
            }
            nextReconnectAt = now.addingTimeInterval(Self.backoffDelay(attempt: reconnectAttempt))
            reconnectAttempt += 1
        }
        onStates?([SpeakerState(name: desired, connected: false)], true)
    }

    static func backoffDelay(attempt: Int) -> TimeInterval {
        min(5 * pow(2, Double(attempt)), 60)
    }

    // MARK: CoreAudio helpers

    public static func currentOutputName() -> String? {
        var addr = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDefaultOutputDevice,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var dev = AudioDeviceID(0)
        var size = UInt32(MemoryLayout<AudioDeviceID>.size)
        guard AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &addr, 0, nil, &size, &dev) == noErr,
              dev != 0 else { return nil }
        return deviceName(dev)
    }

    static func deviceName(_ dev: AudioDeviceID) -> String? {
        var nameAddr = AudioObjectPropertyAddress(
            mSelector: kAudioObjectPropertyName,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var name: Unmanaged<CFString>?
        var nsize = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(dev, &nameAddr, 0, nil, &nsize, &name) == noErr,
              let cf = name?.takeRetainedValue() else { return nil }
        return cf as String
    }

    /// Lists the names of all output-capable devices (menu-bar picker, R2).
    public static func listOutputDevices() -> [String] {
        var addr = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDevices,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var size = UInt32(0)
        guard AudioObjectGetPropertyDataSize(AudioObjectID(kAudioObjectSystemObject), &addr, 0, nil, &size) == noErr else { return [] }
        var devs = [AudioDeviceID](repeating: 0, count: Int(size) / MemoryLayout<AudioDeviceID>.size)
        guard AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &addr, 0, nil, &size, &devs) == noErr else { return [] }
        var out: [String] = []
        for dev in devs {
            var streamsAddr = AudioObjectPropertyAddress(
                mSelector: kAudioDevicePropertyStreams,
                mScope: kAudioObjectPropertyScopeOutput,
                mElement: kAudioObjectPropertyElementMain)
            var ssize = UInt32(0)
            AudioObjectGetPropertyDataSize(dev, &streamsAddr, 0, nil, &ssize)
            guard ssize > 0 else { continue }
            // System-private aggregates (CADefaultDeviceAggregate-…) are
            // plumbing, not speakers a person would pick (Timur, beta).
            var ttAddr = AudioObjectPropertyAddress(
                mSelector: kAudioDevicePropertyTransportType,
                mScope: kAudioObjectPropertyScopeGlobal,
                mElement: kAudioObjectPropertyElementMain)
            var tt = UInt32(0)
            var ttSize = UInt32(MemoryLayout<UInt32>.size)
            AudioObjectGetPropertyData(dev, &ttAddr, 0, nil, &ttSize, &tt)
            if tt == kAudioDeviceTransportTypeAggregate { continue }
            if let name = deviceName(dev), !out.contains(name), !name.hasPrefix("CADefault") {
                out.append(name)
            }
        }
        return out
    }

    /// Sets the system default output to the device with this exact name.
    /// Returns false when the device is not present (e.g. speaker still off).
    public static func setDefaultOutput(named target: String) -> Bool {
        var addr = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDevices,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var size = UInt32(0)
        guard AudioObjectGetPropertyDataSize(AudioObjectID(kAudioObjectSystemObject), &addr, 0, nil, &size) == noErr else { return false }
        var devs = [AudioDeviceID](repeating: 0, count: Int(size) / MemoryLayout<AudioDeviceID>.size)
        guard AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &addr, 0, nil, &size, &devs) == noErr else { return false }

        for dev in devs where deviceName(dev) == target {
            var streamsAddr = AudioObjectPropertyAddress(
                mSelector: kAudioDevicePropertyStreams,
                mScope: kAudioObjectPropertyScopeOutput,
                mElement: kAudioObjectPropertyElementMain)
            var ssize = UInt32(0)
            AudioObjectGetPropertyDataSize(dev, &streamsAddr, 0, nil, &ssize)
            guard ssize > 0 else { continue } // not an output device
            var target = dev
            var defAddr = AudioObjectPropertyAddress(
                mSelector: kAudioHardwarePropertyDefaultOutputDevice,
                mScope: kAudioObjectPropertyScopeGlobal,
                mElement: kAudioObjectPropertyElementMain)
            return AudioObjectSetPropertyData(AudioObjectID(kAudioObjectSystemObject), &defAddr, 0, nil,
                                              UInt32(MemoryLayout<AudioDeviceID>.size), &target) == noErr
        }
        return false
    }
}
