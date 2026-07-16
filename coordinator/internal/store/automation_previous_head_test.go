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

	automationcontract "relux.works/duet/coordinator/internal/automation"
)

const automationPreviousRevision = "6f772ba21000915980275520a6c5a24c388909a2"

func TestAutomationExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current automation rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "rev-parse", automationPreviousRevision+"^{commit}")
	command.Dir = repoRoot
	resolved, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(resolved)) != automationPreviousRevision {
		t.Fatalf("resolve automation predecessor: %v: %s", err, resolved)
	}

	path := filepath.Join(t.TempDir(), "automation-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := current.CreateSelfServiceOrbit("Automation rollback")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	acceptCurrentContentPolicy(t, current, owner, now)
	media := readySavedCueMedia(t, current, owner, now+1,
		now+int64((30*24*time.Hour)/time.Millisecond), "e", 1024, 100)
	cue := createMediaSavedCue(t, current, owner, media, "rollback", now+4)
	if _, err := current.SetAutomationFeatureState(SetAutomationFeatureStateParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		AutomationEnabled: true, Timezone: "UTC", QuietHoursJSON: `[]`,
		OccurredAt: now + 5,
	}); err != nil {
		t.Fatal(err)
	}
	issued, err := current.IssueAutomationPrincipal(IssueAutomationPrincipalParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		DisplayName: "rollback", AllowedCueIDs: []string{cue.ID},
		AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter},
		MaxTargetCount:   1, IssuedAt: now + 6,
		ExpiresAt: now + int64((24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, _, err := current.ClaimAutomationAPIExecution(ClaimAutomationAPIExecutionParams{
		Secret: issued.Secret, CueID: cue.ID,
		AudienceKind:   automationcontract.AudienceOwnBarycenter,
		IdempotencyKey: "previous-head-idempotency-0001",
		RequestDigest:  strings.Repeat("f", 64), ClaimedAt: now + 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	controlSchedule, err := current.CreateAuthorizedAutomationSchedule(
		automationControlTestAuth(owner, "previous-control-schedule", "schedule", now+8),
		AutomationScheduleControlParams{
			CueID: cue.ID, DisplayName: "rollback control", Timezone: "UTC",
			WeekdaysMask: 127, LocalMinute: 600,
			AudienceKind:   automationcontract.AudienceOwnBarycenter,
			PolicyRevision: 1,
		})
	if err != nil {
		t.Fatal(err)
	}
	controlPrincipal, err := current.IssueAuthorizedAutomationPrincipal(
		automationControlTestAuth(owner, "previous-control-principal", "principal", now+9),
		AutomationPrincipalControlParams{
			DisplayName: "rollback control", AllowedCueIDs: []string{cue.ID},
			AllowedAudiences: []automationcontract.AudienceKind{automationcontract.AudienceOwnBarycenter},
			MaxTargetCount:   1, ExpiresAt: now + int64((24*time.Hour)/time.Millisecond),
		})
	if err != nil || !controlPrincipal.SecretAvailable {
		t.Fatalf("control principal=%+v err=%v", controlPrincipal, err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareAutomationPreviousTree(t, repoRoot, storeDir)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command = exec.CommandContext(ctx, "go", "test", "-count=1", "./internal/store",
		"-run", "^TestAutomationPreviousHeadAuthority$")
	command.Dir = previousDir
	command.Env = append(os.Environ(), "BARYCENTER_AUTOMATION_PREVIOUS_DB="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact automation predecessor Store API test: %v\n%s", err, output)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	if probe, err := current.GetSetting("automation_previous_head_probe"); err != nil || probe != "written" {
		t.Fatalf("predecessor probe=%q err=%v", probe, err)
	}
	resolvedPrincipal, err := current.ResolveAutomationPrincipalSecret(issued.Secret, now+8)
	if err != nil || resolvedPrincipal.ID != issued.Principal.ID {
		t.Fatalf("principal after rollback=%+v err=%v", resolvedPrincipal, err)
	}
	storedExecution, err := scanAutomationExecution(current.db.QueryRow(
		`SELECT `+automationExecutionColumns+` FROM automation_executions WHERE id = ?`, execution.ID))
	if err != nil || storedExecution.CueID != cue.ID || storedExecution.Status != "claimed" {
		t.Fatalf("execution after rollback=%+v err=%v", storedExecution, err)
	}
	storedMedia, err := current.GetMediaItem(media.ID)
	if err != nil || storedMedia == nil || storedMedia.Status != MediaStatusReady {
		t.Fatalf("media after rollback=%+v err=%v", storedMedia, err)
	}
	controlSchedules, err := current.AuthorizedAutomationSchedules(owner.ActorID, owner.ControlToken)
	if err != nil || len(controlSchedules) != 1 ||
		controlSchedules[0].Schedule.ID != controlSchedule.Control.Schedule.ID {
		t.Fatalf("control schedules after rollback=%+v err=%v", controlSchedules, err)
	}
	controlPrincipals, err := current.AuthorizedAutomationPrincipals(owner.ActorID, owner.ControlToken)
	if err != nil || len(controlPrincipals) != 2 {
		t.Fatalf("control principals after rollback=%+v err=%v", controlPrincipals, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func prepareAutomationPreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "automation-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar.gz",
		automationPreviousRevision, "coordinator")
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive automation predecessor: %v", err)
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
		storeDir, "testdata", "automation_previous_head_authority_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir,
		"automation_previous_head_authority_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
