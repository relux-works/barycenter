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
)

const mediaProcessingPreviousRevision = "050c9792e328730e33bb65cf03fcda8e3d690061"

type exactMediaProcessingPreviousResult struct {
	ReadyMediaID   string `json:"ready_media_id"`
	ReadySHA256    string `json:"ready_sha256"`
	CreatedUpload  string `json:"created_upload"`
	InsertedLegacy string `json:"inserted_legacy"`
}

func TestMediaProcessingExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current media-processing rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertMediaProcessingPreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "media-processing-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := current.CreateSelfServiceOrbit("SubmitMedia rollback")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	item, err := current.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         MediaKindVoiceClip,
		Source:       MediaSourceTelegram,
		Title:        "canonical-before-rollback",
		CreatedAt:    now,
		ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := current.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	publication := MediaPublication{
		MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000, SizeBytes: 176444,
		SHA256:       strings.Repeat("c", 64),
		LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
	}
	ready, err := current.CompleteMediaPublication(operation.ID, operation.Revision, publication, now+2)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareMediaProcessingPreviousTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "media-processing-previous-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-count=1", "./internal/store", "-run", "^TestMediaProcessingPreviousHeadAuthority$")
	command.Dir = previousDir
	command.Env = append(os.Environ(),
		"BARYCENTER_PROCESSING_PREVIOUS_DB="+path,
		"BARYCENTER_PROCESSING_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_PROCESSING_PREVIOUS_MEDIA="+ready.ID,
		"BARYCENTER_PROCESSING_PREVIOUS_SHA="+ready.SHA256,
		"BARYCENTER_PROCESSING_PREVIOUS_ORBIT="+strconv.FormatInt(credentials.OrbitID, 10),
		"BARYCENTER_PROCESSING_PREVIOUS_ACTOR="+strconv.FormatInt(credentials.ActorID, 10),
		"BARYCENTER_PROCESSING_PREVIOUS_NOW="+strconv.FormatInt(now+10, 10),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact SubmitMedia predecessor Store API test: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result exactMediaProcessingPreviousResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.ReadyMediaID != ready.ID || result.ReadySHA256 != ready.SHA256 ||
		result.CreatedUpload == "" || result.InsertedLegacy == "" {
		t.Fatalf("incomplete predecessor result=%+v", result)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	after, err := current.GetMediaItem(ready.ID)
	if err != nil || after == nil || *after != ready {
		t.Fatalf("ready media after rollback=%+v want=%+v err=%v", after, ready, err)
	}
	found, err := current.FindReadyMediaByCanonicalHash(
		credentials.OrbitID, ready.SHA256, "m_00000000000000000000000000",
	)
	if err != nil || found == nil || found.ID != ready.ID {
		t.Fatalf("dedupe index after rollback=%+v err=%v", found, err)
	}
	createdUpload, err := current.GetMediaUploadSession(result.CreatedUpload)
	if err != nil || createdUpload == nil || createdUpload.Status != UploadStatusOpen {
		t.Fatalf("predecessor-created upload=%+v err=%v", createdUpload, err)
	}
	legacy, err := current.GetMedia(result.InsertedLegacy)
	if err != nil || legacy == nil || legacy.Status != "ready" {
		t.Fatalf("predecessor-created legacy media=%+v err=%v", legacy, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func assertMediaProcessingPreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", mediaProcessingPreviousRevision+"^{commit}")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve SubmitMedia predecessor revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != mediaProcessingPreviousRevision {
		t.Fatalf("resolved SubmitMedia predecessor=%q, want %q", strings.TrimSpace(string(output)), mediaProcessingPreviousRevision)
	}
}

func prepareMediaProcessingPreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "media-processing-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar.gz", mediaProcessingPreviousRevision, "coordinator")
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive SubmitMedia predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "media_processing_previous_head_authority_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir, "media_processing_previous_head_authority_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
