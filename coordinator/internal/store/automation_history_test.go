package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	automationcontract "relux.works/duet/coordinator/internal/automation"
)

func TestAutomationHistorySchemaFailureIsAtomicAndRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automation-history-migration.db")
	injected := errors.New("automation history migration interrupted")
	_, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "automation_history_schema_before_commit" {
			return injected
		}
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("migration error=%v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
WHERE type = 'table' AND name = 'automation_audit_events'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if tables != 0 {
		t.Fatalf("partial history tables=%d", tables)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM automation_audit_events`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("retry rows=%d err=%v", tables, err)
	}
}

func TestAutomationHistoryProjectsLineageDenialsAndForeignIsolation(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture, []automationcontract.AudienceKind{
		automationcontract.AudienceOwnBarycenter,
	}, nil, "")
	now := fixture.now + 20
	accepted, err := fixture.store.TriggerAutomationRuntime(runtimeTriggerParams(
		fixture, issued.Secret, "automation-history-accepted-0001", now))
	if err != nil {
		t.Fatal(err)
	}
	denied := runtimeTriggerParams(fixture, issued.Secret, "automation-history-denied-0001", now+1)
	denied.CueID = "cq_01J00000000000000000000000"
	if _, err := fixture.store.TriggerAutomationRuntime(denied); !errors.Is(err, ErrAutomationExecutionInProgress) {
		// The accepted transmission remains active, so concurrency has the
		// documented precedence and exact vocabulary.
		t.Fatalf("denial=%v", err)
	}
	page, err := fixture.store.QueryAuthorizedHistory(fixture.owner.ActorID,
		Identity{Kind: IdentityBearer, Token: fixture.owner.ControlToken}, "all", 100, "", now+2)
	if err != nil {
		t.Fatal(err)
	}
	var acceptedItem, deniedItem *HistoryQueryItem
	for index := range page.Items {
		item := &page.Items[index]
		if item.Transmission != nil && item.Transmission.ID == accepted.Transmission.Transmission.ID {
			acceptedItem = item
		}
		if item.ItemKind == "automation_attempt" {
			deniedItem = item
		}
	}
	if acceptedItem == nil || acceptedItem.Automation == nil ||
		acceptedItem.Automation.ExecutionID != accepted.Execution.ID ||
		acceptedItem.Automation.PrincipalRef == "" || acceptedItem.Automation.PrincipalLabel != "Kitchen automation" ||
		acceptedItem.Automation.CueRevision != fixture.cue.Revision ||
		acceptedItem.Automation.ResolvedTargetCount != 1 {
		t.Fatalf("accepted history=%+v", acceptedItem)
	}
	if deniedItem == nil || deniedItem.Automation == nil ||
		deniedItem.Automation.ReasonCode != string(automationcontract.DenyExecutionInProgress) ||
		deniedItem.Automation.Outcome != "denied" || !deniedItem.CanRevokePrincipal ||
		!deniedItem.CanEmergencyDisable {
		t.Fatalf("denied history=%+v", deniedItem)
	}
	if _, err := fixture.store.CancelAuthorizedTransmission(fixture.owner.ActorID,
		fixture.owner.ControlToken, accepted.Transmission.Transmission.ID, now+2); err != nil {
		t.Fatal(err)
	}
	var cancelAudits int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM automation_audit_events
WHERE execution_id = ? AND operation = 'automation.execution.cancel.v1'
  AND reason_code = 'sender_cancelled'`, accepted.Execution.ID).Scan(&cancelAudits); err != nil || cancelAudits != 1 {
		t.Fatalf("cancel audits=%d err=%v", cancelAudits, err)
	}
	other, err := fixture.store.CreateSelfServiceOrbit("Foreign history probe")
	if err != nil {
		t.Fatal(err)
	}
	_, foreignErr := fixture.store.GetAuthorizedHistoryItem(other.ActorID,
		Identity{Kind: IdentityBearer, Token: other.ControlToken}, deniedItem.HistoryItemID, now+2)
	_, missingErr := fixture.store.GetAuthorizedHistoryItem(other.ActorID,
		Identity{Kind: IdentityBearer, Token: other.ControlToken}, "hi_a0000000000000000000000001", now+2)
	if !errors.Is(foreignErr, ErrTransmissionNotFound) || !errors.Is(missingErr, ErrTransmissionNotFound) {
		t.Fatalf("foreign=%v missing=%v", foreignErr, missingErr)
	}
	if _, err := fixture.store.db.Exec(`UPDATE automation_audit_events SET reason_code = 'changed'`); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("audit update error=%v", err)
	}
	if _, err := fixture.store.db.Exec(`DELETE FROM automation_audit_events`); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("audit delete error=%v", err)
	}
	var leaked int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM automation_audit_events
WHERE operation LIKE '%' || ? || '%' OR principal_label LIKE '%' || ? || '%'`,
		issued.Secret, issued.Secret).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("secret leakage=%d err=%v", leaked, err)
	}
}

func TestAutomationControlAuditIsAtomicAndIdempotent(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture, []automationcontract.AudienceKind{
		automationcontract.AudienceOwnBarycenter,
	}, nil, "")
	auth := automationControlTestAuth(fixture.owner, "history-revoke", issued.Principal.ID, fixture.now+20)
	result, err := fixture.store.RevokeAuthorizedAutomationPrincipal(auth,
		issued.Principal.ID, issued.Principal.Revision)
	if err != nil || result.Replayed || result.Principal.RevokedAt == 0 {
		t.Fatalf("revoke=%+v err=%v", result, err)
	}
	replay, err := fixture.store.RevokeAuthorizedAutomationPrincipal(auth,
		issued.Principal.ID, issued.Principal.Revision)
	if err != nil || !replay.Replayed {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	var audits int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM automation_audit_events
WHERE event_kind = 'control' AND operation = 'automation.principal.revoke.v1'
  AND owner_orbit_id = ? AND actor_id = ? AND principal_id = ?`,
		fixture.owner.OrbitID, fixture.owner.ActorID, issued.Principal.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("control audits=%d err=%v", audits, err)
	}
}

func TestManualSoundboardUsesOrdinaryTransmissionAndCanonicalHistory(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	feature, err := fixture.store.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		SoundboardEnabled: true, AutomationEnabled: false, Timezone: "UTC",
		QuietHoursJSON: `[]`, ExpectedRevision: fixture.feature.Revision,
		OccurredAt: fixture.now + 10,
	})
	if err != nil || feature.AutomationEnabled || !feature.SoundboardEnabled {
		t.Fatalf("feature=%+v err=%v", feature, err)
	}
	now := fixture.now + 20
	params := ManualSoundboardTriggerParams{CueID: fixture.cue.ID,
		Transmission: CreateResolvedTransmissionParams{
			ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
			IdempotencyKeyHash: hashToken("manual-soundboard-key"),
			RequestHash:        hashToken("manual-soundboard-request"),
			AudienceKind:       TransmissionAudienceOwnBarycenter, IncludeOrigin: true,
			RequestedDelivery: TransmissionDeliveryOverlay, AcceptedAt: now,
			Availability: []TransmissionTargetAvailability{fullTransmissionAvailability(fixture.owner, now)},
		}}
	created, err := fixture.store.TriggerManualSoundboard(params)
	if err != nil || created.ExecutionID == "" || created.Creation.Transmission.ID == "" || created.Reused {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	replayed, err := fixture.store.TriggerManualSoundboard(params)
	if err != nil || !replayed.Reused || replayed.ExecutionID != created.ExecutionID ||
		replayed.Creation.Transmission.ID != created.Creation.Transmission.ID {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	page, err := fixture.store.QueryAuthorizedHistory(fixture.owner.ActorID,
		Identity{Kind: IdentityBearer, Token: fixture.owner.ControlToken}, "sent", 30, "", now+1)
	if err != nil {
		t.Fatal(err)
	}
	var found *HistoryQueryItem
	for index := range page.Items {
		if page.Items[index].Transmission != nil &&
			page.Items[index].Transmission.ID == created.Creation.Transmission.ID {
			found = &page.Items[index]
			break
		}
	}
	if found == nil || found.Automation == nil ||
		found.Automation.TriggerKind != "manual_soundboard" ||
		found.Automation.ExecutionID != created.ExecutionID ||
		found.Automation.CueRevision != fixture.cue.Revision ||
		found.Automation.ResolvedTargetCount != 1 {
		t.Fatalf("history=%+v", found)
	}
	var executions, audits int
	if err := fixture.store.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM manual_soundboard_executions WHERE id = ?),
  (SELECT COUNT(*) FROM automation_audit_events WHERE execution_id = ?
    AND operation = 'automation.trigger.manual_soundboard.v1')`,
		created.ExecutionID, created.ExecutionID).Scan(&executions, &audits); err != nil || executions != 1 || audits != 1 {
		t.Fatalf("execution=%d audits=%d err=%v", executions, audits, err)
	}
}
