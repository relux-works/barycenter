package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

func phase3TestCapabilities(t *testing.T, values ...string) protocol.CapabilitySet {
	t.Helper()
	capabilities, err := protocol.ParseCapabilitySet(values)
	if err != nil {
		t.Fatal(err)
	}
	return capabilities
}

func phase3AcceptedCaptureState(overruns int64) *protocol.CaptureQualityState {
	referenceAge := int64(20)
	return &protocol.CaptureQualityState{
		Contract: protocol.CaptureQualityContract, Generation: 7,
		Workflow: protocol.CaptureWorkflowLivePTT, RequestedMode: protocol.CaptureRouteAuto,
		ResolvedMode: protocol.CaptureRouteSpeaker, Lifecycle: protocol.CaptureLifecycleCapturing,
		Quality: protocol.CaptureQualityAccepted, AEC: protocol.CaptureEffectActive,
		NS: protocol.CaptureEffectActive, AGC: protocol.CaptureEffectActive,
		InputHealth: protocol.CaptureHealthOK, Reason: "none",
		InputCeilingDBFS: protocol.CaptureInputCeilingDBFS, UpdatedMonotonicMS: 42_420,
		ReferenceAgeMS: &referenceAge, ProcessorOverruns: &overruns,
	}
}

func TestPhase3ObservabilityDisabledOptionalFeaturesStayHonestAndHealthy(t *testing.T) {
	view := buildPhase3Observability(
		"test-build", phase3EnvironmentRef("acceptance-us"), false,
		session.NewLivePTTRuntime(), nil, store.Phase3AutomationObservabilitySnapshot{},
		time.Now().UnixMilli(),
	)
	if !view.Readiness.RuntimeReady || view.Readiness.PromotionEvidenceReady ||
		!view.Readiness.ProvenanceReady || view.Readiness.LivePTT.RuntimeStatus != "disabled" ||
		view.Readiness.E2EEMedia.RuntimeStatus != "deferred_unavailable" ||
		view.E2EEMedia.SecretMaterialExposed || view.EvidenceArchive.ManualAppHardware != "not_run" {
		t.Fatalf("disabled Phase 3 view=%+v", view)
	}
	health := map[string]any{"status": "ok"}
	addPhase3Health(health, view)
	if health["status"] != "ok" {
		t.Fatalf("disabled optional health=%+v", health)
	}
}

func TestPhase3ObservabilityEnabledMissingTelemetryFailsClosed(t *testing.T) {
	capabilities := phase3TestCapabilities(t, protocol.CapabilityCaptureQuality)
	view := buildPhase3Observability(
		"test-build", phase3EnvironmentRef("acceptance-us"), true, nil,
		func() map[hub.NodeKey]hub.NodeSnapshot {
			return map[hub.NodeKey]hub.NodeSnapshot{
				{Orbit: 91, Slot: protocol.NodeA}: {
					Connected: true, Capabilities: capabilities,
					CredentialTokenHash: strings.Repeat("a", 64),
				},
			}
		},
		store.Phase3AutomationObservabilitySnapshot{}, time.Now().UnixMilli(),
	)
	if view.Readiness.RuntimeReady || view.Readiness.LivePTT.RuntimeStatus != "blocked" ||
		view.Readiness.CaptureQuality.RuntimeStatus != "blocked" ||
		view.CaptureQuality.MissingWhileConnected != 1 || len(view.Readiness.Alerts) < 2 {
		t.Fatalf("missing telemetry view=%+v", view)
	}
	health := map[string]any{"status": "ok"}
	addPhase3Health(health, view)
	if health["status"] != "degraded" {
		t.Fatalf("missing telemetry health=%+v", health)
	}
}

func TestPhase3ObservabilityRejectsContradictoryAutomationAggregates(t *testing.T) {
	automation := store.Phase3AutomationObservabilitySnapshot{}
	automation.Feature.ObservedScopes = 1
	automation.Feature.AutomationEnabled = 1
	automation.Feature.InvalidCombinationScopes = 1
	view := buildPhase3Observability(
		"test-build", phase3EnvironmentRef("acceptance-us"), false,
		session.NewLivePTTRuntime(), nil, automation, time.Now().UnixMilli(),
	)
	if view.Readiness.Automation.Ready || view.Readiness.RuntimeReady ||
		len(view.Readiness.Alerts) != 1 ||
		view.Readiness.Alerts[0] != "automation_enabled_without_soundboard" {
		t.Fatalf("contradictory automation view=%+v", view.Readiness)
	}
}

func TestPhase3ObservabilityAggregatesCaptureWithoutNodeOrSecretLabels(t *testing.T) {
	capabilities := phase3TestCapabilities(t, protocol.CapabilityCaptureQuality)
	secretHash := strings.Repeat("b", 64)
	localPath := "/Users/private/recordings/capture.wav"
	view := buildPhase3Observability(
		"test-build", phase3EnvironmentRef("acceptance-us"), false,
		session.NewLivePTTRuntime(),
		func() map[hub.NodeKey]hub.NodeSnapshot {
			return map[hub.NodeKey]hub.NodeSnapshot{
				{Orbit: 191919, Slot: protocol.NodeB}: {
					Connected: true, LastSeenAt: 123456, Capabilities: capabilities,
					CaptureQuality: phase3AcceptedCaptureState(3), CredentialTokenHash: secretHash,
				},
			}
		},
		store.Phase3AutomationObservabilitySnapshot{}, time.Now().UnixMilli(),
	)
	if view.CaptureQuality.NodesReporting != 1 || view.CaptureQuality.ProcessorOverrunsTotal != 3 ||
		view.CaptureQuality.Quality[protocol.CaptureQualityAccepted] != 1 {
		t.Fatalf("capture aggregate=%+v", view.CaptureQuality)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"191919", secretHash, localPath, "credential_token_hash", "orbit_id", "node_id",
		"filename", "transcript", "caption", "ciphertext", "audio_payload",
	} {
		if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(forbidden)) {
			t.Fatalf("Phase 3 view leaked %q: %s", forbidden, raw)
		}
	}
}

func TestPhase3ObservabilityHTTPIsAuthorizedNoStoreAndQueryFree(t *testing.T) {
	harness := newOnboardingHarness(t)
	now := time.Now().UnixMilli()
	reader, err := harness.store.ProvisionModerationOperator(
		"Phase 3 HTTP reader", store.ModerationOperatorCapabilities{List: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase3-observability?orbit_id=1", "", reader.Token,
	), http.StatusBadRequest, errorInvalidRequest, nil)
	assertAPIError(t, apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase3-observability", "", "not-a-token",
	), http.StatusUnauthorized, errorUnauthorized, nil)
	response := apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase3-observability", "", reader.Token,
	)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q body=%s", response.Code,
			response.Header().Get("Cache-Control"), response.Body.String())
	}
	body := response.Body.String()
	for _, required := range []string{
		`"contract":"p3-observability-health-evidence.v1"`,
		`"e2ee_media_enabled":false`, `"state":"deferred_unavailable"`,
		`"manual_app_hardware":"not_run"`, `"promotion_evidence_ready":false`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("response missing %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{reader.Token, "principal_id", "actor_id", "orbit_id"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}

	harness.api.phase3Observability = nil
	assertAPIError(t, apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase3-observability", "", reader.Token,
	), http.StatusServiceUnavailable, errorServiceUnavailable, nil)
}
