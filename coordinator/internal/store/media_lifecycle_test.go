package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func readyLifecycleMedia(
	t *testing.T,
	st *Store,
	credentials OnboardingCredentials,
	now int64,
	expiresAt int64,
) MediaItem {
	t.Helper()
	item, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         MediaKindVoiceClip,
		Source:       MediaSourceApp,
		Title:        "lifecycle-fixture",
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := st.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := st.CompleteMediaPublication(
		operation.ID,
		operation.Revision,
		MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000, SizeBytes: 176444,
			SHA256:       strings.Repeat("d", 64),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		},
		now+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func TestAuthorizedMediaDeleteIsNonDisclosingAtomicAndIdempotent(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	foreign, err := st.CreateSelfServiceOrbit("Foreign lifecycle caller")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, st, owner, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)

	if _, err := st.DeleteAuthorizedMedia(
		foreign.ActorID, foreign.ControlToken, ready.ID, now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("foreign delete error=%v", err)
	}
	if _, err := st.DeleteAuthorizedMedia(
		owner.ActorID, owner.ControlToken, "m_00000000000000000000000000", now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("unknown delete error=%v", err)
	}
	if _, err := st.DeleteAuthorizedMedia(
		owner.ActorID, owner.NodeToken, ready.ID, now+3,
	); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("node delete error=%v", err)
	}

	injected := errors.New("delete commit interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "media_authorized_delete_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := st.DeleteAuthorizedMedia(
		owner.ActorID, owner.ControlToken, ready.ID, now+4,
	); !errors.Is(err, injected) {
		t.Fatalf("interrupted delete error=%v", err)
	}
	afterRollback, err := st.GetMediaItem(ready.ID)
	if err != nil || afterRollback == nil || afterRollback.Status != MediaStatusReady ||
		afterRollback.StorageKey != ready.StorageKey {
		t.Fatalf("media after delete rollback=%+v err=%v", afterRollback, err)
	}
	if cancellations, err := st.PendingMediaDeliveryCancellations(10); err != nil || len(cancellations) != 0 {
		t.Fatalf("cancellations after rollback=%+v err=%v", cancellations, err)
	}

	st.testCheckpoint = nil
	deleted, err := st.DeleteAuthorizedMedia(
		owner.ActorID, owner.ControlToken, ready.ID, now+5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != MediaStatusDeleted || deleted.StorageKey != "" ||
		deleted.DeletedAt != now+5 || deleted.Revision != ready.Revision+1 {
		t.Fatalf("deleted media=%+v", deleted)
	}
	cleanups, err := st.PendingMediaStorageOperations(StorageOperationCleanup, 10)
	if err != nil || len(cleanups) != 1 || cleanups[0].StorageKey != ready.StorageKey {
		t.Fatalf("delete cleanup=%+v err=%v", cleanups, err)
	}
	cancellations, err := st.PendingMediaDeliveryCancellations(10)
	if err != nil || len(cancellations) != 1 {
		t.Fatalf("delete cancellations=%+v err=%v", cancellations, err)
	}
	cancellation := cancellations[0]
	if cancellation.MediaID != ready.ID || cancellation.MediaRevision != deleted.Revision ||
		cancellation.Reason != MediaCancellationDeleted ||
		cancellation.PolicyVersion != MediaLifecyclePolicyV1 ||
		cancellation.NotStartedAction != MediaNotStartedActionCancel ||
		cancellation.ActiveAction != MediaActiveActionFadeStop ||
		cancellation.InterruptedMainAction != MediaInterruptedMainActionResumeOnce {
		t.Fatalf("delete policy=%+v", cancellation)
	}

	replayed, err := st.DeleteAuthorizedMedia(
		owner.ActorID, owner.ControlToken, ready.ID, now+6,
	)
	if err != nil || replayed != deleted {
		t.Fatalf("idempotent delete=%+v err=%v", replayed, err)
	}
	if cancellations, err = st.PendingMediaDeliveryCancellations(10); err != nil || len(cancellations) != 1 {
		t.Fatalf("duplicate cancellation=%+v err=%v", cancellations, err)
	}
}

func TestMediaRetentionQueuesExpiryAndCancellationRetry(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(t, st, owner, now, now+100)

	if items, err := st.ExpiredMediaItems(now+99, 10); err != nil || len(items) != 0 {
		t.Fatalf("early retention items=%+v err=%v", items, err)
	}
	items, err := st.ExpiredMediaItems(now+100, 10)
	if err != nil || len(items) != 1 || items[0].ID != ready.ID {
		t.Fatalf("retention items=%+v err=%v", items, err)
	}
	expired, err := st.ExpireMediaItem(ready.ID, ready.Revision, now+100)
	if err != nil || expired.Status != MediaStatusExpired {
		t.Fatalf("expired=%+v err=%v", expired, err)
	}
	cancellations, err := st.PendingMediaDeliveryCancellations(10)
	if err != nil || len(cancellations) != 1 || cancellations[0].Reason != MediaCancellationExpired {
		t.Fatalf("expiry cancellation=%+v err=%v", cancellations, err)
	}

	injected := errors.New("cancellation receipt interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "media_delivery_cancellation_complete_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := st.CompleteMediaDeliveryCancellation(
		cancellations[0].MediaID, cancellations[0].Revision, now+101,
	); !errors.Is(err, injected) {
		t.Fatalf("interrupted cancellation completion=%v", err)
	}
	if pending, err := st.PendingMediaDeliveryCancellations(10); err != nil || len(pending) != 1 {
		t.Fatalf("pending cancellation after rollback=%+v err=%v", pending, err)
	}
	st.testCheckpoint = nil
	completed, err := st.CompleteMediaDeliveryCancellation(
		cancellations[0].MediaID, cancellations[0].Revision, now+102,
	)
	if err != nil || completed.State != StorageOperationDone {
		t.Fatalf("completed cancellation=%+v err=%v", completed, err)
	}
	if _, err := st.CompleteMediaDeliveryCancellation(
		completed.MediaID, completed.Revision, now+103,
	); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("duplicate cancellation completion=%v", err)
	}
}

func TestAuthorizedMediaDeleteConcurrentRetriesCreateOneOutcome(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, st, owner, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	const workers = 24
	start := make(chan struct{})
	errorsByWorker := make([]error, workers)
	items := make([]MediaItem, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			items[index], errorsByWorker[index] = st.DeleteAuthorizedMedia(
				owner.ActorID, owner.ControlToken, ready.ID, now+int64(10+index),
			)
		}(index)
	}
	close(start)
	wait.Wait()
	for index, err := range errorsByWorker {
		if err != nil || items[index].Status != MediaStatusDeleted {
			t.Fatalf("worker %d item=%+v err=%v", index, items[index], err)
		}
	}
	pending, err := st.PendingMediaDeliveryCancellations(10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("concurrent cancellation=%+v err=%v", pending, err)
	}
	cleanups, err := st.PendingMediaStorageOperations(StorageOperationCleanup, 10)
	if err != nil || len(cleanups) != 1 {
		t.Fatalf("concurrent cleanup=%+v err=%v", cleanups, err)
	}
}

func TestMediaDeleteExpiryRaceHasOneTerminalPolicy(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(t, st, owner, now, now+100)
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := st.DeleteAuthorizedMedia(
			owner.ActorID, owner.ControlToken, ready.ID, now+100,
		)
		results <- err
	}()
	go func() {
		<-start
		_, err := st.ExpireMediaItem(ready.ID, ready.Revision, now+100)
		results <- err
	}()
	close(start)
	first, second := <-results, <-results
	successes := 0
	for _, err := range []error{first, second} {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrMediaNotFound) && !errors.Is(err, ErrMediaStateConflict) {
			t.Fatalf("unexpected race error=%v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("terminal race successes=%d errors=(%v,%v)", successes, first, second)
	}
	terminal, err := st.GetMediaItem(ready.ID)
	if err != nil || terminal == nil ||
		(terminal.Status != MediaStatusDeleted && terminal.Status != MediaStatusExpired) {
		t.Fatalf("terminal item=%+v err=%v", terminal, err)
	}
	cancellations, err := st.PendingMediaDeliveryCancellations(10)
	if err != nil || len(cancellations) != 1 || cancellations[0].MediaRevision != terminal.Revision {
		t.Fatalf("terminal cancellation=%+v item=%+v err=%v", cancellations, terminal, err)
	}
	wantReason := MediaCancellationDeleted
	if terminal.Status == MediaStatusExpired {
		wantReason = MediaCancellationExpired
	}
	if cancellations[0].Reason != wantReason {
		t.Fatalf("terminal cancellation reason=%q want=%q", cancellations[0].Reason, wantReason)
	}
}

func TestMediaStorageCleanupCannotDeleteReadyMedia(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, st, owner, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduleStorageCleanupTx(
		tx, ready.ID, ready.StorageKey, ready.Revision, now+3,
	); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	operations, err := st.PendingMediaStorageOperations(StorageOperationCleanup, 10)
	if err != nil || len(operations) != 1 {
		t.Fatalf("injected cleanup=%+v err=%v", operations, err)
	}
	if _, err := st.MediaStorageCleanupCandidate(
		operations[0].ID, operations[0].Revision,
	); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("ready cleanup candidate error=%v", err)
	}
	if _, err := st.CompleteMediaStorageCleanup(
		operations[0].ID, operations[0].Revision, now+4,
	); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("ready cleanup completion error=%v", err)
	}
}

func TestMediaIngestAuditRetentionPrunesOnlyOlderThanNinetyDays(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	old := now - int64((100*24*time.Hour)/time.Millisecond)
	if _, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID, ActorID: owner.ActorID,
		Kind: MediaKindVoiceClip, Source: MediaSourceApp,
		CreatedAt: old, ExpiresAt: old + int64((7*24*time.Hour)/time.Millisecond),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID, ActorID: owner.ActorID,
		Kind: MediaKindVoiceClip, Source: MediaSourceApp,
		CreatedAt: now, ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	}); err != nil {
		t.Fatal(err)
	}
	cutoff := now - int64((90*24*time.Hour)/time.Millisecond)
	pruned, err := st.PruneMediaIngestAudit(cutoff, 10)
	if err != nil || pruned != 1 {
		t.Fatalf("pruned=%d err=%v", pruned, err)
	}
	var oldEvents, recentEvents int
	if err := st.db.QueryRow(`SELECT
  SUM(CASE WHEN created_at <= ? THEN 1 ELSE 0 END),
  SUM(CASE WHEN created_at > ? THEN 1 ELSE 0 END)
FROM media_ingest_audit_events`, cutoff, cutoff).Scan(&oldEvents, &recentEvents); err != nil {
		t.Fatal(err)
	}
	if oldEvents != 0 || recentEvents != 1 {
		t.Fatalf("audit events old=%d recent=%d", oldEvents, recentEvents)
	}
}

func TestMediaLifecycleReconcileBackfillsPredecessorTombstone(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, st, owner, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	deleted, err := st.DeleteMediaItem(ready.ID, ready.Revision, now+3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`DELETE FROM media_delivery_cancellations WHERE media_id = ?`, deleted.ID); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("lifecycle reconcile interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "media_lifecycle_reconcile_before_commit" {
			return injected
		}
		return nil
	}
	if err := st.reconcileMediaLifecycleOutboxes(); !errors.Is(err, injected) {
		t.Fatalf("interrupted reconcile error=%v", err)
	}
	if pending, err := st.PendingMediaDeliveryCancellations(10); err != nil || len(pending) != 0 {
		t.Fatalf("reconcile rollback pending=%+v err=%v", pending, err)
	}
	st.testCheckpoint = nil
	if err := st.reconcileMediaLifecycleOutboxes(); err != nil {
		t.Fatal(err)
	}
	pending, err := st.PendingMediaDeliveryCancellations(10)
	if err != nil || len(pending) != 1 || pending[0].MediaID != deleted.ID ||
		pending[0].MediaRevision != deleted.Revision || pending[0].CreatedAt != deleted.DeletedAt {
		t.Fatalf("backfilled cancellation=%+v err=%v", pending, err)
	}
	if err := st.reconcileMediaLifecycleOutboxes(); err != nil {
		t.Fatal(err)
	}
	if pending, err = st.PendingMediaDeliveryCancellations(10); err != nil || len(pending) != 1 {
		t.Fatalf("idempotent backfill=%+v err=%v", pending, err)
	}
}
