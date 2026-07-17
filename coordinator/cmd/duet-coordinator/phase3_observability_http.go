package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

const phase3ObservabilityContract = "p3-observability-health-evidence.v1"

type phase3SubsystemReadiness struct {
	Enabled        bool   `json:"enabled"`
	RuntimeStatus  string `json:"runtime_status"`
	EvidenceStatus string `json:"evidence_status"`
	Ready          bool   `json:"ready"`
}

type phase3FeatureFlagsView struct {
	LivePTTEnabled          bool  `json:"live_ptt_enabled"`
	E2EEMediaEnabled        bool  `json:"e2ee_media_enabled"`
	SoundboardEnabledScopes int64 `json:"soundboard_enabled_scopes"`
	AutomationEnabledScopes int64 `json:"automation_enabled_scopes"`
}

type phase3LivePTTMetricsView struct {
	StartsTotal                  uint64 `json:"starts_total"`
	RejectedStartsTotal          uint64 `json:"rejected_starts_total"`
	FramesRelayedTotal           uint64 `json:"frames_relayed_total"`
	DuplicateFramesTotal         uint64 `json:"duplicate_frames_total"`
	StaleFramesTotal             uint64 `json:"stale_frames_total"`
	InvalidFramesTotal           uint64 `json:"invalid_frames_total"`
	TargetBackpressureTotal      uint64 `json:"target_backpressure_total"`
	TargetPolicyDropsTotal       uint64 `json:"target_policy_drops_total"`
	SessionsEndedTotal           uint64 `json:"sessions_ended_total"`
	WatchdogCancellationsTotal   uint64 `json:"watchdog_cancellations_total"`
	ActiveSessions               int    `json:"active_sessions"`
	RetainedAudioBytes           int    `json:"retained_audio_bytes"`
	PersistedAudioBytes          int    `json:"persisted_audio_bytes"`
	MouthToEarLatencySampleCount int64  `json:"mouth_to_ear_latency_sample_count"`
	MouthToEarLatencyP95MS       *int64 `json:"mouth_to_ear_latency_p95_ms"`
	JitterSampleCount            int64  `json:"jitter_sample_count"`
	JitterP95MS                  *int64 `json:"jitter_p95_ms"`
	ClientTimingEvidenceStatus   string `json:"client_timing_evidence_status"`
}

type phase3CaptureMetricsView struct {
	NodesObserved               int64            `json:"nodes_observed"`
	NodesConnected              int64            `json:"nodes_connected"`
	NodesCapable                int64            `json:"nodes_capable"`
	NodesReporting              int64            `json:"nodes_reporting"`
	MissingWhileConnected       int64            `json:"missing_while_connected"`
	ProcessorOverrunsTotal      int64            `json:"processor_overruns_total"`
	Lifecycle                   map[string]int64 `json:"lifecycle"`
	Quality                     map[string]int64 `json:"quality"`
	InputHealth                 map[string]int64 `json:"input_health"`
	RequestedMode               map[string]int64 `json:"requested_mode"`
	ResolvedMode                map[string]int64 `json:"resolved_mode"`
	AEC                         map[string]int64 `json:"aec"`
	NS                          map[string]int64 `json:"ns"`
	AGC                         map[string]int64 `json:"agc"`
	CaptureStopCallbackSamples  int64            `json:"capture_stop_callback_samples"`
	CaptureCallbackEvidence     string           `json:"capture_callback_evidence_status"`
	MonotonicFreshnessSemantics string           `json:"monotonic_freshness_semantics"`
}

type phase3CryptoMetricsView struct {
	State                 string `json:"state"`
	EpochSamples          int64  `json:"epoch_samples"`
	RevocationSamples     int64  `json:"revocation_samples"`
	SecretMaterialExposed bool   `json:"secret_material_exposed"`
}

type phase3ReadinessView struct {
	RuntimeReady           bool                     `json:"runtime_ready"`
	PromotionEvidenceReady bool                     `json:"promotion_evidence_ready"`
	ProvenanceReady        bool                     `json:"provenance_ready"`
	LivePTT                phase3SubsystemReadiness `json:"live_ptt"`
	CaptureQuality         phase3SubsystemReadiness `json:"capture_quality"`
	E2EEMedia              phase3SubsystemReadiness `json:"e2ee_media"`
	Soundboard             phase3SubsystemReadiness `json:"soundboard"`
	Automation             phase3SubsystemReadiness `json:"automation"`
	MissingEvidence        []string                 `json:"missing_evidence"`
	Alerts                 []string                 `json:"alerts"`
}

type phase3EvidenceArchiveView struct {
	WindowSeconds          int64  `json:"window_seconds"`
	RuntimeSnapshotStatus  string `json:"runtime_snapshot_status"`
	ManualAppHardware      string `json:"manual_app_hardware"`
	IndependentReviews     string `json:"independent_reviews"`
	BetaIncidentEvidence   string `json:"beta_incident_evidence"`
	ArchiveBindingRequired bool   `json:"archive_binding_required"`
}

type phase3ObservabilityView struct {
	Contract             string                                      `json:"contract"`
	GeneratedAtMS        int64                                       `json:"generated_at_ms"`
	BuildVersion         string                                      `json:"build_version"`
	EnvironmentRefSHA256 string                                      `json:"environment_ref_sha256"`
	Features             phase3FeatureFlagsView                      `json:"features"`
	LivePTT              phase3LivePTTMetricsView                    `json:"live_ptt"`
	CaptureQuality       phase3CaptureMetricsView                    `json:"capture_quality"`
	E2EEMedia            phase3CryptoMetricsView                     `json:"e2ee_media"`
	Automation           store.Phase3AutomationObservabilitySnapshot `json:"automation"`
	Readiness            phase3ReadinessView                         `json:"readiness"`
	EvidenceArchive      phase3EvidenceArchiveView                   `json:"evidence_archive"`
}

type phase3ObservabilityBuilder func(
	store.Phase3AutomationObservabilitySnapshot, int64,
) phase3ObservabilityView

func phase3EnvironmentRef(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("barycenter/phase3-environment/v1:" + value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func newPhase3ObservabilityBuilder(
	buildVersion, environment string,
	livePTTEnabled bool,
	livePTT *session.LivePTTRuntime,
	nodes func() map[hub.NodeKey]hub.NodeSnapshot,
) phase3ObservabilityBuilder {
	return func(automation store.Phase3AutomationObservabilitySnapshot, now int64) phase3ObservabilityView {
		return buildPhase3Observability(
			buildVersion, phase3EnvironmentRef(environment), livePTTEnabled,
			livePTT, nodes, automation, now,
		)
	}
}

func initializedCaptureMetrics() phase3CaptureMetricsView {
	return phase3CaptureMetricsView{
		Lifecycle: map[string]int64{
			protocol.CaptureLifecycleIdle: 0, protocol.CaptureLifecyclePreparing: 0,
			protocol.CaptureLifecycleAwaitingFallback: 0, protocol.CaptureLifecycleCapturing: 0,
			protocol.CaptureLifecycleReconfiguring: 0, protocol.CaptureLifecycleStopping: 0,
			protocol.CaptureLifecycleFailed: 0,
		},
		Quality: map[string]int64{
			protocol.CaptureQualityAccepted: 0, protocol.CaptureQualityDegraded: 0,
			protocol.CaptureQualityUnsupported: 0,
		},
		InputHealth: map[string]int64{
			protocol.CaptureHealthOK: 0, protocol.CaptureHealthSilent: 0,
			protocol.CaptureHealthTooQuiet: 0, protocol.CaptureHealthClipping: 0,
			protocol.CaptureHealthNoDevice: 0, protocol.CaptureHealthPermissionDenied: 0,
			protocol.CaptureHealthReferenceStale: 0, protocol.CaptureHealthClockUnstable: 0,
			protocol.CaptureHealthProcessorOverrun: 0,
		},
		RequestedMode: map[string]int64{
			protocol.CaptureRouteAuto: 0, protocol.CaptureRouteSpeaker: 0,
			protocol.CaptureRouteHeadphone: 0,
		},
		ResolvedMode: map[string]int64{
			protocol.CaptureRouteSpeaker: 0, protocol.CaptureRouteHeadphone: 0,
			protocol.CaptureRouteUnknown: 0,
		},
		AEC: map[string]int64{
			protocol.CaptureEffectActive: 0, protocol.CaptureEffectNotRequired: 0,
			protocol.CaptureEffectUnavailable: 0, protocol.CaptureEffectFaulted: 0,
		},
		NS: map[string]int64{
			protocol.CaptureEffectActive: 0, protocol.CaptureEffectUnavailable: 0,
			protocol.CaptureEffectFaulted: 0,
		},
		AGC: map[string]int64{
			protocol.CaptureEffectActive: 0, protocol.CaptureEffectUnavailable: 0,
			protocol.CaptureEffectFaulted: 0,
		},
		CaptureCallbackEvidence:     "client_evidence_required",
		MonotonicFreshnessSemantics: "connection_generation_only",
	}
}

func captureMetricsForNodes(nodes func() map[hub.NodeKey]hub.NodeSnapshot) phase3CaptureMetricsView {
	view := initializedCaptureMetrics()
	if nodes == nil {
		return view
	}
	for _, snapshot := range nodes() {
		view.NodesObserved++
		if snapshot.Connected {
			view.NodesConnected++
		}
		capable := snapshot.Capabilities.Supports(protocol.CapabilityCaptureQuality)
		if capable {
			view.NodesCapable++
		}
		if snapshot.CaptureQuality == nil {
			if capable && snapshot.Connected {
				view.MissingWhileConnected++
			}
			continue
		}
		state := snapshot.CaptureQuality
		view.NodesReporting++
		view.Lifecycle[state.Lifecycle]++
		view.Quality[state.Quality]++
		view.InputHealth[state.InputHealth]++
		view.RequestedMode[state.RequestedMode]++
		view.ResolvedMode[state.ResolvedMode]++
		view.AEC[state.AEC]++
		view.NS[state.NS]++
		view.AGC[state.AGC]++
		if state.ProcessorOverruns != nil {
			view.ProcessorOverrunsTotal += *state.ProcessorOverruns
		}
	}
	return view
}

func buildPhase3Observability(
	buildVersion, environmentRef string,
	livePTTEnabled bool,
	livePTT *session.LivePTTRuntime,
	nodes func() map[hub.NodeKey]hub.NodeSnapshot,
	automation store.Phase3AutomationObservabilitySnapshot,
	now int64,
) phase3ObservabilityView {
	capture := captureMetricsForNodes(nodes)
	live := phase3LivePTTMetricsView{ClientTimingEvidenceStatus: "client_evidence_required"}
	liveRuntimeReady := !livePTTEnabled
	alerts := make([]string, 0, 4)
	if livePTT != nil {
		metrics := livePTT.Metrics()
		live = phase3LivePTTMetricsView{
			StartsTotal: metrics.StartsTotal, RejectedStartsTotal: metrics.RejectedStartsTotal,
			FramesRelayedTotal: metrics.FramesRelayedTotal, DuplicateFramesTotal: metrics.DuplicateFramesTotal,
			StaleFramesTotal: metrics.StaleFramesTotal, InvalidFramesTotal: metrics.InvalidFramesTotal,
			TargetBackpressureTotal:    metrics.TargetBackpressureTotal,
			TargetPolicyDropsTotal:     metrics.TargetPolicyDropsTotal,
			SessionsEndedTotal:         metrics.SessionsEndedTotal,
			WatchdogCancellationsTotal: metrics.WatchdogCancellations,
			ActiveSessions:             metrics.ActiveSessions, RetainedAudioBytes: metrics.RetainedAudioBytes,
			PersistedAudioBytes:        metrics.PersistedAudioBytes,
			ClientTimingEvidenceStatus: "client_evidence_required",
		}
		liveRuntimeReady = !livePTTEnabled ||
			(metrics.RetainedAudioBytes == 0 && metrics.PersistedAudioBytes == 0)
		if metrics.RetainedAudioBytes != 0 || metrics.PersistedAudioBytes != 0 {
			alerts = append(alerts, "live_ptt_prohibited_audio_retention")
		}
	} else if livePTTEnabled {
		alerts = append(alerts, "live_ptt_runtime_missing")
	}

	captureEnabled := capture.NodesCapable > 0
	captureReady := !captureEnabled || capture.MissingWhileConnected == 0
	if !captureReady {
		alerts = append(alerts, "capture_quality_telemetry_missing")
	}
	soundboardEnabled := automation.Feature.SoundboardEnabled > 0
	automationEnabled := automation.Feature.AutomationEnabled > 0
	soundboardReady := !soundboardEnabled || automation.Feature.ObservedScopes > 0
	automationReady := !automationEnabled ||
		(automation.Feature.InvalidCombinationScopes == 0 &&
			automation.Feature.AutomationEmergencyDisabled < automation.Feature.AutomationEnabled)
	if automation.Feature.InvalidCombinationScopes > 0 {
		alerts = append(alerts, "automation_enabled_without_soundboard")
	}
	if automationEnabled &&
		automation.Feature.AutomationEmergencyDisabled >= automation.Feature.AutomationEnabled {
		alerts = append(alerts, "automation_all_scopes_emergency_disabled")
	}
	provenanceReady := strings.TrimSpace(buildVersion) != "" && environmentRef != ""
	if !provenanceReady {
		alerts = append(alerts, "build_environment_provenance_missing")
	}
	runtimeReady := liveRuntimeReady && captureReady && soundboardReady && automationReady
	missing := []string{
		"c1_c3_manual_app_hardware_matrix", "c4_c6_reviewed_e2ee_acceptance",
		"c7_automation_safety_acceptance", "phase3_independent_reviews",
		"phase3_beta_incident_review", "phase3_rollout_recovery_drills",
	}
	if !provenanceReady {
		missing = append(missing, "build_environment_provenance")
	}
	return phase3ObservabilityView{
		Contract: phase3ObservabilityContract, GeneratedAtMS: now,
		BuildVersion: buildVersion, EnvironmentRefSHA256: environmentRef,
		Features: phase3FeatureFlagsView{
			LivePTTEnabled: livePTTEnabled, E2EEMediaEnabled: false,
			SoundboardEnabledScopes: automation.Feature.SoundboardEnabled,
			AutomationEnabledScopes: automation.Feature.AutomationEnabled,
		},
		LivePTT: live, CaptureQuality: capture,
		E2EEMedia: phase3CryptoMetricsView{
			State: "deferred_unavailable", SecretMaterialExposed: false,
		},
		Automation: automation,
		Readiness: phase3ReadinessView{
			RuntimeReady: runtimeReady, PromotionEvidenceReady: false,
			ProvenanceReady: provenanceReady,
			LivePTT: phase3SubsystemReadiness{
				Enabled: livePTTEnabled, RuntimeStatus: enabledStatus(livePTTEnabled, liveRuntimeReady),
				EvidenceStatus: optionalEvidenceStatus(livePTTEnabled, "client_evidence_required"),
				Ready:          liveRuntimeReady,
			},
			CaptureQuality: phase3SubsystemReadiness{
				Enabled: captureEnabled, RuntimeStatus: enabledStatus(captureEnabled, captureReady),
				EvidenceStatus: optionalEvidenceStatus(captureEnabled, "client_evidence_required"),
				Ready:          captureReady,
			},
			E2EEMedia: phase3SubsystemReadiness{
				Enabled: false, RuntimeStatus: "deferred_unavailable",
				EvidenceStatus: "not_required_while_disabled", Ready: true,
			},
			Soundboard: phase3SubsystemReadiness{
				Enabled: soundboardEnabled, RuntimeStatus: enabledStatus(soundboardEnabled, soundboardReady),
				EvidenceStatus: optionalEvidenceStatus(soundboardEnabled, "runtime_aggregate_only"),
				Ready:          soundboardReady,
			},
			Automation: phase3SubsystemReadiness{
				Enabled: automationEnabled, RuntimeStatus: enabledStatus(automationEnabled, automationReady),
				EvidenceStatus: optionalEvidenceStatus(automationEnabled, "runtime_aggregate_only"),
				Ready:          automationReady,
			},
			MissingEvidence: missing, Alerts: alerts,
		},
		EvidenceArchive: phase3EvidenceArchiveView{
			WindowSeconds:         int64(store.Phase3ObservabilityWindow / time.Second),
			RuntimeSnapshotStatus: "available", ManualAppHardware: "not_run",
			IndependentReviews: "not_run", BetaIncidentEvidence: "not_run",
			ArchiveBindingRequired: true,
		},
	}
}

func enabledStatus(enabled, ready bool) string {
	if !enabled {
		return "disabled"
	}
	if ready {
		return "ready"
	}
	return "blocked"
}

func optionalEvidenceStatus(enabled bool, enabledStatus string) string {
	if !enabled {
		return "not_required_while_disabled"
	}
	return enabledStatus
}

func addPhase3Health(body map[string]any, view phase3ObservabilityView) {
	body["phase3"] = map[string]any{
		"runtime_ready":   view.Readiness.RuntimeReady,
		"live_ptt":        view.Readiness.LivePTT,
		"capture_quality": view.Readiness.CaptureQuality,
		"e2ee_media":      view.Readiness.E2EEMedia,
		"soundboard":      view.Readiness.Soundboard,
		"automation":      view.Readiness.Automation,
	}
	if !view.Readiness.RuntimeReady {
		body["status"] = "degraded"
	}
}

func (api *onboardingAPI) phase3ObservabilityOperatorView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) != 0 ||
		r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	operator, ok := requireStreamAccountingCapability(w, r, false)
	if !ok {
		return
	}
	now := time.Now().UnixMilli()
	automation, err := api.store.GetAuthorizedPhase3AutomationObservability(
		operator.Context.ID, operator.Bearer, now,
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		case errors.Is(err, store.ErrModerationForbidden):
			apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
		case errors.Is(err, store.ErrStreamAccountingInvalid):
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		default:
			api.internalError(w, "read Phase 3 observability view", err)
		}
		return
	}
	if api.phase3Observability == nil {
		apiError(w, http.StatusServiceUnavailable, errorServiceUnavailable, 0)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.phase3Observability(automation, now))
}
