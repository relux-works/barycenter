package media

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type recordingCancellationSink struct {
	mu       sync.Mutex
	requests []store.MediaDeliveryCancellation
	failNext bool
}

func (sink *recordingCancellationSink) CancelMedia(
	_ context.Context,
	request store.MediaDeliveryCancellation,
) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.requests = append(sink.requests, request)
	if sink.failNext {
		sink.failNext = false
		return errors.New("injected cancellation failure")
	}
	return nil
}

func (sink *recordingCancellationSink) snapshot() []store.MediaDeliveryCancellation {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]store.MediaDeliveryCancellation(nil), sink.requests...)
}

func readyLifecycleFixture(t *testing.T, harness *submitHarness, key string) store.MediaItem {
	t.Helper()
	creation := harness.createFinalizingUpload(t, harness.credentials, key, testWAVBytes(100))
	ready, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func TestLifecycleDeleteSweepsBytesAndDeliversFrozenPolicy(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "lifecycle-delete-success-0001")
	canonicalPath, ok := CanonicalPath(harness.service.canonicalDir, ready.StorageKey)
	if !ok {
		t.Fatal("invalid ready storage key")
	}
	lifecycle, err := NewLifecycleService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	sink := &recordingCancellationSink{failNext: true}
	lifecycle.SetDeliveryCancellationSink(sink)

	deleted, err := lifecycle.DeleteAuthorized(
		harness.credentials.ActorID, harness.credentials.ControlToken, ready.ID,
	)
	if err != nil || deleted.Status != store.MediaStatusDeleted {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("DELETE blocked on or removed bytes synchronously: %v", err)
	}
	if err := lifecycle.Sweep(context.Background()); err == nil {
		t.Fatal("first cancellation sink failure was hidden")
	}
	if _, err := os.Stat(canonicalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical bytes after sweep: %v", err)
	}
	if pending, err := harness.store.PendingMediaDeliveryCancellations(10); err != nil || len(pending) != 1 {
		t.Fatalf("pending cancellation=%+v err=%v", pending, err)
	}
	if err := lifecycle.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pending, err := harness.store.PendingMediaStorageOperations(
		store.StorageOperationCleanup, 10,
	); err != nil || len(pending) != 0 {
		t.Fatalf("pending cleanup=%+v err=%v", pending, err)
	}
	if pending, err := harness.store.PendingMediaDeliveryCancellations(10); err != nil || len(pending) != 0 {
		t.Fatalf("pending cancellation after retry=%+v err=%v", pending, err)
	}

	requests := sink.snapshot()
	if len(requests) != 2 {
		t.Fatalf("at-least-once cancellation calls=%d", len(requests))
	}
	request := requests[len(requests)-1]
	if request.PolicyVersion != store.MediaLifecyclePolicyV1 ||
		request.NotStartedAction != store.MediaNotStartedActionCancel ||
		request.ActiveAction != store.MediaActiveActionFadeStop ||
		request.InterruptedMainAction != store.MediaInterruptedMainActionResumeOnce ||
		request.Reason != store.MediaCancellationDeleted {
		t.Fatalf("cancellation policy=%+v", request)
	}
	metrics := lifecycle.Metrics()
	if !metrics.Healthy || metrics.DeleteRequestsTotal != 1 || metrics.StorageCleanupsTotal != 1 ||
		metrics.CancellationsTotal != 1 || metrics.CancellationFailures != 1 ||
		metrics.PendingStorageCleanup != 0 || metrics.PendingCancellation != 0 ||
		metrics.SweepsTotal != 2 || metrics.SweepFailuresTotal != 1 {
		t.Fatalf("lifecycle metrics=%+v", metrics)
	}
}

func TestLifecycleCleanupCrashRetryConvergesFromMissingFile(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "lifecycle-cleanup-crash-0001")
	canonicalPath, _ := CanonicalPath(harness.service.canonicalDir, ready.StorageKey)
	lifecycle, err := NewLifecycleService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	if _, err := lifecycle.DeleteAuthorized(
		harness.credentials.ActorID, harness.credentials.ControlToken, ready.ID,
	); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("process stopped after unlink")
	lifecycle.testAfterStorageRemove = func() error { return injected }
	if err := lifecycle.Sweep(context.Background()); err == nil {
		t.Fatal("interrupted cleanup was hidden")
	}
	if _, err := os.Stat(canonicalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical path survived interrupted unlink: %v", err)
	}
	if pending, err := harness.store.PendingMediaStorageOperations(
		store.StorageOperationCleanup, 10,
	); err != nil || len(pending) != 1 {
		t.Fatalf("cleanup receipt was not pending=%+v err=%v", pending, err)
	}
	lifecycle.testAfterStorageRemove = nil
	if err := lifecycle.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pending, err := harness.store.PendingMediaStorageOperations(
		store.StorageOperationCleanup, 10,
	); err != nil || len(pending) != 0 {
		t.Fatalf("cleanup did not converge=%+v err=%v", pending, err)
	}
}

func TestLifecycleExpiresReadyClipAtSevenDayBoundary(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "lifecycle-seven-day-expiry-0001")
	canonicalPath, _ := CanonicalPath(harness.service.canonicalDir, ready.StorageKey)
	lifecycle, err := NewLifecycleService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.now = func() time.Time { return time.UnixMilli(ready.ExpiresAt) }
	sink := &recordingCancellationSink{}
	lifecycle.SetDeliveryCancellationSink(sink)
	if err := lifecycle.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	expired, err := harness.store.GetMediaItem(ready.ID)
	if err != nil || expired == nil || expired.Status != store.MediaStatusExpired ||
		expired.DeletedAt != ready.ExpiresAt {
		t.Fatalf("expired media=%+v err=%v", expired, err)
	}
	if _, err := os.Stat(canonicalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired canonical bytes survived: %v", err)
	}
	requests := sink.snapshot()
	if len(requests) != 1 || requests[0].Reason != store.MediaCancellationExpired {
		t.Fatalf("expiry cancellation=%+v", requests)
	}
	metrics := lifecycle.Metrics()
	if metrics.MediaExpiredTotal != 1 || metrics.StorageCleanupsTotal != 1 ||
		metrics.CancellationsTotal != 1 || metrics.ExpirableMedia != 0 {
		t.Fatalf("expiry metrics=%+v", metrics)
	}
}

func TestLifecycleCleanupRefusesSymlinkAndLeavesTargetUntouched(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "lifecycle-symlink-refusal-0001")
	canonicalPath, _ := CanonicalPath(harness.service.canonicalDir, ready.StorageKey)
	outside := harness.mediaDir + "-outside-sentinel"
	if err := os.WriteFile(outside, []byte("do-not-delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.DeleteAuthorizedMedia(
		harness.credentials.ActorID, harness.credentials.ControlToken,
		ready.ID, harness.nextMS(),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(canonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, canonicalPath); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewLifecycleService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	if err := lifecycle.Sweep(context.Background()); err == nil {
		t.Fatal("symlink cleanup refusal was hidden")
	}
	if info, err := os.Lstat(canonicalPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("cleanup changed symlink info=%+v err=%v", info, err)
	}
	if bytes, err := os.ReadFile(outside); err != nil || string(bytes) != "do-not-delete" {
		t.Fatalf("cleanup changed outside target bytes=%q err=%v", bytes, err)
	}
	metrics := lifecycle.Metrics()
	if metrics.Healthy || metrics.StorageFailuresTotal != 1 || metrics.PendingStorageCleanup != 1 {
		t.Fatalf("symlink refusal metrics=%+v", metrics)
	}
}

func TestDeleteDuringCanonicalPublicationCannotLeaveOrphanBytes(t *testing.T) {
	harness := newSubmitHarness(t)
	creation := harness.createFinalizingUpload(
		t, harness.credentials, "lifecycle-publish-delete-race-0001", testWAVBytes(100),
	)
	linked := make(chan struct{})
	release := make(chan struct{})
	harness.service.testAfterPublish = func() error {
		close(linked)
		<-release
		return nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID)
		result <- err
	}()
	select {
	case <-linked:
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not reach linked boundary")
	}
	item, err := harness.store.GetMediaItem(creation.Media.ID)
	if err != nil || item == nil {
		t.Fatalf("processing item=%+v err=%v", item, err)
	}
	publish, err := harness.store.PendingMediaPublicationForMedia(item.ID)
	if err != nil || publish == nil {
		t.Fatalf("pending publish=%+v err=%v", publish, err)
	}
	canonicalPath, ok := CanonicalPath(harness.service.canonicalDir, publish.StorageKey)
	if !ok {
		t.Fatal("invalid publication storage key")
	}
	if _, err := harness.store.DeleteAuthorizedMedia(
		harness.credentials.ActorID, harness.credentials.ControlToken,
		item.ID, harness.nextMS(),
	); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewLifecycleService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	swept := make(chan error, 1)
	go func() { swept <- lifecycle.Sweep(context.Background()) }()
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("deleted publication unexpectedly became ready")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not leave cancelled boundary")
	}
	select {
	case err := <-swept:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup deadlocked with publisher")
	}
	if _, err := os.Stat(canonicalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan canonical bytes after publish/delete race: %v", err)
	}
	terminal, err := harness.store.GetMediaItem(item.ID)
	if err != nil || terminal == nil || terminal.Status != store.MediaStatusDeleted {
		t.Fatalf("terminal media=%+v err=%v", terminal, err)
	}
}
