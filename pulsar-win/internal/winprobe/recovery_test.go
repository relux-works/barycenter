package winprobe

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverArtifactsPromotesOnlyReasonAuthorizedPartials(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 4
	format.Channels = 1
	format.BitsPerSample = 32

	writePartialFixture(t, dir, "good", format, 4, ReasonUserStop, 0, true)
	writePartialFixture(t, dir, "revoked", format, 4, ReasonPermissionRevoke, hresultRaw(0x80070005), true)
	writePartialFixture(t, dir, "missing", format, 4, ReasonUserStop, 0, false)
	outcomes, err := RecoverArtifacts(dir, PermissionAllowed, 250)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 3 {
		t.Fatalf("outcomes = %d, want 3", len(outcomes))
	}
	if _, err := os.Stat(filepath.Join(dir, "good.wav")); err != nil {
		t.Fatalf("good artifact not recovered: %v", err)
	}
	for _, name := range []string{"revoked.wav", "revoked.partial", "missing.wav", "missing.partial"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s unexpectedly exists: %v", name, err)
		}
	}
}

func TestRecoverArtifactsRechecksCurrentPermissionAndTruncatesFrames(t *testing.T) {
	t.Parallel()
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 4
	format.Channels = 2
	format.BitsPerSample = 32

	deniedDir := t.TempDir()
	writePartialFixture(t, deniedDir, "denied", format, 4, ReasonUserStop, 0, true)
	if _, err := RecoverArtifacts(deniedDir, PermissionDeniedByUser, 250); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(deniedDir, "denied.wav")); !os.IsNotExist(err) {
		t.Fatalf("denied artifact promoted: %v", err)
	}

	truncatedDir := t.TempDir()
	writePartialFixture(t, truncatedDir, "truncated", format, 4, ReasonUserStop, 0, true)
	partial := filepath.Join(truncatedDir, "truncated.partial")
	file, err := os.OpenFile(partial, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte{1, 2, 3})
	_ = file.Close()
	if _, err := RecoverArtifacts(truncatedDir, PermissionAllowed, 250); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(filepath.Join(truncatedDir, "truncated.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != ProbeWAVHeaderSize+4*2*4 {
		t.Fatalf("recovered size = %d", stat.Size())
	}
}

func TestRecoverArtifactsDiscardAttemptsSidecarAfterPartialRemovalFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := testCaptureFormat(4, 1)
	writePartialFixture(t, dir, "discard-cleanup", format, 4, ReasonPermissionRevoke, hresultRaw(0x80070005), true)
	fileSystem := newFaultArtifactFS()
	partial := filepath.Join(dir, "discard-cleanup.partial")
	sidecar := filepath.Join(dir, "discard-cleanup.partial.reason")
	fileSystem.failNext("remove", partial, errors.New("injected recovered partial remove failure"))
	outcomes, err := recoverArtifacts(fileSystem, dir, PermissionAllowed, 250)
	if err == nil || !strings.Contains(err.Error(), "injected recovered partial remove failure") {
		t.Fatalf("recoverArtifacts() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Result != ResultFail || !strings.Contains(outcomes[0].Cause, "injected recovered partial remove failure") {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if _, statErr := os.Stat(partial); statErr != nil {
		t.Fatalf("partial should remain when removal fails: %v", statErr)
	}
	if _, statErr := os.Stat(sidecar); !os.IsNotExist(statErr) {
		t.Fatalf("sidecar cleanup was not attempted after partial failure: %v", statErr)
	}
}

func TestRecoverArtifactsSyncAndCloseFailuresDiscardOwnedPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		operation string
	}{
		{name: "sync", operation: "sync"},
		{name: "close", operation: "close"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			format := testCaptureFormat(4, 1)
			writePartialFixture(t, dir, "recovery-"+tc.name, format, 4, ReasonUserStop, 0, true)
			fileSystem := newFaultArtifactFS()
			partial := filepath.Join(dir, "recovery-"+tc.name+".partial")
			fileSystem.failNext(tc.operation, partial, errors.New("injected recovery "+tc.name+" failure"))
			outcomes, err := recoverArtifacts(fileSystem, dir, PermissionAllowed, 250)
			if err == nil || !strings.Contains(err.Error(), "injected recovery "+tc.name+" failure") {
				t.Fatalf("recoverArtifacts() error = %v", err)
			}
			if len(outcomes) != 1 || outcomes[0].Result != ResultFail || !strings.Contains(outcomes[0].Cause, "injected recovery "+tc.name+" failure") {
				t.Fatalf("outcomes = %#v", outcomes)
			}
			for _, suffix := range []string{".partial", ".partial.reason", ".wav"} {
				if _, statErr := os.Stat(filepath.Join(dir, "recovery-"+tc.name+suffix)); !os.IsNotExist(statErr) {
					t.Fatalf("%s survived recovery failure cleanup: %v", suffix, statErr)
				}
			}
		})
	}
}

func TestRecoverArtifactsVerificationFailureAttemptsFinalAndSidecarCleanup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := testCaptureFormat(4, 1)
	writePartialFixture(t, dir, "recovery-verify", format, 4, ReasonUserStop, 0, true)
	fileSystem := newFaultArtifactFS()
	final := filepath.Join(dir, "recovery-verify.wav")
	fileSystem.failNext("open", final, errors.New("injected recovered WAV verification failure"))
	fileSystem.failNext("remove", final, errors.New("injected recovered final remove failure"))
	outcomes, err := recoverArtifacts(fileSystem, dir, PermissionAllowed, 250)
	if err == nil || !strings.Contains(err.Error(), "injected recovered final remove failure") {
		t.Fatalf("recoverArtifacts() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Result != ResultFail || !strings.Contains(outcomes[0].Cause, "injected recovered final remove failure") {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if _, statErr := os.Stat(final); statErr != nil {
		t.Fatalf("final should remain only because injected removal failed: %v", statErr)
	}
	for _, suffix := range []string{".partial", ".partial.reason"} {
		if _, statErr := os.Stat(filepath.Join(dir, "recovery-verify"+suffix)); !os.IsNotExist(statErr) {
			t.Fatalf("cleanup did not continue to %s: %v", suffix, statErr)
		}
	}
}

func TestRecoverArtifactsSidecarCleanupFailureCannotReportPass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := testCaptureFormat(4, 1)
	writePartialFixture(t, dir, "recovery-sidecar", format, 4, ReasonUserStop, 0, true)
	fileSystem := newFaultArtifactFS()
	sidecar := filepath.Join(dir, "recovery-sidecar.partial.reason")
	fileSystem.failNext("remove", sidecar, errors.New("injected recovered sidecar cleanup failure"))
	outcomes, err := recoverArtifacts(fileSystem, dir, PermissionAllowed, 250)
	if err == nil || !strings.Contains(err.Error(), "injected recovered sidecar cleanup failure") {
		t.Fatalf("recoverArtifacts() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Result != ResultFail || outcomes[0].Path == "" {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if _, _, verifyErr := VerifyWAVFile(outcomes[0].Path); verifyErr != nil {
		t.Fatalf("recovered final should still be structurally valid: %v", verifyErr)
	}
	if _, statErr := os.Stat(sidecar); statErr != nil {
		t.Fatalf("sidecar should remain when cleanup postcondition fails: %v", statErr)
	}
}

func TestRecoverArtifactsNeverReplacesOrDeletesPreExistingFinal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := testCaptureFormat(4, 1)
	writePartialFixture(t, dir, "recovery-collision", format, 4, ReasonUserStop, 0, true)
	final := filepath.Join(dir, "recovery-collision.wav")
	preexisting := append(streamingWAVHeader(format, 4), make([]byte, 4)...)
	if err := os.WriteFile(final, preexisting, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyWAVFile(final); err != nil {
		t.Fatalf("pre-existing final fixture is invalid: %v", err)
	}
	outcomes, err := RecoverArtifacts(dir, PermissionAllowed, 250)
	if err == nil {
		t.Fatal("RecoverArtifacts unexpectedly reported success across a final-path collision")
	}
	if len(outcomes) != 1 || outcomes[0].Result != ResultFail || outcomes[0].Path != "" {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	got, readErr := os.ReadFile(final)
	if readErr != nil || string(got) != string(preexisting) {
		t.Fatalf("pre-existing final changed: content=%q err=%v", got, readErr)
	}
	for _, suffix := range []string{".partial", ".partial.reason"} {
		if _, statErr := os.Stat(filepath.Join(dir, "recovery-collision"+suffix)); !os.IsNotExist(statErr) {
			t.Fatalf("recovery-owned %s survived failed promotion: %v", suffix, statErr)
		}
	}
}

func TestRecoverArtifactsOrphanSidecarCleanupFailureIsReturnedAndLogged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	orphan := filepath.Join(dir, "orphan.partial.reason")
	if err := os.WriteFile(orphan, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileSystem := newFaultArtifactFS()
	fileSystem.failNext("remove", orphan, errors.New("injected orphan remove failure"))
	outcomes, err := recoverArtifacts(fileSystem, dir, PermissionAllowed, 250)
	if err == nil || !strings.Contains(err.Error(), "injected orphan remove failure") {
		t.Fatalf("recoverArtifacts() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Result != ResultFail || !strings.Contains(outcomes[0].Cause, "orphan sidecar cleanup") {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if _, statErr := os.Stat(orphan); statErr != nil {
		t.Fatalf("orphan should remain when removal fails: %v", statErr)
	}
}

func writePartialFixture(t *testing.T, dir, session string, format CaptureFormat, frames uint32, reason CaptureReason, hr HResult, sidecar bool) {
	t.Helper()
	writer, err := NewArtifactWriter(dir, session, format)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, frames*format.Channels), frames); err != nil {
		t.Fatal(err)
	}
	if err := writer.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := writer.file.Close(); err != nil {
		t.Fatal(err)
	}
	writer.file = nil
	if sidecar {
		record := ReasonRecord{Version: 1, SessionID: session, Reason: reason, ReasonName: reason.String(), HResult: hr, TimestampMS: 1}
		raw, _ := json.Marshal(record)
		if err := os.WriteFile(filepath.Join(dir, session+".partial.reason"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
