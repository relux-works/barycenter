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

const mediaLifecyclePreviousRevision = "451e50bb1375b7db85b6e909c0ae4ef256efd2cc"

type exactMediaLifecyclePreviousResult struct {
	CreatedUpload string `json:"created_upload"`
}

func TestMediaLifecycleExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current media-lifecycle rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertMediaLifecyclePreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "media-lifecycle-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := current.CreateSelfServiceOrbit("Media lifecycle rollback")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, current, credentials, now,
		now+int64((7*24*time.Hour)/time.Millisecond),
	)
	deleted, err := current.DeleteAuthorizedMedia(
		credentials.ActorID, credentials.ControlToken, ready.ID, now+3,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareMediaLifecyclePreviousTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "media-lifecycle-previous-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-count=1", "./internal/store", "-run", "^TestMediaLifecyclePreviousHeadAuthority$")
	command.Dir = previousDir
	command.Env = append(os.Environ(),
		"BARYCENTER_LIFECYCLE_PREVIOUS_DB="+path,
		"BARYCENTER_LIFECYCLE_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_LIFECYCLE_PREVIOUS_MEDIA="+deleted.ID,
		"BARYCENTER_LIFECYCLE_PREVIOUS_ORBIT="+strconv.FormatInt(credentials.OrbitID, 10),
		"BARYCENTER_LIFECYCLE_PREVIOUS_ACTOR="+strconv.FormatInt(credentials.ActorID, 10),
		"BARYCENTER_LIFECYCLE_PREVIOUS_NOW="+strconv.FormatInt(now+10, 10),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact media-lifecycle predecessor Store API test: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result exactMediaLifecyclePreviousResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.CreatedUpload == "" {
		t.Fatalf("incomplete predecessor result=%+v", result)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	after, err := current.GetMediaItem(deleted.ID)
	if err != nil || after == nil || *after != deleted {
		t.Fatalf("deleted media after rollback=%+v want=%+v err=%v", after, deleted, err)
	}
	cancellations, err := current.PendingMediaDeliveryCancellations(10)
	if err != nil || len(cancellations) != 1 || cancellations[0].MediaID != deleted.ID {
		t.Fatalf("cancellation after rollback=%+v err=%v", cancellations, err)
	}
	createdUpload, err := current.GetMediaUploadSession(result.CreatedUpload)
	if err != nil || createdUpload == nil || createdUpload.Status != UploadStatusOpen {
		t.Fatalf("predecessor-created upload=%+v err=%v", createdUpload, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func assertMediaLifecyclePreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", mediaLifecyclePreviousRevision+"^{commit}")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve media-lifecycle predecessor revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != mediaLifecyclePreviousRevision {
		t.Fatalf("resolved media-lifecycle predecessor=%q, want %q", strings.TrimSpace(string(output)), mediaLifecyclePreviousRevision)
	}
}

func prepareMediaLifecyclePreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "media-lifecycle-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar.gz", mediaLifecyclePreviousRevision, "coordinator")
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive media-lifecycle predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "media_lifecycle_previous_head_authority_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir, "media_lifecycle_previous_head_authority_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
