package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func authorizedUploadParams(credentials OnboardingCredentials, now int64, key string, size int64) CreateMediaUploadParams {
	params := mediaUploadParams(credentials, now, key)
	params.DeclaredSizeBytes = size
	return params
}

func permissiveMediaUploadQuota() MediaUploadQuota {
	return MediaUploadQuota{
		MaxStarts: 100, StartWindow: time.Minute,
		MaxConcurrent: 100, MaxDailyBytes: 1 << 40,
		DailyWindow: 24 * time.Hour, MaxItemBytes: 1 << 30,
	}
}

func TestAuthorizedMediaUploadReplayReturnsStableScopedCapability(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	params := authorizedUploadParams(credentials, now, "authorized-replay-0001", 128)
	quota := permissiveMediaUploadQuota()
	created, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, params, quota,
	)
	if err != nil {
		t.Fatal(err)
	}
	retry := params
	retry.Media.CreatedAt++
	retry.Media.ExpiresAt++
	retry.SessionExpiresAt++
	replayed, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, retry, quota,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Reused || replayed.Media.ID != created.Media.ID ||
		replayed.Session.ID != created.Session.ID || replayed.Token != created.Token ||
		len(created.Token) != 64 {
		t.Fatalf("creation=%+v replay=%+v", created, replayed)
	}
	var storedToken, storedIdempotency string
	if err := st.db.QueryRow(`SELECT token_hash, idempotency_key_hash
FROM media_upload_sessions WHERE id = ?`, created.Session.ID).Scan(
		&storedToken, &storedIdempotency); err != nil {
		t.Fatal(err)
	}
	if storedToken == created.Token || storedIdempotency == params.IdempotencyKey {
		t.Fatal("plaintext scoped token or idempotency key reached SQLite")
	}
	wrongID, err := st.AuthorizeMediaUploadSession("up_00000000000000000000000000", created.Token, now+2)
	if err != nil || wrongID != nil {
		t.Fatalf("wrong-id authorization=%+v err=%v", wrongID, err)
	}
	wrongToken, err := st.AuthorizeMediaUploadSession(created.Session.ID, credentials.ControlToken, now+2)
	if err != nil || wrongToken != nil {
		t.Fatalf("wrong-token authorization=%+v err=%v", wrongToken, err)
	}
	authorized, err := st.AuthorizeMediaUploadSession(created.Session.ID, created.Token, now+2)
	if err != nil || authorized == nil || authorized.ID != created.Session.ID {
		t.Fatalf("scoped authorization=%+v err=%v", authorized, err)
	}
}

func TestAuthorizedMediaUploadControlRotationRemintsCapability(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	params := authorizedUploadParams(credentials, now, "control-rotation-replay-0001", 128)
	created, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, params, permissiveMediaUploadQuota(),
	)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := randomHexSecret(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConsumeRecovery(
		credentials.RecoveryID, credentials.RecoverySecret, replacement,
	); err != nil {
		t.Fatal(err)
	}
	retry := params
	retry.Media.CreatedAt += 10
	retry.Media.ExpiresAt += 10
	retry.SessionExpiresAt += 10
	rotated, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, replacement, retry, permissiveMediaUploadQuota(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated.Reused || rotated.Token == created.Token || rotated.Session.ID != created.Session.ID {
		t.Fatalf("created=%+v rotated=%+v", created, rotated)
	}
	if old, err := st.AuthorizeMediaUploadSession(created.Session.ID, created.Token, now+11); err != nil || old != nil {
		t.Fatalf("old scoped token remained authorized: %+v err=%v", old, err)
	}
	if current, err := st.AuthorizeMediaUploadSession(created.Session.ID, rotated.Token, now+11); err != nil || current == nil {
		t.Fatalf("rotated scoped token=%+v err=%v", current, err)
	}
	if _, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, retry, permissiveMediaUploadQuota(),
	); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stale control error=%v", err)
	}
}

func TestAuthorizedMediaUploadQuotaBoundariesAndReplay(t *testing.T) {
	t.Run("hard_item_bytes", func(t *testing.T) {
		st, credentials := newMediaIngestTestStore(t)
		quota := permissiveMediaUploadQuota()
		quota.MaxItemBytes = 10
		params := authorizedUploadParams(credentials, time.Now().UnixMilli(), "hard-bytes-limit-0001", 11)
		if _, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, params, quota); !errors.Is(err, ErrMediaUploadTooLarge) {
			t.Fatalf("oversize error=%v", err)
		}
	})

	t.Run("start_rate_and_idempotent_replay", func(t *testing.T) {
		st, credentials := newMediaIngestTestStore(t)
		quota := permissiveMediaUploadQuota()
		quota.MaxStarts = 1
		now := time.Now().UnixMilli()
		first := authorizedUploadParams(credentials, now, "start-rate-first-0001", 4)
		created, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, first, quota)
		if err != nil {
			t.Fatal(err)
		}
		retry := first
		retry.Media.CreatedAt++
		retry.Media.ExpiresAt++
		retry.SessionExpiresAt++
		if replay, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, retry, quota); err != nil || !replay.Reused || replay.Session.ID != created.Session.ID {
			t.Fatalf("idempotent replay=%+v err=%v", replay, err)
		}
		second := authorizedUploadParams(credentials, now+2, "start-rate-second-0001", 4)
		_, err = st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, second, quota)
		var rate *MediaUploadRateLimitError
		if !errors.As(err, &rate) || rate.RetryAfter <= 0 {
			t.Fatalf("rate error=%v", err)
		}
	})

	t.Run("concurrent_processing", func(t *testing.T) {
		st, credentials := newMediaIngestTestStore(t)
		quota := permissiveMediaUploadQuota()
		quota.MaxConcurrent = 1
		now := time.Now().UnixMilli()
		first := authorizedUploadParams(credentials, now, "concurrent-first-0001", 4)
		if _, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, first, quota); err != nil {
			t.Fatal(err)
		}
		second := authorizedUploadParams(credentials, now+1, "concurrent-second-0001", 4)
		if _, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, second, quota); !errors.Is(err, ErrMediaUploadConcurrent) {
			t.Fatalf("concurrent quota error=%v", err)
		}
	})

	t.Run("daily_reserved_bytes", func(t *testing.T) {
		st, credentials := newMediaIngestTestStore(t)
		quota := permissiveMediaUploadQuota()
		quota.MaxDailyBytes = 6
		now := time.Now().UnixMilli()
		first := authorizedUploadParams(credentials, now, "daily-bytes-first-0001", 4)
		if _, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, first, quota); err != nil {
			t.Fatal(err)
		}
		second := authorizedUploadParams(credentials, now+1, "daily-bytes-second-0001", 3)
		if _, err := st.CreateAuthorizedMediaUpload(credentials.ActorID, credentials.ControlToken, second, quota); !errors.Is(err, ErrMediaUploadDailyBytes) {
			t.Fatalf("daily quota error=%v", err)
		}
	})
}

func TestAuthorizedMediaUploadConcurrentIdempotencyHasOneResultAndToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authorized-idempotency.db")
	first, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	credentials, err := first.CreateSelfServiceOrbit("Authorized upload race")
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	params := authorizedUploadParams(
		credentials, time.Now().UnixMilli(), "authorized-concurrency-0001", 16,
	)
	stores := []*Store{first, second}
	results := make([]MediaUploadCreation, len(stores))
	errs := make([]error, len(stores))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errs[index] = stores[index].CreateAuthorizedMediaUpload(
				credentials.ActorID, credentials.ControlToken, params, permissiveMediaUploadQuota(),
			)
		}(index)
	}
	close(start)
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].Session.ID != results[1].Session.ID ||
		results[0].Media.ID != results[1].Media.ID ||
		results[0].Token != results[1].Token || results[0].Reused == results[1].Reused {
		t.Fatalf("concurrent results=%+v / %+v", results[0], results[1])
	}
	var sessions, media int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM media_upload_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM media_items`).Scan(&media); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || media != 1 {
		t.Fatalf("rows sessions=%d media=%d", sessions, media)
	}
}

func TestAuthorizedMediaUploadConcurrentQuotaHasOneReservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authorized-quota-race.db")
	first, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	credentials, err := first.CreateSelfServiceOrbit("Authorized quota race")
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	quota := permissiveMediaUploadQuota()
	quota.MaxDailyBytes = 5
	now := time.Now().UnixMilli()
	params := []CreateMediaUploadParams{
		authorizedUploadParams(credentials, now, "quota-race-first-0001", 4),
		authorizedUploadParams(credentials, now+1, "quota-race-second-0001", 4),
	}
	stores := []*Store{first, second}
	errs := make([]error, len(stores))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errs[index] = stores[index].CreateAuthorizedMediaUpload(
				credentials.ActorID, credentials.ControlToken, params[index], quota,
			)
		}(index)
	}
	close(start)
	wait.Wait()
	winners, rejected := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrMediaUploadDailyBytes):
			rejected++
		default:
			t.Fatalf("unexpected quota race error=%v", err)
		}
	}
	if winners != 1 || rejected != 1 {
		t.Fatalf("quota race winners=%d rejected=%d errors=%v", winners, rejected, errs)
	}
	var sessions int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM media_upload_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("quota race sessions=%d", sessions)
	}
}

func TestMediaUploadExpiryFailsMediaAndAcknowledgesTempCleanup(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	params := authorizedUploadParams(credentials, now, "expiry-cleanup-0001", 8)
	created, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, params, permissiveMediaUploadQuota(),
	)
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := st.AdvanceMediaUpload(created.Session.ID, 0, 3, now+1)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := st.ExpireMediaUploadSession(
		advanced.ID, advanced.Revision, params.SessionExpiresAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if expired.Status != UploadStatusExpired || expired.TempCleanedAt != 0 {
		t.Fatalf("expired session=%+v", expired)
	}
	item, err := st.GetMediaItem(created.Media.ID)
	if err != nil || item == nil || item.Status != MediaStatusFailed || item.FailureCode != "upload_expired" {
		t.Fatalf("expired media=%+v err=%v", item, err)
	}
	if authorized, err := st.AuthorizeMediaUploadSession(expired.ID, created.Token, params.SessionExpiresAt); err != nil || authorized != nil {
		t.Fatalf("expired authorization=%+v err=%v", authorized, err)
	}
	cleanups, err := st.MediaUploadSessionsForTempCleanup(10)
	if err != nil || len(cleanups) != 1 || cleanups[0].ID != expired.ID {
		t.Fatalf("cleanup candidates=%+v err=%v", cleanups, err)
	}
	cleaned, err := st.MarkMediaUploadTempCleaned(expired.ID, expired.Revision, params.SessionExpiresAt+1)
	if err != nil || cleaned.TempCleanedAt == 0 {
		t.Fatalf("cleaned session=%+v err=%v", cleaned, err)
	}
	cleanups, err = st.MediaUploadSessionsForTempCleanup(10)
	if err != nil || len(cleanups) != 0 {
		t.Fatalf("remaining cleanup candidates=%+v err=%v", cleanups, err)
	}
}
