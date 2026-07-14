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

const mediaIntegrationPreviousRevision = "0d6863c462111da6ed27f851a636e40d95100d73"

type exactMediaIntegrationPreviousResult struct {
	CreatedMediaID string `json:"created_media_id"`
}

func TestMediaIntegrationExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current media integration rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertMediaIntegrationPreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "media-integration-previous-head.db")
	current, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orbit, err := current.BootstrapLegacyOrbit(
		map[string]string{"a": strings.Repeat("a", 64)},
		map[int64]string{7001: "a"},
	)
	if err != nil || orbit == nil {
		t.Fatalf("bootstrap rollback orbit=%+v err=%v", orbit, err)
	}
	if err := current.SetMemberName(orbit.ID, 7001, "Rollback sender"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	created, err := current.CreateTelegramMedia(CreateTelegramMediaParams{
		OwnerOrbitID: orbit.ID, TelegramUserID: 7001,
		TelegramFileID: "tg-before-rollback", CreatedAt: now,
		ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := current.StageMediaPublication(created.Media.ID, created.Media.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := current.CompleteMediaPublication(
		operation.ID, operation.Revision, canonicalPublication(), now+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.UpdateMedia(MediaRecord{
		ID: ready.ID, DurationMS: ready.DurationMS,
		PathWAV: "/srv/current-linked.wav", LoudnormJSON: ready.LoudnessJSON,
		Status: "ready",
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := current.DeleteMediaItem(ready.ID, ready.Revision, now+3)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.CompleteLegacyMediaCleanup(deleted.ID, deleted.Revision, now+4); err != nil {
		t.Fatal(err)
	}
	cancellations, err := current.PendingMediaDeliveryCancellations(10)
	if err != nil || len(cancellations) != 1 {
		t.Fatalf("initial rollback cancellation=%+v err=%v", cancellations, err)
	}
	if _, err := current.CompleteMediaDeliveryCancellation(
		deleted.ID, cancellations[0].Revision, now+5,
	); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareMediaIntegrationPreviousTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "media-integration-previous-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(
		ctx, "go", "test", "-count=1", "./internal/store",
		"-run", "^TestMediaIntegrationPreviousHeadAuthority$",
	)
	command.Dir = previousDir
	command.Env = append(os.Environ(),
		"BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_DB="+path,
		"BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_ORBIT="+strconv.FormatInt(orbit.ID, 10),
		"BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_EXISTING="+deleted.ID,
		"BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_NOW="+strconv.FormatInt(now+10, 10),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact media-integration predecessor Store API test: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result exactMediaIntegrationPreviousResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.CreatedMediaID == "" {
		t.Fatalf("incomplete predecessor result=%+v", result)
	}

	current, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	terminal, err := current.GetMediaItem(deleted.ID)
	if err != nil || terminal == nil || terminal.Status != MediaStatusDeleted ||
		terminal.Revision != deleted.Revision {
		t.Fatalf("terminal generic after rollback=%+v err=%v", terminal, err)
	}
	legacy, err := current.GetMedia(deleted.ID)
	if err != nil || legacy == nil || legacy.Status != "deleted" {
		t.Fatalf("terminal legacy after roll-forward=%+v err=%v", legacy, err)
	}
	if pending, err := current.PendingLegacyMediaCleanups(10); err != nil ||
		len(pending) != 1 || pending[0].MediaID != deleted.ID ||
		pending[0].PathWAV != "/srv/previous-revived.wav" {
		t.Fatalf("legacy cleanup after rollback=%+v err=%v", pending, err)
	}
	if pending, err := current.PendingMediaDeliveryCancellations(10); err != nil ||
		len(pending) != 1 || pending[0].MediaID != deleted.ID || pending[0].Revision <= cancellations[0].Revision {
		t.Fatalf("reopened cancellation after rollback=%+v err=%v", pending, err)
	}
	createdAfter, err := current.GetMediaItem(result.CreatedMediaID)
	if err != nil || createdAfter == nil || createdAfter.Status != MediaStatusProcessing ||
		createdAfter.Source != MediaSourceTelegram {
		t.Fatalf("predecessor-created generic=%+v err=%v", createdAfter, err)
	}
	linkedAfter, err := current.MediaItemForLegacyWAV(result.CreatedMediaID)
	if err != nil || linkedAfter == nil || linkedAfter.ID != result.CreatedMediaID {
		t.Fatalf("predecessor-created compatibility link=%+v err=%v", linkedAfter, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func assertMediaIntegrationPreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", mediaIntegrationPreviousRevision+"^{commit}")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve media integration predecessor revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != mediaIntegrationPreviousRevision {
		t.Fatalf("resolved media integration predecessor=%q, want %q",
			strings.TrimSpace(string(output)), mediaIntegrationPreviousRevision)
	}
}

func prepareMediaIntegrationPreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "media-integration-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(
		ctx, "git", "archive", "--format=tar.gz",
		mediaIntegrationPreviousRevision, "coordinator",
	)
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive media integration predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(
		storeDir, "testdata", "media_integration_previous_head_authority_test.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(
		filepath.Join(previousStoreDir, "media_integration_previous_head_authority_test.go"),
		driver, 0o600,
	); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
