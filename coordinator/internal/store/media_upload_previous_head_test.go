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

const mediaUploadPreviousRevision = "31bbeb9257b2555c86858c4087521466b58d673a"

type exactMediaUploadPreviousResult struct {
	AdvancedOffset  int64  `json:"advanced_offset"`
	CreatedMediaID  string `json:"created_media_id"`
	CreatedUploadID string `json:"created_upload_id"`
}

func TestMediaUploadExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current upload rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertMediaUploadPreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "media-upload-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := current.CreateSelfServiceOrbit("Upload rollback")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	acceptCurrentContentPolicy(t, current, credentials, now-1)
	params := authorizedUploadParams(
		credentials, now, "current-upload-before-rollback-0001", 8,
	)
	created, err := current.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, params, permissiveMediaUploadQuota(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareMediaUploadPreviousTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "media-upload-previous-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-count=1", "./internal/store", "-run", "^TestMediaUploadPreviousHeadAuthority$")
	command.Dir = previousDir
	command.Env = append(os.Environ(),
		"BARYCENTER_UPLOAD_PREVIOUS_DB="+path,
		"BARYCENTER_UPLOAD_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_UPLOAD_PREVIOUS_ORBIT="+strconv.FormatInt(credentials.OrbitID, 10),
		"BARYCENTER_UPLOAD_PREVIOUS_ACTOR="+strconv.FormatInt(credentials.ActorID, 10),
		"BARYCENTER_UPLOAD_PREVIOUS_ID="+created.Session.ID,
		"BARYCENTER_UPLOAD_PREVIOUS_NOW="+strconv.FormatInt(now+10, 10),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact upload predecessor Store API test: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result exactMediaUploadPreviousResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.AdvancedOffset != 3 || result.CreatedMediaID == "" || result.CreatedUploadID == "" {
		t.Fatalf("incomplete predecessor result=%+v", result)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	grant, err := current.RequireCurrentContentPolicy(credentials.ActorID, Identity{
		Kind: IdentityBearer, Token: credentials.ControlToken,
	})
	if err != nil || !grant.Current || grant.Version != CurrentContentPolicyVersion ||
		grant.PolicyHash != CurrentContentPolicyHash {
		t.Fatalf("content policy grant after exact predecessor=%+v err=%v", grant, err)
	}
	advanced, err := current.GetMediaUploadSession(created.Session.ID)
	if err != nil || advanced == nil || advanced.ReceivedSizeBytes != 3 ||
		advanced.TempCleanedAt != 0 || advanced.Status != UploadStatusOpen {
		t.Fatalf("current upload after rollback=%+v err=%v", advanced, err)
	}
	predecessorCreated, err := current.GetMediaUploadSession(result.CreatedUploadID)
	if err != nil || predecessorCreated == nil || predecessorCreated.MediaID != result.CreatedMediaID ||
		predecessorCreated.TempCleanedAt != 0 || predecessorCreated.Status != UploadStatusOpen {
		t.Fatalf("predecessor-created upload=%+v err=%v", predecessorCreated, err)
	}
	if exists, err := columnExists(current.db, "media_upload_sessions", "temp_cleaned_at"); err != nil || !exists {
		t.Fatalf("temp cleanup migration exists=%v err=%v", exists, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func assertMediaUploadPreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", mediaUploadPreviousRevision+"^{commit}")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve upload predecessor revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != mediaUploadPreviousRevision {
		t.Fatalf("resolved upload predecessor=%q, want %q", strings.TrimSpace(string(output)), mediaUploadPreviousRevision)
	}
}

func prepareMediaUploadPreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "media-upload-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar.gz", mediaUploadPreviousRevision, "coordinator")
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive upload predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "media_upload_previous_head_authority_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir, "media_upload_previous_head_authority_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
