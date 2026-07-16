package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func TestStreamAccountingHTTPSeparatesPublicHealthFromOperatorUsageAndAudit(t *testing.T) {
	harness := newOnboardingHarness(t)
	credentials, err := harness.store.CreateSelfServiceOrbit("Accounting HTTP")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	listOnly, err := harness.store.ProvisionModerationOperator(
		"Accounting reader", store.ModerationOperatorCapabilities{List: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	decider, err := harness.store.ProvisionModerationOperator(
		"Accounting administrator", store.ModerationOperatorCapabilities{Decide: true}, now+1,
	)
	if err != nil {
		t.Fatal(err)
	}

	assertAPIError(t, apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/stream-accounting", "",
		credentials.ControlToken,
	), http.StatusUnauthorized, errorUnauthorized, nil)
	view := apiRequest(
		harness.mux, http.MethodGet,
		"/v1/moderation/stream-accounting?scope_kind=actor&scope_id="+
			jsonNumber(credentials.ActorID), "", listOnly.Token,
	)
	if view.Code != http.StatusOK {
		t.Fatalf("operator accounting status=%d body=%s", view.Code, view.Body.String())
	}
	body := view.Body.String()
	for _, forbidden := range []string{"original_filename", "title", "storage_key", credentials.ControlToken} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("operator accounting leaked %q: %s", forbidden, body)
		}
	}

	usage, err := harness.store.GetStreamAccountingUsage("actor", credentials.ActorID, now+2)
	if err != nil {
		t.Fatal(err)
	}
	policy := usage.Policy
	policy.ScopeKind = "actor"
	policy.ScopeID = credentials.ActorID
	policy.Revision = 0
	policy.UpdatedAt = 0
	policy.MaxConcurrentJobs = 1
	request, err := json.Marshal(map[string]any{
		"policy": policy, "expected_revision": 0,
		"reason": "operator_quota_adjustment_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, apiRequest(
		harness.mux, http.MethodPost, "/v1/moderation/stream-accounting/policies",
		string(request), listOnly.Token,
	), http.StatusForbidden, errorModerationForbidden, nil)
	updated := apiRequest(
		harness.mux, http.MethodPost, "/v1/moderation/stream-accounting/policies",
		string(request), decider.Token,
	)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"revision":1`) {
		t.Fatalf("policy update status=%d body=%s", updated.Code, updated.Body.String())
	}
	audit := apiRequest(
		harness.mux, http.MethodGet,
		"/v1/moderation/stream-accounting/policies/audit?scope_kind=actor&scope_id="+
			jsonNumber(credentials.ActorID)+"&limit=10", "", listOnly.Token,
	)
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), "operator_quota_adjustment_test") ||
		!strings.Contains(audit.Body.String(), decider.Operator.ID) {
		t.Fatalf("policy audit status=%d body=%s", audit.Code, audit.Body.String())
	}

	health := map[string]any{"status": "ok"}
	addStreamAccountingHealth(health, harness.store, now+3)
	rawHealth, err := json.Marshal(health)
	if err != nil {
		t.Fatal(err)
	}
	if health["status"] != "ok" || strings.Contains(string(rawHealth), "actual_egress_bytes") ||
		strings.Contains(string(rawHealth), "retained_storage_bytes") ||
		!strings.Contains(string(rawHealth), `"streamed_tracks_enabled":false`) ||
		!strings.Contains(string(rawHealth), `"ready":true`) {
		t.Fatalf("public accounting health=%s", rawHealth)
	}
}

func TestPhase2ObservabilityHTTPIsAuthorizedSanitizedAndFlagAware(t *testing.T) {
	harness := newOnboardingHarness(t)
	credentials, err := harness.store.CreateSelfServiceOrbit("Observability HTTP tenant")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	reader, err := harness.store.ProvisionModerationOperator(
		"Observability HTTP reader", store.ModerationOperatorCapabilities{List: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase2-observability", "",
		credentials.ControlToken,
	), http.StatusUnauthorized, errorUnauthorized, nil)
	assertAPIError(t, apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase2-observability?actor_id=1", "",
		reader.Token,
	), http.StatusBadRequest, errorInvalidRequest, nil)
	view := apiRequest(
		harness.mux, http.MethodGet, "/v1/moderation/phase2-observability", "", reader.Token,
	)
	if view.Code != http.StatusOK {
		t.Fatalf("observability status=%d body=%s", view.Code, view.Body.String())
	}
	body := view.Body.String()
	for _, required := range []string{
		`"contract":"p2-observability-quota-view.v1"`,
		`"streamed_tracks":{"enabled":false`,
		`"air_rooms":{"enabled":false`,
		`"targets_inbox":{"enabled":true`,
		`"client_evidence_required"`,
		`"stream_accounting_required":false`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("observability missing %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{
		"actor_id", "orbit_id", "media_id", "transmission_id", "original_filename",
		credentials.ControlToken,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("observability leaked %q: %s", forbidden, body)
		}
	}

	// Disabled Phase 2 dependencies do not make Phase 1 unhealthy.
	health := map[string]any{"status": "ok"}
	addStreamAccountingHealth(health, harness.store, now+1, false, false)
	if health["status"] != "ok" {
		t.Fatalf("flag-off health=%+v", health)
	}

	// Once Air is authoritative, canonical divergence is mandatory and must
	// fail readiness even though streamed tracks remain disabled.
	inspect, err := sql.Open("sqlite", harness.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec(`UPDATE air_authority SET mode='airs_authoritative',
generation=generation+1, divergence_count=1, updated_at=? WHERE singleton=1`, now+2); err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	if err := inspect.Close(); err != nil {
		t.Fatal(err)
	}
	health = map[string]any{"status": "ok"}
	addStreamAccountingHealth(health, harness.store, now+3, true, true)
	if health["status"] != "degraded" {
		t.Fatalf("authoritative Air divergence health=%+v", health)
	}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
