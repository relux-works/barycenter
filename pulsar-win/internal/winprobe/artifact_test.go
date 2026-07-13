package winprobe

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func testCaptureFormat(rate, channels uint32) CaptureFormat {
	format := NewCaptureFormat()
	format.Valid = 1
	format.Ready = 1
	format.SampleRate = rate
	format.Channels = channels
	format.BitsPerSample = 32
	return format
}

func TestArtifactWriterPeriodicallySyncsAtExactSampleRateThreshold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	partial := filepath.Join(dir, "periodic.partial")
	writer, err := newArtifactWriter(fileSystem, dir, "periodic", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 3), 3); err != nil {
		t.Fatal(err)
	}
	if got := fileSystem.operationCount("sync", partial); got != 0 {
		t.Fatalf("sync count before threshold = %d, want 0", got)
	}
	if err := writer.WriteFrames(make([]float32, 1), 1); err != nil {
		t.Fatal(err)
	}
	if got := fileSystem.operationCount("sync", partial); got != 1 {
		t.Fatalf("sync count at first threshold = %d, want 1", got)
	}
	if err := writer.WriteFrames(make([]float32, 3), 3); err != nil {
		t.Fatal(err)
	}
	if got := fileSystem.operationCount("sync", partial); got != 1 {
		t.Fatalf("sync count below second threshold = %d, want 1", got)
	}
	if err := writer.WriteFrames(make([]float32, 1), 1); err != nil {
		t.Fatal(err)
	}
	if got := fileSystem.operationCount("sync", partial); got != 2 {
		t.Fatalf("sync count at second threshold = %d, want 2", got)
	}
	if err := writer.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactWriterConfirmedShutdownAppendNeverSyncsOrClosesPartial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	partial := filepath.Join(dir, "confirmed-shutdown.partial")
	writer, err := newArtifactWriter(fileSystem, dir, "confirmed-shutdown", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteBufferedFramesWithoutSync(make([]float32, 8), 8); err != nil {
		t.Fatal(err)
	}
	if got := fileSystem.operationCount("sync", partial); got != 0 {
		t.Fatalf("confirmed-shutdown sync count = %d, want 0", got)
	}
	if got := fileSystem.operationCount("close", partial); got != 0 {
		t.Fatalf("confirmed-shutdown close count = %d, want 0", got)
	}
	if got := writer.Frames(); got != 8 {
		t.Fatalf("buffered frames = %d, want 8", got)
	}
	if _, err := os.Stat(partial); err != nil {
		t.Fatalf("recovery-owned partial is unavailable: %v", err)
	}
	if err := writer.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactWriterPeriodicSyncFailureIsLatchedAndFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	partial := filepath.Join(dir, "sync-failure.partial")
	fileSystem.failNext("sync", partial, errors.New("injected periodic sync failure"))
	writer, err := newArtifactWriter(fileSystem, dir, "sync-failure", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 4), 4); err == nil || !strings.Contains(err.Error(), "periodic durable sync") {
		t.Fatalf("WriteFrames() error = %v", err)
	}
	if got := writer.Frames(); got != 4 {
		t.Fatalf("Frames() after append followed by sync failure = %d, want 4 written frames", got)
	}
	path, result, err := writer.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
	if path != "" || result != ResultFail || err == nil {
		t.Fatalf("Finalize() = (%q, %q, %v), want fail", path, result, err)
	}
	assertArtifactPathsAbsent(t, dir, "sync-failure")
}

func TestArtifactWriterAbortAggregatesFailuresAndPreservesUnownedNamespacePaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	writer, err := newArtifactWriter(fileSystem, dir, "abort-failure", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	preexisting := map[string][]byte{
		".partial.reason":     []byte("pre-existing sidecar"),
		".partial.reason.tmp": []byte("pre-existing temp"),
		".wav":                []byte("pre-existing final"),
	}
	for suffix, content := range preexisting {
		if err := os.WriteFile(filepath.Join(dir, "abort-failure"+suffix), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	partial := filepath.Join(dir, "abort-failure.partial")
	fileSystem.failNext("close", partial, errors.New("injected close failure"))
	fileSystem.failNext("remove", partial, errors.New("injected remove failure"))
	err = writer.Abort()
	if err == nil || !strings.Contains(err.Error(), "injected close failure") || !strings.Contains(err.Error(), "injected remove failure") {
		t.Fatalf("Abort() error = %v", err)
	}
	if _, err := os.Stat(partial); err != nil {
		t.Fatalf("failed partial should remain and make cleanup fail closed: %v", err)
	}
	for suffix, want := range preexisting {
		path := filepath.Join(dir, "abort-failure"+suffix)
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != string(want) {
			t.Fatalf("unowned path %s changed: content=%q err=%v", path, got, readErr)
		}
	}
}

func TestArtifactWriterAbortRetriesOwnedCleanupPostcondition(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	writer, err := newArtifactWriter(fileSystem, dir, "abort-retry", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(dir, "abort-retry.partial")
	fileSystem.failNext("remove", partial, errors.New("injected first remove failure"))
	if err := writer.Abort(); err == nil || !strings.Contains(err.Error(), "injected first remove failure") {
		t.Fatalf("first Abort() error = %v", err)
	}
	if _, err := os.Stat(partial); err != nil {
		t.Fatalf("failed owned cleanup should remain retryable: %v", err)
	}
	if err := writer.Abort(); err != nil {
		t.Fatalf("second Abort() error = %v", err)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("retry did not satisfy absence postcondition: %v", err)
	}
}

func TestArtifactWriterNeverOverwritesOrDeletesPreExistingNamespacePaths(t *testing.T) {
	t.Parallel()
	format := testCaptureFormat(4, 1)
	validFinal := append(streamingWAVHeader(format, 4), make([]byte, 4)...)
	tests := []struct {
		name    string
		suffix  string
		content []byte
		reason  CaptureReason
		hresult HResult
	}{
		{name: "final", suffix: ".wav", content: validFinal, reason: ReasonUserStop},
		{name: "sidecar", suffix: ".partial.reason", content: []byte("pre-existing sidecar"), reason: ReasonCancel, hresult: hresultRaw(0x800704c7)},
		{name: "sidecar temp", suffix: ".partial.reason.tmp", content: []byte("pre-existing temp"), reason: ReasonCancel, hresult: hresultRaw(0x800704c7)},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			collision := filepath.Join(dir, "collision"+tc.suffix)
			if err := os.WriteFile(collision, tc.content, 0o600); err != nil {
				t.Fatal(err)
			}
			writer, err := NewArtifactWriter(dir, "collision", format)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.WriteFrames(make([]float32, 4), 4); err != nil {
				t.Fatal(err)
			}
			path, result, finalizeErr := writer.Finalize(tc.reason, tc.hresult, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
			if path != "" || result != ResultFail || finalizeErr == nil {
				t.Fatalf("Finalize() = (%q, %q, %v), want collision failure", path, result, finalizeErr)
			}
			got, readErr := os.ReadFile(collision)
			if readErr != nil || string(got) != string(tc.content) {
				t.Fatalf("pre-existing %s changed: content=%q err=%v", tc.suffix, got, readErr)
			}
			for _, suffix := range []string{".partial", ".partial.reason", ".partial.reason.tmp", ".wav"} {
				candidate := filepath.Join(dir, "collision"+suffix)
				if candidate == collision {
					continue
				}
				if _, statErr := os.Stat(candidate); !os.IsNotExist(statErr) {
					t.Fatalf("writer-owned path %s survived failed claim cleanup: %v", candidate, statErr)
				}
			}
		})
	}
}

func TestNewArtifactWriterFailedPartialClaimPreservesEveryPreExistingPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	paths := map[string][]byte{
		".partial":            []byte("pre-existing partial"),
		".partial.reason":     []byte("pre-existing sidecar"),
		".partial.reason.tmp": []byte("pre-existing temp"),
		".wav":                []byte("pre-existing final"),
	}
	for suffix, content := range paths {
		if err := os.WriteFile(filepath.Join(dir, "claimed"+suffix), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewArtifactWriter(dir, "claimed", testCaptureFormat(4, 1)); err == nil {
		t.Fatal("NewArtifactWriter unexpectedly replaced a pre-existing partial")
	}
	for suffix, want := range paths {
		got, err := os.ReadFile(filepath.Join(dir, "claimed"+suffix))
		if err != nil || string(got) != string(want) {
			t.Fatalf("pre-existing %s changed after failed partial claim: content=%q err=%v", suffix, got, err)
		}
	}
}

func TestNewArtifactWriterHeaderFailureRollsBackOnlyItsPartialClaim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	preexisting := map[string][]byte{
		".partial.reason":     []byte("pre-existing sidecar"),
		".partial.reason.tmp": []byte("pre-existing temp"),
		".wav":                []byte("pre-existing final"),
	}
	for suffix, content := range preexisting {
		if err := os.WriteFile(filepath.Join(dir, "header-failure"+suffix), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fileSystem := newFaultArtifactFS()
	partial := filepath.Join(dir, "header-failure.partial")
	fileSystem.failNext("write", partial, errors.New("injected header write failure"))
	if _, err := newArtifactWriter(fileSystem, dir, "header-failure", testCaptureFormat(4, 1)); err == nil || !strings.Contains(err.Error(), "injected header write failure") {
		t.Fatalf("newArtifactWriter() error = %v", err)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("claimed partial survived constructor rollback: %v", err)
	}
	for suffix, want := range preexisting {
		got, err := os.ReadFile(filepath.Join(dir, "header-failure"+suffix))
		if err != nil || string(got) != string(want) {
			t.Fatalf("pre-existing %s changed during rollback: content=%q err=%v", suffix, got, err)
		}
	}
}

func TestNewArtifactWriterIdentityFailureDoesNotBlindlyDeleteItsClaimPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	partial := filepath.Join(dir, "identity-failure.partial")
	fileSystem := newFaultArtifactFS()
	fileSystem.failNext("fileStat", partial, errors.New("injected identity failure"))
	if _, err := newArtifactWriter(fileSystem, dir, "identity-failure", testCaptureFormat(4, 1)); err == nil ||
		!strings.Contains(err.Error(), "retained because ownership identity is unavailable") {
		t.Fatalf("newArtifactWriter() error = %v", err)
	}
	if got := fileSystem.operationCount("remove", partial); got != 0 {
		t.Fatalf("remove count for unidentified path = %d, want 0", got)
	}
	if _, err := os.Stat(partial); err != nil {
		t.Fatalf("unidentified claim should remain for fail-closed startup recovery: %v", err)
	}
}

func TestArtifactWriterRefusesToDeleteAReplacementAtAnOwnedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writer, err := NewArtifactWriter(dir, "replaced", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.file.Close(); err != nil {
		t.Fatal(err)
	}
	writer.file = nil
	partial := filepath.Join(dir, "replaced.partial")
	if err := os.Rename(partial, filepath.Join(dir, "original.partial")); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("replacement owned by another actor")
	if err := os.WriteFile(partial, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writer.Abort(); err == nil || !strings.Contains(err.Error(), "owned path identity changed") {
		t.Fatalf("Abort() error = %v, want identity refusal", err)
	}
	got, err := os.ReadFile(partial)
	if err != nil || string(got) != string(replacement) {
		t.Fatalf("replacement changed: content=%q err=%v", got, err)
	}
}

func TestArtifactWriterRefusesToPromoteAReplacementAtAnOwnedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writer, err := NewArtifactWriter(dir, "replacement-promotion", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.file.Close(); err != nil {
		t.Fatal(err)
	}
	writer.file = nil
	partial := filepath.Join(dir, "replacement-promotion.partial")
	final := filepath.Join(dir, "replacement-promotion.wav")
	if err := os.Rename(partial, filepath.Join(dir, "original.partial")); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("replacement owned by another actor")
	if err := os.WriteFile(partial, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writer.renameOwnedNoReplace(partial, final); err == nil || !strings.Contains(err.Error(), "artifact path identity changed") {
		t.Fatalf("renameOwnedNoReplace() error = %v, want identity refusal", err)
	}
	got, err := os.ReadFile(partial)
	if err != nil || string(got) != string(replacement) {
		t.Fatalf("replacement changed: content=%q err=%v", got, err)
	}
	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("replacement was promoted to final: %v", err)
	}
}

func TestArtifactWriterDiscardNeverReportsDiscardWhenCleanupFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	writer, err := newArtifactWriter(fileSystem, dir, "discard-failure", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 1), 1); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(dir, "discard-failure.partial")
	sidecar := filepath.Join(dir, "discard-failure.partial.reason")
	fileSystem.failNext("close", partial, errors.New("injected discard close failure"))
	fileSystem.failNext("remove", sidecar, errors.New("injected sidecar remove failure"))
	path, result, err := writer.Finalize(ReasonCancel, hresultRaw(0x800704c7), PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
	if path != "" || result != ResultFail || err == nil {
		t.Fatalf("Finalize() = (%q, %q, %v), want fail", path, result, err)
	}
	if _, statErr := os.Stat(partial); !os.IsNotExist(statErr) {
		t.Fatalf("partial survived while later cleanup was attempted: %v", statErr)
	}
	if _, statErr := os.Stat(sidecar); statErr != nil {
		t.Fatalf("injected sidecar failure should leave asserted postcondition false: %v", statErr)
	}
}

func TestArtifactWriterVerificationFailureCleansAllPathsOrReportsFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	writer, err := newArtifactWriter(fileSystem, dir, "verify-failure", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 4), 4); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(dir, "verify-failure.wav")
	fileSystem.failNext("open", final, errors.New("injected verification open failure"))
	fileSystem.failNext("remove", final, errors.New("injected invalid-final remove failure"))
	path, result, err := writer.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
	if path != "" || result != ResultFail || err == nil || !strings.Contains(err.Error(), "injected invalid-final remove failure") {
		t.Fatalf("Finalize() = (%q, %q, %v), want aggregated fail", path, result, err)
	}
	if _, statErr := os.Stat(final); statErr != nil {
		t.Fatalf("invalid final should remain only because its removal failed: %v", statErr)
	}
	for _, suffix := range []string{".partial", ".partial.reason", ".partial.reason.tmp"} {
		if _, statErr := os.Stat(filepath.Join(dir, "verify-failure"+suffix)); !os.IsNotExist(statErr) {
			t.Fatalf("cleanup did not continue to %s: %v", suffix, statErr)
		}
	}
}

func TestArtifactWriterSidecarCleanupFailureCannotReportPass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileSystem := newFaultArtifactFS()
	writer, err := newArtifactWriter(fileSystem, dir, "sidecar-cleanup", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 4), 4); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(dir, "sidecar-cleanup.partial.reason")
	fileSystem.failNext("remove", sidecar, errors.New("injected final sidecar cleanup failure"))
	path, result, err := writer.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
	if path == "" || result != ResultFail || err == nil {
		t.Fatalf("Finalize() = (%q, %q, %v), want valid path with failed evidence outcome", path, result, err)
	}
	if _, _, verifyErr := VerifyWAVFile(path); verifyErr != nil {
		t.Fatalf("valid final WAV was not preserved: %v", verifyErr)
	}
	if _, statErr := os.Stat(sidecar); statErr != nil {
		t.Fatalf("sidecar should remain when its asserted cleanup failed: %v", statErr)
	}
}

func TestArtifactWriterAbortAfterFinalizePreservesCommittedFinal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writer, err := NewArtifactWriter(dir, "committed", testCaptureFormat(4, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 4), 4); err != nil {
		t.Fatal(err)
	}
	path, result, err := writer.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
	if path == "" || result != ResultPass || err != nil {
		t.Fatalf("Finalize() = (%q, %q, %v), want pass", path, result, err)
	}
	if err := writer.Abort(); err != nil {
		t.Fatalf("Abort() after commit = %v", err)
	}
	if _, _, err := VerifyWAVFile(path); err != nil {
		t.Fatalf("committed final was removed or damaged by later cleanup: %v", err)
	}
}

func assertArtifactPathsAbsent(t *testing.T, dir, session string) {
	t.Helper()
	for _, suffix := range []string{".partial", ".partial.reason", ".partial.reason.tmp", ".wav"} {
		if _, err := os.Stat(filepath.Join(dir, session+suffix)); !os.IsNotExist(err) {
			t.Fatalf("%s survived fail-closed cleanup: %v", session+suffix, err)
		}
	}
}

func TestArtifactWriterFinalizesNativeFloatWAV(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := NewCaptureFormat()
	format.Valid = 1
	format.Ready = 1
	format.SampleRate = 4
	format.Channels = 2
	format.BitsPerSample = 32
	w, err := NewArtifactWriter(dir, "ok", format)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFrames([]float32{0, 0, .5, -.5, 1, -1, .25, -.25}, 4); err != nil {
		t.Fatal(err)
	}
	path, result, err := w.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultPass {
		t.Fatalf("result = %q, want pass", result)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" || string(raw[36:40]) != "data" {
		t.Fatalf("bad WAV header: %q", raw[:44])
	}
	if got := binary.LittleEndian.Uint32(raw[40:44]); got != 32 {
		t.Fatalf("data size = %d, want 32", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "ok.partial.reason")); !os.IsNotExist(err) {
		t.Fatalf("sidecar still exists: %v", err)
	}
}

func TestArtifactWriterDiscardsNonPromotableAndTooShort(t *testing.T) {
	t.Parallel()
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 4
	format.Channels = 1
	format.BitsPerSample = 32
	tests := []struct {
		name   string
		reason CaptureReason
		hr     HResult
		frames uint32
	}{
		{name: "permission", reason: ReasonPermissionRevoke, hr: hresultRaw(0x80070005), frames: 4},
		{name: "overflow", reason: ReasonOverflow, hr: hresultRaw(0x8007006f), frames: 4},
		{name: "too short", reason: ReasonUserStop, hr: 0, frames: 1},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			w, err := NewArtifactWriter(dir, "discard", format)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.WriteFrames(make([]float32, tc.frames), tc.frames); err != nil {
				t.Fatal(err)
			}
			path, result, err := w.Finalize(tc.reason, tc.hr, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 4)
			if err != nil {
				t.Fatal(err)
			}
			if path != "" || result != ResultDiscard {
				t.Fatalf("Finalize() = (%q, %q), want discard", path, result)
			}
			if _, err := os.Stat(filepath.Join(dir, "discard.partial")); !os.IsNotExist(err) {
				t.Fatalf("partial still exists: %v", err)
			}
		})
	}
}

func TestNativeFloatWAVIndependentDecoderGate(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	afinfo, afinfoErr := exec.LookPath("afinfo")
	if err != nil && afinfoErr != nil {
		t.Skip("no independent audio decoder is installed; Windows package gate remains required")
	}
	tests := []struct {
		name     string
		channels uint32
		frames   uint32
	}{
		{name: "mono", channels: 1, frames: 48000},
		{name: "stereo", channels: 2, frames: 48000},
		{name: "four-channel", channels: 4, frames: 24000},
		{name: "eight-channel", channels: 8, frames: 24000},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			format := NewCaptureFormat()
			format.Valid = 1
			format.SampleRate = 48000
			format.Channels = tc.channels
			format.BitsPerSample = 32
			writer, err := NewArtifactWriter(t.TempDir(), tc.name, format)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.WriteFrames(make([]float32, int(tc.frames*tc.channels)), tc.frames); err != nil {
				t.Fatal(err)
			}
			path, result, err := writer.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed, PermissionMonitorReady: true}, 1)
			if err != nil || result != ResultPass {
				t.Fatalf("Finalize() = (%q, %q, %v)", path, result, err)
			}
			if ffprobe != "" {
				output, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name,sample_fmt,sample_rate,channels", "-of", "json", path).CombinedOutput()
				if err != nil {
					t.Fatalf("ffprobe rejected selected 44-byte IEEE-float WAV: %v\n%s", err, output)
				}
				var probe struct {
					Streams []struct {
						CodecName  string `json:"codec_name"`
						SampleFmt  string `json:"sample_fmt"`
						SampleRate string `json:"sample_rate"`
						Channels   int    `json:"channels"`
					} `json:"streams"`
				}
				if err := json.Unmarshal(output, &probe); err != nil || len(probe.Streams) != 1 {
					t.Fatalf("decode ffprobe output: %v\n%s", err, output)
				}
				stream := probe.Streams[0]
				if stream.CodecName != "pcm_f32le" || stream.SampleFmt != "flt" || stream.SampleRate != strconv.Itoa(int(format.SampleRate)) || stream.Channels != int(tc.channels) {
					t.Fatalf("ffprobe stream = %#v", stream)
				}
			} else {
				output, err := exec.Command(afinfo, "-b", path).CombinedOutput()
				if err != nil {
					t.Fatalf("afinfo rejected selected 44-byte IEEE-float WAV: %v\n%s", err, output)
				}
				text := string(output)
				if !strings.Contains(text, fmt.Sprintf("%d ch", tc.channels)) || !strings.Contains(text, "48000 Hz") || !strings.Contains(text, "Float32") {
					t.Fatalf("afinfo did not report expected native float format:\n%s", text)
				}
			}
		})
	}
}

func TestArtifactWriterBlocksPromotionWithoutPermissionMonitor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 4
	format.Channels = 1
	writer, err := NewArtifactWriter(dir, "monitor-blocked", format)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 4), 4); err != nil {
		t.Fatal(err)
	}
	path, result, err := writer.Finalize(ReasonUserStop, 0, PromotionContext{Permission: PermissionAllowed}, 1)
	if path != "" || result != ResultBlocked || err == nil {
		t.Fatalf("Finalize() = (%q, %q, %v), want blocked with no artifact", path, result, err)
	}
	for _, name := range []string{"monitor-blocked.partial", "monitor-blocked.partial.reason", "monitor-blocked.wav"} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s survived fail-closed promotion: %v", name, statErr)
		}
	}
}

func TestVerifyWAVFileRejectsHeaderLengthMismatch(t *testing.T) {
	t.Parallel()
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 48000
	format.Channels = 1
	path := filepath.Join(t.TempDir(), "bad.wav")
	header := streamingWAVHeader(format, 8)
	if err := os.WriteFile(path, append(header, make([]byte, 4)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyWAVFile(path); err == nil {
		t.Fatal("VerifyWAVFile accepted a truncated payload")
	}
}

func TestVerifyWAVFileRejectsInconsistentByteRate(t *testing.T) {
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 48_000
	format.Channels = 2
	header := streamingWAVHeader(format, 8)
	binary.LittleEndian.PutUint32(header[28:32], 1)
	path := filepath.Join(t.TempDir(), "bad-byte-rate.wav")
	if err := os.WriteFile(path, append(header, make([]byte, 8)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyWAVFile(path); err == nil {
		t.Fatal("VerifyWAVFile accepted a byte rate inconsistent with sample rate and block alignment")
	}
}

func TestArtifactWriterAbortRemovesUntrustedQueryFailureEvidence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	format := NewCaptureFormat()
	format.Valid = 1
	format.SampleRate = 48_000
	format.Channels = 1
	writer, err := NewArtifactWriter(dir, "query-failed", format)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFrames(make([]float32, 32), 32); err != nil {
		t.Fatal(err)
	}
	if err := writer.Abort(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".partial", ".partial.reason", ".wav"} {
		if _, err := os.Stat(filepath.Join(dir, "query-failed"+suffix)); !os.IsNotExist(err) {
			t.Fatalf("untrusted artifact %s survived Abort: %v", suffix, err)
		}
	}
}
