package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitPhaseOneDraftState(t *testing.T, composition *WindowsPhaseOneComposition, state PhaseOneDraftState) WindowsPhaseOneSnapshot {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := composition.Snapshot()
		if len(snapshot.Drafts) > 0 && snapshot.Drafts[0].State == state {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("phase state=%+v want=%s", composition.Snapshot(), state)
	return WindowsPhaseOneSnapshot{}
}

func TestWindowsPhaseOneCompositionProjectsCanonicalDataAndSendsDurableDraft(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	store.newID = func() (string, error) { return strings.Repeat("3", 32), nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	historyID := "hi_" + strings.Repeat("D", 26)
	service := &phaseOneFakeService{
		presence: []PhaseOnePresenceNode{{Slot: "a", Online: true, OutputState: "ready", PlaybackState: "playing", EffectiveDND: "allow_all"}},
		history: PhaseOneHistoryPage{Items: []PhaseOneHistoryItem{{
			ID: historyID, Title: "Team update", SenderName: "Ivan", Direction: "received", Status: "played",
			RequestedDelivery: "overlay", EffectiveDelivery: "after_current", PlayedCount: 1,
			Actions: []string{"delete", "replay", "block_actor"},
		}}},
	}
	outbox, err := NewPhaseOneDraftOutbox(service, store, filepath.Join(root, "phase-one", "outbox.json"), []CaptureMediaHandle{draft})
	if err != nil {
		t.Fatal(err)
	}
	workflow := NewWindowsCaptureWorkflowController(nil, nil, nil)
	workflow.ConfigureDraftBoundary(store, []CaptureMediaHandle{draft})
	composition, err := NewWindowsPhaseOneComposition(service, outbox, workflow)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	deadline := time.Now().Add(time.Second)
	for len(composition.Snapshot().History) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	shell := ShellSnapshot{}
	composition.ApplyShellSnapshot(&shell)
	if !shell.PresenceAvailable || shell.PresenceOnline != 1 || shell.PresenceReady != 1 || len(shell.PhaseOneHistory) != 1 ||
		!shell.PhaseOneHistory[0].CanDelete || !shell.PhaseOneHistory[0].CanReplay || !shell.PhaseOneHistory[0].CanBlock {
		t.Fatalf("shell projection=%+v", shell)
	}
	body := NewShellCopy(ShellEnglish).Body(ShellHistory, shell)
	if strings.Contains(body, historyID) || !strings.Contains(body, "Team update") || !strings.Contains(body, "After current") {
		t.Fatalf("history label leaked/missed canonical data: %q", body)
	}
	composition.SelectNextRoute()
	composition.SelectNextDelivery()
	selected := composition.Snapshot()
	if selected.SelectedRoute != PhaseOneOwnBarycenter || selected.SelectedDelivery != PhaseOneInterrupt {
		t.Fatalf("selection=%+v", selected)
	}
	composition.SendSelectedDraft()
	accepted := waitPhaseOneDraftState(t, composition, PhaseOneDraftAccepted)
	if accepted.Drafts[0].EffectiveDelivery != PhaseOneAfterCurrent || accepted.Drafts[0].LocalBytesRetained {
		t.Fatalf("accepted=%+v", accepted.Drafts[0])
	}
	if _, err := os.Stat(draft.Path); !os.IsNotExist(err) {
		t.Fatalf("confirmed upload retained bytes: %v", err)
	}
}

func TestCaptureWorkflowPublishesOnlyNormalFinalizedDrafts(t *testing.T) {
	workflow := NewWindowsCaptureWorkflowController(nil, nil, nil)
	store := NewCaptureMediaStore(filepath.Join(t.TempDir(), "capture-media"))
	normal := CaptureMediaHandle{ID: strings.Repeat("a", 32), Class: CaptureUserRecording, State: CaptureDurableUnsent, Path: "opaque"}
	selfTest := CaptureMediaHandle{ID: strings.Repeat("b", 32), Class: CaptureSelfTest, State: CaptureSelfTestLocal, Path: "opaque"}
	workflow.ConfigureDraftBoundary(store, []CaptureMediaHandle{normal, selfTest})
	var attached []CaptureMediaHandle
	workflow.SetNormalDraftHandler(func(handle CaptureMediaHandle, _ PhaseOneOriginKind) { attached = append(attached, handle) })
	workflow.handleRecordingOutcome(WindowsCaptureOutcome{Draft: &selfTest})
	workflow.handleRecordingOutcome(WindowsCaptureOutcome{Draft: &normal})
	if len(attached) != 2 || attached[0].Class != CaptureUserRecording || attached[1].Class != CaptureUserRecording {
		t.Fatalf("attached=%+v", attached)
	}
	if got := workflow.RecoveredUserDrafts(); len(got) != 1 || got[0].ID != normal.ID {
		t.Fatalf("recovered=%+v", got)
	}
}
