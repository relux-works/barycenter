package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinRecordingCueExactBytes(t *testing.T) {
	t.Parallel()
	data := readCanonicalCue(t)
	if !ValidateBuiltinRecordingCue(data) {
		t.Fatal("canonical cue did not match the frozen format and digest")
	}
	corrupt := append([]byte(nil), data...)
	corrupt[100] ^= 0xff
	if ValidateBuiltinRecordingCue(corrupt) {
		t.Fatal("corrupted cue passed exact validation")
	}
	var metadata struct {
		SHA256 string `json:"sha256"`
		Bytes  int    `json:"bytes"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "assets", "audio", "pulsar-recording-cue.json"))
	if err != nil || json.Unmarshal(raw, &metadata) != nil {
		t.Fatalf("read cue metadata: %v", err)
	}
	if metadata.SHA256 != BuiltinRecordingCueSHA256 || metadata.Bytes != BuiltinRecordingCueByteCount {
		t.Fatalf("cue metadata drifted: %+v", metadata)
	}
}

func TestBuiltinRecordingCuePackageLoaderRejectsMissingAndCorrupt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	audio := filepath.Join(root, "Assets", "Audio")
	if err := os.MkdirAll(audio, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBuiltinRecordingCueFromPackageRoot(root); !errors.Is(err, ErrRecordingCueUnavailable) {
		t.Fatalf("missing cue err=%v", err)
	}
	path := filepath.Join(audio, BuiltinRecordingCueFilename)
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBuiltinRecordingCueFromPackageRoot(root); !errors.Is(err, ErrRecordingCueUnavailable) {
		t.Fatalf("corrupt cue err=%v", err)
	}
	data := readCanonicalCue(t)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBuiltinRecordingCueFromPackageRoot(root)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("canonical package cue err=%v", err)
	}
}

func TestCaptureTransitionTableMatchesSharedContract(t *testing.T) {
	t.Parallel()
	var contract struct {
		Transitions []struct {
			Class  CaptureMediaClass  `json:"class"`
			From   CaptureMediaState  `json:"from"`
			Action CaptureMediaAction `json:"action"`
			To     CaptureMediaState  `json:"to"`
		} `json:"transitions"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "capture-media-lifecycle-v1.json"))
	if err != nil || json.Unmarshal(raw, &contract) != nil {
		t.Fatalf("read lifecycle contract: %v", err)
	}
	if len(contract.Transitions) != 17 {
		t.Fatalf("transition count=%d want 17", len(contract.Transitions))
	}
	for _, transition := range contract.Transitions {
		got, ok := NextCaptureMediaState(transition.Class, transition.From, transition.Action)
		if !ok || got != transition.To {
			t.Fatalf("transition %+v got=%q ok=%v", transition, got, ok)
		}
	}
	if _, ok := NextCaptureMediaState(CaptureSelfTest, CaptureSelfTestLocal, CaptureBeginUpload); ok {
		t.Fatal("self-test entered upload state")
	}
}

func TestCaptureSelfTestDeletesOnCloseAndRecovery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ids := newCaptureIDSequence()
	store := NewCaptureMediaStore(root)
	store.newID = ids.next

	active, err := store.Begin(CaptureSelfTest)
	if err != nil {
		t.Fatal(err)
	}
	writeCue(t, active.Path)
	finalizing, err := store.Stop(active)
	if err != nil {
		t.Fatal(err)
	}
	local, err := store.Finalize(finalizing)
	if err != nil || local.State != CaptureSelfTestLocal {
		t.Fatalf("finalize self-test state=%q err=%v", local.State, err)
	}
	if _, ok := NextCaptureMediaState(CaptureSelfTest, local.State, CaptureBeginUpload); ok {
		t.Fatal("finalized self-test became uploadable")
	}
	if err := store.CloseSelfTest(local); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, local.Path)

	abandoned, err := store.Begin(CaptureSelfTest)
	if err != nil {
		t.Fatal(err)
	}
	writeCue(t, abandoned.Path)
	abandoned, _ = store.Stop(abandoned)
	abandoned, err = store.Finalize(abandoned)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := store.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if recovery.DeletedSelfTestCount != 1 || len(recovery.RetainedDrafts) != 0 {
		t.Fatalf("unexpected recovery: %+v", recovery)
	}
	assertAbsent(t, abandoned.Path)
}

func TestCaptureDurableDraftSurvivesRestartUntilConfirmation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ids := newCaptureIDSequence()
	store := NewCaptureMediaStore(root)
	store.newID = ids.next
	active, err := store.Begin(CaptureUserRecording)
	if err != nil {
		t.Fatal(err)
	}
	writeCue(t, active.Path)
	active, _ = store.Stop(active)
	draft, err := store.Finalize(active)
	if err != nil || draft.State != CaptureDurableUnsent {
		t.Fatalf("finalize user draft state=%q err=%v", draft.State, err)
	}

	restarted := NewCaptureMediaStore(root)
	recovery, err := restarted.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovery.RetainedDrafts, []CaptureMediaHandle{draft}) {
		t.Fatalf("durable draft was not retained: %+v", recovery)
	}
	if next, ok := NextCaptureMediaState(CaptureUserRecording, CaptureUploading, CaptureUploadFailedOrInterrupted); !ok || next != CaptureDurableUnsent {
		t.Fatalf("upload failure next=%q ok=%v", next, ok)
	}
	if err := restarted.ConfirmUploadAndDelete(draft); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, draft.Path)
}

func TestPickerIntakeCopiesIntoOpaquePrivateDraft(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ids := newCaptureIDSequence()
	store := NewCaptureMediaStore(root)
	store.newID = ids.next
	data := readCanonicalCue(t)
	draft, err := store.ImportUserDraft(bytes.NewReader(data))
	if err != nil || draft.State != CaptureDurableUnsent {
		t.Fatalf("import draft=%+v err=%v", draft, err)
	}
	if strings.Contains(draft.Path, "family-voice-message") {
		t.Fatalf("source name leaked into private draft path")
	}
	got, err := os.ReadFile(draft.Path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("private copy mismatch err=%v", err)
	}
	recovery, err := store.Recover()
	if err != nil || !reflect.DeepEqual(recovery.RetainedDrafts, []CaptureMediaHandle{draft}) {
		t.Fatalf("imported draft did not survive restart: %+v err=%v", recovery, err)
	}
}

func TestCaptureCancelInvalidFinalizeAndRecoveryDeletePartials(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ids := newCaptureIDSequence()
	store := NewCaptureMediaStore(root)
	store.newID = ids.next

	cancelled, _ := store.Begin(CaptureUserRecording)
	if err := os.WriteFile(cancelled.Path, []byte("partial microphone bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Cancel(cancelled); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, cancelled.Path)

	invalid, _ := store.Begin(CaptureUserRecording)
	if err := os.WriteFile(invalid.Path, []byte("not a complete wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid, _ = store.Stop(invalid)
	if _, err := store.Finalize(invalid); !errors.Is(err, ErrCaptureInvalidWAV) {
		t.Fatalf("invalid finalize err=%v", err)
	}
	assertAbsent(t, invalid.Path)

	crashed, _ := store.Begin(CaptureUserRecording)
	if err := os.WriteFile(crashed.Path, []byte("RIFF incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovery, err := store.Recover()
	if err != nil || recovery.DeletedPartialCount != 1 || len(recovery.RetainedDrafts) != 0 {
		t.Fatalf("partial recovery=%+v err=%v", recovery, err)
	}
}

func TestCaptureRecoveryRejectsSourceNamesAndTruncatedDrafts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	drafts := filepath.Join(root, "drafts")
	if err := os.MkdirAll(drafts, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		strings.Repeat("a", 32) + ".draft.wav": []byte("RIFF truncated"),
		"family-voice-message.wav":             []byte("source filename canary"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(drafts, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recovery, err := NewCaptureMediaStore(root).Recover()
	if err != nil || recovery.DeletedInvalidDraftCount != 2 || len(recovery.RetainedDrafts) != 0 {
		t.Fatalf("invalid draft recovery=%+v err=%v", recovery, err)
	}
	entries, _ := os.ReadDir(drafts)
	if len(entries) != 0 {
		t.Fatalf("invalid drafts remain: %v", entries)
	}
}

func TestCaptureStorageErrorsNeverExposePaths(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "private-family-recording-canary")
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewCaptureMediaStore(root).Begin(CaptureUserRecording)
	if !errors.Is(err, ErrCaptureStorage) || strings.Contains(err.Error(), root) || strings.Contains(err.Error(), "family") {
		t.Fatalf("storage error leaked path: %v", err)
	}
}

func TestRecordingCueSequenceExcludesCuesFromCapture(t *testing.T) {
	t.Parallel()
	sequence := NewRecordingCueSequencer()
	if sequence.MayCommitMicrophoneSamples() {
		t.Fatal("commit enabled before start cue")
	}
	if command, ok := sequence.Begin(); !ok || command != CommandPlayStartCue {
		t.Fatalf("begin command=%q ok=%v", command, ok)
	}
	if _, ok := sequence.StopRequested(); ok || sequence.MayCommitMicrophoneSamples() {
		t.Fatal("stop or commit accepted during start cue")
	}
	if command, ok := sequence.StartCueCompleted(); !ok || command != CommandEnableCommit || !sequence.MayCommitMicrophoneSamples() {
		t.Fatalf("start completion command=%q ok=%v commit=%v", command, ok, sequence.MayCommitMicrophoneSamples())
	}
	if command, ok := sequence.StopRequested(); !ok || command != CommandCloseCapture || sequence.MayCommitMicrophoneSamples() {
		t.Fatalf("stop command=%q ok=%v commit=%v", command, ok, sequence.MayCommitMicrophoneSamples())
	}
	if command, ok := sequence.CaptureClosed(); !ok || command != CommandPlayStopCue {
		t.Fatalf("capture close command=%q ok=%v", command, ok)
	}
	if command, ok := sequence.StopCueCompleted(); !ok || command != CommandComplete || sequence.Phase != CueComplete {
		t.Fatalf("stop completion command=%q ok=%v phase=%q", command, ok, sequence.Phase)
	}
}

func readCanonicalCue(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "assets", "audio", BuiltinRecordingCueFilename))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeCue(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, readCanonicalCue(t), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path still exists (stat=%v)", err)
	}
}

type captureIDSequence struct {
	value int
}

func newCaptureIDSequence() *captureIDSequence { return &captureIDSequence{} }

func (s *captureIDSequence) next() (string, error) {
	s.value++
	return fmt.Sprintf("%032x", s.value), nil
}
