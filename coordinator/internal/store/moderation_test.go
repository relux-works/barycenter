package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type moderationFixture struct {
	store    *Store
	source   OnboardingCredentials
	reporter OnboardingCredentials
	media    MediaItem
	now      int64
}

func newModerationFixture(t *testing.T) moderationFixture {
	t.Helper()
	st, source := newMediaIngestTestStore(t)
	reporter, err := st.CreateSelfServiceOrbit("Moderation reporter")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	item := readyLifecycleMedia(
		t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond),
	)
	if _, err := st.CreateTransmission(transmissionParams(
		item, source, now+3, transmissionTarget(reporter, true),
	)); err != nil {
		t.Fatal(err)
	}
	return moderationFixture{
		store: st, source: source, reporter: reporter, media: item, now: now,
	}
}

func createFixtureReport(t *testing.T, fixture moderationFixture) ModerationReport {
	t.Helper()
	created, err := fixture.store.CreateModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		CreateModerationReportParams{
			MediaID: fixture.media.ID, Reason: ModerationReasonHarassment,
			Details: "unwanted audio", CreatedAt: fixture.now + 4,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return created.Report
}

func TestModerationReportUsesPersistedForeignTargetAndIsPrivacySafe(t *testing.T) {
	fixture := newModerationFixture(t)
	report := createFixtureReport(t, fixture)
	if report.ReporterActorID != fixture.reporter.ActorID ||
		report.ReportedActorID != fixture.source.ActorID ||
		report.TargetActorID != fixture.reporter.ActorID ||
		report.EvidenceStorageKey != fixture.media.StorageKey ||
		report.EvidenceExpiresAt != fixture.now+4+moderationEvidenceRetention.Milliseconds() {
		t.Fatalf("report snapshot=%+v", report)
	}
	replay, err := fixture.store.CreateModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		CreateModerationReportParams{
			MediaID: fixture.media.ID, Reason: ModerationReasonOther,
			CreatedAt: fixture.now + 5,
		},
	)
	if err != nil || !replay.Reused || replay.Report.ID != report.ID ||
		replay.Report.Reason != ModerationReasonHarassment {
		t.Fatalf("report replay=%+v err=%v", replay, err)
	}
	if _, err := fixture.store.CreateModerationReport(
		fixture.source.ActorID, fixture.source.ControlToken,
		CreateModerationReportParams{
			MediaID: fixture.media.ID, Reason: ModerationReasonSpam,
			CreatedAt: fixture.now + 6,
		},
	); !errors.Is(err, ErrModerationNotFound) {
		t.Fatalf("owner report error=%v", err)
	}
	outsider, err := fixture.store.CreateSelfServiceOrbit("Report outsider")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CreateModerationReport(
		outsider.ActorID, outsider.ControlToken,
		CreateModerationReportParams{
			MediaID: fixture.media.ID, Reason: ModerationReasonSpam,
			CreatedAt: fixture.now + 7,
		},
	); !errors.Is(err, ErrModerationNotFound) {
		t.Fatalf("outsider report error=%v", err)
	}
	if _, err := fixture.store.GetAuthorizedModerationReport(
		fixture.source.ActorID, fixture.source.ControlToken, report.ID,
	); !errors.Is(err, ErrModerationNotFound) {
		t.Fatalf("foreign report status error=%v", err)
	}
}

func TestModerationOperatorDomainsCapabilitiesEvidenceAuditAndRevocation(t *testing.T) {
	fixture := newModerationFixture(t)
	report := createFixtureReport(t, fixture)
	listOnly, err := fixture.store.ProvisionModerationOperator(
		"Queue reviewer", ModerationOperatorCapabilities{List: true}, fixture.now+5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(listOnly.Token, "mod_") {
		t.Fatalf("operator token domain=%q", listOnly.Token)
	}
	if _, err := fixture.store.ResolveTokenActorContext(listOnly.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("operator token resolved as actor: %v", err)
	}
	if _, err := fixture.store.ListModerationReports(
		listOnly.Operator.ID, listOnly.Token, "open", 10,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.AuthorizeModerationEvidence(
		listOnly.Operator.ID, listOnly.Token, report.ID, fixture.now+6,
	); !errors.Is(err, ErrModerationForbidden) {
		t.Fatalf("list-only evidence error=%v", err)
	}
	full, err := fixture.store.ProvisionModerationOperator(
		"Decision reviewer", ModerationOperatorCapabilities{
			List: true, Evidence: true, Decide: true,
		}, fixture.now+7,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := fixture.store.AuthorizeModerationEvidence(
		full.Operator.ID, full.Token, report.ID, fixture.now+8,
	)
	if err != nil || evidence.StorageKey != fixture.media.StorageKey {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if count, err := fixture.store.ModerationAuditCount(report.ID, "evidence.read"); err != nil || count != 1 {
		t.Fatalf("evidence audit count=%d err=%v", count, err)
	}
	if _, err := fixture.store.db.Exec(`UPDATE moderation_audit_events SET created_at = created_at + 1`); err == nil ||
		!strings.Contains(err.Error(), "append-only") {
		t.Fatalf("audit update tamper error=%v", err)
	}
	if _, err := fixture.store.db.Exec(`DELETE FROM moderation_audit_events`); err == nil ||
		!strings.Contains(err.Error(), "append-only") {
		t.Fatalf("audit delete tamper error=%v", err)
	}
	if changed, err := fixture.store.RevokeModerationOperator(full.Operator.ID, fixture.now+9); err != nil || !changed {
		t.Fatalf("revoke changed=%v err=%v", changed, err)
	}
	if _, err := fixture.store.ResolveModerationOperator(full.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked operator resolve error=%v", err)
	}
}

func TestModerationDecisionIsCrashResumableIdempotentAndEnforced(t *testing.T) {
	fixture := newModerationFixture(t)
	report := createFixtureReport(t, fixture)
	operator, err := fixture.store.ProvisionModerationOperator(
		"Moderator", ModerationOperatorCapabilities{Decide: true}, fixture.now+5,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := fixture.store.BeginModerationDecision(
		operator.Operator.ID, operator.Token, report.ID,
		ModerationActionDeleteMedia, fixture.now+6,
	)
	if err != nil || request.Applied || request.Decision.State != "pending" {
		t.Fatalf("decision request=%+v err=%v", request, err)
	}
	replay, err := fixture.store.BeginModerationDecision(
		operator.Operator.ID, operator.Token, report.ID,
		ModerationActionDeleteMedia, fixture.now+7,
	)
	if err != nil || !replay.Reused || replay.Decision.ID != request.Decision.ID {
		t.Fatalf("decision replay=%+v err=%v", replay, err)
	}
	if _, err := fixture.store.BeginModerationDecision(
		operator.Operator.ID, operator.Token, report.ID,
		ModerationActionNoAction, fixture.now+7,
	); !errors.Is(err, ErrModerationDecisionConflict) {
		t.Fatalf("conflicting decision error=%v", err)
	}
	deleted, err := fixture.store.DeleteMediaForModeration(report.MediaID, fixture.now+8)
	if err != nil || deleted.Status != MediaStatusDeleted {
		t.Fatalf("moderation delete=%+v err=%v", deleted, err)
	}
	if _, err := fixture.store.DeleteMediaForModeration(report.MediaID, fixture.now+9); err != nil {
		t.Fatalf("moderation delete replay=%v", err)
	}
	decision, err := fixture.store.CompleteModerationDecision(request.Decision.ID, fixture.now+10)
	if err != nil || decision.State != "applied" {
		t.Fatalf("complete decision=%+v err=%v", decision, err)
	}
	if replay, err := fixture.store.CompleteModerationDecision(request.Decision.ID, fixture.now+11); err != nil || replay != decision {
		t.Fatalf("complete replay=%+v want=%+v err=%v", replay, decision, err)
	}
	status, err := fixture.store.GetAuthorizedModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken, report.ID,
	)
	if err != nil || status.Status != "resolved" {
		t.Fatalf("report status=%+v err=%v", status, err)
	}
}

func TestModerationReportBlockReauthsAndRollsBackAtomically(t *testing.T) {
	fixture := newModerationFixture(t)
	report := createFixtureReport(t, fixture)
	if _, err := fixture.store.CreateAuthorizedModerationReportBlock(
		fixture.reporter.ActorID, fixture.reporter.NodeToken,
		report.ID, fixture.now+5,
	); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("node block error=%v", err)
	}
	injected := errors.New("block interruption")
	fixture.store.testCheckpoint = func(name string) error {
		if name == "moderation_report_block_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := fixture.store.CreateAuthorizedModerationReportBlock(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		report.ID, fixture.now+6,
	); !errors.Is(err, injected) {
		t.Fatalf("interrupted block error=%v", err)
	}
	decision, err := fixture.store.TransmissionBlockDecision(
		context.Background(), fixture.reporter.OrbitID,
		fixture.reporter.ActorID, fixture.source.OrbitID, fixture.source.ActorID,
	)
	if err != nil || decision.Blocked {
		t.Fatalf("block survived rollback=%+v err=%v", decision, err)
	}
	fixture.store.testCheckpoint = nil
	created, err := fixture.store.CreateAuthorizedModerationReportBlock(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		report.ID, fixture.now+7,
	)
	if err != nil || created.Block.Reused || created.Report.ID != report.ID {
		t.Fatalf("created block=%+v err=%v", created, err)
	}
	replay, err := fixture.store.CreateAuthorizedModerationReportBlock(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		report.ID, fixture.now+8,
	)
	if err != nil || !replay.Block.Reused || replay.Block.Block.ID != created.Block.Block.ID {
		t.Fatalf("block replay=%+v err=%v", replay, err)
	}
}

func TestModerationEvidenceHoldDefersCanonicalCleanup(t *testing.T) {
	fixture := newModerationFixture(t)
	report := createFixtureReport(t, fixture)
	if _, err := fixture.store.DeleteMediaForModeration(report.MediaID, fixture.now+5); err != nil {
		t.Fatal(err)
	}
	held, err := fixture.store.PendingMediaStorageOperationsAt(
		StorageOperationCleanup, report.EvidenceExpiresAt-1, 10,
	)
	if err != nil || len(held) != 0 {
		t.Fatalf("held cleanups=%+v err=%v", held, err)
	}
	released, err := fixture.store.PendingMediaStorageOperationsAt(
		StorageOperationCleanup, report.EvidenceExpiresAt, 10,
	)
	if err != nil || len(released) != 1 || released[0].StorageKey != fixture.media.StorageKey {
		t.Fatalf("released cleanups=%+v err=%v", released, err)
	}
	scrubbed, _, err := fixture.store.PruneModerationRetention(report.EvidenceExpiresAt)
	if err != nil || scrubbed != 1 {
		t.Fatalf("retention scrubbed=%d err=%v", scrubbed, err)
	}
	after, err := fixture.store.GetAuthorizedModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken, report.ID,
	)
	if err != nil || after.Details != "" {
		t.Fatalf("retained report details=%q err=%v", after.Details, err)
	}
}

func TestModerationDisableRevokesCanonicalCredentialsIdempotently(t *testing.T) {
	fixture := newModerationFixture(t)
	actorResult, err := fixture.store.DisableActorForModeration(
		fixture.source.ActorID, fixture.now+5,
	)
	if err != nil || !actorResult.Changed || len(actorResult.Nodes) != 1 {
		t.Fatalf("actor disable=%+v err=%v", actorResult, err)
	}
	if _, err := fixture.store.ResolveTokenActorContext(fixture.source.ControlToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled actor control auth=%v", err)
	}
	if replay, err := fixture.store.DisableActorForModeration(fixture.source.ActorID, fixture.now+6); err != nil || replay.Changed {
		t.Fatalf("actor disable replay=%+v err=%v", replay, err)
	}

	second := newModerationFixture(t)
	orbitResult, err := second.store.DisableOrbitForModeration(
		second.source.OrbitID, second.now+5,
	)
	if err != nil || !orbitResult.Changed || len(orbitResult.Nodes) != 1 {
		t.Fatalf("orbit disable=%+v err=%v", orbitResult, err)
	}
	if _, err := second.store.ResolveTokenActorContext(second.source.ControlToken); !errors.Is(err, ErrInsufficientCapability) && !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled orbit control auth=%v", err)
	}
}

func TestModerationReportRateLimitAndMigrationRollback(t *testing.T) {
	fixture := newModerationFixture(t)
	for index := 0; index < moderationReportRateLimit; index++ {
		item := fixture.media
		if index > 0 {
			item = readyLifecycleMedia(
				t, fixture.store, fixture.source, fixture.now+int64(index*10),
				fixture.now+int64((45*24*time.Hour)/time.Millisecond)+int64(index*10),
			)
			if _, err := fixture.store.CreateTransmission(transmissionParams(
				item, fixture.source, fixture.now+int64(index*10)+3,
				transmissionTarget(fixture.reporter, true),
			)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := fixture.store.CreateModerationReport(
			fixture.reporter.ActorID, fixture.reporter.ControlToken,
			CreateModerationReportParams{
				MediaID: item.ID, Reason: ModerationReasonSpam,
				CreatedAt: fixture.now + 1000 + int64(index),
			},
		); err != nil {
			t.Fatalf("report %d: %v", index, err)
		}
	}
	extra := readyLifecycleMedia(
		t, fixture.store, fixture.source, fixture.now+200,
		fixture.now+int64((45*24*time.Hour)/time.Millisecond)+200,
	)
	if _, err := fixture.store.CreateTransmission(transmissionParams(
		extra, fixture.source, fixture.now+203,
		transmissionTarget(fixture.reporter, true),
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CreateModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		CreateModerationReportParams{
			MediaID: extra.ID, Reason: ModerationReasonSpam,
			CreatedAt: fixture.now + 1011,
		},
	); !errors.Is(err, ErrModerationRateLimited) {
		t.Fatalf("rate-limit error=%v", err)
	}

	fixture.store.testCheckpoint = func(name string) error {
		if name == "moderation_decision_begin_before_commit" {
			return errors.New("decision interruption")
		}
		return nil
	}
	operator, err := fixture.store.ProvisionModerationOperator(
		"Rollback moderator", ModerationOperatorCapabilities{List: true, Decide: true},
		fixture.now+6000,
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.store.GetAuthorizedModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		func() string {
			rows, queryErr := fixture.store.ListModerationReports(
				operator.Operator.ID, operator.Token, "open", 1,
			)
			if queryErr != nil || len(rows) != 1 {
				t.Fatalf("list for rollback=%+v err=%v", rows, queryErr)
			}
			return rows[0].ID
		}(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.BeginModerationDecision(
		operator.Operator.ID, operator.Token, first.ID,
		ModerationActionNoAction, fixture.now+6001,
	); err == nil || !strings.Contains(err.Error(), "decision interruption") {
		t.Fatalf("interrupted decision error=%v", err)
	}
	if decision, err := fixture.store.GetModerationDecision(first.ID); err != nil || decision != nil {
		t.Fatalf("decision survived rollback=%+v err=%v", decision, err)
	}
}
