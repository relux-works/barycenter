package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
)

func TestAutomationRuntimeSchemaFailureIsAtomicAndRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automation-runtime-migration.db")
	injected := errors.New("automation runtime migration interrupted")
	_, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "automation_runtime_schema_before_commit" {
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
	var runtimeTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'
AND name IN ('automation_runtime_attempts','automation_builtin_media')`).Scan(&runtimeTables); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if runtimeTables != 0 {
		t.Fatalf("partial runtime tables=%d", runtimeTables)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM automation_runtime_attempts`).Scan(&runtimeTables); err != nil || runtimeTables != 0 {
		t.Fatalf("retry rows=%d err=%v", runtimeTables, err)
	}
}

func runtimeTriggerParams(fixture automationLineageFixture, secret, key string, now int64) AutomationRuntimeTriggerParams {
	return AutomationRuntimeTriggerParams{
		Secret: secret, IdempotencyKey: key, RequestDigest: strings.Repeat("a", 64),
		CueID: fixture.cue.ID, AudienceKind: automationcontract.AudienceOwnBarycenter,
		Availability: []TransmissionTargetAvailability{
			fullTransmissionAvailability(fixture.owner, now),
		},
		AttemptedAt: now,
	}
}

func TestAutomationRuntimeAtomicallyCreatesOneTransmissionAndReplays(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter}, nil, "")
	now := fixture.now + 20
	params := runtimeTriggerParams(fixture, issued.Secret, "automation-runtime-once-0001", now)
	const workers = 12
	type outcome struct {
		result AutomationRuntimeResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			result, err := fixture.store.TriggerAutomationRuntime(params)
			outcomes <- outcome{result, err}
		}()
	}
	close(start)
	group.Wait()
	close(outcomes)
	transmissionID, executionID, created := "", "", 0
	for current := range outcomes {
		if current.err != nil {
			t.Fatalf("trigger error=%v", current.err)
		}
		if current.result.Transmission.Transmission.ID == "" || current.result.Execution.ID == "" {
			t.Fatalf("empty result=%+v", current.result)
		}
		if transmissionID == "" {
			transmissionID = current.result.Transmission.Transmission.ID
			executionID = current.result.Execution.ID
		}
		if current.result.Transmission.Transmission.ID != transmissionID ||
			current.result.Execution.ID != executionID {
			t.Fatalf("split idempotency result=%+v", current.result)
		}
		if !current.result.Replayed {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
	var transmissions, executions, attempts int
	if err := fixture.store.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM transmissions WHERE id = ?),
  (SELECT COUNT(*) FROM automation_executions WHERE id = ?),
  (SELECT COUNT(*) FROM automation_runtime_attempts WHERE execution_id = ?)`,
		transmissionID, executionID, executionID).Scan(&transmissions, &executions, &attempts); err != nil {
		t.Fatal(err)
	}
	if transmissions != 1 || executions != 1 || attempts != 1 {
		t.Fatalf("rows transmission=%d execution=%d attempt=%d", transmissions, executions, attempts)
	}
	params.RequestDigest = strings.Repeat("b", 64)
	if _, err := fixture.store.TriggerAutomationRuntime(params); !errors.Is(err, ErrAutomationIdempotencyConflict) {
		t.Fatalf("conflicting replay error=%v", err)
	}
	var conflictAudits int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM automation_audit_events
WHERE principal_id = ? AND reason_code = 'idempotency_conflict'`, issued.Principal.ID).Scan(&conflictAudits); err != nil || conflictAudits != 1 {
		t.Fatalf("idempotency conflict audits=%d err=%v", conflictAudits, err)
	}
}

func TestAutomationRuntimeBoundsAttemptsConcurrencyAndPruning(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter}, nil, "")
	base := fixture.now + 100
	for i := 0; i < automationcontract.MaxAcceptedPerMinute; i++ {
		params := runtimeTriggerParams(fixture, issued.Secret,
			fmt.Sprintf("automation-denied-%04d", i), base+int64(i))
		params.CueID = "cq_01J00000000000000000000000"
		if _, err := fixture.store.TriggerAutomationRuntime(params); !errors.Is(err, ErrInsufficientCapability) {
			t.Fatalf("attempt %d error=%v", i, err)
		}
	}
	limited := runtimeTriggerParams(fixture, issued.Secret, "automation-denied-limit", base+10)
	limited.CueID = "cq_01J00000000000000000000000"
	var rate *AutomationRateLimitError
	if _, err := fixture.store.TriggerAutomationRuntime(limited); !errors.As(err, &rate) || rate.RetryAfter <= 0 {
		t.Fatalf("rate error=%v retry=%v", err, rate)
	}
	if count, err := fixture.store.AutomationRuntimeAttemptCount(fixture.owner.OrbitID); err != nil ||
		count != automationcontract.MaxAcceptedPerMinute+1 {
		t.Fatalf("bounded attempts=%d err=%v", count, err)
	}
	pruned, err := fixture.store.PruneAutomationRuntimeAttempts(
		base+AutomationExecutionRetention.Milliseconds()+100, 1000)
	if err != nil || pruned != automationcontract.MaxAcceptedPerMinute+1 {
		t.Fatalf("pruned=%d err=%v", pruned, err)
	}
}

func TestAutomationRuntimeReconcilesPastMinuteUnlinkedClaim(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter}, nil, "")
	claimedAt := fixture.now + 20
	execution, _, err := fixture.store.ClaimAutomationAPIExecution(ClaimAutomationAPIExecutionParams{
		Secret: issued.Secret, CueID: fixture.cue.ID,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		IdempotencyKey: "automation-abandoned-claim-0001",
		RequestDigest:  strings.Repeat("d", 64), ClaimedAt: claimedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := fixture.store.ReconcileAutomationRuntimeClaims(
		claimedAt + int64(2*time.Minute/time.Millisecond))
	if err != nil || reconciled != 1 {
		t.Fatalf("reconciled=%d err=%v", reconciled, err)
	}
	stored, err := scanAutomationExecution(fixture.store.db.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, execution.ID))
	if err != nil || stored.Status != "failed" || stored.ReasonCode != "runtime_restart_abandoned" {
		t.Fatalf("execution=%+v err=%v", stored, err)
	}
}

func TestAutomationRuntimeRevokeCancelsOrdinarySchedulerWork(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter}, nil, "")
	now := fixture.now + 20
	created, err := fixture.store.TriggerAutomationRuntime(runtimeTriggerParams(
		fixture, issued.Secret, "automation-revoke-cancel-0001", now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.RevokeAutomationPrincipal(fixture.owner.ActorID,
		fixture.owner.ControlToken, issued.Principal.ID, issued.Principal.Revision, now+1); err != nil {
		t.Fatal(err)
	}
	results, err := fixture.store.CancelInvalidAutomationRuntime(now+2, 100)
	if err != nil || len(results) != 1 {
		t.Fatalf("cancellations=%+v err=%v", results, err)
	}
	transmission, err := fixture.store.GetTransmission(created.Transmission.Transmission.ID)
	if err != nil || transmission == nil ||
		transmission.CancellationCause != TransmissionReasonPrincipalRevoked {
		t.Fatalf("transmission=%+v err=%v", transmission, err)
	}
	stored, err := scanAutomationExecution(fixture.store.db.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, created.Execution.ID))
	if err != nil || (stored.Status != "cancelling" && stored.Status != "cancelled") ||
		stored.ReasonCode != string(TransmissionReasonPrincipalRevoked) {
		t.Fatalf("execution=%+v err=%v", stored, err)
	}
}

func TestAutomationRuntimePublishesBuiltinThroughGenericTransmission(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	builtin, reused, err := fixture.store.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		Title: "Builtin runtime cue", BuiltinAssetID: BuiltinRecordingCueAssetID,
		BuiltinSHA256: BuiltinRecordingCueSHA256, OccurredAt: fixture.now + 6,
	})
	if err != nil || reused {
		t.Fatalf("builtin=%+v reused=%v err=%v", builtin, reused, err)
	}
	issued, err := fixture.store.IssueAutomationPrincipal(IssueAutomationPrincipalParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		DisplayName: "Builtin automation", AllowedCueIDs: []string{builtin.ID},
		AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter},
		MaxTargetCount:   8, IssuedAt: fixture.now + 7,
		ExpiresAt: fixture.now + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := fixture.now + 20
	mediaItem, err := fixture.store.EnsureAutomationBuiltinMedia(issued.Secret, builtin.ID, now)
	if err != nil || mediaItem.Kind != MediaKindBuiltinCue || mediaItem.Source != MediaSourceSystem ||
		mediaItem.StorageKey != AutomationBuiltinStorageKey(fixture.owner.OrbitID) {
		t.Fatalf("builtin media=%+v err=%v", mediaItem, err)
	}
	params := runtimeTriggerParams(fixture, issued.Secret, "automation-builtin-0001", now+1)
	params.CueID = builtin.ID
	params.Availability[0].LastSeenAt = now + 1
	result, err := fixture.store.TriggerAutomationRuntime(params)
	if err != nil || result.Transmission.Transmission.MediaID != mediaItem.ID ||
		result.Transmission.Transmission.OriginKind != TransmissionOriginBuiltin {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestAutomationRuntimeScheduleUsesOnlyCurrentCanonicalMinute(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	current := time.UnixMilli(fixture.now + int64(time.Minute/time.Millisecond)).UTC()
	current = current.Truncate(time.Minute)
	created, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "Current minute", Timezone: "UTC",
		WeekdaysMask: 1 << int(current.Weekday()), LocalMinute: current.Hour()*60 + current.Minute(),
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.db.Exec(`INSERT INTO automation_schedule_controls(
schedule_id, additional_quiet_hours_json, additional_quiet_hours_hash)
VALUES(?, '[]', ?)`, created.ID, hashToken("[]")); err != nil {
		t.Fatal(err)
	}
	now := current.UnixMilli() + 10
	results, err := fixture.store.RunDueAutomationRuntime([]TransmissionTargetAvailability{
		fullTransmissionAvailability(fixture.owner, now),
	}, now, 100)
	if err != nil || len(results) != 1 || results[0].Execution.ScheduledUTC != current.UnixMilli() {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	replay, err := fixture.store.RunDueAutomationRuntime([]TransmissionTargetAvailability{
		fullTransmissionAvailability(fixture.owner, now+1),
	}, now+1, 100)
	if err != nil || len(replay) != 0 {
		t.Fatalf("duplicate tick results=%+v err=%v", replay, err)
	}
	late, err := fixture.store.RunDueAutomationRuntime(nil,
		current.Add(time.Minute).UnixMilli()+10, 100)
	if err != nil || len(late) != 0 {
		t.Fatalf("catch-up results=%+v err=%v", late, err)
	}
}
