package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func transitionTargetToInboxReceipt(
	t *testing.T,
	st *Store,
	created TransmissionCreation,
	status TransmissionTargetStatus,
	reason TransmissionReason,
	now int64,
) TransmissionTargetTransition {
	t.Helper()
	target := created.Targets[0]
	transition, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID:   created.Transmission.ID,
		OrbitID:          target.OrbitID,
		ActorID:          target.ActorID,
		Slot:             target.Slot,
		ExpectedRevision: target.Revision,
		Generation:       target.Generation,
		Status:           status,
		ReasonCode:       reason,
		OccurredAt:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return transition
}

func TestTransmissionInboxEligibleReceiptIsAtomicExactAndIdempotent(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Inbox target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := st.CreateSelfServiceOrbit("Inbox non-target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond))

	eligible := []struct {
		status TransmissionTargetStatus
		reason TransmissionReason
	}{
		{TransmissionTargetMissedOffline, TransmissionReasonOfflineAtAcceptance},
		{TransmissionTargetMissedOffline, TransmissionReasonOfflineBeforePrepare},
		{TransmissionTargetMissedOffline, TransmissionReasonOfflineBeforeStart},
		{TransmissionTargetMissedDND, TransmissionReasonLocalDND},
		{TransmissionTargetMissedDND, TransmissionReasonOrbitDND},
		{TransmissionTargetMissedNotReady, TransmissionReasonPrepareDeadline},
		{TransmissionTargetFailed, TransmissionReasonConnectionLost},
		{TransmissionTargetFailed, TransmissionReasonDeviceUnavailable},
		{TransmissionTargetFailed, TransmissionReasonAudioGraphFailed},
	}
	var firstTransition TransmissionTargetTransition
	for index, receipt := range eligible {
		acceptedAt := now + 10 + int64(index*10)
		created, err := st.CreateTransmission(transmissionParams(
			media, source, acceptedAt, transmissionTarget(target, true),
		))
		if err != nil {
			t.Fatal(err)
		}
		if created.Targets[0].CapabilitySetHash != transmissionTargetCapabilityHash(true, true, true, true) ||
			created.Targets[0].ResolvedAtMS != acceptedAt {
			t.Fatalf("target snapshot=%+v", created.Targets[0])
		}
		transition := transitionTargetToInboxReceipt(
			t, st, created, receipt.status, receipt.reason, acceptedAt+1,
		)
		if index == 0 {
			firstTransition = transition
		}
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_inbox_items`).Scan(&count); err != nil || count != len(eligible) {
		t.Fatalf("inbox count=%d err=%v", count, err)
	}

	// An exact receipt retry is accepted but cannot create a second row.
	retry, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: firstTransition.Target.TransmissionID,
		OrbitID:        firstTransition.Target.OrbitID, ActorID: firstTransition.Target.ActorID,
		Slot: firstTransition.Target.Slot, ExpectedRevision: firstTransition.Target.Revision,
		Generation: firstTransition.Target.Generation, Status: firstTransition.Target.Status,
		ReasonCode: firstTransition.Target.ReasonCode, OccurredAt: firstTransition.Target.UpdatedAt,
	})
	if err != nil || retry.Changed {
		t.Fatalf("receipt retry=%+v err=%v", retry, err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_inbox_items`).Scan(&count); err != nil || count != len(eligible) {
		t.Fatalf("inbox count after retry=%d err=%v", count, err)
	}

	targetContext, err := st.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	page, err := st.ListTransmissionInboxItems(ListTransmissionInboxParams{
		Target: targetContext, View: "all", Limit: 3, Now: now + 1000,
	})
	if err != nil || len(page.Items) != 3 || page.Next.ID == "" || page.Upper.ID == "" {
		t.Fatalf("first page=%+v err=%v", page, err)
	}
	// A row accepted after the frozen upper key cannot appear on page two.
	newCreated, err := st.CreateTransmission(transmissionParams(
		media, source, now+2000, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, newCreated,
		TransmissionTargetMissedDND, TransmissionReasonLocalDND, now+2001)
	second, err := st.ListTransmissionInboxItems(ListTransmissionInboxParams{
		Target: targetContext, View: "all", Limit: 100,
		Upper: page.Upper, After: page.Next, Now: now + 2002,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range second.Items {
		if item.TransmissionID == newCreated.Transmission.ID {
			t.Fatalf("new item expanded frozen page: %+v", item)
		}
	}

	nontargetContext, err := st.ResolveTokenActorContext(nontarget.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := st.ListTransmissionInboxItems(ListTransmissionInboxParams{
		Target: nontargetContext, View: "all", Limit: 20, Now: now + 2002,
	})
	if err != nil || len(empty.Items) != 0 {
		t.Fatalf("non-target page=%+v err=%v", empty, err)
	}

	// Replacing the installation binding revokes both inbox discovery and the
	// existing generic media ACL; no current membership lookup can restore it.
	newPairedAt := targetContext.ActorID + now + 3000
	if _, err := st.db.Exec(`UPDATE slots SET paired_at = ? WHERE orbit_id = ? AND slot = ?`,
		newPairedAt, target.OrbitID, target.Slot); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE installation_credentials SET slot_paired_at = ?
WHERE actor_id = ?`, newPairedAt, target.ActorID); err != nil {
		t.Fatal(err)
	}
	reboundContext, err := st.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	rebound, err := st.ListTransmissionInboxItems(ListTransmissionInboxParams{
		Target: reboundContext, View: "all", Limit: 20, Now: now + 3001,
	})
	if err != nil || len(rebound.Items) != 0 {
		t.Fatalf("replacement inherited inbox=%+v err=%v", rebound, err)
	}
	allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
	})
	if err != nil || allowed {
		t.Fatalf("replacement inherited media allowed=%v err=%v", allowed, err)
	}
}

func TestTransmissionInboxIneligibleReceiptsAndCanonicalRevocation(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Inbox revocation target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond))
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, created,
		TransmissionTargetFailed, TransmissionReasonMediaAuthFailed, now+4)
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_inbox_items`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("ineligible inbox count=%d err=%v", count, err)
	}

	eligibleCreated, err := st.CreateTransmission(transmissionParams(
		media, source, now+5, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, eligibleCreated,
		TransmissionTargetFailed, TransmissionReasonConnectionLost, now+6)
	item, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT ` + transmissionInboxColumns + ` FROM transmission_inbox_items LIMIT 1`,
	))
	if err != nil || item.Availability != TransmissionInboxAvailable {
		t.Fatalf("eligible item=%+v err=%v", item, err)
	}
	if _, err := st.DeleteMediaItem(media.ID, media.Revision, now+7); err != nil {
		t.Fatal(err)
	}
	revoked, err := st.GetTransmissionInboxItem(item.ID)
	if err != nil || revoked == nil || revoked.Availability != TransmissionInboxUnavailable ||
		revoked.RevocationReason != TransmissionReasonMediaDeleted || revoked.RevokedAt != now+7 {
		t.Fatalf("deleted-media inbox=%+v err=%v", revoked, err)
	}

	moderationMedia := readyLifecycleMedia(t, st, source, now+10,
		now+10+int64((45*24*time.Hour)/time.Millisecond))
	moderationCreated, err := st.CreateTransmission(transmissionParams(
		moderationMedia, source, now+13, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, moderationCreated,
		TransmissionTargetFailed, TransmissionReasonDeviceUnavailable, now+14)
	moderationItem, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ?`, moderationCreated.Transmission.ID,
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DisableActorForModeration(source.ActorID, now+15); err != nil {
		t.Fatal(err)
	}
	disabled, err := st.GetTransmissionInboxItem(moderationItem.ID)
	if err != nil || disabled == nil ||
		disabled.Availability != TransmissionInboxUnavailable ||
		disabled.RevocationReason != TransmissionReasonModerationDisabled ||
		disabled.RevokedAt != now+15 {
		t.Fatalf("disabled-source inbox=%+v err=%v", disabled, err)
	}
}

func TestTransmissionInboxReplayLineageIsStableAndConsumesOriginal(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Inbox replay target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond))
	original, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, original,
		TransmissionTargetMissedOffline, TransmissionReasonOfflineBeforeStart, now+4)
	originalInbox, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ?`, original.Transmission.ID,
	))
	if err != nil {
		t.Fatal(err)
	}

	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	replayParams := transmissionParams(
		media, source, now+5, transmissionTarget(target, true),
	)
	replay, err := st.createTransmissionTx(tx, replayParams, media)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitTransmissionReplayLineageTx(
		tx, originalInbox.ID, replay.Transmission.ID, now+5,
	); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	consumed, err := st.GetTransmissionInboxItem(originalInbox.ID)
	if err != nil || consumed == nil || consumed.Availability != TransmissionInboxReplayed ||
		consumed.ConsumedAt != now+5 {
		t.Fatalf("consumed original=%+v err=%v", consumed, err)
	}
	transitionTargetToInboxReceipt(t, st, replay,
		TransmissionTargetFailed, TransmissionReasonConnectionLost, now+6)
	child, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ?`, replay.Transmission.ID,
	))
	if err != nil {
		t.Fatal(err)
	}
	if child.ReplayOfInboxID != originalInbox.ID ||
		child.ReplayOfTransmissionID != original.Transmission.ID ||
		child.ReplayRootTransmissionID != original.Transmission.ID ||
		child.ReplayDepth != 1 {
		t.Fatalf("child lineage=%+v", child)
	}
}

func TestTransmissionInboxAdditiveUpgradeBackfillsSnapshotAndReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox-upgrade.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := st.CreateSelfServiceOrbit("Inbox upgrade source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.CreateSelfServiceOrbit("Inbox upgrade target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond))
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, created,
		TransmissionTargetMissedOffline, TransmissionReasonOfflineBeforeStart, now+4)
	// Recreate the immediately preceding target shape and trigger while keeping
	// its accepted receipt. This exercises the production Open migration path.
	for _, statement := range []string{
		`DROP TABLE transmission_replay_lineage`,
		`DROP TABLE transmission_inbox_cursors`,
		`DROP TABLE transmission_inbox_items`,
		`DROP INDEX transmission_targets_receipt_history`,
		`DROP INDEX transmission_targets_inbox_owner`,
		`DROP TRIGGER transmission_targets_snapshot_immutable`,
		`ALTER TABLE transmission_targets DROP COLUMN capability_set_hash`,
		`ALTER TABLE transmission_targets DROP COLUMN resolved_at_ms`,
		`CREATE TRIGGER transmission_targets_snapshot_immutable
BEFORE UPDATE ON transmission_targets
WHEN NEW.transmission_id <> OLD.transmission_id
  OR NEW.orbit_id <> OLD.orbit_id OR NEW.actor_id <> OLD.actor_id
  OR NEW.slot <> OLD.slot OR NEW.binding_paired_at <> OLD.binding_paired_at
  OR NEW.online_at_acceptance <> OLD.online_at_acceptance
  OR NEW.media_clip_capable <> OLD.media_clip_capable
  OR NEW.overlay_capable <> OLD.overlay_capable
  OR NEW.interrupt_capable <> OLD.interrupt_capable
  OR NEW.interrupt_resume_ready <> OLD.interrupt_resume_ready
BEGIN SELECT RAISE(ABORT, 'transmission target snapshot is immutable'); END`,
	} {
		if _, err := st.db.Exec(statement); err != nil {
			t.Fatalf("downgrade statement %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	targets, err := st.TransmissionTargets(created.Transmission.ID)
	if err != nil || len(targets) != 1 || targets[0].ResolvedAtMS != created.Transmission.AcceptedAt ||
		targets[0].CapabilitySetHash != transmissionTargetCapabilityHash(true, true, true, true) {
		t.Fatalf("upgraded targets=%+v err=%v", targets, err)
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_inbox_items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("backfilled inbox count=%d err=%v", count, err)
	}
	if err := foreignKeyCheck(st.db); err != nil {
		t.Fatal(err)
	}
}

func TestTransmissionInboxCurrentBindingFailureIsNonexistent(t *testing.T) {
	st, _ := newMediaIngestTestStore(t)
	_, err := st.ListTransmissionInboxItems(ListTransmissionInboxParams{
		Target: ActorContext{ActorID: 999, OrbitID: 999, Slot: "a", Capabilities: CapabilityNode},
		View:   "all", Limit: 20, Now: time.Now().UnixMilli(),
	})
	if !errors.Is(err, ErrTransmissionInboxNotFound) {
		t.Fatalf("unknown binding error=%v", err)
	}
}
