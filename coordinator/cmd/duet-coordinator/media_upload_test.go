package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func createMediaUploadRequest(handler http.Handler, bearer, idempotencyKey string, size int64) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"kind":"voice_clip","title":"Morning note","size_bytes":%d}`, size)
	req := httptest.NewRequest(http.MethodPost, "/v1/media/uploads", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
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
