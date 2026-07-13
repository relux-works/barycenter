package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func newWindowsOnboardingTestService(t testing.TB, repository *ProtectedCredentialRepository, doer HTTPDoer) *WindowsOnboardingService {
	t.Helper()
	client, err := NewOnboardingClient("https://coord.example", doer)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := NewRecoveryService(repository, client, &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewWindowsOnboardingService(client, repository, recovery)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestOnboardingJoinRetainsConsumedResultWhenProtectedSaveFails(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	files.failAt["write"] = 1
	service := newWindowsOnboardingTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, testJoinResponse()), nil
	}))
	result, err := service.Join(context.Background(), "ABCDEFGHJKMNPQRSTVWXYZ23456")
	if err == nil || result.Bundle.Node == nil || result.Bundle.Control == nil || result.OrbitID != 7 {
		t.Fatalf("consumed result=%#v err=%v", result, err)
	}
	if bundle, loadErr := repository.LoadBundle(); loadErr != nil || bundle != nil {
		t.Fatalf("failed storage claimed persisted bundle=%#v err=%v", bundle, loadErr)
	}
}

func TestOnboardingRotateUsesMetadataWithoutRevealingSecret(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	initial := sampleBundle()
	initial.RecoveryBackupAcknowledged = true
	if err := repository.SaveBundle(initial); err != nil {
		t.Fatal(err)
	}
	newID := "rec_abcdefabcdefabcdefabcdefabcdefab"
	service := newWindowsOnboardingTestService(t, repository, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/recovery/rotate" {
			return nil, errors.New("unexpected endpoint")
		}
		return jsonResponse(http.StatusOK, []byte(`{"actor_id":9,"recovery_id":"rec_abcdefabcdefabcdefabcdefabcdefab","recovery_secret":"ABCDEFGHJKMNPQRSTVWXYZ23456","shown_once":true}`)), nil
	}))
	material, err := service.RotateRecovery(context.Background(), activeControlCapability(t))
	if err != nil {
		t.Fatal(err)
	}
	material.mu.Lock()
	revealed := material.revealedForDisplay
	material.mu.Unlock()
	if revealed {
		t.Fatal("metadata persistence materialized the recovery secret string")
	}
	loaded, err := repository.LoadBundle()
	if err != nil || loaded.RecoveryID != newID || loaded.RecoveryBackupAcknowledged {
		t.Fatalf("rotated metadata=%#v err=%v", loaded, err)
	}
	if _, recoveryID, secret, ok := material.RevealForDisplay(); !ok || recoveryID != newID || secret != "ABCDEFGHJKMNPQRSTVWXYZ23456" {
		t.Fatal("explicit reveal was not available after metadata persistence")
	}
}

func TestOnboardingServiceRejectsIncoherentCompositionWithoutIO(t *testing.T) {
	repositoryA, filesA := newTestCredentialRepository(t)
	repositoryB, filesB := newTestCredentialRepository(t)
	var network atomic.Int32
	doer := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		network.Add(1)
		return nil, errors.New("must not send")
	})
	clientA, _ := NewOnboardingClient("https://coord.example", doer)
	clientB, _ := NewOnboardingClient("https://other.example", doer)
	recoveryA, _ := NewRecoveryService(repositoryA, clientA, nil)
	beforeA, beforeB := len(filesA.calls), len(filesB.calls)
	if _, err := NewWindowsOnboardingService(clientA, repositoryB, recoveryA); !errors.Is(err, errOnboardingServiceIncoherent) {
		t.Fatalf("mismatched repository error=%v", err)
	}
	if _, err := NewWindowsOnboardingService(clientB, repositoryA, recoveryA); !errors.Is(err, errOnboardingServiceIncoherent) {
		t.Fatalf("mismatched client/origin error=%v", err)
	}
	clientSameOrigin, _ := NewOnboardingClient("https://coord.example", doer)
	if _, err := NewWindowsOnboardingService(clientSameOrigin, repositoryA, recoveryA); !errors.Is(err, errOnboardingServiceIncoherent) {
		t.Fatalf("different client pointer error=%v", err)
	}
	if network.Load() != 0 || len(filesA.calls) != beforeA || len(filesB.calls) != beforeB {
		t.Fatal("composition validation performed I/O")
	}
}
