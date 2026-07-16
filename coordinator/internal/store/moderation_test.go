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

func TestModerationReportProtectsOnlyReporterFetchInboxReplayAndFutureDelivery(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	reporter, err := st.CreateSelfServiceOrbit("Report-local recipient")
	if err != nil {
		t.Fatal(err)
	}
	companion := addTransmissionInstallation(t, st, source, "companion")
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3,
		transmissionTarget(reporter, true),
		transmissionTarget(companion, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	inboxByActor := make(map[int64]string)
	for index, target := range created.Targets {
		transition, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
			TransmissionID: target.TransmissionID,
			OrbitID:        target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			ExpectedRevision: target.Revision, Generation: target.Generation,
			Status: TransmissionTargetFailed, ReasonCode: TransmissionReasonConnectionLost,
			OccurredAt: now + 4 + int64(index),
		})
		if err != nil {
			t.Fatal(err)
		}
		inbox, err := scanTransmissionInboxItem(st.db.QueryRow(
			`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ? AND actor_id = ?`, transition.Transmission.ID, target.ActorID,
		))
		if err != nil {
			t.Fatal(err)
		}
		inboxByActor[target.ActorID] = inbox.ID
	}

	if allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: reporter.OrbitID,
		ActorID: reporter.ActorID, Slot: reporter.Slot,
	}); err != nil || !allowed {
		t.Fatalf("reporter pre-report ACL allowed=%v err=%v", allowed, err)
	}
	if _, err := st.CreateModerationReport(
		reporter.ActorID, reporter.ControlToken,
		CreateModerationReportParams{
			MediaID: media.ID, Reason: ModerationReasonHarassment,
			Details: "local protection", CreatedAt: now + 10,
		},
	); err != nil {
		t.Fatal(err)
	}

	reporterInbox, err := st.GetTransmissionInboxItem(inboxByActor[reporter.ActorID])
	if err != nil || reporterInbox == nil ||
		reporterInbox.Availability != TransmissionInboxUnavailable ||
		reporterInbox.RevocationReason != TransmissionReasonReported ||
		reporterInbox.RevokedAt != now+10 {
		t.Fatalf("reporter inbox=%+v err=%v", reporterInbox, err)
	}
	companionInbox, err := st.GetTransmissionInboxItem(inboxByActor[companion.ActorID])
	if err != nil || companionInbox == nil ||
		companionInbox.Availability != TransmissionInboxAvailable {
		t.Fatalf("unrelated inbox=%+v err=%v", companionInbox, err)
	}
	if _, err := st.CreateAuthorizedInboxReplay(CreateAuthorizedInboxReplayParams{
		ExpectedActorID:    reporter.ActorID,
		Identity:           Identity{Kind: IdentityBearer, Token: reporter.ControlToken},
		InboxID:            inboxByActor[reporter.ActorID],
		IdempotencyKeyHash: strings.Repeat("a", 64),
		RequestHash:        strings.Repeat("b", 64),
		RequestedDelivery:  TransmissionDeliveryOverlay,
		AcceptedAt:         now + 11,
	}); !errors.Is(err, ErrTransmissionInboxNotFound) {
		t.Fatalf("reported inbox replay error=%v", err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: reporter.OrbitID,
		ActorID: reporter.ActorID, Slot: reporter.Slot,
	}); err != nil || allowed {
		t.Fatalf("reporter post-report ACL allowed=%v err=%v", allowed, err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: companion.OrbitID,
		ActorID: companion.ActorID, Slot: companion.Slot,
	}); err != nil || !allowed {
		t.Fatalf("unrelated post-report ACL allowed=%v err=%v", allowed, err)
	}
	lateReceiptTransmission, err := st.CreateTransmission(transmissionParams(
		media, source, now+12, transmissionTarget(reporter, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	lateTarget := lateReceiptTransmission.Targets[0]
	if _, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: lateTarget.TransmissionID,
		OrbitID:        lateTarget.OrbitID, ActorID: lateTarget.ActorID, Slot: lateTarget.Slot,
		ExpectedRevision: lateTarget.Revision, Generation: lateTarget.Generation,
		Status: TransmissionTargetFailed, ReasonCode: TransmissionReasonConnectionLost,
		OccurredAt: now + 13,
	}); err != nil {
		t.Fatal(err)
	}
	lateInbox, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ?`, lateReceiptTransmission.Transmission.ID,
	))
	if err != nil || lateInbox.Availability != TransmissionInboxUnavailable ||
		lateInbox.RevocationReason != TransmissionReasonReported {
		t.Fatalf("post-report receipt inbox=%+v err=%v", lateInbox, err)
	}

	activateTransmissionApproach(t, st, source, reporter)
	params := resolvedTransmissionParams(source, media, now+20)
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
		fullTransmissionAvailability(companion, params.AcceptedAt),
		fullTransmissionAvailability(reporter, params.AcceptedAt),
	}
	future, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	statuses := make(map[int64]TransmissionTarget)
	for _, target := range future.Creation.Targets {
		statuses[target.ActorID] = target
	}
	if target := statuses[reporter.ActorID]; target.Status != TransmissionTargetBlocked || target.ReasonCode != TransmissionReasonReported {
		t.Fatalf("reported target=%+v", target)
	}
	for _, actorID := range []int64{source.ActorID, companion.ActorID} {
		if target := statuses[actorID]; target.Status != TransmissionTargetAccepted {
			t.Fatalf("unrelated target actor=%d target=%+v", actorID, target)
		}
	}
	current, err := st.GetMediaItem(media.ID)
	if err != nil || current == nil || current.Status != MediaStatusReady {
		t.Fatalf("report globally changed media=%+v err=%v", current, err)
	}
}

func TestModerationReportRollbackDoesNotHideInbox(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	reporter, err := st.CreateSelfServiceOrbit("Report rollback recipient")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(reporter, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(
		t, st, created, TransmissionTargetFailed,
		TransmissionReasonConnectionLost, now+4,
	)
	inbox, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ?`, created.Transmission.ID,
	))
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("report transaction interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "moderation_report_create_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := st.CreateModerationReport(
		reporter.ActorID, reporter.ControlToken,
		CreateModerationReportParams{
			MediaID: media.ID, Reason: ModerationReasonHarassment,
			CreatedAt: now + 5,
		},
	); !errors.Is(err, injected) {
		t.Fatalf("report rollback error=%v", err)
	}
	current, err := st.GetTransmissionInboxItem(inbox.ID)
	if err != nil || current == nil || current.Availability != TransmissionInboxAvailable {
		t.Fatalf("rolled-back report hid inbox=%+v err=%v", current, err)
	}
	var reports int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM moderation_reports
WHERE reporter_actor_id = ? AND media_id = ?`, reporter.ActorID, media.ID).Scan(&reports); err != nil || reports != 0 {
		t.Fatalf("rolled-back reports=%d err=%v", reports, err)
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
	audit, err := fixture.store.ListModerationAuditEvents(
		listOnly.Operator.ID, listOnly.Token, report.ID, 10,
	)
	if err != nil || len(audit) != 1 || audit[0].EventType != "report.created" ||
		audit[0].ReportID != report.ID || audit[0].OperatorID != "" ||
		audit[0].Action != "" {
		t.Fatalf("content-free audit=%+v err=%v", audit, err)
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
	audit, err = fixture.store.ListModerationAuditEvents(
		full.Operator.ID, full.Token, report.ID, 10,
	)
	if err != nil || len(audit) != 2 || audit[1].EventType != "evidence.read" ||
		audit[1].OperatorID != full.Operator.ID {
		t.Fatalf("evidence audit export=%+v err=%v", audit, err)
	}
	if _, err := fixture.store.ListModerationAuditEvents(
		full.Operator.ID, full.Token, "rp_00000000000000000000000000", 10,
	); !errors.Is(err, ErrModerationNotFound) {
		t.Fatalf("missing report audit error=%v", err)
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
