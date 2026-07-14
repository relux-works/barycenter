package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func TestLegacyRetentionRemovesTelegramFailureSourceAtMediaExpiry(t *testing.T) {
	mediaDir := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "legacy-retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UnixMilli()
	const mediaID = "m_00000000000000000000000000"
	wavPath := filepath.Join(mediaDir, "compatibility.wav")
	sourceDir := filepath.Join(mediaDir, ".telegram")
	sourcePath := filepath.Join(sourceDir, mediaID+".source")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wavPath, []byte("compatibility"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("retained failure source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertMedia(store.MediaRecord{
		ID: mediaID, PathWAV: wavPath, Status: "failed",
		CreatedAt: now - 2, ExpiresAt: now - 1, OrbitID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := sweepExpiredLegacyMedia(slog.Default(), st, mediaDir, now); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{wavPath, sourcePath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expired private file %q still exists: %v", filepath.Base(path), err)
		}
	}
	legacy, err := st.GetMedia(mediaID)
	if err != nil || legacy == nil || legacy.Status != "deleted" {
		t.Fatalf("expired legacy media=%+v err=%v", legacy, err)
	}
}

func TestLegacyRetentionRetriesBeforeCommittingDeletedState(t *testing.T) {
	mediaDir := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "legacy-retention-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UnixMilli()
	const mediaID = "m_11111111111111111111111111"
	sourcePath := filepath.Join(mediaDir, ".telegram", mediaID+".source")
	if err := os.MkdirAll(sourcePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "cannot-remove-directory"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertMedia(store.MediaRecord{
		ID: mediaID, Status: "failed", CreatedAt: now - 2,
		ExpiresAt: now - 1, OrbitID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := sweepExpiredLegacyMedia(slog.Default(), st, mediaDir, now); err != nil {
		t.Fatal(err)
	}
	legacy, err := st.GetMedia(mediaID)
	if err != nil || legacy == nil || legacy.Status != "failed" {
		t.Fatalf("cleanup failure committed terminal state: media=%+v err=%v", legacy, err)
	}
	if err := os.RemoveAll(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := sweepExpiredLegacyMedia(slog.Default(), st, mediaDir, now); err != nil {
		t.Fatal(err)
	}
	legacy, err = st.GetMedia(mediaID)
	if err != nil || legacy == nil || legacy.Status != "deleted" {
		t.Fatalf("cleanup retry media=%+v err=%v", legacy, err)
	}
}

func readyHTTPMedia(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	now int64,
) store.MediaItem {
	t.Helper()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         store.MediaKindVoiceClip,
		Source:       store.MediaSourceApp,
		Title:        "http-delete-fixture",
		CreatedAt:    now,
		ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := harness.store.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := harness.store.CompleteMediaPublication(
		operation.ID,
		operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000, SizeBytes: 176444,
			SHA256:       strings.Repeat("e", 64),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		},
		now+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func deleteMediaRequest(handler http.Handler, mediaID, bearer string) *httptest.ResponseRecorder {
	return apiRequest(handler, http.MethodDelete, "/v1/media/"+mediaID, "", bearer)
}

func TestMediaDeleteHTTPIsImmediateIdempotentAndNonDisclosing(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP delete owner")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := harness.store.CreateSelfServiceOrbit("HTTP delete foreign")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	ready := readyHTTPMedia(t, harness, owner, now)
	unknownID := "m_00000000000000000000000000"

	foreignResponse := deleteMediaRequest(harness.mux, ready.ID, foreign.ControlToken)
	unknownResponse := deleteMediaRequest(harness.mux, unknownID, owner.ControlToken)
	if foreignResponse.Code != http.StatusNotFound || unknownResponse.Code != http.StatusNotFound ||
		foreignResponse.Body.String() != unknownResponse.Body.String() {
		t.Fatalf("non-disclosing responses foreign=(%d,%q) unknown=(%d,%q)",
			foreignResponse.Code, foreignResponse.Body.String(),
			unknownResponse.Code, unknownResponse.Body.String())
	}
	if nodeResponse := deleteMediaRequest(harness.mux, ready.ID, owner.NodeToken); nodeResponse.Code != http.StatusForbidden {
		t.Fatalf("node delete status=%d body=%q", nodeResponse.Code, nodeResponse.Body.String())
	}
	if malformed := deleteMediaRequest(harness.mux, "not-a-media-id", owner.ControlToken); malformed.Code != http.StatusNotFound {
		t.Fatalf("malformed delete status=%d body=%q", malformed.Code, malformed.Body.String())
	}
	unknownLength := httptest.NewRequest(http.MethodDelete, "/v1/media/"+ready.ID, strings.NewReader("x"))
	unknownLength.RemoteAddr = "127.0.0.1:34567"
	unknownLength.Header.Set("Authorization", "Bearer "+owner.ControlToken)
	unknownLength.ContentLength = -1
	unknownLengthRecorder := httptest.NewRecorder()
	harness.mux.ServeHTTP(unknownLengthRecorder, unknownLength)
	if unknownLengthRecorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown-length delete status=%d body=%q", unknownLengthRecorder.Code, unknownLengthRecorder.Body.String())
	}

	deletedResponse := deleteMediaRequest(harness.mux, ready.ID, owner.ControlToken)
	if deletedResponse.Code != http.StatusNoContent || deletedResponse.Body.Len() != 0 {
		t.Fatalf("delete status=%d body=%q", deletedResponse.Code, deletedResponse.Body.String())
	}
	replayed := deleteMediaRequest(harness.mux, ready.ID, owner.ControlToken)
	if replayed.Code != http.StatusNoContent || replayed.Body.Len() != 0 {
		t.Fatalf("replayed delete status=%d body=%q", replayed.Code, replayed.Body.String())
	}
	item, err := harness.store.GetMediaItem(ready.ID)
	if err != nil || item == nil || item.Status != store.MediaStatusDeleted || item.StorageKey != "" {
		t.Fatalf("deleted item=%+v err=%v", item, err)
	}
	if cancellations, err := harness.store.PendingMediaDeliveryCancellations(10); err != nil || len(cancellations) != 1 {
		t.Fatalf("delete cancellations=%+v err=%v", cancellations, err)
	}
	if metrics := harness.api.mediaLifecycle.Metrics(); metrics.DeleteRequestsTotal != 2 {
		t.Fatalf("delete metrics=%+v", metrics)
	}
	for _, secret := range []string{owner.ControlToken, foreign.ControlToken, ready.ID} {
		if strings.Contains(harness.logs.String(), secret) {
			t.Fatalf("delete logs contain request identity")
		}
	}
}

func TestFailedUploadTempCleanupIsRetrySafeAndWithinRetentionBound(t *testing.T) {
	harness := newOnboardingHarness(t)
	credentials, err := harness.store.CreateSelfServiceOrbit("Failed upload cleanup")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	creation, err := harness.store.CreateMediaUpload(store.CreateMediaUploadParams{
		Media: store.CreateMediaItemParams{
			OwnerOrbitID: credentials.OrbitID,
			ActorID:      credentials.ActorID,
			Kind:         store.MediaKindVoiceClip,
			Source:       store.MediaSourceApp,
			CreatedAt:    now,
			ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
		},
		DeclaredSizeBytes: 4,
		SessionExpiresAt:  now + int64(time.Hour/time.Millisecond),
		IdempotencyKey:    "failed-upload-cleanup-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(harness.api.mediaUploadDir, creation.Session.ID+".part")
	if err := os.WriteFile(path, []byte("bad!"), 0o600); err != nil {
		t.Fatal(err)
	}
	failed, err := harness.store.FailMediaUploadSession(
		creation.Session.ID, creation.Session.Revision, "media_signature_unsupported", now+1,
	)
	if err != nil || failed.Status != store.UploadStatusFailed {
		t.Fatalf("failed session=%+v err=%v", failed, err)
	}
	if err := harness.api.maintainMediaUploadStorage(time.UnixMilli(now + 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed upload bytes survived cleanup: %v", err)
	}
	cleaned, err := harness.store.GetMediaUploadSession(creation.Session.ID)
	if err != nil || cleaned == nil || cleaned.TempCleanedAt != now+2 {
		t.Fatalf("cleaned session=%+v err=%v", cleaned, err)
	}
	if err := harness.api.maintainMediaUploadStorage(time.UnixMilli(now + 3)); err != nil {
		t.Fatalf("idempotent failed cleanup retry: %v", err)
	}
}
