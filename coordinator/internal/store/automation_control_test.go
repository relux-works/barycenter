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

func TestAutomationControlSchemaMigrationFailureIsAtomicAndRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "automation-control-migration.db")
	injected := errors.New("automation control migration interrupted")
	_, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "automation_control_schema_before_commit" {
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
	defer db.Close()
	var controlTables, baseTables int
	if err := db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN (
    'automation_control_mutation_results', 'saved_cue_order_state',
    'saved_cue_order_items', 'automation_schedule_controls',
    'automation_schedule_targets')),
  (SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'automation_schedules')`).Scan(
		&controlTables, &baseTables); err != nil {
		t.Fatal(err)
	}
	if controlTables != 0 || baseTables != 1 {
		t.Fatalf("partial control migration control=%d base=%d", controlTables, baseTables)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM automation_control_mutation_results`).Scan(&controlTables); err != nil || controlTables != 0 {
		t.Fatalf("retry control rows=%d err=%v", controlTables, err)
	}
}

func automationControlTestAuth(owner OnboardingCredentials, key, request string, now int64) AutomationControlAuth {
	return AutomationControlAuth{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		IdempotencyKeyHash: hashToken("test-key:" + key),
		RequestHash:        hashToken("test-request:" + request), Now: now,
	}
}

func TestAutomationControlSavedCueCRUDOrderIdempotencyAndIsolation(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	one := readySavedCueMedia(t, st, owner, now, now+100_000, "1", 1000, 500)
	two := readySavedCueMedia(t, st, owner, now+10, now+100_000, "2", 2000, 700)

	firstAuth := automationControlTestAuth(owner, "cue-create-1", one.ID, now+20)
	first, err := st.CreateAuthorizedSavedCue(firstAuth, CreateSavedCueControlParams{
		Title: "One", MediaID: one.ID,
	})
	if err != nil || first.Replayed || first.Cue.MediaID != one.ID || first.OrderRevision != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	replay, err := st.CreateAuthorizedSavedCue(firstAuth, CreateSavedCueControlParams{
		Title: "ignored after canonical request hash", MediaID: two.ID,
	})
	if err != nil || !replay.Replayed || replay.Cue.ID != first.Cue.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	conflict := firstAuth
	conflict.RequestHash = hashToken("different")
	if _, err := st.CreateAuthorizedSavedCue(conflict, CreateSavedCueControlParams{
		Title: "Two", MediaID: two.ID,
	}); !errors.Is(err, ErrAutomationControlIdempotencyConflict) {
		t.Fatalf("idempotency conflict=%v", err)
	}
	second, err := st.CreateAuthorizedSavedCue(
		automationControlTestAuth(owner, "cue-create-2", two.ID, now+21),
		CreateSavedCueControlParams{Title: "Two", MediaID: two.ID})
	if err != nil || second.Cue.MediaID != two.ID {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	listed, err := st.AuthorizedSavedCueControlList(owner.ActorID, owner.ControlToken)
	if err != nil || listed.OrderRevision != 1 || len(listed.Items) != 2 ||
		listed.Items[0].Cue.ID != first.Cue.ID || listed.Items[1].Position != 1 {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	ordered, err := st.ReorderAuthorizedSavedCues(
		automationControlTestAuth(owner, "cue-order", "reverse", now+22),
		[]string{second.Cue.ID, first.Cue.ID}, listed.OrderRevision)
	if err != nil || ordered.OrderRevision != 2 || ordered.CueIDs[0] != second.Cue.ID {
		t.Fatalf("ordered=%+v err=%v", ordered, err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for index, ids := range [][]string{
		{first.Cue.ID, second.Cue.ID}, {second.Cue.ID, first.Cue.ID},
	} {
		wg.Add(1)
		go func(index int, ids []string) {
			defer wg.Done()
			_, err := st.ReorderAuthorizedSavedCues(
				automationControlTestAuth(owner, fmt.Sprintf("cue-race-%d", index),
					fmt.Sprint(ids), now+int64(30+index)), ids, 2)
			results <- err
		}(index, ids)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSavedCueStateConflict):
			conflicts++
		default:
			t.Fatalf("unexpected reorder race error=%v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("race successes=%d conflicts=%d", successes, conflicts)
	}

	foreign, err := st.CreateSelfServiceOrbit("Foreign control")
	if err != nil {
		t.Fatal(err)
	}
	acceptCurrentContentPolicy(t, st, foreign, now+40)
	if _, err := st.RenameAuthorizedSavedCue(
		automationControlTestAuth(foreign, "foreign-rename", "same", now+41),
		first.Cue.ID, "probe", first.Cue.Revision); !errors.Is(err, ErrSavedCueNotFound) {
		t.Fatalf("foreign cue error=%v", err)
	}

	current, err := st.AuthorizedSavedCueControlList(owner.ActorID, owner.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	var deleteCue SavedCue
	for _, item := range current.Items {
		if item.Cue.ID == first.Cue.ID {
			deleteCue = item.Cue
		}
	}
	deleted, err := st.DeleteAuthorizedSavedCue(
		automationControlTestAuth(owner, "cue-delete", first.Cue.ID, now+50),
		first.Cue.ID, deleteCue.Revision)
	if err != nil || deleted.Cue.State != SavedCueDeleted || deleted.OrderRevision != 4 {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	deleteReplay, err := st.DeleteAuthorizedSavedCue(
		automationControlTestAuth(owner, "cue-delete", first.Cue.ID, now+50),
		first.Cue.ID, deleteCue.Revision)
	if err != nil || !deleteReplay.Replayed || deleteReplay.Cue.ID != first.Cue.ID {
		t.Fatalf("delete replay=%+v err=%v", deleteReplay, err)
	}
}

func TestAutomationControlFeatureScheduleAndOneTimePrincipal(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	featureAuth := automationControlTestAuth(fixture.owner, "feature", "enable", fixture.now+20)
	feature, err := fixture.store.ReplaceAuthorizedAutomationFeatureState(featureAuth,
		AutomationFeatureControlParams{
			SoundboardEnabled: true, AutomationEnabled: true, Timezone: "UTC",
			QuietHours:       []AutomationQuietWindow{{Weekday: 1, StartMinute: 1320, EndMinute: 360}},
			ExpectedRevision: fixture.feature.Revision,
		})
	if err != nil || feature.State.Revision != 2 || feature.State.QuietHoursJSON !=
		`[{"weekday":1,"start_minute":1320,"end_minute":360}]` {
		t.Fatalf("feature=%+v err=%v", feature, err)
	}
	featureReplay, err := fixture.store.ReplaceAuthorizedAutomationFeatureState(featureAuth,
		AutomationFeatureControlParams{})
	if err != nil || !featureReplay.Replayed || featureReplay.State.Revision != 2 {
		t.Fatalf("feature replay=%+v err=%v", featureReplay, err)
	}
	if _, _, _, err := NormalizeAutomationQuietHours([]AutomationQuietWindow{
		{Weekday: 1, StartMinute: 100, EndMinute: 300},
		{Weekday: 1, StartMinute: 200, EndMinute: 400},
	}); !errors.Is(err, ErrAutomationInvalid) {
		t.Fatalf("overlapping quiet hours error=%v", err)
	}

	scheduleAuth := automationControlTestAuth(fixture.owner, "schedule-create", "daily", fixture.now+21)
	schedule, err := fixture.store.CreateAuthorizedAutomationSchedule(scheduleAuth,
		AutomationScheduleControlParams{
			CueID: fixture.cue.ID, DisplayName: "Morning", Timezone: "UTC",
			WeekdaysMask: 127, LocalMinute: 480,
			AudienceKind:         automationcontract.AudienceOwnBarycenter,
			AdditionalQuietHours: []AutomationQuietWindow{{Weekday: 0, StartMinute: 0, EndMinute: 60}},
			PolicyRevision:       feature.State.Revision,
		})
	if err != nil || schedule.Replayed || schedule.Control.Schedule.Revision != 1 ||
		schedule.Control.Schedule.Enabled ||
		len(schedule.Control.AdditionalQuietHours) != 1 {
		t.Fatalf("schedule=%+v err=%v", schedule, err)
	}
	scheduleReplay, err := fixture.store.CreateAuthorizedAutomationSchedule(scheduleAuth,
		AutomationScheduleControlParams{})
	if err != nil || !scheduleReplay.Replayed ||
		scheduleReplay.Control.Schedule.ID != schedule.Control.Schedule.ID {
		t.Fatalf("schedule replay=%+v err=%v", scheduleReplay, err)
	}
	type scheduleResult struct {
		value AutomationScheduleControlMutation
		err   error
	}
	disableResults := make(chan scheduleResult, 2)
	for index := 0; index < 2; index++ {
		go func(index int) {
			value, err := fixture.store.SetAuthorizedAutomationScheduleEnabled(
				automationControlTestAuth(fixture.owner,
					fmt.Sprintf("schedule-disable-%d", index), "disable", fixture.now+22+int64(index)),
				schedule.Control.Schedule.ID, schedule.Control.Schedule.Revision, true)
			disableResults <- scheduleResult{value: value, err: err}
		}(index)
	}
	var disabled AutomationScheduleControlMutation
	disableSuccesses, disableConflicts := 0, 0
	for index := 0; index < 2; index++ {
		result := <-disableResults
		switch {
		case result.err == nil:
			disableSuccesses++
			disabled = result.value
		case errors.Is(result.err, ErrAutomationStateConflict):
			disableConflicts++
		default:
			t.Fatalf("disable race error=%v", result.err)
		}
	}
	if disableSuccesses != 1 || disableConflicts != 1 ||
		!disabled.Control.Schedule.Enabled || disabled.Control.Schedule.Revision != 2 {
		t.Fatalf("disable race successes=%d conflicts=%d value=%+v",
			disableSuccesses, disableConflicts, disabled)
	}
	disabledAfterRace, err := fixture.store.SetAuthorizedAutomationScheduleEnabled(
		automationControlTestAuth(fixture.owner, "schedule-disable", "disable", fixture.now+23),
		schedule.Control.Schedule.ID, disabled.Control.Schedule.Revision, false)
	if err != nil || disabledAfterRace.Control.Schedule.Enabled ||
		disabledAfterRace.Control.Schedule.Revision != 3 {
		t.Fatalf("disabled after race=%+v err=%v", disabledAfterRace, err)
	}
	enabled, err := fixture.store.SetAuthorizedAutomationScheduleEnabled(
		automationControlTestAuth(fixture.owner, "schedule-enable", "enable", fixture.now+24),
		schedule.Control.Schedule.ID, disabledAfterRace.Control.Schedule.Revision, true)
	if err != nil || !enabled.Control.Schedule.Enabled || enabled.Control.Schedule.Revision != 4 {
		t.Fatalf("enabled=%+v err=%v", enabled, err)
	}
	policyChanged, err := fixture.store.ReplaceAuthorizedAutomationFeatureState(
		automationControlTestAuth(fixture.owner, "feature-change", "new-policy", fixture.now+25),
		AutomationFeatureControlParams{
			SoundboardEnabled: true, AutomationEnabled: true, Timezone: "UTC",
			QuietHours:       []AutomationQuietWindow{},
			ExpectedRevision: feature.State.Revision,
		})
	if err != nil || policyChanged.State.Revision != 3 {
		t.Fatalf("policy changed=%+v err=%v", policyChanged, err)
	}
	disarmedSchedules, err := fixture.store.AuthorizedAutomationSchedules(
		fixture.owner.ActorID, fixture.owner.ControlToken)
	if err != nil || len(disarmedSchedules) != 1 ||
		disarmedSchedules[0].Schedule.Enabled || disarmedSchedules[0].Schedule.Revision != 5 {
		t.Fatalf("policy disarm schedules=%+v err=%v", disarmedSchedules, err)
	}

	issueAuth := automationControlTestAuth(fixture.owner, "principal-issue", "narrow", fixture.now+25)
	issued, err := fixture.store.IssueAuthorizedAutomationPrincipal(issueAuth,
		AutomationPrincipalControlParams{
			DisplayName: "Narrow", AllowedCueIDs: []string{fixture.cue.ID},
			AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter},
			MaxTargetCount:   1, ExpiresAt: fixture.now + int64((24*time.Hour)/time.Millisecond),
		})
	if err != nil || !issued.SecretAvailable || len(issued.Secret) != 64 || issued.Replayed {
		t.Fatalf("issued=%+v err=%v", issued, err)
	}
	var secretHash, storedResponse string
	if err := fixture.store.db.QueryRow(`SELECT secret_hash FROM automation_principals
WHERE id = ?`, issued.Principal.ID).Scan(&secretHash); err != nil {
		t.Fatal(err)
	}
	if secretHash == issued.Secret || strings.Contains(secretHash, issued.Secret) {
		t.Fatalf("plaintext secret persisted hash=%q", secretHash)
	}
	if err := fixture.store.db.QueryRow(`SELECT response_json
FROM automation_control_mutation_results
WHERE actor_id = ? AND idempotency_key_hash = ?`, fixture.owner.ActorID,
		issueAuth.IdempotencyKeyHash).Scan(&storedResponse); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedResponse, issued.Secret) || strings.Contains(storedResponse, secretHash) {
		t.Fatalf("secret material in replay projection=%s", storedResponse)
	}
	issueReplay, err := fixture.store.IssueAuthorizedAutomationPrincipal(issueAuth,
		AutomationPrincipalControlParams{})
	if err != nil || !issueReplay.Replayed || issueReplay.SecretAvailable || issueReplay.Secret != "" ||
		issueReplay.Principal.ID != issued.Principal.ID {
		t.Fatalf("issue replay=%+v err=%v", issueReplay, err)
	}
	listed, err := fixture.store.AuthorizedAutomationPrincipals(
		fixture.owner.ActorID, fixture.owner.ControlToken)
	if err != nil || len(listed) != 1 || listed[0].ID != issued.Principal.ID {
		t.Fatalf("principals=%+v err=%v", listed, err)
	}
	concurrentAuth := automationControlTestAuth(
		fixture.owner, "principal-concurrent", "same", fixture.now+30)
	concurrentParams := AutomationPrincipalControlParams{
		DisplayName: "Concurrent", AllowedCueIDs: []string{fixture.cue.ID},
		AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter},
		MaxTargetCount:   1, ExpiresAt: fixture.now + int64((24*time.Hour)/time.Millisecond),
	}
	principalResults := make(chan AutomationPrincipalControlIssue, 2)
	principalErrors := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			value, err := fixture.store.IssueAuthorizedAutomationPrincipal(concurrentAuth, concurrentParams)
			principalResults <- value
			principalErrors <- err
		}()
	}
	principalIDs := map[string]struct{}{}
	secretsShown := 0
	for index := 0; index < 2; index++ {
		value, err := <-principalResults, <-principalErrors
		if err != nil {
			t.Fatal(err)
		}
		principalIDs[value.Principal.ID] = struct{}{}
		if value.SecretAvailable {
			secretsShown++
		}
	}
	if len(principalIDs) != 1 || secretsShown != 1 {
		t.Fatalf("concurrent principal ids=%v secrets_shown=%d", principalIDs, secretsShown)
	}
	revoked, err := fixture.store.RevokeAuthorizedAutomationPrincipal(
		automationControlTestAuth(fixture.owner, "principal-revoke", "revoke", fixture.now+31),
		issued.Principal.ID, issued.Principal.Revision)
	if err != nil || revoked.Principal.RevokedAt == 0 {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
	if _, err := fixture.store.ResolveAutomationPrincipalSecret(issued.Secret, fixture.now+32); !errors.Is(err, ErrAutomationInvalidCredential) {
		t.Fatalf("revoked secret resolution=%v", err)
	}

	deleted, err := fixture.store.DeleteAuthorizedAutomationSchedule(
		automationControlTestAuth(fixture.owner, "schedule-delete", "delete", fixture.now+32),
		disarmedSchedules[0].Schedule.ID, disarmedSchedules[0].Schedule.Revision)
	if err != nil || deleted.Control.Schedule.Enabled || deleted.Control.Schedule.Revision != 6 {
		t.Fatalf("deleted schedule=%+v err=%v", deleted, err)
	}
	schedules, err := fixture.store.AuthorizedAutomationSchedules(
		fixture.owner.ActorID, fixture.owner.ControlToken)
	if err != nil || len(schedules) != 0 {
		t.Fatalf("post-delete schedules=%+v err=%v", schedules, err)
	}
}

func TestAutomationControlMixedVersionMalformedQuietPolicyFailsClosed(t *testing.T) {
	fixture := newAutomationLineageFixture(t, "UTC")
	legacy, err := fixture.store.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: fixture.owner.ActorID, Bearer: fixture.owner.ControlToken,
		AutomationEnabled: true, Timezone: "UTC", QuietHoursJSON: `{}`,
		ExpectedRevision: fixture.feature.Revision, OccurredAt: fixture.now + 20,
	})
	if err != nil || AutomationQuietPolicyValid(
		legacy.Timezone, legacy.QuietHoursJSON, legacy.QuietHoursHash) {
		t.Fatalf("legacy policy=%+v err=%v", legacy, err)
	}
	_, err = fixture.store.CreateAuthorizedAutomationSchedule(
		automationControlTestAuth(fixture.owner, "mixed-schedule", "legacy", fixture.now+21),
		AutomationScheduleControlParams{
			CueID: fixture.cue.ID, DisplayName: "legacy", Timezone: "UTC",
			WeekdaysMask: 1, LocalMinute: 60,
			AudienceKind:   automationcontract.AudienceOwnBarycenter,
			PolicyRevision: legacy.Revision,
		})
	if !errors.Is(err, ErrAutomationDisabled) {
		t.Fatalf("mixed-version schedule error=%v", err)
	}
}

func TestAutomationControlForeignAndUnknownTargetReferencesCollapse(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	acceptCurrentContentPolicy(t, st, owner, now)
	media := readySavedCueMedia(t, st, owner, now+1, now+100_000, "3", 1000, 500)
	cue := createMediaSavedCue(t, st, owner, media, "target", now+4)
	ownerRef, err := st.IssueTransmissionTargetReference(IssueTransmissionTargetReferenceParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		Kind: TransmissionSelectorBarycenter, OrbitID: owner.OrbitID, IssuedAt: now + 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	positive, err := st.IssueAuthorizedAutomationPrincipal(
		automationControlTestAuth(owner, "owner-target", "canonical", now+6),
		AutomationPrincipalControlParams{
			DisplayName: "owner explicit", AllowedCueIDs: []string{cue.ID},
			AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceExplicit},
			TargetReferences: []string{ownerRef}, MaxTargetCount: 1,
			ExpiresAt: now + int64(time.Hour/time.Millisecond),
		})
	if err != nil || len(positive.Principal.TargetRefDigests) != 1 ||
		positive.Principal.TargetRefDigests[0] == ownerRef {
		t.Fatalf("positive target principal=%+v err=%v", positive, err)
	}
	var rawReferenceRows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM automation_principal_target_refs
WHERE target_ref_digest = ?`, ownerRef).Scan(&rawReferenceRows); err != nil || rawReferenceRows != 0 {
		t.Fatalf("raw target rows=%d err=%v", rawReferenceRows, err)
	}
	foreign, err := st.CreateSelfServiceOrbit("Foreign target")
	if err != nil {
		t.Fatal(err)
	}
	acceptCurrentContentPolicy(t, st, foreign, now+5)
	foreignRef, err := st.IssueTransmissionTargetReference(IssueTransmissionTargetReferenceParams{
		ExpectedActorID: foreign.ActorID, Bearer: foreign.ControlToken,
		Kind: TransmissionSelectorBarycenter, OrbitID: foreign.OrbitID, IssuedAt: now + 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	params := AutomationPrincipalControlParams{
		DisplayName: "explicit", AllowedCueIDs: []string{cue.ID},
		AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceExplicit},
		TargetReferences: []string{foreignRef}, MaxTargetCount: 1,
		ExpiresAt: now + int64((time.Hour)/time.Millisecond),
	}
	_, foreignErr := st.IssueAuthorizedAutomationPrincipal(
		automationControlTestAuth(owner, "foreign-target", "same-shape", now+8), params)
	params.TargetReferences = []string{"trf_" + strings.Repeat("A", 43)}
	_, unknownErr := st.IssueAuthorizedAutomationPrincipal(
		automationControlTestAuth(owner, "unknown-target", "same-shape", now+9), params)
	if !errors.Is(foreignErr, ErrAutomationAudienceNotAllowed) ||
		!errors.Is(unknownErr, ErrAutomationAudienceNotAllowed) {
		t.Fatalf("foreign=%v unknown=%v", foreignErr, unknownErr)
	}
}
