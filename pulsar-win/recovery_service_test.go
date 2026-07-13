package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type fixedTokenGenerator struct {
	token string
	calls atomic.Int32
}

func (g *fixedTokenGenerator) Generate() (string, error) {
	g.calls.Add(1)
	return g.token, nil
}

func newRecoveryTestService(t *testing.T, repository *ProtectedCredentialRepository, doer HTTPDoer, generator ControlTokenGenerator) *RecoveryService {
	t.Helper()
	client, err := NewOnboardingClient("https://coord.example", doer)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRecoveryService(repository, client, generator)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func recoveryInput(t *testing.T, actorID int64, recoveryID string) RecoveryInput {
	t.Helper()
	input, err := NewRecoveryInput(actorID, recoveryID, "ABCDEFGH-JKMNP-QRSTV-WXYZ2-3456")
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func apiErrorResponse(status int, code, message string, retry int) *http.Response {
	retryValue := any(nil)
	if retry > 0 {
		retryValue = retry
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message, "retry_after_seconds": retryValue}})
	response := jsonResponse(status, body)
	if retry > 0 {
		response.Header.Set("Retry-After", fmt.Sprint(retry))
	}
	return response
}

func TestRecoverySendBarrierPromotesControlOnlyAndPreservesNode(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	initial := sampleBundle()
	initial.RecoveryBackupAcknowledged = true
	if err := repository.SaveBundle(initial); err != nil {
		t.Fatal(err)
	}
	replacement := strings.Repeat("ef", 32)
	generator := &fixedTokenGenerator{token: replacement}
	var sends atomic.Int32
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/recovery/consume" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		sends.Add(1)
		pending, err := repository.LoadPending(origin, 9)
		if err != nil || pending == nil || !pending.EverSent || pending.PendingControlToken != replacement {
			t.Fatalf("send crossed before durable sent record: %#v %v", pending, err)
		}
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
	}), generator)
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, initial.RecoveryID))
	if err != nil {
		t.Fatal(err)
	}
	if sends.Load() != 1 || result.State != RecoveryStatePromoted || result.Bundle == nil {
		t.Fatalf("result %#v sends=%d", result, sends.Load())
	}
	if *result.Bundle.Node != *initial.Node {
		t.Fatal("recovery modified node capability")
	}
	if result.Bundle.Control.ControlToken != replacement || !result.Bundle.RecoveryConsumed || result.Bundle.RecoveryBackupAcknowledged {
		t.Fatal("replacement control token was not promoted")
	}
	if pending, _ := repository.LoadPending(service.client.Origin(), 9); pending != nil {
		t.Fatal("pending record survived verified promotion")
	}
	secret := []byte("ABCDEFGHJKMNPQRSTVWXYZ23456")
	for path, value := range files.data {
		if bytes.Contains(value, secret) {
			t.Fatalf("recovery secret persisted at %s", filepath.Base(path))
		}
	}
}

func TestRecoveryCleanInstallPromotesControlOnly(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"companion"}`)), nil
	}), &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, sampleBundle().RecoveryID))
	if err != nil || result.Bundle == nil || result.Bundle.Node != nil || result.Bundle.Control == nil || result.Bundle.Control.Context != ControlContextActive {
		t.Fatalf("control-only recovery=%#v err=%v", result, err)
	}
	if paired, err := LoadCredentialsFromRepository(repository); err != nil || paired != nil {
		t.Fatalf("control-only recovery masqueraded as paired: %#v err=%v", paired, err)
	}
}

func TestRecoveryReadCloseFailureBlocksSend(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	// Reset counts so the sixth close is mark-sent read-back close:
	// active probe, replace(temp/readback), mark load/temp/readback.
	files.counts = map[string]int{}
	files.failAt["close"] = 6
	var sends atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
	}), &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	if _, err := service.Recover(context.Background(), recoveryInput(t, 9, sampleBundle().RecoveryID)); err == nil {
		t.Fatal("read-handle close failure claimed success")
	}
	if sends.Load() != 0 {
		t.Fatal("network send crossed failed read-handle close")
	}
}

func TestRecoveryPromotionBeforeDeleteConvergesWithoutDuplicateRequest(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	files.counts = map[string]int{}
	files.failAt["delete"] = 1
	var sends atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		sends.Add(1)
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
	}), &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, sampleBundle().RecoveryID))
	if err == nil || result.Bundle == nil {
		t.Fatalf("expected delete failure after promotion, result=%#v err=%v", result, err)
	}
	files.failAt["delete"] = 0
	resumed, err := service.Resume(context.Background(), 9)
	if err != nil || resumed.State != RecoveryStatePromoted {
		t.Fatalf("resume %#v err=%v", resumed, err)
	}
	if sends.Load() != 1 {
		t.Fatalf("resume duplicated recovery request: %d", sends.Load())
	}
}

func TestRecoveryProbeResponseTable(t *testing.T) {
	tests := []struct {
		name        string
		response    func() (*http.Response, error)
		wantState   RecoveryState
		wantPending bool
		wantRetry   int
	}{
		{"200", func() (*http.Response, error) {
			return jsonResponse(200, []byte(`{"orbit_id":7,"actor_id":9,"role":"companion"}`)), nil
		}, RecoveryStatePromoted, false, 0},
		{"403 limited", func() (*http.Response, error) {
			return apiErrorResponse(403, "insufficient_capability", "This token does not have the required capability.", 0), nil
		}, RecoveryStateLimited, false, 0},
		{"401", func() (*http.Response, error) {
			return apiErrorResponse(401, "unauthorized", "Authentication is required.", 0), nil
		}, RecoveryStateNeedsSecret, true, 0},
		{"429", func() (*http.Response, error) {
			return apiErrorResponse(429, "too_many_attempts", "Too many attempts. Please wait before retrying.", 11), nil
		}, RecoveryStatePending, true, 11},
		{"500", func() (*http.Response, error) {
			return apiErrorResponse(500, "internal_error", "An internal error occurred.", 0), nil
		}, RecoveryStatePending, true, 0},
		{"network", func() (*http.Response, error) { return nil, errors.New("network secret canary") }, RecoveryStatePending, true, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, _ := newTestCredentialRepository(t)
			if err := repository.SaveBundle(sampleBundle()); err != nil {
				t.Fatal(err)
			}
			origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
			unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: 9, RecoveryID: sampleBundle().RecoveryID, PendingControlToken: strings.Repeat("ef", 32)}
			if err := repository.CreatePendingUnsent(unsent); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.MarkPendingSent(unsent); err != nil {
				t.Fatal(err)
			}
			service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) { return test.response() }), nil)
			result, _ := service.Resume(context.Background(), 9)
			if result.State != test.wantState || result.RetryAfterSeconds != test.wantRetry {
				t.Fatalf("result %#v", result)
			}
			pending, err := repository.LoadPending(origin, 9)
			if err != nil || (pending != nil) != test.wantPending {
				t.Fatalf("pending=%#v err=%v", pending, err)
			}
			if test.wantState == RecoveryStateLimited && (result.Bundle == nil || result.Bundle.Control.Context != ControlContextLimited || result.Bundle.Control.ControlToken != strings.Repeat("ef", 32) || result.Bundle.Control.LastKnownOrbitID != 7) {
				t.Fatal("limited probe did not promote valid token")
			}
		})
	}
}

func TestRecoveryLimitedPromotionRemainsLimitedAcrossDeleteCrash(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: 9, RecoveryID: sampleBundle().RecoveryID, PendingControlToken: strings.Repeat("ef", 32)}
	if err := repository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.MarkPendingSent(unsent); err != nil {
		t.Fatal(err)
	}
	files.counts = map[string]int{}
	files.failAt["delete"] = 1
	var probes atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		probes.Add(1)
		return apiErrorResponse(403, "insufficient_capability", "This token does not have the required capability.", 0), nil
	}), nil)
	result, err := service.Resume(context.Background(), 9)
	if err == nil || result.Bundle == nil || result.Bundle.Control.Context != ControlContextLimited || result.Bundle.Control.OrbitID != 0 || result.Bundle.Control.Role != "" {
		t.Fatalf("limited promotion result=%#v err=%v", result, err)
	}
	files.failAt["delete"] = 0
	resumed, err := service.Resume(context.Background(), 9)
	if err != nil || resumed.State != RecoveryStateLimited || resumed.Bundle == nil || resumed.Bundle.Control.Context != ControlContextLimited {
		t.Fatalf("limited restart=%#v err=%v", resumed, err)
	}
	if probes.Load() != 1 {
		t.Fatalf("restart re-probed after durable promotion: %d", probes.Load())
	}
}

func TestRecoveryDeleteCrashPreservesLaterRotationAndAcknowledgement(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	unsent := PendingRecoveryRecord{
		CanonicalCoordinatorOrigin: origin.String(), ActorID: 9,
		RecoveryID: sampleBundle().RecoveryID, PendingControlToken: strings.Repeat("ef", 32),
	}
	if err := repository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	sent, err := repository.MarkPendingSent(unsent)
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := repository.PromotePending(sent, &ActorContext{OrbitID: 7, ActorID: 9, Role: "primary"})
	if err != nil {
		t.Fatal(err)
	}

	rotatedID := "rec_abcdefabcdefabcdefabcdefabcdefab"
	capability, _ := promoted.ControlCapability()
	if err := repository.UpdateRecoveryMetadata(capability, rotatedID); err != nil {
		t.Fatal(err)
	}
	if err := repository.AcknowledgeRecoveryBackup(origin, 9, rotatedID); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), files.data[repository.activePath()]...)
	var network atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		network.Add(1)
		return nil, errors.New("must not probe an already promoted token")
	}), nil)
	result, err := service.Resume(context.Background(), 9)
	if err != nil || result.Bundle == nil || result.Bundle.RecoveryID != rotatedID ||
		!result.Bundle.RecoveryBackupAcknowledged || result.Bundle.RecoveryConsumed {
		t.Fatalf("rotated convergence result=%#v err=%v", result, err)
	}
	if network.Load() != 0 {
		t.Fatalf("already promoted recovery used network %d times", network.Load())
	}
	if !bytes.Equal(before, files.data[repository.activePath()]) {
		t.Fatal("stale pending convergence rewrote the later recovery generation")
	}
	pending, err := repository.LoadPending(origin, 9)
	if err != nil || pending != nil {
		t.Fatalf("exact stale pending was not deleted: %#v err=%v", pending, err)
	}
}

func TestRecoveryExactPromotionIdentityFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CredentialBundle)
	}{
		{"wrong origin", func(bundle *CredentialBundle) {
			bundle.CoordinatorOrigin = "https://other.example"
			bundle.Node.WSURL = "wss://other.example/ws"
		}},
		{"wrong recovery", func(bundle *CredentialBundle) { bundle.RecoveryID = "rec_abcdefabcdefabcdefabcdefabcdefab" }},
		{"wrong actor", func(bundle *CredentialBundle) { bundle.Control.ActorID = 10 }},
		{"not consumed", func(bundle *CredentialBundle) { bundle.RecoveryConsumed = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, files := newTestCredentialRepository(t)
			active := sampleBundle()
			active.Control.ControlToken = strings.Repeat("ef", 32)
			active.RecoveryConsumed = true
			test.mutate(&active)
			if err := repository.SaveBundle(active); err != nil {
				t.Fatal(err)
			}
			origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
			unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: 9, RecoveryID: sampleBundle().RecoveryID, PendingControlToken: strings.Repeat("ef", 32)}
			if err := repository.CreatePendingUnsent(unsent); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.MarkPendingSent(unsent); err != nil {
				t.Fatal(err)
			}
			files.counts = map[string]int{}
			var network atomic.Int32
			service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
				network.Add(1)
				return nil, errors.New("must not send")
			}), nil)
			result, err := service.Resume(context.Background(), 9)
			if !errors.Is(err, errCredentialStorageConflict) || result.State != RecoveryStatePending || network.Load() != 0 {
				t.Fatalf("result=%#v err=%v network=%d", result, err, network.Load())
			}
			pending, loadErr := repository.LoadPending(origin, 9)
			if loadErr != nil || pending == nil || !pending.EverSent {
				t.Fatalf("pending changed: %#v err=%v", pending, loadErr)
			}
			if files.counts["delete"] != 0 {
				t.Fatalf("mismatched identity deleted state: %d", files.counts["delete"])
			}
		})
	}
}

func TestRecoveryProbe401Then403RetainsAndOffersAbandon(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: 9, RecoveryID: sampleBundle().RecoveryID, PendingControlToken: strings.Repeat("ef", 32)}
	if err := repository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.MarkPendingSent(unsent); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return apiErrorResponse(401, "unauthorized", "Authentication is required.", 0), nil
		}
		return apiErrorResponse(403, "credential_invalid", "The provided credential is not valid.", 0), nil
	}), nil)
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, sampleBundle().RecoveryID))
	if result.State != RecoveryStateCanAbandon || err == nil {
		t.Fatalf("result %#v err=%v", result, err)
	}
	if pending, _ := repository.LoadPending(origin, 9); pending == nil || !pending.EverSent {
		t.Fatal("401->403 deleted ambiguous sent candidate")
	}
	if err := service.Abandon(context.Background(), 9, false); err == nil {
		t.Fatal("abandon without explicit confirmation succeeded")
	}
	if err := service.Abandon(context.Background(), 9, true); err != nil {
		t.Fatal(err)
	}
	if pending, _ := repository.LoadPending(origin, 9); pending != nil {
		t.Fatal("confirmed abandon did not delete exact candidate")
	}
}

func TestRecoveryDifferentGenerationCannotBypassSentCandidate(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: 9, RecoveryID: sampleBundle().RecoveryID, PendingControlToken: strings.Repeat("ef", 32)}
	if err := repository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.MarkPendingSent(unsent); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodGet {
			t.Fatal("different generation sent recovery request")
		}
		return apiErrorResponse(401, "unauthorized", "Authentication is required.", 0), nil
	}), &fixedTokenGenerator{token: strings.Repeat("aa", 32)})
	newID := "rec_abcdefabcdefabcdefabcdefabcdefab"
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, newID))
	if err != nil || result.State != RecoveryStateNeedsSecret || calls.Load() != 1 {
		t.Fatalf("result %#v err=%v calls=%d", result, err, calls.Load())
	}
}

func TestRecoveryRestartReusesUnsentCandidateBeforeGeneration(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	unsent := PendingRecoveryRecord{
		CanonicalCoordinatorOrigin: origin.String(),
		ActorID:                    9,
		RecoveryID:                 sampleBundle().RecoveryID,
		PendingControlToken:        strings.Repeat("ef", 32),
	}
	if err := repository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	generator := &fixedTokenGenerator{token: strings.Repeat("aa", 32)}
	var sends atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		sends.Add(1)
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["replacement_control_token"] != unsent.PendingControlToken {
			t.Fatalf("restart replaced candidate: %#v", body)
		}
		return jsonResponse(200, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
	}), generator)
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, unsent.RecoveryID))
	if err != nil || result.State != RecoveryStatePromoted {
		t.Fatalf("result %#v err=%v", result, err)
	}
	if generator.calls.Load() != 0 || sends.Load() != 1 {
		t.Fatalf("generator calls=%d sends=%d", generator.calls.Load(), sends.Load())
	}

	// A different generation cannot overwrite even an unsent crash record.
	otherRepository, _ := newTestCredentialRepository(t)
	if err := otherRepository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	if err := otherRepository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	otherGenerator := &fixedTokenGenerator{token: strings.Repeat("aa", 32)}
	otherService := newRecoveryTestService(t, otherRepository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("different generation crossed send")
		return nil, nil
	}), otherGenerator)
	otherID := "rec_abcdefabcdefabcdefabcdefabcdefab"
	blocked, err := otherService.Recover(context.Background(), recoveryInput(t, 9, otherID))
	if err != nil || blocked.State != RecoveryStateUnsent || otherGenerator.calls.Load() != 0 {
		t.Fatalf("blocked=%#v err=%v generator calls=%d", blocked, err, otherGenerator.calls.Load())
	}
	pending, err := otherRepository.LoadPending(origin, 9)
	if err != nil || pending == nil || *pending != unsent {
		t.Fatalf("unresolved candidate changed: %#v err=%v", pending, err)
	}
}

func TestRecoveryUnsentCandidateMatchingActiveFailsClosed(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	active := sampleBundle()
	active.Control.ControlToken = strings.Repeat("ef", 32)
	if err := repository.SaveBundle(active); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: 9, RecoveryID: active.RecoveryID, PendingControlToken: active.Control.ControlToken}
	if err := repository.CreatePendingUnsent(unsent); err != nil {
		t.Fatal(err)
	}
	generator := &fixedTokenGenerator{token: strings.Repeat("aa", 32)}
	var network atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		network.Add(1)
		return nil, errors.New("must not send")
	}), generator)
	result, err := service.Recover(context.Background(), recoveryInput(t, 9, active.RecoveryID))
	if !errors.Is(err, errCredentialStorageConflict) || result.State != RecoveryStateUnsent || network.Load() != 0 || generator.calls.Load() != 0 {
		t.Fatalf("result=%#v err=%v network=%d generator=%d", result, err, network.Load(), generator.calls.Load())
	}
	pending, loadErr := repository.LoadPending(origin, 9)
	if loadErr != nil || pending == nil || *pending != unsent {
		t.Fatalf("impossible unsent state changed: %#v err=%v", pending, loadErr)
	}
}

func TestRecoveryConcurrentServicesCrossOneCandidate(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	initial := sampleBundle()
	if err := repository.SaveBundle(initial); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	releaseSend := make(chan struct{})
	var sends atomic.Int32
	doer := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/v1/recovery/consume" {
			if sends.Add(1) == 1 {
				close(entered)
				<-releaseSend
			}
			return jsonResponse(200, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
		}
		return nil, errors.New("unexpected")
	})
	client, _ := NewOnboardingClient("https://coord.example", doer)
	serviceA, _ := NewRecoveryService(repository, client, &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	serviceB, _ := NewRecoveryService(repository, client, &fixedTokenGenerator{token: strings.Repeat("aa", 32)})
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	inputA := recoveryInput(t, 9, initial.RecoveryID)
	inputB := recoveryInput(t, 9, initial.RecoveryID)
	go func() { defer wg.Done(); _, err := serviceA.Recover(context.Background(), inputA); errs <- err }()
	<-entered
	go func() { defer wg.Done(); _, err := serviceB.Recover(context.Background(), inputB); errs <- err }()
	close(releaseSend)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if sends.Load() != 1 {
		t.Fatalf("%d candidates crossed send", sends.Load())
	}
}

func TestRecoveryCancellationBeforeGateDeletesOnlyUnsent(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	var sends atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		return nil, errors.New("unexpected")
	}), &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Recover(ctx, recoveryInput(t, 9, sampleBundle().RecoveryID)); err == nil {
		t.Fatal("cancelled recovery returned success")
	}
	if sends.Load() != 0 {
		t.Fatal("cancelled recovery sent")
	}
	if pending, _ := repository.LoadPending(service.client.Origin(), 9); pending != nil {
		t.Fatal("exact unsent candidate survived pre-gate cancellation")
	}
}

func TestRecoveryInputCopiesHaveSingleRaceSafeConsumer(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var sends atomic.Int32
	service := newRecoveryTestService(t, repository, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		if sends.Add(1) == 1 {
			close(entered)
			<-release
		}
		return jsonResponse(200, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
	}), &fixedTokenGenerator{token: strings.Repeat("ef", 32)})
	input := recoveryInput(t, 9, sampleBundle().RecoveryID)
	copyOfInput := input
	results := make(chan error, 2)
	go func() { _, err := service.Recover(context.Background(), input); results <- err }()
	<-entered
	go func() { _, err := service.Recover(context.Background(), copyOfInput); results <- err }()
	second := <-results
	if !errors.Is(second, errInvalidRequest) {
		t.Fatalf("shared input second consumer error=%v", second)
	}
	close(release)
	if first := <-results; first != nil {
		t.Fatalf("exclusive consumer failed: %v", first)
	}
	if sends.Load() != 1 {
		t.Fatalf("shared input sent %d requests", sends.Load())
	}
}
