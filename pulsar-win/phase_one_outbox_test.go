package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type phaseOneFakeService struct {
	mu                  sync.Mutex
	uploadKeys          []string
	transmissionKeys    []string
	deletedMediaIDs     []string
	failTransmissions   int
	presence            []PhaseOnePresenceNode
	history             PhaseOneHistoryPage
	requireConfirmation bool
	confirmations       int
	origins             []PhaseOneOriginKind
}

func (s *phaseOneFakeService) Upload(_ context.Context, path, _ string, key string) (PhaseOneUploadConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(path); err != nil {
		return PhaseOneUploadConfirmation{}, err
	}
	s.uploadKeys = append(s.uploadKeys, key)
	return PhaseOneUploadConfirmation{MediaID: "m_" + strings.Repeat("A", 26)}, nil
}

func (s *phaseOneFakeService) Transmit(_ context.Context, _ string, _ PhaseOneRoute, delivery PhaseOneDelivery, originKind PhaseOneOriginKind, key string, fallback *PhaseOneFallbackConfirmation) (PhaseOneTransmissionReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transmissionKeys = append(s.transmissionKeys, key)
	s.origins = append(s.origins, originKind)
	if s.requireConfirmation && fallback == nil {
		return PhaseOneTransmissionReceipt{}, &PhaseOneClientError{
			Kind: PhaseOneRejected, Status: 409, Code: "requires_confirmation",
			ConfirmationToken: "fc_" + strings.Repeat("c", 64),
			Alternatives:      []PhaseOneFallbackAlternative{{Delivery: PhaseOneAfterCurrent, Available: true}},
		}
	}
	if fallback != nil {
		s.confirmations++
	}
	if s.failTransmissions > 0 {
		s.failTransmissions--
		return PhaseOneTransmissionReceipt{}, &PhaseOneClientError{Kind: PhaseOneTransport}
	}
	return PhaseOneTransmissionReceipt{
		TransmissionID: "tr_" + strings.Repeat("B", 26), RequestedDelivery: delivery,
		EffectiveDelivery: PhaseOneAfterCurrent, DowngradeReason: "mandatory_target_missing_overlay_capability", Status: "accepted",
	}, nil
}

func (s *phaseOneFakeService) DeleteMedia(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedMediaIDs = append(s.deletedMediaIDs, id)
	return nil
}
func (s *phaseOneFakeService) Presence(context.Context) ([]PhaseOnePresenceNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PhaseOnePresenceNode(nil), s.presence...), nil
}
func (s *phaseOneFakeService) History(context.Context, int, string) (PhaseOneHistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.history
	result.Items = append([]PhaseOneHistoryItem(nil), s.history.Items...)
	return result, nil
}
func (*phaseOneFakeService) DeleteHistoryItem(context.Context, string) error         { return nil }
func (*phaseOneFakeService) BlockHistoryActor(context.Context, string, string) error { return nil }
func (*phaseOneFakeService) ReplayHistoryItem(context.Context, string, PhaseOneRoute, PhaseOneDelivery, string, *PhaseOneFallbackConfirmation) (PhaseOneTransmissionReceipt, error) {
	return PhaseOneTransmissionReceipt{}, nil
}

func phaseOneCueBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "assets", "audio", "pulsar-recording-cue.wav"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPhaseOneOutboxRestartRetryIsIdempotentAndFrozen(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	store.newID = func() (string, error) { return strings.Repeat("0", 31) + "1", nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	service := &phaseOneFakeService{failTransmissions: 1}
	statePath := filepath.Join(root, "phase-one", "outbox.json")
	outbox, err := NewPhaseOneDraftOutbox(service, store, statePath, []CaptureMediaHandle{draft})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := outbox.Send(context.Background(), draft.ID, PhaseOneOwnBarycenter, PhaseOneOverlay, PhaseOneMicrophone)
	if err == nil || failed.State != PhaseOneDraftRetryableFailure || failed.LocalBytesRetained || failed.FailureCode != "coordinator_unavailable" {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	if _, err := os.Stat(draft.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("confirmed upload retained local bytes: %v", err)
	}
	restartedStore := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	recovery, err := restartedStore.Recover()
	if err != nil || len(recovery.RetainedDrafts) != 0 {
		t.Fatalf("recovery=%+v err=%v", recovery, err)
	}
	restarted, err := NewPhaseOneDraftOutbox(service, restartedStore, statePath, recovery.RetainedDrafts)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := restarted.Send(context.Background(), draft.ID, PhaseOneOwnBarycenter, PhaseOneOverlay, PhaseOneMicrophone)
	if err != nil || accepted.State != PhaseOneDraftAccepted || accepted.EffectiveDelivery != PhaseOneAfterCurrent {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	if _, err := restarted.Send(context.Background(), draft.ID, PhaseOneCurrentAir, PhaseOneOverlay, PhaseOneMicrophone); !errors.Is(err, ErrPhaseOneInvalidDraft) {
		t.Fatalf("changed frozen route err=%v", err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.uploadKeys) != 1 || len(service.transmissionKeys) != 2 || service.transmissionKeys[0] != service.transmissionKeys[1] {
		t.Fatalf("uploads=%v transmissions=%v", service.uploadKeys, service.transmissionKeys)
	}
}

func TestPhaseOneOutboxPersistsPickedFileOriginAcrossRestart(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	store.newID = func() (string, error) { return strings.Repeat("6", 32), nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	service := &phaseOneFakeService{failTransmissions: 1}
	statePath := filepath.Join(root, "phase-one", "outbox.json")
	outbox, err := NewPhaseOneDraftOutbox(service, store, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Attach(draft, "Picked file", PhaseOneFile); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Send(context.Background(), draft.ID, PhaseOneOwnBarycenter, PhaseOneOverlay, PhaseOneFile); err == nil {
		t.Fatal("first transmission unexpectedly succeeded")
	}
	restarted, err := NewPhaseOneDraftOutbox(service, NewCaptureMediaStore(filepath.Join(root, "capture-media")), statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := restarted.Send(context.Background(), draft.ID, PhaseOneOwnBarycenter, PhaseOneOverlay, PhaseOneFile)
	if err != nil || accepted.OriginKind != PhaseOneFile {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.origins) != 2 || service.origins[0] != PhaseOneFile || service.origins[1] != PhaseOneFile {
		t.Fatalf("origins=%v", service.origins)
	}
}

func TestPhaseOneOutboxExplicitDeleteAndSelfTestBoundary(t *testing.T) {
	store := NewCaptureMediaStore(filepath.Join(t.TempDir(), "capture-media"))
	ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32)}
	store.newID = func() (string, error) { id := ids[0]; ids = ids[1:]; return id, nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := NewPhaseOneDraftOutbox(&phaseOneFakeService{}, store, filepath.Join(t.TempDir(), "outbox.json"), []CaptureMediaHandle{draft})
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Delete(context.Background(), draft.ID); err != nil || len(outbox.Snapshots()) != 0 {
		t.Fatalf("delete err=%v snapshots=%+v", err, outbox.Snapshots())
	}
	partial, err := store.Begin(CaptureSelfTest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partial.Path, phaseOneCueBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	stopped, _ := store.Stop(partial)
	selfTest, err := store.Finalize(stopped)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Attach(selfTest, "self test", PhaseOneMicrophone); !errors.Is(err, ErrPhaseOneInvalidDraft) {
		t.Fatalf("self-test attach err=%v", err)
	}
}

func TestPhaseOneOutboxRestartFinishesConfirmedUploadCleanupWithoutReupload(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	store.newID = func() (string, error) { return strings.Repeat("4", 32), nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	record := newPhaseOneDraftRecord(draft.ID, "Pulsar recording", PhaseOneMicrophone)
	record.MediaID = "m_" + strings.Repeat("A", 26)
	record.State = PhaseOneDraftRetryableFailure
	record.FailureCode = "local_cleanup_failed"
	statePath := filepath.Join(root, "phase-one", "outbox.json")
	if err := writePhaseOneDraftRecords(statePath, []phaseOneDraftRecord{record}); err != nil {
		t.Fatal(err)
	}
	service := &phaseOneFakeService{}
	outbox, err := NewPhaseOneDraftOutbox(service, store, statePath, []CaptureMediaHandle{draft})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := outbox.Send(context.Background(), draft.ID, PhaseOneThisPulsar, PhaseOneOverlay, PhaseOneMicrophone)
	if err != nil || accepted.State != PhaseOneDraftAccepted || accepted.LocalBytesRetained {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.uploadKeys) != 0 || len(service.transmissionKeys) != 1 {
		t.Fatalf("uploads=%v transmissions=%v", service.uploadKeys, service.transmissionKeys)
	}
}

func TestPhaseOneOutboxRequiresExplicitInMemoryFallbackConfirmation(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "capture-media"))
	store.newID = func() (string, error) { return strings.Repeat("5", 32), nil }
	draft, err := store.ImportUserDraft(bytes.NewReader(phaseOneCueBytes(t)))
	if err != nil {
		t.Fatal(err)
	}
	service := &phaseOneFakeService{requireConfirmation: true}
	outbox, err := NewPhaseOneDraftOutbox(service, store, filepath.Join(root, "outbox.json"), []CaptureMediaHandle{draft})
	if err != nil {
		t.Fatal(err)
	}
	challenged, err := outbox.Send(context.Background(), draft.ID, PhaseOneOwnBarycenter, PhaseOneInterrupt, PhaseOneMicrophone)
	if err == nil || challenged.FailureCode != "requires_confirmation" {
		t.Fatalf("challenged=%+v err=%v", challenged, err)
	}
	snapshots := outbox.Snapshots()
	if len(snapshots) != 1 || !snapshots[0].FallbackConfirmationAvailable {
		t.Fatalf("fallback was not explicitly exposed: %+v", snapshots)
	}
	accepted, err := outbox.ConfirmFallback(context.Background(), draft.ID, PhaseOneAfterCurrent, PhaseOneMicrophone)
	if err != nil || accepted.RequestedDelivery != PhaseOneInterrupt || accepted.EffectiveDelivery != PhaseOneAfterCurrent {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.confirmations != 1 || len(service.uploadKeys) != 1 || len(service.transmissionKeys) != 2 || service.transmissionKeys[0] != service.transmissionKeys[1] {
		t.Fatalf("confirmations=%d uploads=%v transmissions=%v", service.confirmations, service.uploadKeys, service.transmissionKeys)
	}
}
