package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

type httpTargetSnapshotReader struct {
	grants map[store.MediaTargetIdentity]bool
	calls  []store.MediaTargetIdentity
	err    error
}

func (reader *httpTargetSnapshotReader) AllowsMediaDownload(
	_ context.Context,
	target store.MediaTargetIdentity,
) (bool, error) {
	reader.calls = append(reader.calls, target)
	return reader.grants[target], reader.err
}

func readyDownloadHTTPMedia(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	createdAt int64,
	expiresAt int64,
	payload []byte,
) store.MediaItem {
	t.Helper()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         store.MediaKindVoiceClip,
		Source:       store.MediaSourceApp,
		Title:        "private-download-title",
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := harness.store.StageMediaPublication(item.ID, item.Revision, createdAt+1)
	if err != nil {
		t.Fatal(err)
	}
	path, ok := media.CanonicalPath(
		filepath.Join(harness.api.config.MediaDir, "canonical"), operation.StorageKey,
	)
	if !ok {
		t.Fatal("invalid HTTP download storage key")
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	ready, err := harness.store.CompleteMediaPublication(
		operation.ID,
		operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000,
			SizeBytes: int64(len(payload)), SHA256: fmt.Sprintf("%x", digest),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		},
		createdAt+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func TestMediaDownloadHTTPEnforcesOwnerAndExactTargetACL(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP download owner")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("HTTP download target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := harness.store.CreateSelfServiceOrbit("HTTP download non-target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	payload := []byte("canonical-private-wav-bytes")
	ready := readyDownloadHTTPMedia(
		t, harness, owner, now, now+int64((7*24*time.Hour)/time.Millisecond), payload,
	)
	targetContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	reader := &httpTargetSnapshotReader{grants: map[store.MediaTargetIdentity]bool{
		{
			MediaID: ready.ID, OrbitID: targetContext.OrbitID,
			ActorID: targetContext.ActorID, Slot: targetContext.Slot,
		}: true,
	}}
	harness.api.mediaDownload.SetTargetSnapshotReader(reader)

	ownerResponse := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", owner.ControlToken,
	)
	if ownerResponse.Code != http.StatusOK || ownerResponse.Body.String() != string(payload) {
		t.Fatalf("owner download status=%d body=%q", ownerResponse.Code, ownerResponse.Body.String())
	}
	if ownerResponse.Header().Get("Cache-Control") != "no-store" ||
		ownerResponse.Header().Get("Content-Type") != "audio/wav" ||
		ownerResponse.Header().Get("X-Content-Type-Options") != "nosniff" ||
		ownerResponse.Header().Get("ETag") != `"`+ready.SHA256+`"` {
		t.Fatalf("owner download headers=%v", ownerResponse.Header())
	}
	if len(reader.calls) != 0 {
		t.Fatalf("owner control consulted snapshot calls=%+v", reader.calls)
	}
	targetResponse := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if targetResponse.Code != http.StatusOK || targetResponse.Body.String() != string(payload) {
		t.Fatalf("target download status=%d body=%q", targetResponse.Code, targetResponse.Body.String())
	}
	if len(reader.calls) != 1 || reader.calls[0].ActorID != target.ActorID {
		t.Fatalf("target snapshot calls=%+v", reader.calls)
	}

	ownerNode := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", owner.NodeToken,
	)
	nontargetResponse := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", nontarget.NodeToken,
	)
	foreignControl := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.ControlToken,
	)
	unknown := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/m_00000000000000000000000000", "", target.NodeToken,
	)
	malformed := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/copied-url", "", target.NodeToken,
	)
	for name, response := range map[string]*httptest.ResponseRecorder{
		"owner_node_without_snapshot": ownerNode,
		"nontarget":                   nontargetResponse,
		"foreign_control":             foreignControl,
		"unknown":                     unknown,
		"malformed":                   malformed,
	} {
		if response.Code != http.StatusNotFound || response.Body.String() != unknown.Body.String() {
			t.Fatalf("%s response=(%d,%q) unknown=(%d,%q)",
				name, response.Code, response.Body.String(), unknown.Code, unknown.Body.String())
		}
	}
	if response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID+"?token=forbidden", "", target.NodeToken,
	); response.Code != http.StatusBadRequest {
		t.Fatalf("query credential status=%d body=%q", response.Code, response.Body.String())
	}
	if response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "body", target.NodeToken,
	); response.Code != http.StatusBadRequest {
		t.Fatalf("GET body status=%d body=%q", response.Code, response.Body.String())
	}
	if response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", "",
	); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status=%d body=%q", response.Code, response.Body.String())
	}
	if response := apiRequest(
		harness.mux, http.MethodDelete, "/v1/media/"+ready.ID, "", target.NodeToken,
	); response.Code != http.StatusForbidden {
		t.Fatalf("node DELETE status=%d body=%q", response.Code, response.Body.String())
	}
	reader.err = fmt.Errorf(
		"sensitive target reader error token=%s media=%s path=%s",
		target.NodeToken, ready.ID, harness.api.config.MediaDir,
	)
	readerFailure := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if readerFailure.Code != http.StatusInternalServerError ||
		!strings.Contains(readerFailure.Body.String(), errorInternal) {
		t.Fatalf("target reader failure status=%d body=%q",
			readerFailure.Code, readerFailure.Body.String())
	}
	reader.err = nil

	deleted := apiRequest(
		harness.mux, http.MethodDelete, "/v1/media/"+ready.ID, "", owner.ControlToken,
	)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%q", deleted.Code, deleted.Body.String())
	}
	deletedRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if deletedRead.Code != http.StatusNotFound || deletedRead.Body.String() != unknown.Body.String() {
		t.Fatalf("deleted read status=%d body=%q", deletedRead.Code, deletedRead.Body.String())
	}

	createdAt := time.Now().Add(-10 * time.Second).UnixMilli()
	expired := readyDownloadHTTPMedia(
		t, harness, owner, createdAt, createdAt+int64((5*time.Second)/time.Millisecond), []byte("expired"),
	)
	expiredContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	reader.grants[store.MediaTargetIdentity{
		MediaID: expired.ID, OrbitID: expiredContext.OrbitID,
		ActorID: expiredContext.ActorID, Slot: expiredContext.Slot,
	}] = true
	expiredRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+expired.ID, "", target.NodeToken,
	)
	if expiredRead.Code != http.StatusNotFound || expiredRead.Body.String() != unknown.Body.String() {
		t.Fatalf("expired read status=%d body=%q", expiredRead.Code, expiredRead.Body.String())
	}

	for _, secret := range []string{
		owner.ControlToken, target.NodeToken, nontarget.NodeToken,
		ready.ID, expired.ID, "private-download-title", harness.api.config.MediaDir,
	} {
		if strings.Contains(harness.logs.String(), secret) {
			t.Fatalf("media download logs contain request identity")
		}
	}
}

func TestLegacyMediaHTTPKeepsOnlyNodeApproachCompatibility(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Legacy media owner")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := harness.store.CreateSelfServiceOrbit("Legacy media peer")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy-compatible.wav")
	payload := []byte("legacy-compatible-bytes")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.InsertMedia(store.MediaRecord{
		ID: "legacy-compat", PathWAV: path, Status: "ready", OrbitID: owner.OrbitID,
		CreatedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	code, err := harness.store.ProposeLink(owner.OrbitID, owner.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := harness.store.AcceptByCode(code, peer.OrbitID)
	if err != nil || linkID == 0 {
		t.Fatalf("accept legacy approach=%d err=%v", linkID, err)
	}
	if err := harness.store.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	handler := mediaHandler(harness.store)
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/media/legacy-compat.wav", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	for name, token := range map[string]string{
		"owner_node":  owner.NodeToken,
		"linked_node": peer.NodeToken,
	} {
		response := request(token)
		if response.Code != http.StatusOK || response.Body.String() != string(payload) {
			t.Fatalf("%s status=%d body=%q", name, response.Code, response.Body.String())
		}
	}
	if response := request(owner.ControlToken); response.Code != http.StatusUnauthorized {
		t.Fatalf("legacy control status=%d body=%q", response.Code, response.Body.String())
	}
	if err := harness.store.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	if response := request(peer.NodeToken); response.Code != http.StatusNotFound {
		t.Fatalf("legacy unlinked peer status=%d body=%q", response.Code, response.Body.String())
	}
}
