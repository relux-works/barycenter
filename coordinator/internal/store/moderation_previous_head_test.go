//go:build previoushead

package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const moderationPreviousRevision = "45cb0fbbd954fac12818915abbb52647b6f045c5"

func TestModerationExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current moderation rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", moderationPreviousRevision+"^{commit}")
	command.Dir = repoRoot
	resolved, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(resolved)) != moderationPreviousRevision {
		t.Fatalf("resolve moderation predecessor: %v: %s", err, resolved)
	}

	path := filepath.Join(t.TempDir(), "moderation-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := current.CreateSelfServiceOrbit("Moderation rollback source")
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := current.CreateSelfServiceOrbit("Moderation rollback reporter")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, current, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond),
	)
	if _, err := current.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(reporter, true),
	)); err != nil {
		t.Fatal(err)
	}
	created, err := current.CreateModerationReport(
		reporter.ActorID, reporter.ControlToken,
		CreateModerationReportParams{
			MediaID: media.ID, Reason: ModerationReasonSpam, CreatedAt: now + 4,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := current.ProvisionModerationOperator(
		"Rollback operator", ModerationOperatorCapabilities{List: true}, now+5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareModerationPreviousTree(t, repoRoot, storeDir)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command = exec.CommandContext(
		ctx, "go", "test", "-count=1", "./internal/store",
		"-run", "^TestModerationPreviousHeadAuthority$",
	)
	command.Dir = previousDir
	command.Env = append(os.Environ(), "BARYCENTER_MODERATION_PREVIOUS_DB="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact moderation predecessor Store API test: %v\n%s", err, output)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	got, err := current.GetAuthorizedModerationReport(
		reporter.ActorID, reporter.ControlToken, created.Report.ID,
	)
	if err != nil || got != created.Report {
		t.Fatalf("report after predecessor=%+v want=%+v err=%v", got, created.Report, err)
	}
	resolvedOperator, err := current.ResolveModerationOperator(operator.Token)
	if err != nil || resolvedOperator != operator.Operator {
		t.Fatalf("operator after predecessor=%+v want=%+v err=%v",
			resolvedOperator, operator.Operator, err)
	}
	if probe, err := current.GetSetting("moderation_previous_head_probe"); err != nil || probe != "written" {
		t.Fatalf("predecessor probe=%q err=%v", probe, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func prepareModerationPreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "moderation-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(
		ctx, "git", "archive", "--format=tar.gz", moderationPreviousRevision, "coordinator",
	)
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive moderation predecessor: %v", err)
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
		storeDir, "testdata", "moderation_previous_head_authority_test.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(
		filepath.Join(previousStoreDir, "moderation_previous_head_authority_test.go"),
		driver, 0o600,
	); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
