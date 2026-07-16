package main

import (
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
		strings.Contains(string(rawHealth), "retained_storage_bytes") {
		t.Fatalf("public accounting health=%s", rawHealth)
	}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
