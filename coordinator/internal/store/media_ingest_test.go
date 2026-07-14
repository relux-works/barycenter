package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/session"
)

func newMediaIngestTestStore(t *testing.T) (*Store, OnboardingCredentials) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "media-ingest.db")
	store, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	credentials, err := store.CreateSelfServiceOrbit("Media ingest test")
	if err != nil {
		t.Fatal(err)
	}
	return store, credentials
}

func mediaItemParams(credentials OnboardingCredentials, now int64) CreateMediaItemParams {
	return CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         MediaKindAudioClip,
		Source:       MediaSourceApp,
		Title:        "voice-note.wav",
		CreatedAt:    now,
		ExpiresAt:    now + int64((30*24*time.Hour)/time.Millisecond),
	}
}

func mediaUploadParams(credentials OnboardingCredentials, now int64, idempotencyKey string) CreateMediaUploadParams {
	media := mediaItemParams(credentials, now)
	return CreateMediaUploadParams{
		Media:             media,
		DeclaredSizeBytes: 1024,
		SessionExpiresAt:  now + int64((15*time.Minute)/time.Millisecond),
		IdempotencyKey:    idempotencyKey,
	}
}

func newFeatureOffTelegramMediaStore(t *testing.T) (*Store, *Orbit) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "telegram-media.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	orbit, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": strings.Repeat("a", 64)},
		map[int64]string{7001: "a"},
	)
	if err != nil || orbit == nil {
		t.Fatalf("bootstrap legacy orbit=%+v err=%v", orbit, err)
	}
	if err := st.SetMemberName(orbit.ID, 7001, "Legacy sender"); err != nil {
		t.Fatal(err)
	}
	return st, orbit
}

func TestCreateTelegramMediaAtomicallyProjectsFeatureOffMemberAndLegacyWAV(t *testing.T) {
	st, orbit := newFeatureOffTelegramMediaStore(t)
	var actorsBefore int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM actors WHERE kind = 'telegram_user'`).Scan(&actorsBefore); err != nil {
		t.Fatal(err)
	}
	if actorsBefore != 0 {
		t.Fatalf("feature-off bootstrap unexpectedly projected %d Telegram actor(s)", actorsBefore)
	}

	now := time.Now().UnixMilli()
	created, err := st.CreateTelegramMedia(CreateTelegramMediaParams{
		OwnerOrbitID:   orbit.ID,
		TelegramUserID: 7001,
		TelegramFileID: "tg-file-opaque",
		Title:          "Legacy sender",
		CreatedAt:      now,
		ExpiresAt:      now + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Media.ID != created.Legacy.ID || created.Media.ActorID <= 0 ||
		created.Media.OwnerOrbitID != orbit.ID || created.Media.Kind != MediaKindVoiceClip ||
		created.Media.Source != MediaSourceTelegram || created.Media.Status != MediaStatusProcessing ||
		created.Legacy.TGFileID != "tg-file-opaque" || created.Legacy.Status != "processing" ||
		created.Legacy.OrbitID != orbit.ID || created.Legacy.CreatedAt != now {
		t.Fatalf("Telegram media creation=%+v", created)
	}
	var kind, externalRef, displayName, role string
	var leftAt sql.NullInt64
	err = st.db.QueryRow(`SELECT a.kind, a.external_ref, a.display_name, m.role, m.left_at
FROM actors a JOIN memberships m ON m.actor_id = a.id
WHERE a.id = ? AND m.orbit_id = ?`, created.Media.ActorID, orbit.ID).Scan(
		&kind, &externalRef, &displayName, &role, &leftAt,
	)
	if err != nil || kind != "telegram_user" || externalRef != "7001" ||
		displayName != "Legacy sender" || role == "" || leftAt.Valid {
		t.Fatalf("projected actor kind=%q ref=%q name=%q role=%q left=%v err=%v",
			kind, externalRef, displayName, role, leftAt, err)
	}
}

func TestCreateTelegramMediaRollsBackBothRegistriesAndActorProjection(t *testing.T) {
	st, orbit := newFeatureOffTelegramMediaStore(t)
	now := time.Now().UnixMilli()
	st.testCheckpoint = func(name string) error {
		if name == "telegram_media_create_before_commit" {
			return errors.New("forced Telegram acceptance rollback")
		}
		return nil
	}
	_, err := st.CreateTelegramMedia(CreateTelegramMediaParams{
		OwnerOrbitID: orbit.ID, TelegramUserID: 7001,
		TelegramFileID: "tg-rollback", Title: "Rollback sender",
		CreatedAt: now, ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err == nil {
		t.Fatal("Telegram media acceptance unexpectedly committed")
	}
	st.testCheckpoint = nil
	for _, table := range []string{"media_items", "media", "actors", "memberships"} {
		var count int
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback left %d row(s) in %s", count, table)
		}
	}
}

func TestCreateTelegramMediaRejectsMissingMemberAndDisabledOrbit(t *testing.T) {
	st, orbit := newFeatureOffTelegramMediaStore(t)
	now := time.Now().UnixMilli()
	params := CreateTelegramMediaParams{
		OwnerOrbitID: orbit.ID, TelegramUserID: 7999,
		TelegramFileID: "tg-denied", CreatedAt: now,
		ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
	}
	if _, err := st.CreateTelegramMedia(params); !errors.Is(err, ErrMediaOwnerInvalid) {
		t.Fatalf("missing member error=%v", err)
	}
	if _, err := st.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	params.TelegramUserID = 7001
	if _, err := st.CreateTelegramMedia(params); !errors.Is(err, ErrMediaOwnerInvalid) {
		t.Fatalf("disabled orbit error=%v", err)
	}
}

func canonicalPublication() MediaPublication {
	return MediaPublication{
		MIME:         "audio/wav",
		Codec:        "pcm_s16le",
		DurationMS:   1250,
		SizeBytes:    64000,
		SHA256:       strings.Repeat("a", 64),
		LoudnessJSON: `{"input_i":-20.1,"input_tp":-1.2}`,
	}
}

func TestMediaIngestFreshLifecycleIdempotencyAndLegacyWAV(t *testing.T) {
	store, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	params := mediaUploadParams(credentials, now, "fresh-lifecycle-idempotency-001")

	created, err := store.CreateMediaUpload(params)
	if err != nil {
		t.Fatal(err)
	}
	if created.Reused || created.Token == "" || len(created.Token) != 64 {
		t.Fatalf("new upload reused=%v token length=%d", created.Reused, len(created.Token))
	}
	if !strings.HasPrefix(created.Media.ID, "m_") || !strings.HasPrefix(created.Session.ID, "up_") {
		t.Fatalf("generated ids media=%q session=%q", created.Media.ID, created.Session.ID)
	}
	if created.Media.Status != MediaStatusProcessing || created.Media.Revision != 1 ||
		created.Session.Status != UploadStatusOpen || created.Session.Revision != 1 {
		t.Fatalf("initial media=%+v session=%+v", created.Media, created.Session)
	}
	byToken, err := store.GetMediaUploadSessionByToken(created.Token)
	if err != nil || byToken == nil || byToken.ID != created.Session.ID {
		t.Fatalf("token lookup session=%+v err=%v", byToken, err)
	}
	if malformed, err := store.GetMediaUploadSessionByToken("not-a-scoped-token"); err != nil || malformed != nil {
		t.Fatalf("malformed token lookup session=%+v err=%v", malformed, err)
	}

	retryParams := params
	// Server-derived timestamps naturally change when an HTTP request is
	// retried; they are not part of the caller's idempotent intent.
	retryParams.Media.CreatedAt++
	retryParams.Media.ExpiresAt++
	retryParams.SessionExpiresAt++
	replayed, err := store.CreateMediaUpload(retryParams)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Reused || replayed.Token != "" || replayed.Media.ID != created.Media.ID ||
		replayed.Session.ID != created.Session.ID {
		t.Fatalf("idempotent replay=%+v", replayed)
	}
	mismatch := params
	mismatch.Media.Title = "different.wav"
	if _, err := store.CreateMediaUpload(mismatch); !errors.Is(err, ErrMediaIdempotencyMismatch) {
		t.Fatalf("idempotency mismatch error=%v", err)
	}

	advanced, err := store.AdvanceMediaUpload(created.Session.ID, 0, 512, now+1)
	if err != nil {
		t.Fatal(err)
	}
	if advanced.ReceivedSizeBytes != 512 || advanced.Revision != 2 {
		t.Fatalf("first advance=%+v", advanced)
	}
	if _, err := store.AdvanceMediaUpload(created.Session.ID, 0, 512, now+2); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("stale upload offset error=%v", err)
	}
	advanced, err = store.AdvanceMediaUpload(created.Session.ID, 512, 512, now+3)
	if err != nil {
		t.Fatal(err)
	}
	finalizing, err := store.BeginMediaUploadFinalization(created.Session.ID, advanced.Revision, now+4)
	if err != nil {
		t.Fatal(err)
	}
	if finalizing.Status != UploadStatusFinalizing || finalizing.Revision != 4 {
		t.Fatalf("finalizing session=%+v", finalizing)
	}
	if _, err := store.BeginMediaUploadFinalization(created.Session.ID, finalizing.Revision, now+5); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("duplicate finalize error=%v", err)
	}

	operation, err := store.StageMediaPublication(created.Media.ID, created.Media.Revision, now+6)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Kind != StorageOperationPublish || operation.State != StorageOperationPending ||
		!strings.HasPrefix(operation.StorageKey, "media/v1/") || len(operation.StorageKey) != 73 {
		t.Fatalf("publication operation=%+v", operation)
	}
	processing, err := store.GetMediaItem(created.Media.ID)
	if err != nil || processing == nil {
		t.Fatalf("processing media=%+v err=%v", processing, err)
	}
	if processing.StorageKey != "" || processing.Revision != 2 {
		t.Fatalf("staged media leaked key or revision=%+v", processing)
	}
	if strings.Contains(operation.StorageKey, "voice-note") || strings.Contains(operation.StorageKey, params.IdempotencyKey) {
		t.Fatalf("storage key derived from caller input: %q", operation.StorageKey)
	}
	if _, err := store.StageMediaPublication(created.Media.ID, created.Media.Revision, now+7); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("stale publication stage error=%v", err)
	}

	ready, err := store.CompleteMediaPublication(operation.ID, operation.Revision, canonicalPublication(), now+8)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != MediaStatusReady || ready.StorageKey != operation.StorageKey || ready.Revision != 3 {
		t.Fatalf("ready media=%+v", ready)
	}
	if _, err := store.CompleteMediaPublication(operation.ID, operation.Revision, canonicalPublication(), now+9); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("duplicate publication completion error=%v", err)
	}
	completedUpload, err := store.GetMediaUploadSession(created.Session.ID)
	if err != nil || completedUpload == nil || completedUpload.Status != UploadStatusCompleted || completedUpload.CompletedAt != now+8 {
		t.Fatalf("completed upload=%+v err=%v", completedUpload, err)
	}

	legacy := MediaRecord{
		ID: "m_legacy_voice", TGFileID: "telegram-file-1", DurationMS: 1250,
		PathWAV: "/srv/duet/media/legacy.wav", LoudnormJSON: `{"input_i":-20.1}`,
		CreatedAt: now, ExpiresAt: params.Media.ExpiresAt, Status: "ready", OrbitID: 0,
	}
	if err := store.InsertMedia(legacy); err != nil {
		t.Fatal(err)
	}
	if err := store.LinkLegacyWAV(ready.ID, ready.Revision, legacy.ID, now+10); err != nil {
		t.Fatal(err)
	}
	compat, err := store.LegacyWAVForMediaItem(ready.ID)
	if err != nil || compat == nil || compat.PathWAV != legacy.PathWAV || compat.OrbitID != credentials.OrbitID {
		t.Fatalf("legacy WAV=%+v err=%v", compat, err)
	}
	legacyRead, err := store.GetMedia(legacy.ID)
	if err != nil || legacyRead == nil || legacyRead.TGFileID != legacy.TGFileID || legacyRead.PathWAV != legacy.PathWAV {
		t.Fatalf("legacy read=%+v err=%v", legacyRead, err)
	}
	genericRead, err := store.MediaItemForLegacyWAV(legacy.ID)
	if err != nil || genericRead == nil || genericRead.ID != ready.ID {
		t.Fatalf("generic reverse lookup=%+v err=%v", genericRead, err)
	}
}

func TestMediaIngestRejectsNonCanonicalPublicationMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MediaPublication)
	}{
		{"header-shaped MIME", func(value *MediaPublication) { value.MIME = "audio/wav\r\nx-test: injected" }},
		{"uppercase MIME", func(value *MediaPublication) { value.MIME = "Audio/WAV" }},
		{"unsafe codec", func(value *MediaPublication) { value.Codec = "pcm s16le" }},
		{"array loudness", func(value *MediaPublication) { value.LoudnessJSON = "[]" }},
		{"null loudness", func(value *MediaPublication) { value.LoudnessJSON = "null" }},
		{"oversized loudness", func(value *MediaPublication) {
			value.LoudnessJSON = `{"padding":"` + strings.Repeat("x", 16384) + `"}`
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publication := canonicalPublication()
			test.mutate(&publication)
			if err := validateMediaPublication(publication); !errors.Is(err, ErrMediaInvalid) {
				t.Fatalf("validation error=%v", err)
			}
		})
	}
}

func TestMediaIngestRepositoryRequiresLiveOwnerActorBinding(t *testing.T) {
	store, first := newMediaIngestTestStore(t)
	second, err := store.CreateSelfServiceOrbit("Different media owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	crossTenant := mediaItemParams(first, now)
	crossTenant.OwnerOrbitID = second.OrbitID
	if _, err := store.CreateMediaItem(crossTenant); !errors.Is(err, ErrMediaOwnerInvalid) {
		t.Fatalf("cross-tenant owner error=%v", err)
	}
	if _, err := store.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, first.OrbitID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateMediaItem(mediaItemParams(first, now+1)); !errors.Is(err, ErrMediaOwnerInvalid) {
		t.Fatalf("disabled owner error=%v", err)
	}
}

func TestMediaIngestStalePublicationCannotReviveDeletedMedia(t *testing.T) {
	store, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	item, err := store.CreateMediaItem(mediaItemParams(credentials, now))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteMediaItem(item.ID, operation.MediaRevision, now+2)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Status != MediaStatusDeleted || deleted.StorageKey != "" || deleted.DeletedAt != now+2 {
		t.Fatalf("deleted media=%+v", deleted)
	}
	if _, err := store.CompleteMediaPublication(operation.ID, operation.Revision, canonicalPublication(), now+3); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("stale publication completion error=%v", err)
	}
	pendingPublish, err := store.PendingMediaStorageOperations(StorageOperationPublish, 10)
	if err != nil || len(pendingPublish) != 0 {
		t.Fatalf("pending publish=%+v err=%v", pendingPublish, err)
	}
	pendingCleanup, err := store.PendingMediaStorageOperations(StorageOperationCleanup, 10)
	if err != nil || len(pendingCleanup) != 1 || pendingCleanup[0].StorageKey != operation.StorageKey {
		t.Fatalf("pending cleanup=%+v err=%v", pendingCleanup, err)
	}

	injected := errors.New("cleanup receipt interrupted")
	store.testCheckpoint = func(name string) error {
		if name == "media_cleanup_complete_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := store.CompleteMediaStorageCleanup(pendingCleanup[0].ID, pendingCleanup[0].Revision, now+4); !errors.Is(err, injected) {
		t.Fatalf("injected cleanup error=%v", err)
	}
	pendingCleanup, err = store.PendingMediaStorageOperations(StorageOperationCleanup, 10)
	if err != nil || len(pendingCleanup) != 1 || pendingCleanup[0].State != StorageOperationPending {
		t.Fatalf("cleanup after rollback=%+v err=%v", pendingCleanup, err)
	}
	store.testCheckpoint = nil
	completed, err := store.CompleteMediaStorageCleanup(pendingCleanup[0].ID, pendingCleanup[0].Revision, now+5)
	if err != nil || completed.State != StorageOperationDone {
		t.Fatalf("completed cleanup=%+v err=%v", completed, err)
	}
	if _, err := store.CompleteMediaStorageCleanup(completed.ID, completed.Revision, now+6); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("duplicate cleanup completion error=%v", err)
	}
}

func TestMediaIngestInterruptedPublicationIsRecoverable(t *testing.T) {
	store, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	item, err := store.CreateMediaItem(mediaItemParams(credentials, now))
	if err != nil {
		t.Fatal(err)
	}

	injectedStage := errors.New("stage interrupted")
	store.testCheckpoint = func(name string) error {
		if name == "media_publication_stage_before_commit" {
			return injectedStage
		}
		return nil
	}
	if _, err := store.StageMediaPublication(item.ID, item.Revision, now+1); !errors.Is(err, injectedStage) {
		t.Fatalf("injected stage error=%v", err)
	}
	afterStageRollback, err := store.GetMediaItem(item.ID)
	if err != nil || afterStageRollback == nil || afterStageRollback.Revision != item.Revision {
		t.Fatalf("media after stage rollback=%+v err=%v", afterStageRollback, err)
	}
	operations, err := store.PendingMediaStorageOperations(StorageOperationPublish, 10)
	if err != nil || len(operations) != 0 {
		t.Fatalf("operations after stage rollback=%+v err=%v", operations, err)
	}

	store.testCheckpoint = nil
	operation, err := store.StageMediaPublication(item.ID, item.Revision, now+2)
	if err != nil {
		t.Fatal(err)
	}
	injectedComplete := errors.New("database receipt interrupted after file rename")
	store.testCheckpoint = func(name string) error {
		if name == "media_publication_complete_before_commit" {
			return injectedComplete
		}
		return nil
	}
	if _, err := store.CompleteMediaPublication(operation.ID, operation.Revision, canonicalPublication(), now+3); !errors.Is(err, injectedComplete) {
		t.Fatalf("injected publication error=%v", err)
	}
	afterCompleteRollback, err := store.GetMediaItem(item.ID)
	if err != nil || afterCompleteRollback == nil || afterCompleteRollback.Status != MediaStatusProcessing ||
		afterCompleteRollback.StorageKey != "" || afterCompleteRollback.Revision != operation.MediaRevision {
		t.Fatalf("media after publication rollback=%+v err=%v", afterCompleteRollback, err)
	}
	operations, err = store.PendingMediaStorageOperations(StorageOperationPublish, 10)
	if err != nil || len(operations) != 1 || operations[0].ID != operation.ID {
		t.Fatalf("recoverable publish operation=%+v err=%v", operations, err)
	}
	store.testCheckpoint = nil
	ready, err := store.CompleteMediaPublication(operation.ID, operation.Revision, canonicalPublication(), now+4)
	if err != nil || ready.Status != MediaStatusReady {
		t.Fatalf("recovered publication=%+v err=%v", ready, err)
	}
}

func TestMediaIngestTerminalTransitionsAreConditional(t *testing.T) {
	store, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	failedCandidate, err := store.CreateMediaItem(mediaItemParams(credentials, now))
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.MarkMediaItemFailed(failedCandidate.ID, failedCandidate.Revision, "ffprobe_timeout", now+1)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != MediaStatusFailed || failed.FailureCode != "ffprobe_timeout" || failed.Revision != 2 {
		t.Fatalf("failed media=%+v", failed)
	}
	if _, err := store.MarkMediaItemFailed(failed.ID, failedCandidate.Revision, "retry", now+2); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("stale failed transition error=%v", err)
	}
	deleted, err := store.DeleteMediaItem(failed.ID, failed.Revision, now+3)
	if err != nil || deleted.Status != MediaStatusDeleted || deleted.FailureCode != "" {
		t.Fatalf("failed-to-deleted media=%+v err=%v", deleted, err)
	}

	expiryParams := mediaItemParams(credentials, now+10)
	expiryParams.ExpiresAt = now + 100
	expiryCandidate, err := store.CreateMediaItem(expiryParams)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ExpireMediaItem(expiryCandidate.ID, expiryCandidate.Revision, now+99); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("early expiry error=%v", err)
	}
	expired, err := store.ExpireMediaItem(expiryCandidate.ID, expiryCandidate.Revision, now+100)
	if err != nil || expired.Status != MediaStatusExpired || expired.DeletedAt != now+100 {
		t.Fatalf("expired media=%+v err=%v", expired, err)
	}
}

func TestMediaIngestOrbitDissolutionRevokesCanonicalStorageAtomically(t *testing.T) {
	store, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	item, err := store.CreateMediaItem(mediaItemParams(credentials, now))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := store.CompleteMediaPublication(operation.ID, operation.Revision, canonicalPublication(), now+2)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteOrbit(credentials.OrbitID); err != nil {
		t.Fatal(err)
	}
	var orbitCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM orbits WHERE id = ?`, credentials.OrbitID).Scan(&orbitCount); err != nil || orbitCount != 0 {
		t.Fatalf("orbit count=%d err=%v", orbitCount, err)
	}
	revoked, err := store.GetMediaItem(item.ID)
	if err != nil || revoked == nil || revoked.Status != MediaStatusDeleted || revoked.StorageKey != "" {
		t.Fatalf("revoked media=%+v err=%v", revoked, err)
	}
	cleanups, err := store.PendingMediaStorageOperations(StorageOperationCleanup, 10)
	if err != nil || len(cleanups) != 1 || cleanups[0].StorageKey != ready.StorageKey {
		t.Fatalf("orbit cleanup=%+v err=%v", cleanups, err)
	}
}

func TestMediaIngestConcurrentUploadOffsetHasOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media-upload-race.db")
	first, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	credentials, err := first.CreateSelfServiceOrbit("Upload race")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	params := mediaUploadParams(credentials, now, "concurrent-upload-offset-001")
	created, err := first.CreateMediaUpload(params)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	start := make(chan struct{})
	errorsByWorker := make([]error, 2)
	stores := []*Store{first, second}
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errorsByWorker[index] = stores[index].AdvanceMediaUpload(created.Session.ID, 0, 100, now+1)
		}(index)
	}
	close(start)
	wait.Wait()
	winners, conflicts := 0, 0
	for _, err := range errorsByWorker {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrMediaStateConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error=%v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("concurrent results winners=%d conflicts=%d errors=%v", winners, conflicts, errorsByWorker)
	}
	session, err := first.GetMediaUploadSession(created.Session.ID)
	if err != nil || session == nil || session.ReceivedSizeBytes != 100 || session.Revision != 2 {
		t.Fatalf("session after race=%+v err=%v", session, err)
	}
}

func TestMediaIngestConcurrentIdempotencyCreatesOneUpload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media-idempotency-race.db")
	first, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	credentials, err := first.CreateSelfServiceOrbit("Idempotency race")
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	now := time.Now().UnixMilli()
	params := mediaUploadParams(credentials, now, "concurrent-idempotency-key-001")

	start := make(chan struct{})
	results := make([]MediaUploadCreation, 2)
	errorsByWorker := make([]error, 2)
	stores := []*Store{first, second}
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errorsByWorker[index] = stores[index].CreateMediaUpload(params)
		}(index)
	}
	close(start)
	wait.Wait()
	for _, err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent idempotency error=%v", err)
		}
	}
	if results[0].Media.ID != results[1].Media.ID || results[0].Session.ID != results[1].Session.ID {
		t.Fatalf("idempotency results diverged: %+v / %+v", results[0], results[1])
	}
	if results[0].Reused == results[1].Reused || (results[0].Token == "") == (results[1].Token == "") {
		t.Fatalf("want one creation and one replay: %+v / %+v", results[0], results[1])
	}
	var mediaRows, sessionRows int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM media_items`).Scan(&mediaRows); err != nil {
		t.Fatal(err)
	}
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM media_upload_sessions`).Scan(&sessionRows); err != nil {
		t.Fatal(err)
	}
	if mediaRows != 1 || sessionRows != 1 {
		t.Fatalf("idempotency rows media=%d sessions=%d", mediaRows, sessionRows)
	}
}

func TestMediaIngestSchemaInstallIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media-schema-atomic.db")
	store := openMigrationStore(t, path)
	if _, err := store.db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("ddl interrupted")
	store.testCheckpoint = func(name string) error {
		if name == "media_ingest_ddl_before_commit" {
			return injected
		}
		return nil
	}
	if err := store.initMediaIngestSchema(); !errors.Is(err, injected) {
		t.Fatalf("injected schema error=%v", err)
	}
	for _, table := range []string{
		"media_items", "media_upload_sessions", "media_storage_operations",
		"media_delivery_cancellations", "media_legacy_wav_links",
		"media_ingest_audit_events",
	} {
		exists, err := tableExists(store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("partially committed table %s", table)
		}
	}
	store.testCheckpoint = nil
	if err := store.initMediaIngestSchema(); err != nil {
		t.Fatal(err)
	}
	if err := foreignKeyCheck(store.db); err != nil {
		t.Fatal(err)
	}
}

func TestMediaIngestMigratesLegacyDatabaseWithoutAuthorityLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-media-migration.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	nodeToken := strings.Repeat("1", 64)
	now := time.Now().UnixMilli()
	snapshot := SessionSnapshot{Mode: session.ModeSolo, State: session.StatePlaying, SavedPositionMS: 321}
	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO orbits(id, title, created_at) VALUES(7, 'Legacy orbit', ?)`, []any{now}},
		{`INSERT INTO members(orbit_id, tg_user_id, role, joined_at, display_name) VALUES(7, 7001, 'primary', ?, 'Legacy owner')`, []any{now}},
		{`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, provider, paired_at) VALUES(7, 'a', ?, 7001, 'spotify', ?)`, []any{hashToken(nodeToken), now}},
		{`INSERT INTO media(id, tg_file_id, duration_ms, path_wav, loudnorm_json, created_at, expires_at, status, orbit_id)
VALUES('m_legacy_before_ingest', 'tg-before', 900, '/srv/legacy-before.wav', '{}', ?, ?, 'ready', 7)`, []any{now, now + 100000}},
		{`INSERT INTO settings(key, value) VALUES('session_state_7', ?)`, []any{string(rawSnapshot)}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			db.Close()
			t.Fatalf("legacy fixture statement failed: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	legacy, err := store.GetMedia("m_legacy_before_ingest")
	if err != nil || legacy == nil || legacy.PathWAV != "/srv/legacy-before.wav" || legacy.TGFileID != "tg-before" {
		t.Fatalf("legacy media after migration=%+v err=%v", legacy, err)
	}
	if orbitID, slot, ok, err := store.LookupPlaybackToken(nodeToken); err != nil || !ok || orbitID != 7 || slot != "a" {
		t.Fatalf("legacy token after migration orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	loaded, err := store.LoadSession(7)
	if err != nil || loaded == nil || loaded.State != session.StatePaused || loaded.SavedPositionMS != 321 {
		t.Fatalf("legacy session after migration=%+v err=%v", loaded, err)
	}
	actor, err := store.ResolveTokenActorContext(nodeToken)
	if err != nil {
		t.Fatal(err)
	}
	params := mediaUploadParams(OnboardingCredentials{OrbitID: 7, ActorID: actor.ActorID}, now+1, "migrated-database-upload-001")
	created, err := store.CreateMediaUpload(params)
	if err != nil || created.Media.OwnerOrbitID != 7 || created.Media.ActorID != actor.ActorID {
		t.Fatalf("generic upload on migrated DB=%+v err=%v", created, err)
	}
	if err := foreignKeyCheck(store.db); err != nil {
		t.Fatal(err)
	}
}

func TestMediaIngestSecretsAndCallerPathAbsentFromPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "media-secret-artifacts.db")
	store, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := store.CreateSelfServiceOrbit("Media artifacts")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	idempotencyKey := "caller-idempotency-secret-should-not-persist-001"
	params := mediaUploadParams(credentials, now, idempotencyKey)
	params.Media.Title = `../../caller-controlled/audio.wav`
	created, err := store.CreateMediaUpload(params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE media_items SET storage_key = '../../escape.wav' WHERE id = ?`, created.Media.ID); !isCheckConstraintError(err) {
		t.Fatalf("caller-shaped storage key error=%v", err)
	}
	if _, err := store.db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		t.Fatal(err)
	}
	assertCredentialPlaintextAbsentFromSQLiteArtifacts(t, path, map[string]string{
		"upload session token": created.Token,
		"idempotency key":      idempotencyKey,
	})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	assertCredentialPlaintextAbsentFromSQLiteArtifacts(t, path, map[string]string{
		"upload session token": created.Token,
		"idempotency key":      idempotencyKey,
	})
}

func TestMediaItemForLegacyWAVUsesUnambiguousProjection(t *testing.T) {
	store, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	item, err := store.CreateMediaItem(mediaItemParams(credentials, now))
	if err != nil {
		t.Fatal(err)
	}
	legacyID := fmt.Sprintf("legacy-%d", now)
	if err := store.InsertMedia(MediaRecord{
		ID: legacyID, CreatedAt: now, ExpiresAt: now + 1000, Status: "processing", OrbitID: credentials.OrbitID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.LinkLegacyWAV(item.ID, item.Revision, legacyID, now+1); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.MediaItemForLegacyWAV(legacyID)
	if err != nil || resolved == nil || resolved.ID != item.ID {
		t.Fatalf("resolved media=%+v err=%v", resolved, err)
	}
}
