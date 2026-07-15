package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type identityCompositionService struct {
	mu           sync.Mutex
	attempts     []string
	createFails  int
	joinFails    int
	acknowledged bool
}

func (s *identityCompositionService) Create(_ context.Context, title, attempt string) (CreateOrbitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = append(s.attempts, attempt)
	if s.createFails > 0 {
		s.createFails--
		return CreateOrbitResult{}, &OnboardingClientError{Kind: ClientErrorTransport}
	}
	material, _ := newRecoveryMaterial(2, "rec_"+strings.Repeat("a", 32), strings.Repeat("A", 27))
	return CreateOrbitResult{Title: title, Recovery: material}, nil
}

func (s *identityCompositionService) Join(context.Context, string) (JoinOrbitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.joinFails > 0 {
		s.joinFails--
		return JoinOrbitResult{}, &OnboardingClientError{Kind: ClientErrorTransport}
	}
	return JoinOrbitResult{Title: "Joined"}, nil
}

func (s *identityCompositionService) AcknowledgeRecoveryBackup(actorID int64, recoveryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if actorID != 2 || recoveryID != "rec_"+strings.Repeat("a", 32) {
		return errors.New("wrong recovery metadata")
	}
	s.acknowledged = true
	return nil
}

type identityCompositionExporter struct {
	mu    sync.Mutex
	paths []string
}

func (e *identityCompositionExporter) SaveSelectedDestination(path string, _ *RecoveryMaterial) error {
	e.mu.Lock()
	e.paths = append(e.paths, path)
	e.mu.Unlock()
	return nil
}

func waitIdentityState(t *testing.T, composition *WindowsIdentityComposition, state ShellIdentityOperation) WindowsIdentitySnapshot {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := composition.Snapshot()
		if snapshot.Operation == state {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("identity state=%+v want=%s", composition.Snapshot(), state)
	return WindowsIdentitySnapshot{}
}

func TestWindowsIdentityCreateKeepsAttemptAndRequiresRecoveryExport(t *testing.T) {
	service := &identityCompositionService{createFails: 1}
	exporter := &identityCompositionExporter{}
	activated := make(chan struct{}, 1)
	composition, err := NewWindowsIdentityComposition(service, exporter, func() { activated <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	composition.Create("Home")
	waitIdentityState(t, composition, ShellIdentityFailed)
	composition.Create("Home")
	state := waitIdentityState(t, composition, ShellIdentityRecoveryRequired)
	if !state.RecoveryExportRequired {
		t.Fatal("Create activated without explicit recovery export")
	}
	select {
	case <-activated:
		t.Fatal("Create activated before recovery export")
	default:
	}
	service.mu.Lock()
	if len(service.attempts) != 2 || service.attempts[0] != service.attempts[1] || !installationAttemptPattern.MatchString(service.attempts[0]) {
		t.Fatalf("attempts=%v", service.attempts)
	}
	service.mu.Unlock()
	composition.SaveRecovery(`C:\Users\Ivan\pulsar-recovery.json`)
	waitIdentityState(t, composition, ShellIdentityActive)
	select {
	case <-activated:
	case <-time.After(time.Second):
		t.Fatal("recovery acknowledgement did not activate identity")
	}
	service.mu.Lock()
	acknowledged := service.acknowledged
	service.mu.Unlock()
	if !acknowledged {
		t.Fatal("export did not acknowledge exact recovery metadata")
	}
}

func TestWindowsIdentityJoinActivatesOnlyAfterServiceSuccess(t *testing.T) {
	service := &identityCompositionService{joinFails: 1}
	composition, err := NewWindowsIdentityComposition(service, &identityCompositionExporter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	composition.Join("ABCDEFGHJKMNPQRSTVWXYZ23456")
	waitIdentityState(t, composition, ShellIdentityFailed)
	composition.Join("ABCDEFGHJKMNPQRSTVWXYZ23456")
	state := waitIdentityState(t, composition, ShellIdentityActive)
	if state.RecoveryExportRequired {
		t.Fatal("Join incorrectly requested Create recovery export")
	}
}
