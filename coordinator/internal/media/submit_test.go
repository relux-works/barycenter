package media

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type submitHarness struct {
	store       *store.Store
	credentials store.OnboardingCredentials
	service     *SubmitService
	runner      *fakeCommandRunner
	mediaDir    string
	clock       atomic.Int64
}

func newSubmitHarness(t *testing.T) *submitHarness {
	t.Helper()
	root := t.TempDir()
	st, err := store.OpenWithOptions(
		filepath.Join(root, "coordinator.db"),
		store.Options{SelfServiceOnboarding: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	credentials, err := st.CreateSelfServiceOrbit("SubmitMedia test")
	if err != nil {
		t.Fatal(err)
	}
	runner := newFakeCommandRunner()
	processor := newProcessorForTest(runner, DefaultLimits())
	mediaDir := filepath.Join(root, "media")
	service, err := newSubmitService(st, mediaDir, PresetDefault, processor)
	if err != nil {
		t.Fatal(err)
	}
	harness := &submitHarness{
		store: st, credentials: credentials, service: service,
		runner: runner, mediaDir: mediaDir,
	}
	harness.clock.Store(time.Now().UnixMilli())
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	return harness
}

func (harness *submitHarness) nextMS() int64 {
	return harness.clock.Add(1)
}

func (harness *submitHarness) createFinalizingUpload(
	t *testing.T,
	credentials store.OnboardingCredentials,
	idempotencyKey string,
	raw []byte,
) store.MediaUploadCreation {
	t.Helper()
	now := harness.nextMS()
	creation, err := harness.store.CreateMediaUpload(store.CreateMediaUploadParams{
		Media: store.CreateMediaItemParams{
			OwnerOrbitID: credentials.OrbitID,
			ActorID:      credentials.ActorID,
			Kind:         store.MediaKindVoiceClip,
			Source:       store.MediaSourceApp,
			Title:        "opaque-source-name.wav",
			CreatedAt:    now,
			ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
		},
		DeclaredSizeBytes: int64(len(raw)),
		SessionExpiresAt:  now + int64(time.Hour/time.Millisecond),
		IdempotencyKey:    idempotencyKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := harness.store.AdvanceMediaUpload(
		creation.Session.ID, 0, int64(len(raw)), harness.nextMS(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.BeginMediaUploadFinalization(
		creation.Session.ID, advanced.Revision, harness.nextMS(),
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(harness.service.uploadDir, creation.Session.ID+".part")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return creation
}

func (harness *submitHarness) createGenericItem(
	t *testing.T,
	credentials store.OnboardingCredentials,
	source store.MediaSource,
) store.MediaItem {
	t.Helper()
	now := harness.nextMS()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         store.MediaKindVoiceClip,
		Source:       source,
		Title:        "transport-neutral-source",
		CreatedAt:    now,
		ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestSubmitUploadPublishesCanonicalMetadataCleansSourceAndReplays(t *testing.T) {
	harness := newSubmitHarness(t)
	raw := testWAVBytes(100)
	creation := harness.createFinalizingUpload(
		t, harness.credentials, "submit-success-replay-0001", raw,
	)
	sourcePath := filepath.Join(harness.service.uploadDir, creation.Session.ID+".part")

	ready, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(harness.runner.output))
	if ready.ID != creation.Media.ID || ready.Status != store.MediaStatusReady ||
		ready.MIME != "audio/wav" || ready.Codec != "pcm_s16le" ||
		ready.DurationMS != 1000 || ready.SizeBytes != int64(len(harness.runner.output)) ||
		ready.SHA256 != wantHash || ready.LoudnessJSON != harness.runner.loudness ||
		ready.StorageKey == "" || ready.PublishedAt == 0 {
		t.Fatalf("ready media=%+v", ready)
	}
	canonicalPath, ok := CanonicalPath(harness.service.canonicalDir, ready.StorageKey)
	if !ok {
		t.Fatalf("canonical storage key=%q", ready.StorageKey)
	}
	info, err := os.Stat(canonicalPath)
	if err != nil || info.Mode().Perm() != 0o600 || info.Size() != ready.SizeBytes {
		t.Fatalf("canonical info=%+v err=%v", info, err)
	}
	if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful source still exists: %v", err)
	}
	session, err := harness.store.GetMediaUploadSession(creation.Session.ID)
	if err != nil || session == nil || session.Status != store.UploadStatusCompleted ||
		session.CompletedAt == 0 || session.TempCleanedAt == 0 {
		t.Fatalf("completed session=%+v err=%v", session, err)
	}
	commandsBeforeRetry := len(harness.runner.commandSnapshot())
	replayed, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID)
	if err != nil || replayed != ready {
		t.Fatalf("replayed media=%+v err=%v", replayed, err)
	}
	if got := len(harness.runner.commandSnapshot()); got != commandsBeforeRetry || got != 3 {
		t.Fatalf("retry worker commands=%d before=%d", got, commandsBeforeRetry)
	}
}

func TestSubmitMediaFailuresRemainNonReadyAndKeepSourceForRetention(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		raw       func() []byte
		configure func(*submitHarness)
	}{
		{
			name: "unsupported", code: "media_signature_unsupported",
			raw: func() []byte { return []byte("not an audio container") },
		},
		{
			name: "duration_bomb", code: "media_duration_exceeded",
			raw:       func() []byte { return testWAVBytes(10) },
			configure: func(harness *submitHarness) { harness.runner.probeDuration = "180.001" },
		},
		{
			name: "probe_timeout", code: "ffprobe_timeout",
			raw: func() []byte { return testWAVBytes(10) },
			configure: func(harness *submitHarness) {
				harness.runner.probeBlock = true
				harness.service.processor.limits.ProbeTimeout = 5 * time.Millisecond
			},
		},
		{
			name: "worker_crash", code: "ffmpeg_failed",
			raw: func() []byte { return testWAVBytes(10) },
			configure: func(harness *submitHarness) {
				harness.runner.transcodeError = errors.New("private worker crash detail")
			},
		},
		{
			name: "oversized_output", code: "canonical_output_oversized",
			raw: func() []byte { return testWAVBytes(10) },
			configure: func(harness *submitHarness) {
				harness.runner.output = testWAVBytes(100)
				harness.service.processor.limits.MaxOutputBytes = 100
				harness.service.processor.limits.WorkerOutputBytes = 100
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newSubmitHarness(t)
			if test.configure != nil {
				test.configure(harness)
			}
			creation := harness.createFinalizingUpload(
				t, harness.credentials, "submit-failure-"+test.name+"-0001", test.raw(),
			)
			sourcePath := filepath.Join(harness.service.uploadDir, creation.Session.ID+".part")
			_, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID)
			assertProcessingCode(t, err, test.code)
			if err.Error() != test.code || contains(err.Error(), sourcePath) || contains(err.Error(), "private worker") {
				t.Fatalf("unsanitized processing error=%q", err)
			}
			item, lookupErr := harness.store.GetMediaItem(creation.Media.ID)
			if lookupErr != nil || item == nil || item.Status != store.MediaStatusFailed ||
				item.FailureCode != test.code || item.StorageKey != "" || item.PublishedAt != 0 {
				t.Fatalf("failed media=%+v err=%v", item, lookupErr)
			}
			session, lookupErr := harness.store.GetMediaUploadSession(creation.Session.ID)
			if lookupErr != nil || session == nil || session.Status != store.UploadStatusFailed ||
				session.CompletedAt != 0 || session.TempCleanedAt != 0 {
				t.Fatalf("failed session=%+v err=%v", session, lookupErr)
			}
			if _, statErr := os.Stat(sourcePath); statErr != nil {
				t.Fatalf("failed source was not retained: %v", statErr)
			}
			canonical, readErr := os.ReadDir(harness.service.canonicalDir)
			if readErr != nil || len(canonical) != 0 {
				t.Fatalf("failed canonical entries=%v err=%v", canonical, readErr)
			}
		})
	}
}

func TestSubmitMediaRejectsSparseOversizedSourceBeforeWorker(t *testing.T) {
	harness := newSubmitHarness(t)
	item := harness.createGenericItem(t, harness.credentials, store.MediaSourceTelegram)
	path := filepath.Join(t.TempDir(), "oversized-source.bin")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	oversized := MaxClipBytes + 1
	if err := file.Truncate(oversized); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = harness.service.SubmitMedia(context.Background(), Submission{
		MediaID: item.ID, SourcePath: path, ExpectedSize: oversized,
	})
	assertProcessingCode(t, err, "media_input_oversized")
	if len(harness.runner.commandSnapshot()) != 0 {
		t.Fatal("oversized source reached a media worker")
	}
	failed, lookupErr := harness.store.GetMediaItem(item.ID)
	if lookupErr != nil || failed == nil || failed.Status != store.MediaStatusFailed {
		t.Fatalf("oversized media=%+v err=%v", failed, lookupErr)
	}
}

func TestSubmitMediaGlobalWorkerCapacityIsBoundedAndRetryable(t *testing.T) {
	harness := newSubmitHarness(t)
	limits := DefaultLimits()
	limits.WorkerConcurrency = 1
	limits.WorkerQueueTimeout = 5 * time.Millisecond
	processor := newProcessorForTest(harness.runner, limits)
	service, err := newSubmitService(harness.store, harness.mediaDir, PresetDefault, processor)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	service.workerSlots <- struct{}{}
	defer service.releaseWorker()
	item := harness.createGenericItem(t, harness.credentials, store.MediaSourceTelegram)
	raw := testWAVBytes(10)
	sourcePath := filepath.Join(t.TempDir(), "capacity-source.wav")
	if err := os.WriteFile(sourcePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitMedia(context.Background(), Submission{
		MediaID: item.ID, SourcePath: sourcePath, ExpectedSize: int64(len(raw)),
	})
	if err == nil || err.Error() != "media worker capacity unavailable" {
		t.Fatalf("worker capacity error=%v", err)
	}
	if _, processing := FailureCode(err); processing {
		t.Fatalf("capacity pressure became a terminal processing failure: %v", err)
	}
	current, lookupErr := harness.store.GetMediaItem(item.ID)
	if lookupErr != nil || current == nil || current.Status != store.MediaStatusProcessing || current.Revision != item.Revision {
		t.Fatalf("capacity-pressure media=%+v err=%v", current, lookupErr)
	}
	if _, statErr := os.Stat(sourcePath); statErr != nil {
		t.Fatalf("capacity-pressure source stat=%v", statErr)
	}
	if len(harness.runner.commandSnapshot()) != 0 {
		t.Fatal("capacity-pressure submission invoked a worker")
	}
}

func TestSubmitUploadConcurrentRetryRunsOnePublication(t *testing.T) {
	harness := newSubmitHarness(t)
	creation := harness.createFinalizingUpload(
		t, harness.credentials, "submit-concurrent-retry-0001", testWAVBytes(100),
	)
	start := make(chan struct{})
	type response struct {
		item store.MediaItem
		err  error
	}
	responses := make(chan response, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			item, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID)
			responses <- response{item: item, err: err}
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-responses, <-responses
	if first.err != nil || second.err != nil || first.item.Status != store.MediaStatusReady || first.item != second.item {
		t.Fatalf("concurrent results first=%+v second=%+v", first, second)
	}
	if got := len(harness.runner.commandSnapshot()); got != 3 {
		t.Fatalf("concurrent worker commands=%d", got)
	}
	operations, err := harness.store.PendingMediaStorageOperations(store.StorageOperationPublish, 10)
	if err != nil || len(operations) != 0 {
		t.Fatalf("pending publications=%+v err=%v", operations, err)
	}
}

func TestSubmitUploadRecoversCrashAfterAtomicPublishBeforeCAS(t *testing.T) {
	harness := newSubmitHarness(t)
	creation := harness.createFinalizingUpload(
		t, harness.credentials, "submit-publish-crash-0001", testWAVBytes(100),
	)
	harness.service.testAfterPublish = func() error { return errors.New("simulated process termination") }
	if _, err := harness.service.SubmitUpload(context.Background(), creation.Session.ID); err == nil {
		t.Fatal("publish-boundary crash unexpectedly succeeded")
	}
	pending, err := harness.store.PendingMediaPublicationForMedia(creation.Media.ID)
	if err != nil || pending == nil {
		t.Fatalf("pending publication=%+v err=%v", pending, err)
	}
	path, ok := CanonicalPath(harness.service.canonicalDir, pending.StorageKey)
	if !ok {
		t.Fatalf("pending storage key=%q", pending.StorageKey)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	processing, err := harness.store.GetMediaItem(creation.Media.ID)
	if err != nil || processing == nil || processing.Status != store.MediaStatusProcessing ||
		processing.StorageKey != "" || processing.Revision != pending.MediaRevision {
		t.Fatalf("crash-boundary media=%+v err=%v", processing, err)
	}
	sourcePath := filepath.Join(harness.service.uploadDir, creation.Session.ID+".part")
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("crash-boundary source missing: %v", err)
	}

	restarted, err := newSubmitService(
		harness.store, harness.mediaDir, PresetDefault, harness.service.processor,
	)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	ready, err := restarted.SubmitUpload(context.Background(), creation.Session.ID)
	if err != nil || ready.Status != store.MediaStatusReady || ready.StorageKey != pending.StorageKey {
		t.Fatalf("recovered media=%+v err=%v", ready, err)
	}
	after, err := os.Stat(path)
	sameFile := err == nil && os.SameFile(before, after)
	if !sameFile {
		t.Fatalf("recovery preserved canonical inode=%v err=%v", sameFile, err)
	}
	if pendingAfter, err := harness.store.PendingMediaPublicationForMedia(creation.Media.ID); err != nil || pendingAfter != nil {
		t.Fatalf("pending after recovery=%+v err=%v", pendingAfter, err)
	}
	entries, err := os.ReadDir(restarted.canonicalDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("canonical entries after recovery=%v err=%v", entries, err)
	}
}

func TestSubmitMediaDedupeIsPhysicalWithinOrbitAndAbsentAcrossOrbits(t *testing.T) {
	harness := newSubmitHarness(t)
	firstCreation := harness.createFinalizingUpload(
		t, harness.credentials, "submit-dedupe-first-0001", testWAVBytes(100),
	)
	first, err := harness.service.SubmitUpload(context.Background(), firstCreation.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondCreation := harness.createFinalizingUpload(
		t, harness.credentials, "submit-dedupe-second-0001", testWAVBytes(100),
	)
	second, err := harness.service.SubmitUpload(context.Background(), secondCreation.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstPath, firstOK := CanonicalPath(harness.service.canonicalDir, first.StorageKey)
	secondPath, secondOK := CanonicalPath(harness.service.canonicalDir, second.StorageKey)
	firstInfo, firstErr := os.Stat(firstPath)
	secondInfo, secondErr := os.Stat(secondPath)
	if !firstOK || !secondOK || firstErr != nil || secondErr != nil ||
		first.StorageKey == second.StorageKey || !os.SameFile(firstInfo, secondInfo) {
		t.Fatalf("same-orbit dedupe first=%+v second=%+v errors=%v/%v", first, second, firstErr, secondErr)
	}

	foreign, err := harness.store.CreateSelfServiceOrbit("Foreign SubmitMedia tenant")
	if err != nil {
		t.Fatal(err)
	}
	thirdCreation := harness.createFinalizingUpload(
		t, foreign, "submit-dedupe-foreign-0001", testWAVBytes(100),
	)
	third, err := harness.service.SubmitUpload(context.Background(), thirdCreation.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	thirdPath, ok := CanonicalPath(harness.service.canonicalDir, third.StorageKey)
	thirdInfo, statErr := os.Stat(thirdPath)
	if !ok || statErr != nil || os.SameFile(firstInfo, thirdInfo) {
		t.Fatalf("cross-orbit bytes were deduped third=%+v err=%v", third, statErr)
	}
	if first.SHA256 != second.SHA256 || first.SHA256 != third.SHA256 {
		t.Fatalf("deterministic canonical hashes first=%q second=%q third=%q", first.SHA256, second.SHA256, third.SHA256)
	}
}

func TestSubmitMediaTransportNeutralTelegramSourceUsesSamePipeline(t *testing.T) {
	harness := newSubmitHarness(t)
	item := harness.createGenericItem(t, harness.credentials, store.MediaSourceTelegram)
	raw := testWAVBytes(100)
	sourcePath := filepath.Join(t.TempDir(), "telegram-private-download.bin")
	if err := os.WriteFile(sourcePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	ready, err := harness.service.SubmitMedia(context.Background(), Submission{
		MediaID: item.ID, SourcePath: sourcePath, ExpectedSize: int64(len(raw)),
	})
	if err != nil || ready.Status != store.MediaStatusReady || ready.Source != store.MediaSourceTelegram {
		t.Fatalf("Telegram SubmitMedia result=%+v err=%v", ready, err)
	}
	if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Telegram source still exists: %v", err)
	}
	if got := len(harness.runner.commandSnapshot()); got != 3 {
		t.Fatalf("Telegram worker commands=%d", got)
	}
}

func TestSubmitServiceRestartRemovesOnlyPrivateStagingArtifacts(t *testing.T) {
	harness := newSubmitHarness(t)
	processingArtifact := filepath.Join(harness.service.processingDir, "interrupted-worker")
	if err := os.MkdirAll(processingArtifact, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(processingArtifact, "input.bin"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonicalArtifact := filepath.Join(harness.service.canonicalDir, ".canonical-interrupted.wav")
	if err := os.WriteFile(canonicalArtifact, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	published := filepath.Join(harness.service.canonicalDir, fmt.Sprintf("%064x.wav", 1))
	if err := os.WriteFile(published, []byte("published"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSubmitService(
		harness.store, harness.mediaDir, PresetDefault, harness.service.processor,
	); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{processingArtifact, canonicalArtifact} {
		if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("restart artifact %q stat=%v", removed, err)
		}
	}
	if raw, err := os.ReadFile(published); err != nil || string(raw) != "published" {
		t.Fatalf("published canonical changed raw=%q err=%v", raw, err)
	}
}
