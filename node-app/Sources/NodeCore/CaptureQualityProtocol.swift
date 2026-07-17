import Foundation

public enum CaptureQualityProtocolError: Error, Equatable {
    case invalid
}

public struct CaptureQualityState: Codable, Equatable, Sendable {
    public var contract: String
    public var generation: Int64
    public var workflow: String
    public var requestedMode: String
    public var resolvedMode: String
    public var lifecycle: String
    public var quality: String
    public var aec: String
    public var ns: String
    public var agc: String
    public var inputHealth: String
    public var reason: String
    public var inputCeilingDBFS: Double
    public var updatedMonotonicMs: Int64
    public var referenceAgeMs: Int64?
    public var processorOverruns: Int64?

    public init(
        contract: String = CaptureQualityContract.version,
        generation: Int64, workflow: String, requestedMode: String,
        resolvedMode: String, lifecycle: String, quality: String,
        aec: String, ns: String, agc: String, inputHealth: String,
        reason: String, inputCeilingDBFS: Double = CaptureQualityContract.inputCeilingDBFS,
        updatedMonotonicMs: Int64, referenceAgeMs: Int64? = nil,
        processorOverruns: Int64? = nil
    ) {
        self.contract = contract
        self.generation = generation
        self.workflow = workflow
        self.requestedMode = requestedMode
        self.resolvedMode = resolvedMode
        self.lifecycle = lifecycle
        self.quality = quality
        self.aec = aec
        self.ns = ns
        self.agc = agc
        self.inputHealth = inputHealth
        self.reason = reason
        self.inputCeilingDBFS = inputCeilingDBFS
        self.updatedMonotonicMs = updatedMonotonicMs
        self.referenceAgeMs = referenceAgeMs
        self.processorOverruns = processorOverruns
    }

    enum CodingKeys: String, CodingKey {
        case contract, generation, workflow
        case requestedMode = "requested_mode"
        case resolvedMode = "resolved_mode"
        case lifecycle, quality, aec, ns, agc
        case inputHealth = "input_health"
        case reason
        case inputCeilingDBFS = "input_ceiling_dbfs"
        case updatedMonotonicMs = "updated_monotonic_ms"
        case referenceAgeMs = "reference_age_ms"
        case processorOverruns = "processor_overruns"
    }
}

public enum CaptureQualityContract {
    public static let version = "capture-quality.v1"
    public static let capability = "capture_quality_v1"
    public static let inputCeilingDBFS = -3.0
    public static let outputCeilingDBFS = -1.0

    private static let workflows: Set<String> = ["recorded_clip", "local_self_test", "live_ptt"]
    private static let requestedModes: Set<String> = ["auto", "speaker", "headphone"]
    private static let resolvedModes: Set<String> = ["speaker", "headphone", "unknown"]
    private static let lifecycles: Set<String> = [
        "idle", "preparing", "awaiting_fallback_consent", "capturing",
        "reconfiguring", "stopping", "failed",
    ]
    private static let qualities: Set<String> = ["accepted", "degraded", "unsupported"]
    private static let aecStates: Set<String> = ["active", "not_required", "unavailable", "faulted"]
    private static let requiredEffectStates: Set<String> = ["active", "unavailable", "faulted"]
    private static let inputHealthStates: Set<String> = [
        "ok", "silent", "too_quiet", "clipping", "no_device", "permission_denied",
        "reference_stale", "clock_unstable", "processor_overrun",
    ]
    private static let reasons: Set<String> = [
        "none", "user_selected_unprocessed", "aec_unavailable", "reference_unavailable",
        "reference_stale", "route_unknown", "route_excluded", "ns_unavailable",
        "agc_unavailable", "device_lost", "permission_denied", "clock_unstable",
        "processor_overrun", "rearm_timeout", "mixed_version",
    ]

    public static func validate(_ state: CaptureQualityState?) throws {
        guard let state else { return }
        guard state.contract == version, state.generation > 0,
              state.updatedMonotonicMs > 0, state.inputCeilingDBFS == inputCeilingDBFS,
              workflows.contains(state.workflow), requestedModes.contains(state.requestedMode),
              resolvedModes.contains(state.resolvedMode), lifecycles.contains(state.lifecycle),
              qualities.contains(state.quality), aecStates.contains(state.aec),
              requiredEffectStates.contains(state.ns), requiredEffectStates.contains(state.agc),
              inputHealthStates.contains(state.inputHealth), reasons.contains(state.reason)
        else { throw CaptureQualityProtocolError.invalid }
        if state.aec == "not_required" && state.resolvedMode != "headphone" {
            throw CaptureQualityProtocolError.invalid
        }
        if let age = state.referenceAgeMs, !(0...100).contains(age) {
            throw CaptureQualityProtocolError.invalid
        }
        if let overruns = state.processorOverruns, overruns < 0 {
            throw CaptureQualityProtocolError.invalid
        }
        if state.quality == "accepted" {
            guard state.reason == "none", state.inputHealth == "ok",
                  state.resolvedMode != "unknown", state.ns == "active", state.agc == "active",
                  !["awaiting_fallback_consent", "reconfiguring", "failed"].contains(state.lifecycle)
            else { throw CaptureQualityProtocolError.invalid }
            if state.resolvedMode == "speaker" &&
                (state.aec != "active" || state.referenceAgeMs == nil) {
                throw CaptureQualityProtocolError.invalid
            }
            if state.resolvedMode == "headphone" &&
                !["active", "not_required"].contains(state.aec) {
                throw CaptureQualityProtocolError.invalid
            }
        } else if state.reason == "none" {
            throw CaptureQualityProtocolError.invalid
        }
        if state.quality == "unsupported" && state.lifecycle == "capturing" {
            throw CaptureQualityProtocolError.invalid
        }
    }

    public static func validate(_ message: Message) throws {
        guard case .state(let payload) = message else { return }
        try validate(payload.captureQuality)
    }

    public static func heartbeatState(
        _ provided: StatePayload, advertisedCapabilities: [String]
    ) -> StatePayload {
        var state = provided
        if state.captureQuality != nil && !advertisedCapabilities.contains(capability) {
            state.captureQuality = nil
        }
        return state
    }
}

public enum CaptureQualityGenerationResult: String, Equatable, Sendable {
    case apply, duplicate, stale, invalid
}

public struct CaptureQualityGenerationGuard: Sendable {
    private var generation: Int64 = 0
    private var updatedMs: Int64 = 0

    public init() {}

    public mutating func accept(generation: Int64, updatedMs: Int64) -> CaptureQualityGenerationResult {
        guard generation > 0, updatedMs > 0 else { return .invalid }
        if generation < self.generation { return .stale }
        if generation > self.generation {
            self.generation = generation
            self.updatedMs = updatedMs
            return .apply
        }
        if updatedMs < self.updatedMs { return .stale }
        if updatedMs == self.updatedMs { return .duplicate }
        self.updatedMs = updatedMs
        return .apply
    }
}

public struct CaptureQualityGuidance: Equatable, Sendable {
    public var available: Bool
    public var quality: String
    public var reason: String
    public var key: String
    public var requestedMode: String
    public var resolvedMode: String
    public var aec: String
    public var ns: String
    public var agc: String
    public var inputHealth: String
    public var inputCeilingDBFS: Double
    public var outputCeilingDBFS: Double

    public init(
        available: Bool = false, quality: String, reason: String, key: String,
        requestedMode: String = "", resolvedMode: String = "", aec: String = "",
        ns: String = "", agc: String = "", inputHealth: String = "",
        inputCeilingDBFS: Double = 0, outputCeilingDBFS: Double = 0
    ) {
        self.available = available
        self.quality = quality
        self.reason = reason
        self.key = key
        self.requestedMode = requestedMode
        self.resolvedMode = resolvedMode
        self.aec = aec
        self.ns = ns
        self.agc = agc
        self.inputHealth = inputHealth
        self.inputCeilingDBFS = inputCeilingDBFS
        self.outputCeilingDBFS = outputCeilingDBFS
    }
}

public enum CaptureQualityPresentation {
    public static func guidance(capabilities: [String], state: CaptureQualityState?) -> CaptureQualityGuidance {
        guard capabilities.contains(CaptureQualityContract.capability), let state else {
            return CaptureQualityGuidance(
                quality: "unsupported", reason: "mixed_version",
                key: "capture_quality.mixed_version")
        }
        var result = CaptureQualityGuidance(
            available: true, quality: state.quality, reason: state.reason, key: "",
            requestedMode: state.requestedMode, resolvedMode: state.resolvedMode,
            aec: state.aec, ns: state.ns, agc: state.agc, inputHealth: state.inputHealth,
            inputCeilingDBFS: state.inputCeilingDBFS,
            outputCeilingDBFS: CaptureQualityContract.outputCeilingDBFS)
        if state.inputHealth != "ok" {
            result.key = "capture_quality.input.\(state.inputHealth)"
            return result
        }
        result.key = "capture_quality.\(state.quality).\(state.reason)"
        return result
    }

    public static func diagnosticLogFields(_ state: CaptureQualityState?) -> [String: Any] {
        guard let state else { return ["quality": "unsupported", "reason": "mixed_version"] }
        var fields: [String: Any] = [
            "contract": state.contract,
            "generation": state.generation,
            "workflow": state.workflow,
            "quality": state.quality,
            "reason": state.reason,
            "input_health": state.inputHealth,
        ]
        if let overruns = state.processorOverruns { fields["processor_overruns"] = overruns }
        return fields
    }
}
