package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
)

func TestPhase3AutomationObservabilityIsBoundedAndContentFree(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	now := fixture.now + 1000
	issued := issueAutomationPrincipal(t, fixture, []automationcontract.AudienceKind{
		automationcontract.AudienceOwnBarycenter,
	}, nil, "")
	accepted := runtimeTriggerParams(fixture, issued.Secret, "phase3-observability-accepted", now+1)
	if result, err := fixture.store.TriggerAutomationRuntime(accepted); err != nil || result.Execution.ID == "" {
		t.Fatalf("accepted=%+v err=%v", result, err)
	}
	denied := runtimeTriggerParams(fixture, issued.Secret, "phase3-observability-denied", now+2)
	denied.CueID = "cq_01J00000000000000000000000"
	if result, err := fixture.store.TriggerAutomationRuntime(denied); err == nil {
		t.Fatalf("denied=%+v err=%v", result, err)
	}
	view, err := fixture.store.Phase3AutomationObservabilitySnapshot(now + 3)
	if err != nil {
		t.Fatal(err)
	}
	if view.Feature.ObservedScopes != 1 || view.Feature.SoundboardEnabled != 1 ||
		view.Feature.AutomationEnabled != 1 || view.Attempts.Total24h != 2 ||
		view.Attempts.Accepted24h != 1 || view.Attempts.Denied24h != 1 ||
		view.Attempts.RateLimited24h != 0 || view.Resources.AuditEvents24h != 2 {
		t.Fatalf("phase3 automation view=%+v", view)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"orbit_id", "actor_id", "cue_id", "schedule_id", "principal_id",
		issued.Secret, issued.Principal.ID, accepted.CueID,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("sanitized Phase 3 view leaked %q: %s", forbidden, raw)
		}
	}
}

func TestAuthorizedPhase3ObservabilityRechecksListCapability(t *testing.T) {
	st, _ := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	reader, err := st.ProvisionModerationOperator(
		"Phase 3 observability reader", ModerationOperatorCapabilities{List: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAuthorizedPhase3AutomationObservability(
		reader.Operator.ID, reader.Token, now+1,
	); err != nil {
		t.Fatal(err)
	}
	if revoked, err := st.RevokeModerationOperator(reader.Operator.ID, now+2); err != nil || !revoked {
		t.Fatalf("revoke=%v err=%v", revoked, err)
	}
	if _, err := st.GetAuthorizedPhase3AutomationObservability(
		reader.Operator.ID, reader.Token, now+3,
	); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked observability error=%v", err)
	}
}
