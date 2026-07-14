//go:build previoushead

package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/session"
)

const mediaIngestPreviousRevision = "06a06c099ed5b4f37f5e2dd3648772ffd041dfd9"

type exactMediaPreviousResult struct {
	LegacyStatus      string `json:"legacy_status"`
	SessionPositionMS int64  `json:"session_position_ms"`
	InsertedLegacyID  string `json:"inserted_legacy_id"`
	DissolvedOrbitID  int64  `json:"dissolved_orbit_id"`
}

func TestMediaIngestExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current media rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertMediaIngestPreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "media-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := current.CreateSelfServiceOrbit("Keep through media rollback")
	if err != nil {
		t.Fatal(err)
	}
	dissolve, err := current.CreateSelfServiceOrbit("Dissolve through media rollback")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	legacy := MediaRecord{
		ID: "m_before_media_ingest", TGFileID: "tg-before-media-ingest", DurationMS: 1000,
		PathWAV: "/srv/before-media-ingest.wav", LoudnormJSON: "{}",
		CreatedAt: now, ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
		Status: "ready", OrbitID: keep.OrbitID,
	}
	if err := current.InsertMedia(legacy); err != nil {
		t.Fatal(err)
	}
	if err := current.SaveSession(keep.OrbitID, SessionSnapshot{
		Mode: session.ModeSolo, State: session.StatePlaying, SavedPositionMS: 101,
	}); err != nil {
		t.Fatal(err)
	}
	keepUpload, err := current.CreateMediaUpload(mediaUploadParams(keep, now+1, "exact-previous-head-upload-001"))
	if err != nil {
		t.Fatal(err)
	}
	keepUploadSession, err := current.AdvanceMediaUpload(keepUpload.Session.ID, 0, 128, now+2)
	if err != nil {
		t.Fatal(err)
	}
	dissolveItem, err := current.CreateMediaItem(mediaItemParams(dissolve, now+3))
	if err != nil {
		t.Fatal(err)
	}
	dissolvePublish, err := current.StageMediaPublication(dissolveItem.ID, dissolveItem.Revision, now+4)
	if err != nil {
		t.Fatal(err)
	}
	dissolveReady, err := current.CompleteMediaPublication(
		dissolvePublish.ID, dissolvePublish.Revision, canonicalPublication(), now+5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareMediaIngestPreviousTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "media-previous-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-count=1", "./internal/store", "-run", "^TestMediaPreviousHeadAuthority$")
	command.Dir = previousDir
	command.Env = append(os.Environ(),
		"BARYCENTER_MEDIA_PREVIOUS_DB="+path,
		"BARYCENTER_MEDIA_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_MEDIA_KEEP_ORBIT="+strconv.FormatInt(keep.OrbitID, 10),
		"BARYCENTER_MEDIA_DISSOLVE_ORBIT="+strconv.FormatInt(dissolve.OrbitID, 10),
		"BARYCENTER_MEDIA_NODE_TOKEN="+keep.NodeToken,
		"BARYCENTER_MEDIA_LEGACY_ID="+legacy.ID,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact media predecessor Store API test: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result exactMediaPreviousResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.LegacyStatus != "processing" || result.SessionPositionMS != 909 ||
		result.InsertedLegacyID == "" || result.DissolvedOrbitID != dissolve.OrbitID {
		t.Fatalf("incomplete predecessor result=%+v", result)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	legacyAfter, err := current.GetMedia(legacy.ID)
	if err != nil || legacyAfter == nil || legacyAfter.Status != "processing" || legacyAfter.DurationMS != 1777 {
		t.Fatalf("legacy media after roll-forward=%+v err=%v", legacyAfter, err)
	}
	insertedAfter, err := current.GetMedia(result.InsertedLegacyID)
	if err != nil || insertedAfter == nil || insertedAfter.PathWAV != "/srv/previous-added.wav" {
		t.Fatalf("predecessor-created media=%+v err=%v", insertedAfter, err)
	}
	snapshot, err := current.LoadSession(keep.OrbitID)
	if err != nil || snapshot == nil || snapshot.SavedPositionMS != 909 {
		t.Fatalf("session after roll-forward=%+v err=%v", snapshot, err)
	}
	if orbitID, slot, ok, err := current.LookupPlaybackToken(keep.NodeToken); err != nil || !ok || orbitID != keep.OrbitID || slot != "a" {
		t.Fatalf("token after roll-forward orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	keepUploadAfter, err := current.GetMediaUploadSession(keepUpload.Session.ID)
	if err != nil || keepUploadAfter == nil || keepUploadAfter.ReceivedSizeBytes != keepUploadSession.ReceivedSizeBytes ||
		keepUploadAfter.Revision != keepUploadSession.Revision {
		t.Fatalf("generic upload after rollback=%+v err=%v", keepUploadAfter, err)
	}
	dissolvedAfter, err := current.GetMediaItem(dissolveReady.ID)
	if err != nil || dissolvedAfter == nil || dissolvedAfter.Status != MediaStatusDeleted || dissolvedAfter.StorageKey != "" {
		t.Fatalf("orphaned media after roll-forward=%+v err=%v", dissolvedAfter, err)
	}
	cleanups, err := current.PendingMediaStorageOperations(StorageOperationCleanup, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundCleanup := false
	for _, cleanup := range cleanups {
		if cleanup.MediaID == dissolveReady.ID && cleanup.StorageKey == dissolvePublish.StorageKey {
			foundCleanup = true
		}
	}
	if !foundCleanup {
		t.Fatalf("cleanup for predecessor-dissolved media missing: %+v", cleanups)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func assertMediaIngestPreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", mediaIngestPreviousRevision+"^{commit}")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve media predecessor revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != mediaIngestPreviousRevision {
		t.Fatalf("resolved media predecessor=%q, want %q", strings.TrimSpace(string(output)), mediaIngestPreviousRevision)
	}
}

func prepareMediaIngestPreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "media-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar.gz", mediaIngestPreviousRevision, "coordinator")
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive media predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "media_previous_head_authority_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir, "media_previous_head_authority_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
