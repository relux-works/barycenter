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

type automationLineageFixture struct {
	store   *Store
	owner   OnboardingCredentials
	media   MediaItem
	cue     SavedCue
	feature AutomationFeatureState
	now     int64
}

func newAutomationLineageFixture(t *testing.T, timezone string) automationLineageFixture {
	t.Helper()
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readySavedCueMedia(t, st, owner, now, now+int64((30*24*time.Hour)/time.Millisecond), "c", 4096, 500)
	cue := createMediaSavedCue(t, st, owner, media, "Automation cue", now+3)
	feature, err := st.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		SoundboardEnabled: true, AutomationEnabled: true, Timezone: timezone,
		QuietHoursJSON: `[]`, ExpectedRevision: 0, OccurredAt: now + 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	return automationLineageFixture{st, owner, media, cue, feature, now}
}

func issueAutomationPrincipal(t *testing.T, fixture automationLineageFixture, audiences []automationcontract.AudienceKind, targets []string, airID string) AutomationPrincipalIssue {
	t.Helper()
	issued, err := fixture.store.IssueAutomationPrincipal(IssueAutomationPrincipalParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		DisplayName: "Kitchen automation", AllowedCueIDs: []string{fixture.cue.ID},
		AllowedAudiences: audiences, TargetRefDigests: targets, BoundAirID: airID,
		MaxTargetCount: 8, IssuedAt: fixture.now + 5,
		ExpiresAt: fixture.now + 5 + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

func TestAutomationSchemaStartsProductionDarkAndPreservesSavedMedia(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	if fixture.feature.Revision != 1 || !fixture.feature.AutomationEnabled ||
		fixture.feature.PolicyVersion != automationcontract.ContractVersion ||
		fixture.feature.QuietHoursJSON != "[]" || len(fixture.feature.QuietHoursHash) != 64 {
		t.Fatalf("feature=%+v", fixture.feature)
	}
	stored, err := fixture.store.GetMediaItem(fixture.media.ID)
	if err != nil || stored == nil || stored.Status != MediaStatusReady || stored.StorageKey == "" {
		t.Fatalf("media after additive schema=%+v err=%v", stored, err)
	}
	var schedules, principals, executions int
	if err := fixture.store.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM automation_schedules),
  (SELECT COUNT(*) FROM automation_principals),
  (SELECT COUNT(*) FROM automation_executions)`).Scan(&schedules, &principals, &executions); err != nil {
		t.Fatal(err)
	}
	if schedules != 0 || principals != 0 || executions != 0 {
		t.Fatalf("schema activated runtime rows schedules=%d principals=%d executions=%d", schedules, principals, executions)
	}
}

func TestAutomationSchemaMigrationFailureIsAtomicAndRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automation-migration.db")
	injected := errors.New("automation migration interrupted")
	_, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "automation_schema_before_commit" {
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
	var automationTables, mediaTables int
	if err := db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name LIKE 'automation_%'),
  (SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'media_items')`).Scan(
		&automationTables, &mediaTables); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if automationTables != 0 || mediaTables != 1 {
		t.Fatalf("partial migration automation=%d media=%d", automationTables, mediaTables)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM automation_executions`).Scan(&automationTables); err != nil || automationTables != 0 {
		t.Fatalf("retry schema rows=%d err=%v", automationTables, err)
	}
}

func TestAutomationPrincipalSecretIsOneTimeHashedScopedAndImmediatelyRevocable(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	targetA := strings.Repeat("a", 64)
	targetB := strings.Repeat("b", 64)
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceExplicit},
		[]string{targetB, targetA, targetA}, "")
	if len(issued.Secret) != 64 || issued.Principal.Permission != "automation:trigger" ||
		len(issued.Principal.AllowedCueIDs) != 1 || len(issued.Principal.TargetRefDigests) != 2 ||
		issued.Principal.TargetRefDigests[0] != targetA {
		t.Fatalf("issue=%+v secret_length=%d", issued.Principal, len(issued.Secret))
	}
	resolved, err := fixture.store.ResolveAutomationPrincipalSecret(issued.Secret, fixture.now+6)
	if err != nil || resolved.ID != issued.Principal.ID || resolved.DisplayName != "Kitchen automation" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	var secretHash, hashVersion string
	if err := fixture.store.db.QueryRow(`SELECT secret_hash, secret_hash_version
FROM automation_principals WHERE id = ?`, issued.Principal.ID).Scan(&secretHash, &hashVersion); err != nil {
		t.Fatal(err)
	}
	if secretHash == issued.Secret || hashVersion != AutomationSecretHashVersion ||
		strings.Contains(fmt.Sprintf("%+v", issued.Principal), issued.Secret) {
		t.Fatalf("unsafe principal storage hash=%q version=%q", secretHash, hashVersion)
	}
	var plaintextRows int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM automation_principals
WHERE secret_hash = ? OR display_name = ?`, issued.Secret, issued.Secret).Scan(&plaintextRows); err != nil || plaintextRows != 0 {
		t.Fatalf("plaintext rows=%d err=%v", plaintextRows, err)
	}
	revoked, err := fixture.store.RevokeAutomationPrincipal(
		fixture.owner.ActorID, fixture.owner.ControlToken, issued.Principal.ID,
		issued.Principal.Revision, fixture.now+7,
	)
	if err != nil || revoked.RevokedAt != fixture.now+7 || revoked.RevokedByActorID != fixture.owner.ActorID {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
	if _, err := fixture.store.ResolveAutomationPrincipalSecret(issued.Secret, fixture.now+8); !errors.Is(err, ErrAutomationInvalidCredential) {
		t.Fatalf("revoked resolution error=%v", err)
	}
}

func TestAutomationAPIExecutionIdempotencyScopeAndRevocationLineage(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	targetA := strings.Repeat("c", 64)
	targetB := strings.Repeat("d", 64)
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceExplicit},
		[]string{targetA, targetB}, "")
	params := ClaimAutomationAPIExecutionParams{
		Secret: issued.Secret, CueID: fixture.cue.ID,
		AudienceKind:           automationcontract.AudienceExplicit,
		TargetReferenceDigests: []string{targetB, targetA},
		IdempotencyKey:         "api-execution-idempotency-0001",
		RequestDigest:          strings.Repeat("e", 64), ClaimedAt: fixture.now + 10,
	}
	created, replay, err := fixture.store.ClaimAutomationAPIExecution(params)
	if err != nil || replay || created.TriggerKind != automationcontract.TriggerScopedAPI ||
		created.PrincipalID != issued.Principal.ID || created.SelectorDigest == "" ||
		created.FeatureRevision != fixture.feature.Revision || created.Status != "claimed" {
		t.Fatalf("created=%+v replay=%v err=%v", created, replay, err)
	}
	replayed, replay, err := fixture.store.ClaimAutomationAPIExecution(params)
	if err != nil || !replay || replayed.ID != created.ID {
		t.Fatalf("replayed=%+v replay=%v err=%v", replayed, replay, err)
	}
	params.RequestDigest = strings.Repeat("f", 64)
	if _, _, err := fixture.store.ClaimAutomationAPIExecution(params); !errors.Is(err, ErrAutomationIdempotencyConflict) {
		t.Fatalf("conflict error=%v", err)
	}
	params.IdempotencyKey = "api-execution-idempotency-0002"
	params.RequestDigest = strings.Repeat("1", 64)
	params.TargetReferenceDigests = []string{strings.Repeat("2", 64)}
	if _, _, err := fixture.store.ClaimAutomationAPIExecution(params); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("out-of-scope target error=%v", err)
	}
	if _, err := fixture.store.RevokeAutomationPrincipal(fixture.owner.ActorID,
		fixture.owner.ControlToken, issued.Principal.ID, issued.Principal.Revision,
		fixture.now+11); err != nil {
		t.Fatal(err)
	}
	candidates, err := fixture.store.PendingAutomationCancellationCandidates(fixture.owner.OrbitID, 10)
	if err != nil || len(candidates) != 1 || candidates[0].ExecutionID != created.ID || candidates[0].Reason != "principal_revoked" {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	params.Secret = issued.Secret
	params.IdempotencyKey = "api-execution-idempotency-0003"
	params.TargetReferenceDigests = []string{targetA}
	if _, _, err := fixture.store.ClaimAutomationAPIExecution(params); !errors.Is(err, ErrAutomationInvalidCredential) {
		t.Fatalf("post-revoke claim error=%v", err)
	}
}

func TestAutomationScheduleDSTGapFoldClockJumpAndConcurrentClaims(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "America/New_York")
	fold, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "Fold", Timezone: "America/New_York",
		WeekdaysMask: 1 << int(time.Sunday), LocalMinute: 90,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	earliest := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).UnixMilli()
	later := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC).UnixMilli()
	created, replay, err := fixture.store.ClaimScheduledAutomationOccurrence(fold.ID, fold.Revision, earliest, earliest+1)
	if err != nil || replay || created.OccurrenceKey != fold.ID+"/1/2026-11-01/01:30" ||
		created.ScheduledUTC != earliest || created.TriggerKind != automationcontract.TriggerSchedule {
		t.Fatalf("fold created=%+v replay=%v err=%v", created, replay, err)
	}
	if _, _, err := fixture.store.ClaimScheduledAutomationOccurrence(fold.ID, fold.Revision, later, later+1); !errors.Is(err, ErrAutomationOccurrenceLaterFold) {
		t.Fatalf("later fold error=%v", err)
	}
	if replayed, replay, err := fixture.store.ClaimScheduledAutomationOccurrence(fold.ID, fold.Revision, earliest, earliest+2); err != nil || !replay || replayed.ID != created.ID {
		t.Fatalf("fold replay=%+v replay=%v err=%v", replayed, replay, err)
	}
	if _, _, err := fixture.store.ClaimScheduledAutomationOccurrence(fold.ID, fold.Revision, earliest, earliest+2*int64(time.Minute/time.Millisecond)); !errors.Is(err, ErrAutomationOccurrenceNotCurrent) {
		t.Fatalf("forward jump error=%v", err)
	}

	gap, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "Gap", Timezone: "America/New_York",
		WeekdaysMask: 1 << int(time.Sunday), LocalMinute: 150,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 21,
	})
	if err != nil {
		t.Fatal(err)
	}
	gapCandidate := time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC).UnixMilli()
	if _, _, err := fixture.store.ClaimScheduledAutomationOccurrence(gap.ID, gap.Revision, gapCandidate, gapCandidate+1); !errors.Is(err, ErrAutomationOccurrenceNotCurrent) {
		t.Fatalf("spring gap error=%v", err)
	}

	concurrent, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "Concurrent", Timezone: "UTC",
		WeekdaysMask: 1 << int(time.Monday), LocalMinute: 12 * 60,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 22,
	})
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC).UnixMilli()
	var wg sync.WaitGroup
	type result struct {
		execution AutomationExecution
		replay    bool
		err       error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			execution, replay, err := fixture.store.ClaimScheduledAutomationOccurrence(
				concurrent.ID, concurrent.Revision, tick, tick+int64(offset+1))
			results <- result{execution, replay, err}
		}(i)
	}
	wg.Wait()
	close(results)
	ids := map[string]struct{}{}
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		ids[result.execution.ID] = struct{}{}
		if !result.replay {
			createdCount++
		}
	}
	if len(ids) != 1 || createdCount != 1 {
		t.Fatalf("concurrent ids=%v created=%d", ids, createdCount)
	}
	if _, err := fixture.store.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		SoundboardEnabled: true, AutomationEnabled: true, EmergencyDisabled: true,
		Timezone: "America/New_York", QuietHoursJSON: `[]`,
		ExpectedRevision: fixture.feature.Revision, OccurredAt: tick + 20,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.store.ClaimScheduledAutomationOccurrence(
		concurrent.ID, concurrent.Revision, tick, tick+21); !errors.Is(err, ErrAutomationDisabled) {
		t.Fatalf("disabled occurrence replay error=%v", err)
	}
}

func TestAutomationClaimAndLeaseCrashBoundariesReconcile(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	schedule, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "Crash", Timezone: "UTC",
		WeekdaysMask: 1 << int(time.Monday), LocalMinute: 600,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC).UnixMilli()
	injected := errors.New("claim interrupted")
	fixture.store.testCheckpoint = func(name string) error {
		if name == "automation_schedule_occurrence_before_commit" {
			return injected
		}
		return nil
	}
	if _, _, err := fixture.store.ClaimScheduledAutomationOccurrence(schedule.ID, schedule.Revision, tick, tick+1); !errors.Is(err, injected) {
		t.Fatalf("claim rollback error=%v", err)
	}
	fixture.store.testCheckpoint = nil
	var count int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM automation_executions
WHERE schedule_id = ?`, schedule.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("claim rollback rows=%d err=%v", count, err)
	}
	execution, _, err := fixture.store.ClaimScheduledAutomationOccurrence(schedule.ID, schedule.Revision, tick, tick+2)
	if err != nil {
		t.Fatal(err)
	}
	leaseNow := tick + 3
	leased, err := fixture.store.ClaimAutomationExecutionLease(execution.ID, 0, "worker-a", leaseNow, leaseNow+500)
	if err != nil || leased.Status != "leased" || leased.LeaseGeneration != 1 || leased.LeaseOwnerHash == "worker-a" {
		t.Fatalf("leased=%+v err=%v", leased, err)
	}
	if _, err := fixture.store.ClaimAutomationExecutionLease(execution.ID, 0, "worker-b", leaseNow+1, leaseNow+501); !errors.Is(err, ErrAutomationStateConflict) {
		t.Fatalf("second worker error=%v", err)
	}
	reconciled, err := fixture.store.ReconcileAutomationExecutionLeases(leaseNow + 501)
	if err != nil || reconciled != 1 {
		t.Fatalf("reconciled=%d err=%v", reconciled, err)
	}
	retried, err := fixture.store.ClaimAutomationExecutionLease(execution.ID, 1, "worker-b", leaseNow+502, leaseNow+700)
	if err != nil || retried.LeaseGeneration != 2 || retried.RetryGeneration != 1 {
		t.Fatalf("retried=%+v err=%v", retried, err)
	}
}

func TestAutomationQuickDisableAndScheduleDisableExposePendingCancellation(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter}, nil, "")
	apiExecution, _, err := fixture.store.ClaimAutomationAPIExecution(ClaimAutomationAPIExecutionParams{
		Secret: issued.Secret, CueID: fixture.cue.ID,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		IdempotencyKey: "quick-disable-idempotency-0001",
		RequestDigest:  strings.Repeat("3", 64), ClaimedAt: fixture.now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "disable", Timezone: "UTC",
		WeekdaysMask: 1 << int(time.Monday), LocalMinute: 720,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC).UnixMilli()
	scheduleExecution, _, err := fixture.store.ClaimScheduledAutomationOccurrence(
		schedule.ID, schedule.Revision, tick, tick+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.DisableAutomationSchedule(fixture.owner.ActorID,
		fixture.owner.ControlToken, schedule.ID, schedule.Revision, tick+2); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		SoundboardEnabled: true, AutomationEnabled: true, EmergencyDisabled: true,
		Timezone: "UTC", QuietHoursJSON: `[]`, ExpectedRevision: fixture.feature.Revision,
		OccurredAt: tick + 3,
	}); err != nil {
		t.Fatal(err)
	}
	candidates, err := fixture.store.PendingAutomationCancellationCandidates(fixture.owner.OrbitID, 10)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	reasons := map[string]string{}
	for _, candidate := range candidates {
		reasons[candidate.ExecutionID] = candidate.Reason
	}
	if reasons[apiExecution.ID] != "automation_disabled" ||
		reasons[scheduleExecution.ID] != "schedule_disabled" {
		t.Fatalf("reasons=%v", reasons)
	}
}

func TestAutomationIssuerAuthorityLossInvalidatesClaims(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	issued := issueAutomationPrincipal(t, fixture,
		[]automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter}, nil, "")
	apiExecution, _, err := fixture.store.ClaimAutomationAPIExecution(ClaimAutomationAPIExecutionParams{
		Secret: issued.Secret, CueID: fixture.cue.ID,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		IdempotencyKey: "authority-loss-idempotency-0001",
		RequestDigest:  strings.Repeat("4", 64), ClaimedAt: fixture.now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := fixture.store.CreateAutomationSchedule(CreateAutomationScheduleParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		CueID: fixture.cue.ID, DisplayName: "authority loss", Timezone: "UTC",
		WeekdaysMask: 1 << int(time.Monday), LocalMinute: 720,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		PolicyRevision: fixture.feature.Revision, CreatedAt: fixture.now + 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC).UnixMilli()
	scheduleExecution, _, err := fixture.store.ClaimScheduledAutomationOccurrence(
		schedule.ID, schedule.Revision, tick, tick+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.DisableActorForModeration(fixture.owner.ActorID, tick+2); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ResolveAutomationPrincipalSecret(issued.Secret, tick+3); !errors.Is(err, ErrAutomationInvalidCredential) {
		t.Fatalf("authority-loss principal error=%v", err)
	}
	nextTick := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC).UnixMilli()
	if _, _, err := fixture.store.ClaimScheduledAutomationOccurrence(schedule.ID, schedule.Revision, nextTick, nextTick+1); !errors.Is(err, ErrAutomationDisabled) {
		t.Fatalf("authority-loss schedule error=%v", err)
	}
	candidates, err := fixture.store.PendingAutomationCancellationCandidates(fixture.owner.OrbitID, 10)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	reasons := map[string]string{}
	for _, candidate := range candidates {
		reasons[candidate.ExecutionID] = candidate.Reason
	}
	if reasons[apiExecution.ID] != "principal_revoked" ||
		reasons[scheduleExecution.ID] != "schedule_disabled" {
		t.Fatalf("reasons=%v", reasons)
	}
}

func TestAutomationStartupReconcilesExpiredLeaseAndAccounting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automation-restart.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateSelfServiceOrbit("automation restart")
	if err != nil {
		t.Fatal(err)
	}
	acceptCurrentContentPolicy(t, st, owner, time.Now().UnixMilli())
	now := time.Now().Add(-2 * time.Second).UnixMilli()
	media := readySavedCueMedia(t, st, owner, now, now+int64((30*24*time.Hour)/time.Millisecond), "d", 1024, 100)
	cue := createMediaSavedCue(t, st, owner, media, "restart", now+3)
	feature, err := st.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		AutomationEnabled: true, Timezone: "UTC", QuietHoursJSON: `[]`,
		OccurredAt: now + 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := st.IssueAutomationPrincipal(IssueAutomationPrincipalParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		DisplayName: "restart", AllowedCueIDs: []string{cue.ID},
		AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter},
		MaxTargetCount:   1, IssuedAt: now + 5, ExpiresAt: now + int64((24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, _, err := st.ClaimAutomationAPIExecution(ClaimAutomationAPIExecutionParams{
		Secret: issued.Secret, CueID: cue.ID,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		IdempotencyKey: "restart-idempotency-0001", RequestDigest: strings.Repeat("a", 64),
		ClaimedAt: now + 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimAutomationExecutionLease(execution.ID, 0, "crashed-worker", now+7, now+100); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var status string
	var retry int64
	if err := st.db.QueryRow(`SELECT status, retry_generation FROM automation_executions
WHERE id = ?`, execution.ID).Scan(&status, &retry); err != nil || status != "claimed" || retry != 1 {
		t.Fatalf("startup execution status=%q retry=%d err=%v", status, retry, err)
	}
	usage, err := st.AutomationLineageUsage(owner.OrbitID, time.Now().UnixMilli())
	if err != nil || usage.ActivePrincipals != 1 || usage.PendingExecutions != 1 ||
		usage.ActiveSchedules != 0 || feature.Revision != 1 {
		t.Fatalf("usage=%+v feature=%+v err=%v", usage, feature, err)
	}
}
