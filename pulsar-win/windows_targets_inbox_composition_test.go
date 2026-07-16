package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type targetsInboxFakeService struct {
	mu                                          sync.Mutex
	projection                                  TargetsInboxProjection
	projectionErr                               error
	projectionCalls                             int
	replays, dismisses, deletes, reports, mutes int
}

func (s *targetsInboxFakeService) Projection(context.Context) (TargetsInboxProjection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectionCalls++
	return s.projection, s.projectionErr
}
func (s *targetsInboxFakeService) Inbox(context.Context, string) (TargetsInboxPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projection.Inbox, s.projectionErr
}
func (s *targetsInboxFakeService) History(context.Context, string) (TargetsInboxHistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projection.History, s.projectionErr
}
func (s *targetsInboxFakeService) Receipts(_ context.Context, historyID, _ string) (TargetsInboxReceiptPage, error) {
	return TargetsInboxReceiptPage{Items: []TargetsInboxReceipt{{TargetLabel: "Living room", Status: targetsLabel("receipt.played", "Played", "Проиграно")}}}, nil
}
func (s *targetsInboxFakeService) DismissInbox(context.Context, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dismisses++
	return "inbox_dismissed", nil
}
func (s *targetsInboxFakeService) ReplayInbox(context.Context, string, PhaseOneDelivery, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replays++
	return "replay_accepted", nil
}
func (s *targetsInboxFakeService) DeleteTargetsHistory(context.Context, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return "media_deleted", nil
}
func (s *targetsInboxFakeService) ReportTargetsHistory(context.Context, string, PhaseOneModerationReason, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports++
	return "report_received", nil
}
func (s *targetsInboxFakeService) MuteTargetsHistorySender(context.Context, string, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutes++
	return "sender_blocked", nil
}

func windowsTargetsProjection(now time.Time) TargetsInboxProjection {
	fixture := targetsInboxFixture(now)
	return TargetsInboxProjection{Targets: fixture.Targets[:1], Inbox: TargetsInboxPage{Items: fixture.Inbox, NextCursor: fixture.InboxNextCursor},
		History: TargetsInboxHistoryPage{Items: fixture.History, NextCursor: fixture.HistoryNextCursor}, ContentPolicyState: "current"}
}

func waitTargetsState(t *testing.T, composition *WindowsTargetsInboxComposition, predicate func(WindowsTargetsInboxSnapshot) bool) WindowsTargetsInboxSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := composition.Snapshot()
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("targets state timeout: %+v", composition.Snapshot())
	return WindowsTargetsInboxSnapshot{}
}

func TestWindowsTargetsInboxCompositionPreservesExplicitIntentAndNeverAutoplays(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	store.newID = func() (string, error) { return strings.Repeat("8", 32), nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	phaseService := &phaseOneFakeService{}
	outbox, err := NewPhaseOneDraftOutbox(phaseService, store, filepath.Join(root, "outbox.json"), []CaptureMediaHandle{draft})
	if err != nil {
		t.Fatal(err)
	}
	workflow := NewWindowsCaptureWorkflowController(nil, nil, nil)
	workflow.ConfigureDraftBoundary(store, []CaptureMediaHandle{draft})
	phaseOne, err := NewWindowsPhaseOneComposition(phaseService, outbox, workflow)
	if err != nil {
		t.Fatal(err)
	}
	defer phaseOne.Close()
	service := &targetsInboxFakeService{projection: windowsTargetsProjection(time.Now().Add(time.Hour))}
	composition, err := NewWindowsTargetsInboxComposition(service, phaseOne)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	waitTargetsState(t, composition, func(snapshot WindowsTargetsInboxSnapshot) bool { return snapshot.Projection.State == TargetsInboxReady })
	service.mu.Lock()
	initialReplays := service.replays
	service.mu.Unlock()
	if initialReplays != 0 {
		t.Fatal("projection refresh caused inbox autoplay")
	}
	composition.ToggleSelectedTarget()
	selected := composition.Snapshot()
	if selected.Projection.SelectedAudience != TargetsInboxExplicitAudience || len(selected.Projection.SelectedReferences) != 1 {
		t.Fatalf("selection=%+v", selected.Projection)
	}
	composition.ToggleIncludeOrigin()
	composition.SendSelectedDraft()
	waitPhaseOneDraftState(t, phaseOne, PhaseOneDraftAccepted)
	phaseService.mu.Lock()
	if len(phaseService.explicitReferences) != 1 || len(phaseService.explicitReferences[0]) != 1 || !phaseService.explicitOrigins[0] {
		t.Fatalf("explicit send refs=%v include=%v", phaseService.explicitReferences, phaseService.explicitOrigins)
	}
	phaseService.mu.Unlock()
	composition.Refresh()
	waitTargetsState(t, composition, func(snapshot WindowsTargetsInboxSnapshot) bool {
		service.mu.Lock()
		calls := service.projectionCalls
		service.mu.Unlock()
		return calls >= 2 && snapshot.Projection.State == TargetsInboxReady && len(snapshot.Projection.SelectedReferences) == 1 && !snapshot.Busy
	})
	composition.ReplaySelectedInbox()
	waitTargetsState(t, composition, func(snapshot WindowsTargetsInboxSnapshot) bool {
		return snapshot.ActionOutcome == "replay_accepted" && !snapshot.Busy
	})
	service.mu.Lock()
	replays := service.replays
	service.mu.Unlock()
	if replays != 1 {
		t.Fatalf("replays=%d", replays)
	}
}

func TestWindowsTargetsInboxCompositionRetainsRowsButRemovesStaleAuthority(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	workflow := NewWindowsCaptureWorkflowController(nil, nil, nil)
	workflow.ConfigureDraftBoundary(store, nil)
	phaseService := &phaseOneFakeService{}
	outbox, _ := NewPhaseOneDraftOutbox(phaseService, store, filepath.Join(root, "outbox.json"), nil)
	phaseOne, _ := NewWindowsPhaseOneComposition(phaseService, outbox, workflow)
	defer phaseOne.Close()
	service := &targetsInboxFakeService{projection: windowsTargetsProjection(time.Now().Add(time.Hour))}
	composition, _ := NewWindowsTargetsInboxComposition(service, phaseOne)
	defer composition.Close()
	waitTargetsState(t, composition, func(snapshot WindowsTargetsInboxSnapshot) bool { return snapshot.Projection.State == TargetsInboxReady })
	service.mu.Lock()
	service.projectionErr = &PhaseOneClientError{Kind: PhaseOneTransport}
	service.mu.Unlock()
	composition.Refresh()
	failed := waitTargetsState(t, composition, func(snapshot WindowsTargetsInboxSnapshot) bool {
		return snapshot.Projection.State == TargetsInboxOffline
	})
	if len(failed.Projection.Inbox) == 0 || len(failed.Projection.History) == 0 || len(failed.Projection.SelectedReferences) != 0 {
		t.Fatalf("offline projection=%+v", failed.Projection)
	}
	composition.ReplaySelectedInbox()
	time.Sleep(20 * time.Millisecond)
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.replays != 0 {
		t.Fatal("stale capability reached replay service")
	}
}

func TestWindowsTargetsInboxCompositionRefreshesExpiredSelectedCapabilities(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	workflow := NewWindowsCaptureWorkflowController(nil, nil, nil)
	workflow.ConfigureDraftBoundary(store, nil)
	phaseService := &phaseOneFakeService{}
	outbox, _ := NewPhaseOneDraftOutbox(phaseService, store, filepath.Join(root, "outbox.json"), nil)
	phaseOne, _ := NewWindowsPhaseOneComposition(phaseService, outbox, workflow)
	defer phaseOne.Close()

	service := &targetsInboxFakeService{projection: windowsTargetsProjection(time.Now())}
	composition, _ := NewWindowsTargetsInboxComposition(service, phaseOne)
	defer composition.Close()
	waitTargetsState(t, composition, func(snapshot WindowsTargetsInboxSnapshot) bool {
		return snapshot.Projection.State == TargetsInboxReady
	})
	composition.ToggleSelectedTarget()

	service.mu.Lock()
	initialCalls := service.projectionCalls
	service.mu.Unlock()
	if err := composition.refreshProjection(false, ""); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	callsBeforeExpiry := service.projectionCalls
	service.mu.Unlock()
	if callsBeforeExpiry != initialCalls {
		t.Fatalf("automatic refresh ignored active selection: calls=%d initial=%d", callsBeforeExpiry, initialCalls)
	}

	composition.model.mu.Lock()
	composition.model.snapshot.Targets[0].ExpiresAt = time.Now().Add(-time.Second)
	composition.model.mu.Unlock()
	service.mu.Lock()
	service.projection.Targets[0].Reference = "trf_" + strings.Repeat("B", 43)
	service.mu.Unlock()
	if err := composition.refreshProjection(false, ""); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	callsAfterExpiry := service.projectionCalls
	service.mu.Unlock()
	if callsAfterExpiry != initialCalls+1 {
		t.Fatalf("expired capability was not refreshed: calls=%d initial=%d", callsAfterExpiry, initialCalls)
	}
	if selected := composition.Snapshot().Projection.SelectedReferences; len(selected) != 0 {
		t.Fatalf("expired selected capability retained: %v", selected)
	}
}
