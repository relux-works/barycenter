// Code mirrored from coordinator/internal/protocol — keep in sync via golden tests.
// Do not edit below this header: golden_test.go verifies both the wire contract
// (round-trip of every golden file) and byte-equality with the coordinator source.
package protocol

import "fmt"

const (
	CaptureQualityContract   = "capture-quality.v1"
	CaptureInputCeilingDBFS  = -3.0
	CaptureOutputCeilingDBFS = -1.0
)

const (
	CaptureWorkflowRecordedClip  = "recorded_clip"
	CaptureWorkflowLocalSelfTest = "local_self_test"
	CaptureWorkflowLivePTT       = "live_ptt"
)

const (
	CaptureRouteAuto      = "auto"
	CaptureRouteSpeaker   = "speaker"
	CaptureRouteHeadphone = "headphone"
	CaptureRouteUnknown   = "unknown"
)

const (
	CaptureLifecycleIdle             = "idle"
	CaptureLifecyclePreparing        = "preparing"
	CaptureLifecycleAwaitingFallback = "awaiting_fallback_consent"
	CaptureLifecycleCapturing        = "capturing"
	CaptureLifecycleReconfiguring    = "reconfiguring"
	CaptureLifecycleStopping         = "stopping"
	CaptureLifecycleFailed           = "failed"
)

const (
	CaptureQualityAccepted    = "accepted"
	CaptureQualityDegraded    = "degraded"
	CaptureQualityUnsupported = "unsupported"
)

const (
	CaptureEffectActive      = "active"
	CaptureEffectNotRequired = "not_required"
	CaptureEffectUnavailable = "unavailable"
	CaptureEffectFaulted     = "faulted"
)

const (
	CaptureHealthOK               = "ok"
	CaptureHealthSilent           = "silent"
	CaptureHealthTooQuiet         = "too_quiet"
	CaptureHealthClipping         = "clipping"
	CaptureHealthNoDevice         = "no_device"
	CaptureHealthPermissionDenied = "permission_denied"
	CaptureHealthReferenceStale   = "reference_stale"
	CaptureHealthClockUnstable    = "clock_unstable"
	CaptureHealthProcessorOverrun = "processor_overrun"
)

var captureQualityReasons = map[string]bool{
	"none": true, "user_selected_unprocessed": true, "aec_unavailable": true,
	"reference_unavailable": true, "reference_stale": true, "route_unknown": true,
	"route_excluded": true, "ns_unavailable": true, "agc_unavailable": true,
	"device_lost": true, "permission_denied": true, "clock_unstable": true,
	"processor_overrun": true, "rearm_timeout": true, "mixed_version": true,
}

// CaptureQualityState is the optional, content-free state heartbeat extension.
// It is observational only: no field can open, resume or configure a microphone.
type CaptureQualityState struct {
	Contract           string  `json:"contract"`
	Generation         int64   `json:"generation"`
	Workflow           string  `json:"workflow"`
	RequestedMode      string  `json:"requested_mode"`
	ResolvedMode       string  `json:"resolved_mode"`
	Lifecycle          string  `json:"lifecycle"`
	Quality            string  `json:"quality"`
	AEC                string  `json:"aec"`
	NS                 string  `json:"ns"`
	AGC                string  `json:"agc"`
	InputHealth        string  `json:"input_health"`
	Reason             string  `json:"reason"`
	InputCeilingDBFS   float64 `json:"input_ceiling_dbfs"`
	UpdatedMonotonicMS int64   `json:"updated_monotonic_ms"`
	ReferenceAgeMS     *int64  `json:"reference_age_ms,omitempty"`
	ProcessorOverruns  *int64  `json:"processor_overruns,omitempty"`
}

func in(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// ValidateCaptureQualityState enforces both enum bounds and the relational
// rules that prevent an unsupported route/effect from being labelled accepted.
func ValidateCaptureQualityState(state *CaptureQualityState) error {
	if state == nil {
		return nil // additive field absent: legacy/mixed-version node
	}
	if state.Contract != CaptureQualityContract || state.Generation <= 0 ||
		state.UpdatedMonotonicMS <= 0 || state.InputCeilingDBFS != CaptureInputCeilingDBFS {
		return fmt.Errorf("invalid capture quality contract, generation, clock or input ceiling")
	}
	if !in(state.Workflow, CaptureWorkflowRecordedClip, CaptureWorkflowLocalSelfTest, CaptureWorkflowLivePTT) ||
		!in(state.RequestedMode, CaptureRouteAuto, CaptureRouteSpeaker, CaptureRouteHeadphone) ||
		!in(state.ResolvedMode, CaptureRouteSpeaker, CaptureRouteHeadphone, CaptureRouteUnknown) ||
		!in(state.Lifecycle, CaptureLifecycleIdle, CaptureLifecyclePreparing, CaptureLifecycleAwaitingFallback,
			CaptureLifecycleCapturing, CaptureLifecycleReconfiguring, CaptureLifecycleStopping, CaptureLifecycleFailed) ||
		!in(state.Quality, CaptureQualityAccepted, CaptureQualityDegraded, CaptureQualityUnsupported) ||
		!in(state.AEC, CaptureEffectActive, CaptureEffectNotRequired, CaptureEffectUnavailable, CaptureEffectFaulted) ||
		!in(state.NS, CaptureEffectActive, CaptureEffectUnavailable, CaptureEffectFaulted) ||
		!in(state.AGC, CaptureEffectActive, CaptureEffectUnavailable, CaptureEffectFaulted) ||
		!in(state.InputHealth, CaptureHealthOK, CaptureHealthSilent, CaptureHealthTooQuiet, CaptureHealthClipping,
			CaptureHealthNoDevice, CaptureHealthPermissionDenied, CaptureHealthReferenceStale,
			CaptureHealthClockUnstable, CaptureHealthProcessorOverrun) || !captureQualityReasons[state.Reason] {
		return fmt.Errorf("invalid capture quality enum")
	}
	if state.AEC == CaptureEffectNotRequired && state.ResolvedMode != CaptureRouteHeadphone {
		return fmt.Errorf("AEC may be not_required only for a positively resolved headphone route")
	}
	if state.ReferenceAgeMS != nil && (*state.ReferenceAgeMS < 0 || *state.ReferenceAgeMS > 100) {
		return fmt.Errorf("capture quality reference is stale")
	}
	if state.ProcessorOverruns != nil && *state.ProcessorOverruns < 0 {
		return fmt.Errorf("negative capture processor overruns")
	}
	if state.Quality == CaptureQualityAccepted {
		if state.Reason != "none" || state.InputHealth != CaptureHealthOK ||
			state.ResolvedMode == CaptureRouteUnknown || state.NS != CaptureEffectActive ||
			state.AGC != CaptureEffectActive || in(state.Lifecycle, CaptureLifecycleAwaitingFallback,
			CaptureLifecycleReconfiguring, CaptureLifecycleFailed) {
			return fmt.Errorf("accepted capture quality has degraded state")
		}
		if state.ResolvedMode == CaptureRouteSpeaker &&
			(state.AEC != CaptureEffectActive || state.ReferenceAgeMS == nil) {
			return fmt.Errorf("accepted speaker capture lacks active AEC reference")
		}
		if state.ResolvedMode == CaptureRouteHeadphone &&
			!in(state.AEC, CaptureEffectActive, CaptureEffectNotRequired) {
			return fmt.Errorf("accepted headphone capture lacks valid AEC disposition")
		}
	} else if state.Reason == "none" {
		return fmt.Errorf("non-accepted capture quality lacks typed reason")
	}
	if state.Quality == CaptureQualityUnsupported && state.Lifecycle == CaptureLifecycleCapturing {
		return fmt.Errorf("unsupported capture may not be capturing")
	}
	return nil
}

type CaptureQualityGenerationResult string

const (
	CaptureQualityApply     CaptureQualityGenerationResult = "apply"
	CaptureQualityDuplicate CaptureQualityGenerationResult = "duplicate"
	CaptureQualityStale     CaptureQualityGenerationResult = "stale"
	CaptureQualityInvalid   CaptureQualityGenerationResult = "invalid"
)

// CaptureQualityGenerationGuard is reset for every authenticated connection.
// Monotonic timestamps are compared only within one generation; a fresh local
// process/connection starts with a fresh guard and cannot inherit stale state.
type CaptureQualityGenerationGuard struct {
	generation int64
	updatedMS  int64
}

func (g *CaptureQualityGenerationGuard) Accept(generation, updatedMS int64) CaptureQualityGenerationResult {
	if generation <= 0 || updatedMS <= 0 {
		return CaptureQualityInvalid
	}
	if generation < g.generation {
		return CaptureQualityStale
	}
	if generation > g.generation {
		g.generation, g.updatedMS = generation, updatedMS
		return CaptureQualityApply
	}
	if updatedMS < g.updatedMS {
		return CaptureQualityStale
	}
	if updatedMS == g.updatedMS {
		return CaptureQualityDuplicate
	}
	g.updatedMS = updatedMS
	return CaptureQualityApply
}

type CaptureQualityGuidance struct {
	Available         bool
	Quality           string
	Reason            string
	Key               string
	RequestedMode     string
	ResolvedMode      string
	AEC               string
	NS                string
	AGC               string
	InputHealth       string
	InputCeilingDBFS  float64
	OutputCeilingDBFS float64
}

// PresentCaptureQuality keeps mixed-version and unsupported nodes honest using
// shared keys that both platform shells can localize without copying policy.
func PresentCaptureQuality(capabilities CapabilitySet, state *CaptureQualityState) CaptureQualityGuidance {
	if !capabilities.Supports(CapabilityCaptureQuality) || state == nil {
		return CaptureQualityGuidance{
			Quality: CaptureQualityUnsupported, Reason: "mixed_version",
			Key: "capture_quality.mixed_version",
		}
	}
	result := CaptureQualityGuidance{
		Available: true, Quality: state.Quality, Reason: state.Reason,
		RequestedMode: state.RequestedMode, ResolvedMode: state.ResolvedMode,
		AEC: state.AEC, NS: state.NS, AGC: state.AGC, InputHealth: state.InputHealth,
		InputCeilingDBFS: state.InputCeilingDBFS, OutputCeilingDBFS: CaptureOutputCeilingDBFS,
	}
	if state.InputHealth != CaptureHealthOK {
		result.Key = "capture_quality.input." + state.InputHealth
		return result
	}
	result.Key = "capture_quality." + state.Quality + "." + state.Reason
	return result
}

// CaptureQualityDiagnosticFields is the canonical structured-log projection. It
// intentionally excludes audio, levels, routes' device identity and local paths.
func CaptureQualityDiagnosticFields(state *CaptureQualityState) map[string]any {
	if state == nil {
		return map[string]any{"quality": CaptureQualityUnsupported, "reason": "mixed_version"}
	}
	fields := map[string]any{
		"contract": state.Contract, "generation": state.Generation, "workflow": state.Workflow,
		"quality": state.Quality, "reason": state.Reason, "input_health": state.InputHealth,
	}
	if state.ProcessorOverruns != nil {
		fields["processor_overruns"] = *state.ProcessorOverruns
	}
	return fields
}

func CloneCaptureQualityState(state *CaptureQualityState) *CaptureQualityState {
	if state == nil {
		return nil
	}
	clone := *state
	if state.ReferenceAgeMS != nil {
		value := *state.ReferenceAgeMS
		clone.ReferenceAgeMS = &value
	}
	if state.ProcessorOverruns != nil {
		value := *state.ProcessorOverruns
		clone.ProcessorOverruns = &value
	}
	return &clone
}
