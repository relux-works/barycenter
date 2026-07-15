package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitPhaseOneActionState(t *testing.T, composition *WindowsPhaseOneComposition, outcome, failure string) WindowsPhaseOneSnapshot {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := composition.Snapshot()
		if snapshot.ActionOutcome == outcome && snapshot.FailureCode == failure {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("action state=%+v want outcome=%q failure=%q", composition.Snapshot(), outcome, failure)
	return WindowsPhaseOneSnapshot{}
}

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
		policyRequired: true,
		presence:       []PhaseOnePresenceNode{{Slot: "a", Online: true, OutputState: "ready", PlaybackState: "playing", EffectiveDND: "allow_all"}},
		history: PhaseOneHistoryPage{Items: []PhaseOneHistoryItem{{
			ID: historyID, Title: "Team update", SenderName: "Ivan", Direction: "received", Status: "played",
			RequestedDelivery: "overlay", EffectiveDelivery: "after_current", PlayedCount: 1,
			Actions: []string{"report", "replay", "block_actor"},
		}, {
			ID: "hi_" + strings.Repeat("E", 26), Title: "My update", Direction: "sent", Status: "accepted",
			RequestedDelivery: "overlay", EffectiveDelivery: "overlay", Actions: []string{"delete", "replay"},
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
	if !shell.PresenceAvailable || shell.PresenceOnline != 1 || shell.PresenceReady != 1 || len(shell.PhaseOneHistory) != 2 ||
		shell.PhaseOneHistory[0].CanDelete || !shell.PhaseOneHistory[0].CanReport || !shell.PhaseOneHistory[0].CanReplay || !shell.PhaseOneHistory[0].CanBlock ||
		!shell.PhaseOneHistory[1].CanDelete || shell.PhaseOneHistory[1].CanReport || shell.PhaseOneHistory[1].CanBlock {
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
	service.mu.Lock()
	policyDisplays, policyAccepts := service.policyDisplays, service.policyAccepts
	service.mu.Unlock()
	if policyDisplays != 1 || policyAccepts != 1 {
		t.Fatalf("policy display=%d accepts=%d", policyDisplays, policyAccepts)
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

func TestWindowsHistoryModerationUsesAllowedActionsAndPrivacySafeOutcomes(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	workflow := NewWindowsCaptureWorkflowController(nil, nil, nil)
	workflow.ConfigureDraftBoundary(store, nil)
	foreignID := "hi_" + strings.Repeat("F", 26)
	service := &phaseOneFakeService{
		history: PhaseOneHistoryPage{Items: []PhaseOneHistoryItem{{
			ID: foreignID, Title: "Foreign clip", SenderName: "Sender", Direction: "received", Status: "played",
			Actions: []string{"report", "block_actor"},
		}}},
		reportReceipt: PhaseOneHistoryActionReceipt{Outcome: "report_received"},
		blockReceipt:  PhaseOneHistoryActionReceipt{Outcome: "sender_blocked"},
		deleteReceipt: PhaseOneHistoryActionReceipt{Outcome: "media_deleted"},
	}
	outbox, err := NewPhaseOneDraftOutbox(service, store, filepath.Join(root, "outbox.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := NewWindowsPhaseOneComposition(service, outbox, workflow)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	deadline := time.Now().Add(time.Second)
	for len(composition.Snapshot().History) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	composition.SelectNextReportReason()
	composition.ReportSelectedHistoryItem("  policy evidence  ")
	waitPhaseOneActionState(t, composition, "report_received", "")
	service.mu.Lock()
	if service.reportedHistory != 1 || service.reportedReason != PhaseOneReportHarassment || service.reportedDetails != "policy evidence" {
		t.Fatalf("report call count=%d reason=%s details=%q", service.reportedHistory, service.reportedReason, service.reportedDetails)
	}
	service.reportReceipt = PhaseOneHistoryActionReceipt{Outcome: "report_already_received", Reused: true}
	service.mu.Unlock()
	composition.ReportSelectedHistoryItem("")
	waitPhaseOneActionState(t, composition, "report_already_received", "")

	composition.BlockSelectedHistoryActor()
	waitPhaseOneActionState(t, composition, "sender_blocked", "")

	service.mu.Lock()
	service.history.Items[0].Status = "playing"
	service.history.Items[0].Actions = []string{"report"}
	blockedCalls := service.blockedHistory
	deletedCalls := service.deletedHistory
	service.mu.Unlock()
	composition.refreshRemote()
	composition.DeleteSelectedHistoryItem()
	waitPhaseOneActionState(t, composition, "", "action_not_allowed")
	composition.BlockSelectedHistoryActor()
	waitPhaseOneActionState(t, composition, "", "action_not_allowed")
	service.mu.Lock()
	if service.deletedHistory != deletedCalls || service.blockedHistory != blockedCalls {
		t.Fatalf("unauthorized operation reached service: delete=%d block=%d", service.deletedHistory, service.blockedHistory)
	}
	service.history.Items[0] = PhaseOneHistoryItem{ID: "hi_" + strings.Repeat("G", 26), Title: "Owned clip", Direction: "sent", Status: "accepted", Actions: []string{"delete"}}
	service.mu.Unlock()
	composition.refreshRemote()
	composition.DeleteSelectedHistoryItem()
	waitPhaseOneActionState(t, composition, "media_deleted", "")

	service.mu.Lock()
	service.history.Items[0] = PhaseOneHistoryItem{ID: foreignID, Title: "Foreign clip", Direction: "received", Status: "played", Actions: []string{"report"}}
	service.historyActionErr = &PhaseOneClientError{Kind: PhaseOneTransport}
	service.mu.Unlock()
	composition.refreshRemote()
	composition.ReportSelectedHistoryItem("")
	waitPhaseOneActionState(t, composition, "", "coordinator_unavailable")
	if got := NewShellCopy(ShellEnglish).PhaseOneActionMessage("coordinator_unavailable"); strings.Contains(got, foreignID) || !strings.Contains(got, "try again") {
		t.Fatalf("unsafe/non-retryable error copy: %q", got)
	}
}
