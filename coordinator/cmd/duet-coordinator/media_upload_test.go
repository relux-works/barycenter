package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

type mediaUploadSubmitterFunc func(context.Context, string) (store.MediaItem, error)

func (submit mediaUploadSubmitterFunc) SubmitUpload(ctx context.Context, sessionID string) (store.MediaItem, error) {
	return submit(ctx, sessionID)
}

func TestOnboardingAPIUsesInjectedSharedMediaSubmitter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared-submitter.db")
	st, err := store.OpenWithOptions(path, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	called := false
	injected := mediaUploadSubmitterFunc(func(context.Context, string) (store.MediaItem, error) {
		called = true
		return store.MediaItem{ID: "m_injected"}, nil
	})
	initErr := errors.New("shared SubmitMedia initialization marker")
	api := newOnboardingAPIWithMediaSubmitter(
		st,
		&config.Config{SelfServiceOnboarding: true, MediaDir: t.TempDir()},
		slog.Default(),
		"@barycenter_bot",
		injected,
		initErr,
	)
	if !errors.Is(api.mediaSubmitterInitErr, initErr) {
		t.Fatalf("injected initialization error=%v", api.mediaSubmitterInitErr)
	}
	item, err := api.mediaSubmitter.SubmitUpload(context.Background(), "up_shared")
	if err != nil || !called || item.ID != "m_injected" {
		t.Fatalf("injected submitter item=%+v called=%v err=%v", item, called, err)
	}
}

func completeStubbedMediaUpload(harness onboardingHarness, sessionID string) (store.MediaItem, error) {
	session, err := harness.store.GetMediaUploadSession(sessionID)
	if err != nil || session == nil || session.Status != store.UploadStatusFinalizing {
		return store.MediaItem{}, fmt.Errorf("unexpected finalizing session: %+v: %v", session, err)
	}
	item, err := harness.store.GetMediaItem(session.MediaID)
	if err != nil || item == nil {
		return store.MediaItem{}, fmt.Errorf("load upload item: %v", err)
	}
	now := time.Now().UnixMilli()
	operation, err := harness.store.StageMediaPublication(item.ID, item.Revision, now)
	if err != nil {
		return store.MediaItem{}, err
	}
	ready, err := harness.store.CompleteMediaPublication(
		operation.ID,
		operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000, SizeBytes: 4,
			SHA256:       strings.Repeat("b", 64),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		},
		now+1,
	)
	if err != nil {
		return store.MediaItem{}, err
	}
	path := filepath.Join(harness.api.mediaUploadDir, session.ID+".part")
	if err := os.Remove(path); err != nil {
		return store.MediaItem{}, err
	}
	completed, err := harness.store.GetMediaUploadSession(session.ID)
	if err != nil || completed == nil {
		return store.MediaItem{}, err
	}
	if _, err := harness.store.MarkMediaUploadTempCleaned(completed.ID, completed.Revision, now+2); err != nil {
		return store.MediaItem{}, err
	}
	return ready, nil
}

func createMediaUploadRequest(handler http.Handler, bearer, idempotencyKey string, size int64) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"kind":"voice_clip","title":"Morning note","size_bytes":%d,"rights_acknowledged":true}`, size)
	req := httptest.NewRequest(http.MethodPost, "/v1/media/uploads", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func createAudioTrackUploadRequest(handler http.Handler, bearer, idempotencyKey string, size int64) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"kind":"audio_track","title":"Long track.wav","size_bytes":%d,"rights_acknowledged":true}`, size)
	req := httptest.NewRequest(http.MethodPost, "/v1/media/uploads", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestMediaUploadHTTPAudioTrackUsesLongInputGateAndCurrentConsent(t *testing.T) {
	withoutConsent := newOnboardingHarness(t)
	unaccepted := createViaAPIWithoutContentPolicy(t, withoutConsent)
	denied := createAudioTrackUploadRequest(
		withoutConsent.mux, unaccepted["control_token"].(string),
		"audio-track-consent-required-0001", media.MaxTrackBytes,
	)
	if denied.Code != http.StatusPreconditionRequired {
		t.Fatalf("unaccepted track status=%d body=%s", denied.Code, denied.Body.String())
	}

	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	control := identity["control_token"].(string)
	accepted := createAudioTrackUploadRequest(
		harness.mux, control, "audio-track-max-item-accepted-0001", media.MaxTrackBytes,
	)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("max track status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	response := decodeMediaUpload(t, accepted)
	item, err := harness.store.GetMediaItem(response.MediaID)
	if err != nil || item == nil || item.Kind != store.MediaKindAudioTrack ||
		item.Status != store.MediaStatusProcessing {
		t.Fatalf("created track=%+v err=%v", item, err)
	}

	oversizedHarness := newOnboardingHarness(t)
	oversizedIdentity := createViaAPI(t, oversizedHarness)
	oversized := createAudioTrackUploadRequest(
		oversizedHarness.mux, oversizedIdentity["control_token"].(string),
		"audio-track-max-item-rejected-0001", media.MaxTrackBytes+1,
	)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized track status=%d body=%s", oversized.Code, oversized.Body.String())
	}
}

func decodeMediaUpload(t *testing.T, recorder *httptest.ResponseRecorder) mediaUploadResponse {
	t.Helper()
	var response mediaUploadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response status=%d bytes=%d: %v", recorder.Code, recorder.Body.Len(), err)
	}
	return response
}

func putMediaUploadRequest(handler http.Handler, uploadID, bearer, offset string, body []byte, contentLength int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/v1/media/uploads/"+uploadID, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Upload-Offset", offset)
	req.Header.Set("Content-Type", "application/octet-stream")
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestMediaUploadHTTPCreateResumeFinalizeAndReplay(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	control := identity["control_token"].(string)
	const key = "clip-create-replay-0001"

	createdRecorder := createMediaUploadRequest(harness.mux, control, key, 10)
	if createdRecorder.Code != http.StatusCreated || createdRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create status=%d cache=%q body_bytes=%d", createdRecorder.Code, createdRecorder.Header().Get("Cache-Control"), createdRecorder.Body.Len())
	}
	created := decodeMediaUpload(t, createdRecorder)
	if created.UploadID == "" || created.MediaID == "" || len(created.UploadToken) != 64 ||
		created.Offset != 0 || created.Length != 10 || created.Status != string(store.UploadStatusOpen) || created.Reused {
		t.Fatalf("created upload=%+v", created)
	}
	location := createdRecorder.Header().Get("Location")
	if location != "/v1/media/uploads/"+created.UploadID || strings.Contains(location, created.UploadToken) || strings.Contains(location, control) {
		t.Fatalf("unsafe upload location=%q", location)
	}

	replayRecorder := createMediaUploadRequest(harness.mux, control, key, 10)
	if replayRecorder.Code != http.StatusOK {
		t.Fatalf("replay status=%d body_bytes=%d", replayRecorder.Code, replayRecorder.Body.Len())
	}
	replay := decodeMediaUpload(t, replayRecorder)
	if !replay.Reused || replay.UploadID != created.UploadID || replay.MediaID != created.MediaID ||
		replay.UploadToken != created.UploadToken {
		t.Fatalf("created=%+v replay=%+v", created, replay)
	}

	first := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("hello"), 5,
	)
	if first.Code != http.StatusOK || first.Header().Get("Upload-Offset") != "5" {
		t.Fatalf("first append status=%d offset=%q body_bytes=%d", first.Code, first.Header().Get("Upload-Offset"), first.Body.Len())
	}
	firstState := decodeMediaUpload(t, first)
	if firstState.Status != string(store.UploadStatusOpen) || firstState.Offset != 5 {
		t.Fatalf("first state=%+v", firstState)
	}

	final := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "5", []byte("world"), 5,
	)
	if final.Code != http.StatusOK || final.Header().Get("Upload-Offset") != "10" {
		t.Fatalf("final append status=%d offset=%q body_bytes=%d", final.Code, final.Header().Get("Upload-Offset"), final.Body.Len())
	}
	finalState := decodeMediaUpload(t, final)
	if finalState.Status != string(store.UploadStatusFinalizing) || finalState.Offset != 10 {
		t.Fatalf("final state=%+v", finalState)
	}

	for name, repeated := range map[string]*httptest.ResponseRecorder{
		"same_final_chunk": putMediaUploadRequest(
			harness.mux, created.UploadID, created.UploadToken, "5", []byte("world"), 5,
		),
		"zero_length_finalize": putMediaUploadRequest(
			harness.mux, created.UploadID, created.UploadToken, "10", nil, 0,
		),
	} {
		if repeated.Code != http.StatusOK {
			t.Fatalf("%s status=%d body_bytes=%d", name, repeated.Code, repeated.Body.Len())
		}
		state := decodeMediaUpload(t, repeated)
		if state.Status != string(store.UploadStatusFinalizing) || state.Offset != 10 {
			t.Fatalf("%s state=%+v", name, state)
		}
	}

	path := filepath.Join(harness.api.mediaUploadDir, created.UploadID+".part")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "helloworld" {
		t.Fatalf("uploaded bytes=%q", raw)
	}
	session, err := harness.store.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.Status != store.UploadStatusFinalizing {
		t.Fatalf("persisted session=%+v err=%v", session, err)
	}
	item, err := harness.store.GetMediaItem(created.MediaID)
	if err != nil || item == nil || item.Status != store.MediaStatusProcessing || item.StorageKey != "" {
		t.Fatalf("pre-processing media=%+v err=%v", item, err)
	}

	var rows int
	inspect, err := sql.Open("sqlite", harness.path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM media_upload_sessions`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("upload rows=%d", rows)
	}
	logs := harness.logs.String()
	if strings.Contains(logs, control) || strings.Contains(logs, created.UploadToken) ||
		strings.Contains(logs, key) || strings.Contains(logs, path) {
		t.Fatalf("upload secret or local path entered logs (bytes=%d)", len(logs))
	}
}

func TestMediaUploadHTTPStableFailuresAndQuota(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	control := identity["control_token"].(string)
	node := identity["node_token"].(string)
	createdRecorder := createMediaUploadRequest(harness.mux, control, "stable-failure-upload-0001", 4)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d", createdRecorder.Code)
	}
	created := decodeMediaUpload(t, createdRecorder)

	assertAPIError(t, putMediaUploadRequest(
		harness.mux, created.UploadID, runtimeHTTPTestToken(t), "0", []byte("ab"), 2,
	), http.StatusUnauthorized, errorUploadCredential, nil)
	assertAPIError(t, putMediaUploadRequest(
		harness.mux, "up_00000000000000000000000000", created.UploadToken, "0", []byte("ab"), 2,
	), http.StatusUnauthorized, errorUploadCredential, nil)

	offsetConflict := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "1", []byte("ab"), 2,
	)
	assertAPIError(t, offsetConflict, http.StatusConflict, errorUploadOffsetConflict, nil)
	if offsetConflict.Header().Get("Upload-Offset") != "0" {
		t.Fatalf("authoritative offset=%q", offsetConflict.Header().Get("Upload-Offset"))
	}
	assertAPIError(t, putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("ab"), 3,
	), http.StatusBadRequest, errorUploadLengthMismatch, nil)
	assertAPIError(t, putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("abc"), 2,
	), http.StatusBadRequest, errorUploadLengthMismatch, nil)
	assertAPIError(t, putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("abcde"), 5,
	), http.StatusRequestEntityTooLarge, errorUploadTooLarge, nil)

	originalUploadDir := harness.api.mediaUploadDir
	privateMissingPath := filepath.Join(t.TempDir(), "private-local-upload-path")
	harness.api.mediaUploadDir = privateMissingPath
	assertAPIError(t, putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("ab"), 2,
	), http.StatusInternalServerError, errorInternal, nil)
	if strings.Contains(harness.logs.String(), privateMissingPath) {
		t.Fatalf("filesystem failure logged private path (bytes=%d)", harness.logs.Len())
	}
	harness.api.mediaUploadDir = originalUploadDir
	harness.api.mediaUploadInitErr = nil

	mismatch := createMediaUploadRequest(harness.mux, control, "stable-failure-upload-0001", 3)
	assertAPIError(t, mismatch, http.StatusConflict, errorUploadStateConflict, nil)
	nodeCreate := createMediaUploadRequest(harness.mux, node, "node-cannot-upload-0001", 3)
	assertAPIError(t, nodeCreate, http.StatusForbidden, errorInsufficientCapability, nil)

	harness.api.mediaUploadQuota.MaxConcurrent = 1
	quota := createMediaUploadRequest(harness.mux, control, "concurrent-quota-http-0001", 3)
	assertAPIError(t, quota, http.StatusTooManyRequests, errorUploadQuota, nil)

	harness.api.mediaUploadNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	assertAPIError(t, putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("ab"), 2,
	), http.StatusUnauthorized, errorUploadCredential, nil)
	if strings.Contains(harness.logs.String(), control) || strings.Contains(harness.logs.String(), created.UploadToken) ||
		strings.Contains(harness.logs.String(), "stable-failure-upload-0001") {
		t.Fatalf("failure logs contain request secrets (bytes=%d)", harness.logs.Len())
	}
}

func TestMediaUploadHTTPConcurrentSameOffsetHasOneWriter(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	createdRecorder := createMediaUploadRequest(
		harness.mux, identity["control_token"].(string), "same-offset-race-0001", 4,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d", createdRecorder.Code)
	}
	created := decodeMediaUpload(t, createdRecorder)
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, body := range [][]byte{[]byte("left"), []byte("rite")} {
		body := body
		go func() {
			ready.Done()
			<-start
			results <- putMediaUploadRequest(
				harness.mux, created.UploadID, created.UploadToken, "0", body, 4,
			)
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results
	winners, conflicts := 0, 0
	for _, result := range []*httptest.ResponseRecorder{first, second} {
		switch result.Code {
		case http.StatusOK:
			winners++
		case http.StatusConflict:
			conflicts++
			var body apiErrorBody
			if err := json.Unmarshal(result.Body.Bytes(), &body); err != nil || body.Error.Code != errorUploadOffsetConflict {
				t.Fatalf("conflict body=%q err=%v", result.Body.String(), err)
			}
		default:
			t.Fatalf("unexpected concurrent status=%d body=%q", result.Code, result.Body.String())
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("concurrent winners=%d conflicts=%d", winners, conflicts)
	}
	raw, err := os.ReadFile(filepath.Join(harness.api.mediaUploadDir, created.UploadID+".part"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "left" && string(raw) != "rite" {
		t.Fatalf("non-atomic winner bytes=%q", raw)
	}
	session, err := harness.store.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.ReceivedSizeBytes != 4 || session.Status != store.UploadStatusFinalizing {
		t.Fatalf("concurrent session=%+v err=%v", session, err)
	}
}

func TestMediaUploadHTTPRestartReconcilesCrashTail(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	createdRecorder := createMediaUploadRequest(
		harness.mux, identity["control_token"].(string), "restart-crash-tail-0001", 4,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d", createdRecorder.Code)
	}
	created := decodeMediaUpload(t, createdRecorder)
	partial := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("ab"), 2,
	)
	if partial.Code != http.StatusOK {
		t.Fatalf("partial status=%d", partial.Code)
	}
	path := filepath.Join(harness.api.mediaUploadDir, created.UploadID+".part")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("UNCOMMITTED"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.OpenWithOptions(harness.path, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	api := newOnboardingAPI(reopened, harness.api.config, harness.api.log, "@barycenter_bot")
	api.mediaSubmitter = nil
	api.mediaSubmitterInitErr = nil
	mux := http.NewServeMux()
	api.register(mux)
	final := putMediaUploadRequest(mux, created.UploadID, created.UploadToken, "2", []byte("cd"), 2)
	if final.Code != http.StatusOK {
		t.Fatalf("restart final status=%d body=%q", final.Code, final.Body.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "abcd" {
		t.Fatalf("reconciled bytes=%q", raw)
	}
	session, err := reopened.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.Status != store.UploadStatusFinalizing || session.ReceivedSizeBytes != 4 {
		t.Fatalf("restarted session=%+v err=%v", session, err)
	}
}

func TestMediaUploadHTTPRestartExpiresAbandonedTempBytes(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	base := time.Now().Add(-2 * time.Hour).UTC()
	harness.api.mediaUploadNow = func() time.Time { return base }
	createdRecorder := createMediaUploadRequest(
		harness.mux, identity["control_token"].(string), "abandoned-restart-0001", 4,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdRecorder.Code, createdRecorder.Body.String())
	}
	created := decodeMediaUpload(t, createdRecorder)
	partial := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("ab"), 2,
	)
	if partial.Code != http.StatusOK {
		t.Fatalf("partial status=%d body=%q", partial.Code, partial.Body.String())
	}
	path := filepath.Join(harness.api.mediaUploadDir, created.UploadID+".part")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	orphanChunk := filepath.Join(harness.api.mediaUploadDir, ".chunk-interrupted-restart")
	if err := os.WriteFile(orphanChunk, []byte("staged-but-uncommitted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.OpenWithOptions(harness.path, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	logs := new(bytes.Buffer)
	_ = newOnboardingAPI(
		reopened, harness.api.config, slog.New(slog.NewJSONHandler(logs, nil)), "@barycenter_bot",
	)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned temp file stat error=%v", err)
	}
	if _, err := os.Stat(orphanChunk); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted staging file stat error=%v", err)
	}
	session, err := reopened.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.Status != store.UploadStatusExpired || session.TempCleanedAt == 0 {
		t.Fatalf("expired session=%+v err=%v", session, err)
	}
	item, err := reopened.GetMediaItem(created.MediaID)
	if err != nil || item == nil || item.Status != store.MediaStatusFailed || item.FailureCode != "upload_expired" {
		t.Fatalf("expired media=%+v err=%v", item, err)
	}
	if strings.Contains(logs.String(), path) || strings.Contains(logs.String(), created.UploadToken) {
		t.Fatalf("restart cleanup logs contain local or secret data (bytes=%d)", logs.Len())
	}
}

func TestMediaUploadHTTPRechecksControlInsideCreateTransaction(t *testing.T) {
	harness := newOnboardingHarness(t)
	credentials, err := harness.store.CreateSelfServiceOrbit("Stale upload control")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	harness.api.testAfterAuth = func(store.ActorContext) {
		close(entered)
		<-release
	}
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- createMediaUploadRequest(
			harness.mux, credentials.ControlToken, "stale-control-upload-0001", 4,
		)
	}()
	<-entered
	replacement := runtimeHTTPTestToken(t)
	if _, err := harness.store.ConsumeRecovery(
		credentials.RecoveryID, credentials.RecoverySecret, replacement,
	); err != nil {
		t.Fatal(err)
	}
	close(release)
	assertAPIError(t, <-response, http.StatusUnauthorized, errorUnauthorized, nil)
	var rows int
	inspect, err := sql.Open("sqlite", harness.path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM media_upload_sessions`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("stale-control upload rows=%d", rows)
	}
}

func TestMediaUploadHTTPFinalChunkInvokesSubmitMediaAndReturnsCompleted(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	control := identity["control_token"].(string)
	called := 0
	harness.api.mediaSubmitter = mediaUploadSubmitterFunc(func(_ context.Context, sessionID string) (store.MediaItem, error) {
		called++
		return completeStubbedMediaUpload(harness, sessionID)
	})
	harness.api.mediaSubmitterInitErr = nil

	createdRecorder := createMediaUploadRequest(
		harness.mux, control, "submitmedia-http-success-0001", 4,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdRecorder.Code, createdRecorder.Body.String())
	}
	created := decodeMediaUpload(t, createdRecorder)
	completedRecorder := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("RIFF"), 4,
	)
	if completedRecorder.Code != http.StatusOK || called != 1 {
		t.Fatalf("complete status=%d calls=%d body=%q", completedRecorder.Code, called, completedRecorder.Body.String())
	}
	completed := decodeMediaUpload(t, completedRecorder)
	if completed.Status != string(store.UploadStatusCompleted) || completed.Offset != 4 {
		t.Fatalf("completed response=%+v", completed)
	}
	item, err := harness.store.GetMediaItem(created.MediaID)
	if err != nil || item == nil || item.Status != store.MediaStatusReady || item.StorageKey == "" {
		t.Fatalf("ready media=%+v err=%v", item, err)
	}
	if _, err := os.Stat(filepath.Join(harness.api.mediaUploadDir, created.UploadID+".part")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed upload source stat=%v", err)
	}
}

func TestMediaUploadHTTPLiveSubmitMedia(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	harness := newOnboardingHarness(t)
	submitter, err := media.NewSubmitService(
		harness.store, harness.api.config.MediaDir, media.PresetDefault,
	)
	if err != nil {
		t.Fatal(err)
	}
	harness.api.mediaSubmitter = submitter
	harness.api.mediaSubmitterInitErr = nil
	fixture := filepath.Join(t.TempDir(), "live-upload.wav")
	generate := exec.Command(
		"ffmpeg", "-v", "error", "-nostdin", "-y", "-f", "lavfi",
		"-i", "sine=frequency=440:sample_rate=48000:duration=1",
		"-af", "volume=0.2", "-ac", "1", "-ar", "48000",
		"-c:a", "pcm_s16le", "-f", "wav", fixture,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate live upload fixture: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	identity := createViaAPI(t, harness)
	createdRecorder := createMediaUploadRequest(
		harness.mux, identity["control_token"].(string), "submitmedia-http-live-0001", int64(len(raw)),
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdRecorder.Code, createdRecorder.Body.String())
	}
	created := decodeMediaUpload(t, createdRecorder)
	completedRecorder := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", raw, int64(len(raw)),
	)
	if completedRecorder.Code != http.StatusOK {
		t.Fatalf("live complete status=%d body=%q", completedRecorder.Code, completedRecorder.Body.String())
	}
	completed := decodeMediaUpload(t, completedRecorder)
	if completed.Status != string(store.UploadStatusCompleted) || completed.Offset != int64(len(raw)) {
		t.Fatalf("live completed response=%+v", completed)
	}
	item, err := harness.store.GetMediaItem(created.MediaID)
	if err != nil || item == nil || item.Status != store.MediaStatusReady ||
		item.MIME != "audio/wav" || item.Codec != "pcm_s16le" || item.SHA256 == "" {
		t.Fatalf("live ready media=%+v err=%v", item, err)
	}
	canonical, ok := media.CanonicalPath(
		filepath.Join(harness.api.config.MediaDir, "canonical"), item.StorageKey,
	)
	if !ok {
		t.Fatalf("live canonical storage key=%q", item.StorageKey)
	}
	if info, err := os.Stat(canonical); err != nil || info.Size() != item.SizeBytes || info.Mode().Perm() != 0o600 {
		t.Fatalf("live canonical info=%+v err=%v", info, err)
	}
}

func TestMediaUploadHTTPProcessingFailureIsStableAndNonDisclosing(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	control := identity["control_token"].(string)
	harness.api.mediaSubmitter = mediaUploadSubmitterFunc(func(_ context.Context, _ string) (store.MediaItem, error) {
		return store.MediaItem{}, &media.ProcessingError{Code: "media_signature_unsupported"}
	})
	harness.api.mediaSubmitterInitErr = nil
	createdRecorder := createMediaUploadRequest(
		harness.mux, control, "submitmedia-http-failure-0001", 4,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdRecorder.Code, createdRecorder.Body.String())
	}
	created := decodeMediaUpload(t, createdRecorder)
	failed := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("nope"), 4,
	)
	assertAPIError(t, failed, http.StatusUnprocessableEntity, errorMediaProcessing, nil)
	path := filepath.Join(harness.api.mediaUploadDir, created.UploadID+".part")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("processing-failure source stat=%v", err)
	}
	session, err := harness.store.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.Status != store.UploadStatusFinalizing {
		t.Fatalf("stubbed failure session=%+v err=%v", session, err)
	}
	logs := harness.logs.String()
	for _, secret := range []string{control, created.UploadToken, path, "submitmedia-http-failure-0001"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("processing failure logged private material (log_bytes=%d)", len(logs))
		}
	}
}

func TestMediaUploadHTTPFinalizingRetryRecoversInterruptedProcessor(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPI(t, harness)
	control := identity["control_token"].(string)
	privateFailure := filepath.Join(t.TempDir(), "private-worker-crash-detail")
	harness.api.mediaSubmitter = mediaUploadSubmitterFunc(func(_ context.Context, _ string) (store.MediaItem, error) {
		return store.MediaItem{}, errors.New(privateFailure)
	})
	harness.api.mediaSubmitterInitErr = nil
	createdRecorder := createMediaUploadRequest(
		harness.mux, control, "submitmedia-http-recovery-0001", 4,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdRecorder.Code, createdRecorder.Body.String())
	}
	created := decodeMediaUpload(t, createdRecorder)
	interrupted := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", []byte("RIFF"), 4,
	)
	assertAPIError(t, interrupted, http.StatusInternalServerError, errorInternal, nil)
	session, err := harness.store.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.Status != store.UploadStatusFinalizing {
		t.Fatalf("interrupted session=%+v err=%v", session, err)
	}
	if strings.Contains(harness.logs.String(), privateFailure) {
		t.Fatalf("processor infrastructure detail entered logs (log_bytes=%d)", harness.logs.Len())
	}

	recoveryCalls := 0
	harness.api.mediaSubmitter = mediaUploadSubmitterFunc(func(_ context.Context, sessionID string) (store.MediaItem, error) {
		recoveryCalls++
		return completeStubbedMediaUpload(harness, sessionID)
	})
	recovered := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "4", nil, 0,
	)
	if recovered.Code != http.StatusOK || recoveryCalls != 1 {
		t.Fatalf("recovery status=%d calls=%d body=%q", recovered.Code, recoveryCalls, recovered.Body.String())
	}
	state := decodeMediaUpload(t, recovered)
	if state.Status != string(store.UploadStatusCompleted) || state.Offset != 4 {
		t.Fatalf("recovered response=%+v", state)
	}
}
