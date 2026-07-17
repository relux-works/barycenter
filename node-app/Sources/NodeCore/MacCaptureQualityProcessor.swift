import CoreAudio
import Foundation

public enum MacCaptureQualityMode: String, CaseIterable, Equatable, Sendable {
    case auto, speaker, headphone
}

public struct MacCaptureQualityRequest: Equatable, Sendable {
    public var mode: MacCaptureQualityMode
    public var processingRequested: Bool
    public var degradedConsent: Bool

    public init(
        mode: MacCaptureQualityMode,
        processingRequested: Bool = true,
        degradedConsent: Bool = false
    ) {
        self.mode = mode
        self.processingRequested = processingRequested
        self.degradedConsent = degradedConsent
    }

    /// Existing Phase-1 recording remains an explicitly unprocessed action.
    /// It never advertises capture_quality_v1 or claims accepted DSP.
    public static let legacyUnprocessed = MacCaptureQualityRequest(
        mode: .auto, processingRequested: false, degradedConsent: true)
}

public protocol MacCaptureQualityBackendConfiguring: AnyObject {
    func configureCaptureQuality(
        workflow: String,
        request: MacCaptureQualityRequest,
        onState: @escaping @Sendable (CaptureQualityState?) -> Void
    )
}

public protocol MacCaptureQualityWorkflowSelecting: AnyObject {
    func selectCaptureQualityWorkflow(_ workflow: String)
    func setCaptureQualityRequest(_ request: MacCaptureQualityRequest)
}

public protocol MacCaptureOutputRouteResolving: AnyObject {
    func resolvedMode() -> String
}

/// Conservative CoreAudio resolver. Unknown, remote, aggregate and ambiguous
/// outputs are never inferred accepted; physical route proof remains manual.
public final class SystemMacCaptureOutputRouteResolver:
    MacCaptureOutputRouteResolving, @unchecked Sendable
{
    public init() {}

    public func resolvedMode() -> String {
        guard let name = Self.defaultOutputName()?.lowercased() else { return "unknown" }
        let headphoneTokens = ["headphone", "headset", "airpods", "earbuds"]
        if headphoneTokens.contains(where: name.contains) { return "headphone" }
        let speakerTokens = ["speaker", "built-in output", "macbook"]
        if speakerTokens.contains(where: name.contains) { return "speaker" }
        return "unknown"
    }

    private static func defaultOutputName() -> String? {
        var address = AudioObjectPropertyAddress(
            mSelector: kAudioHardwarePropertyDefaultOutputDevice,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var device = AudioDeviceID(0)
        var size = UInt32(MemoryLayout<AudioDeviceID>.size)
        guard AudioObjectGetPropertyData(
            AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size, &device) == noErr,
            device != 0
        else { return nil }
        address = AudioObjectPropertyAddress(
            mSelector: kAudioObjectPropertyName,
            mScope: kAudioObjectPropertyScopeGlobal,
            mElement: kAudioObjectPropertyElementMain)
        var name: Unmanaged<CFString>?
        size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
        guard AudioObjectGetPropertyData(device, &address, 0, nil, &size, &name) == noErr,
              let value = name?.takeUnretainedValue()
        else { return nil }
        return value as String
    }
}

struct MacCaptureInputMetrics: Equatable, Sendable {
    var rmsDBFS: Double
    var peakDBFS: Double
    var appliedGainDB: Double
    var clippedFraction: Double
}

/// Thread-confined post-VPIO safety stage shared by clip, local self-test and
/// live PTT backends. It cannot amplify more than 12 dB, changes gain at most
/// 3 dB/s, and always applies the distinct -3 dBFS input ceiling last.
final class MacCaptureInputSafetyProcessor {
    static let targetRMSDBFS = -20.0
    static let targetToleranceDB = 3.0
    static let maximumGainDB = 12.0
    static let maximumGainChangeDBPerSecond = 3.0
    static let peakCeilingDBFS = -3.0

    private let sampleRate: Double
    private var gainDB = 0.0

    init(sampleRate: Double = 48_000) { self.sampleRate = sampleRate }

    func reset() { gainDB = 0 }

    func process(_ samples: inout [Float]) -> MacCaptureInputMetrics {
        guard !samples.isEmpty else {
            return MacCaptureInputMetrics(
                rmsDBFS: -240, peakDBFS: -240, appliedGainDB: gainDB, clippedFraction: 0)
        }
        var sum = 0.0
        var inputPeak = 0.0
        for sample in samples {
            let value = Double(sample)
            sum += value * value
            inputPeak = max(inputPeak, abs(value))
        }
        let inputRMS = sqrt(sum / Double(samples.count))
        let inputDB = Self.db(inputRMS)
        let desired = min(Self.maximumGainDB, Self.targetRMSDBFS - inputDB)
        let maximumStep = Self.maximumGainChangeDBPerSecond
            * Double(samples.count) / sampleRate
        gainDB += min(max(desired - gainDB, -maximumStep), maximumStep)
        let gain = pow(10, gainDB / 20)
        let ceiling = pow(10, Self.peakCeilingDBFS / 20)
        let ceilingScale = inputPeak > 0 ? min(1, ceiling / (inputPeak * gain)) : 1
        let finalGain = gain * ceilingScale
        var outputSum = 0.0
        var outputPeak = 0.0
        var clipped = 0
        for index in samples.indices {
            let value = min(ceiling, max(-ceiling, Double(samples[index]) * finalGain))
            samples[index] = Float(value)
            outputSum += value * value
            outputPeak = max(outputPeak, abs(value))
            if abs(value) >= ceiling - 1e-7 { clipped += 1 }
        }
        return MacCaptureInputMetrics(
            rmsDBFS: Self.db(sqrt(outputSum / Double(samples.count))),
            peakDBFS: Self.db(outputPeak),
            appliedGainDB: gainDB + 20 * log10(max(ceilingScale, 1e-12)),
            clippedFraction: Double(clipped) / Double(samples.count))
    }

    private static func db(_ value: Double) -> Double {
        20 * log10(max(value, 1e-12))
    }
}

enum MacCaptureQualityGeneration {
    private static let lock = NSLock()
    private nonisolated(unsafe) static var value: Int64 = 0

    static func next() -> Int64 {
        lock.withLock {
            value += 1
            return value
        }
    }
}

struct MacCaptureQualitySession {
    let generation: Int64
    let workflow: String
    let request: MacCaptureQualityRequest
    let resolvedMode: String
    let quality: String
    let aec: String
    let ns: String
    let agc: String
    let reason: String

    func state(lifecycle: String, nowMs: Int64) -> CaptureQualityState {
        CaptureQualityState(
            generation: generation,
            workflow: workflow,
            requestedMode: request.mode.rawValue,
            resolvedMode: resolvedMode,
            lifecycle: lifecycle,
            quality: quality,
            aec: aec,
            ns: ns,
            agc: agc,
            inputHealth: "ok",
            reason: reason,
            updatedMonotonicMs: max(1, nowMs),
            referenceAgeMs: nil,
            processorOverruns: 0)
    }
}

struct MacCaptureQualityDecision: Equatable, Sendable {
    let quality: String
    let aec: String
    let ns: String
    let agc: String
    let reason: String

    static func evaluate(
        request: MacCaptureQualityRequest,
        resolvedMode: String,
        voiceProcessingEnabled: Bool
    ) -> MacCaptureQualityDecision {
        let mismatch = request.mode != .auto && request.mode.rawValue != resolvedMode
        if !request.processingRequested {
            return MacCaptureQualityDecision(
                quality: "degraded", aec: "unavailable", ns: "unavailable",
                agc: "unavailable", reason: "user_selected_unprocessed")
        }
        if !voiceProcessingEnabled {
            return MacCaptureQualityDecision(
                quality: "degraded", aec: "unavailable", ns: "unavailable",
                agc: "active", reason: "aec_unavailable")
        }
        if mismatch {
            return MacCaptureQualityDecision(
                quality: "degraded", aec: "active", ns: "active",
                agc: "active", reason: "route_excluded")
        }
        if resolvedMode == "unknown" {
            return MacCaptureQualityDecision(
                quality: "degraded", aec: "active", ns: "active",
                agc: "active", reason: "route_unknown")
        }
        if resolvedMode == "speaker" {
            // VPIO owns a private render reference whose age cannot be proved
            // from this API. Keep speaker honest until the manual acoustic gate.
            return MacCaptureQualityDecision(
                quality: "degraded", aec: "active", ns: "active",
                agc: "active", reason: "reference_unavailable")
        }
        return MacCaptureQualityDecision(
            quality: "accepted", aec: "active", ns: "active",
            agc: "active", reason: "none")
    }
}
