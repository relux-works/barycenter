package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

// This is deterministic synthetic acceptance evidence. It deliberately does
// not claim real-app playback, physical-device behavior or listening quality.
func TestMediaIngestAcceptanceHTTPUploadACLDeleteCleanup(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}

	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Acceptance upload owner")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("Acceptance upload target")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := harness.store.CreateSelfServiceOrbit("Acceptance upload foreign")
	if err != nil {
		t.Fatal(err)
	}
	submitter, err := media.NewSubmitService(
		harness.store, harness.api.config.MediaDir, media.PresetDefault,
	)
	if err != nil {
		t.Fatal(err)
	}
	harness.api.mediaSubmitter = submitter
	harness.api.mediaSubmitterInitErr = nil

	fixture := filepath.Join(t.TempDir(), "acceptance-upload.wav")
	generate := exec.Command(
		"ffmpeg", "-v", "error", "-nostdin", "-y", "-f", "lavfi",
		"-i", "sine=frequency=523.25:sample_rate=48000:duration=1",
		"-af", "volume=0.2", "-ac", "1", "-ar", "48000",
		"-c:a", "pcm_s16le", "-f", "wav", fixture,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate acceptance fixture: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}

	const idempotencyKey = "http-ingest-acceptance-0001"
	createdResponse := createMediaUploadRequest(
		harness.mux, owner.ControlToken, idempotencyKey, int64(len(raw)),
	)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdResponse.Code, createdResponse.Body.String())
	}
	created := decodeMediaUpload(t, createdResponse)
	replayResponse := createMediaUploadRequest(
		harness.mux, owner.ControlToken, idempotencyKey, int64(len(raw)),
	)
	if replayResponse.Code != http.StatusOK {
		t.Fatalf("create replay status=%d body=%q", replayResponse.Code, replayResponse.Body.String())
	}
	replayedCreate := decodeMediaUpload(t, replayResponse)
	if !replayedCreate.Reused || replayedCreate.UploadID != created.UploadID ||
		replayedCreate.MediaID != created.MediaID || replayedCreate.UploadToken != created.UploadToken {
		t.Fatalf("created=%+v replayed=%+v", created, replayedCreate)
	}

	split := len(raw) / 2
	first := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken, "0", raw[:split], int64(split),
	)
	if first.Code != http.StatusOK {
		t.Fatalf("first chunk status=%d body=%q", first.Code, first.Body.String())
	}
	firstState := decodeMediaUpload(t, first)
	if firstState.Status != string(store.UploadStatusOpen) || firstState.Offset != int64(split) {
		t.Fatalf("first chunk state=%+v", firstState)
	}
	final := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken,
		strconv.FormatInt(firstState.Offset, 10), raw[split:], int64(len(raw)-split),
	)
	if final.Code != http.StatusOK {
		t.Fatalf("final chunk status=%d body=%q", final.Code, final.Body.String())
	}
	finalState := decodeMediaUpload(t, final)
	if finalState.Status != string(store.UploadStatusCompleted) || finalState.Offset != int64(len(raw)) {
		t.Fatalf("final state=%+v", finalState)
	}
	repeatedFinal := putMediaUploadRequest(
		harness.mux, created.UploadID, created.UploadToken,
		strconv.FormatInt(finalState.Offset, 10), nil, 0,
	)
	if repeatedFinal.Code != http.StatusOK ||
		decodeMediaUpload(t, repeatedFinal).Status != string(store.UploadStatusCompleted) {
		t.Fatalf("repeated final status=%d body=%q", repeatedFinal.Code, repeatedFinal.Body.String())
	}

	ready, err := harness.store.GetMediaItem(created.MediaID)
	if err != nil || ready == nil || ready.Status != store.MediaStatusReady ||
		ready.MIME != "audio/wav" || ready.Codec != "pcm_s16le" || ready.StorageKey == "" {
		t.Fatalf("ready media=%+v err=%v", ready, err)
	}
	canonicalPath, ok := media.CanonicalPath(
		filepath.Join(harness.api.config.MediaDir, "canonical"), ready.StorageKey,
	)
	if !ok {
		t.Fatalf("canonical storage key=%q", ready.StorageKey)
	}
	targetContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	harness.api.mediaDownload.SetTargetSnapshotReader(&httpTargetSnapshotReader{
		grants: map[store.MediaTargetIdentity]bool{{
			MediaID: ready.ID, OrbitID: targetContext.OrbitID,
			ActorID: targetContext.ActorID, Slot: targetContext.Slot,
		}: true},
	})

	ownerRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", owner.ControlToken,
	)
	targetRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if ownerRead.Code != http.StatusOK || targetRead.Code != http.StatusOK ||
		ownerRead.Body.Len() != int(ready.SizeBytes) || ownerRead.Body.String() != targetRead.Body.String() {
		t.Fatalf("owner=(%d,%d) target=(%d,%d) ready_bytes=%d",
			ownerRead.Code, ownerRead.Body.Len(), targetRead.Code, targetRead.Body.Len(), ready.SizeBytes)
	}
	unknown := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/m_00000000000000000000000000", "", foreign.NodeToken,
	)
	foreignRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", foreign.NodeToken,
	)
	if unknown.Code != http.StatusNotFound || foreignRead.Code != http.StatusNotFound ||
		unknown.Body.String() != foreignRead.Body.String() {
		t.Fatalf("unknown=(%d,%q) foreign=(%d,%q)",
			unknown.Code, unknown.Body.String(), foreignRead.Code, foreignRead.Body.String())
	}

	deleted := deleteMediaRequest(harness.mux, ready.ID, owner.ControlToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%q", deleted.Code, deleted.Body.String())
	}
	revoked := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if revoked.Code != http.StatusNotFound || revoked.Body.String() != unknown.Body.String() {
		t.Fatalf("revoked status=%d body=%q", revoked.Code, revoked.Body.String())
	}
	if _, err := os.Stat(canonicalPath); err != nil {
		t.Fatalf("delete blocked on asynchronous byte cleanup: %v", err)
	}
	if err := harness.api.mediaLifecycle.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(canonicalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical bytes survived cleanup: %v", err)
	}
	if replayDelete := deleteMediaRequest(
		harness.mux, ready.ID, owner.ControlToken,
	); replayDelete.Code != http.StatusNoContent {
		t.Fatalf("delete replay status=%d body=%q", replayDelete.Code, replayDelete.Body.String())
	}
	session, err := harness.store.GetMediaUploadSession(created.UploadID)
	if err != nil || session == nil || session.Status != store.UploadStatusCompleted ||
		session.TempCleanedAt == 0 {
		t.Fatalf("completed session=%+v err=%v", session, err)
	}
	for _, secret := range []string{
		owner.ControlToken, created.UploadToken, idempotencyKey, fixture, ready.ID,
	} {
		if strings.Contains(harness.logs.String(), secret) {
			t.Fatal("acceptance flow logged private request material")
		}
	}
}
