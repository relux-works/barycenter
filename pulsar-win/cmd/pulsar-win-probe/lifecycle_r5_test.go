package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestR5F12ConfirmedShutdownDoesNotWaitForPrepareAndHandsOffLateOwner(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin capture: %s", reason)
	}
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}

	prepareEntered := make(chan struct{})
	prepareRelease := make(chan struct{})
	prepareResult := make(chan capturePrepareCoordinatorResult, 1)
	var stopCalls, wakeCalls atomic.Int32
	var stoppedOperation atomic.Uint32
	go func() {
		prepareResult <- runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			close(prepareEntered)
			<-prepareRelease
			return 5101, true
		}, func(operationID uint32) winprobe.HResult {
			stoppedOperation.Store(operationID)
			stopCalls.Add(1)
			return 0
		}, func(operationID uint32) winprobe.HResult {
			stoppedOperation.Store(operationID)
			stopCalls.Add(1)
			return 0
		})
	}()
	<-prepareEntered

	confirmed := make(chan bool, 1)
	go func() {
		confirmed <- adapter.confirm(func(operationID uint32) winprobe.HResult {
			stoppedOperation.Store(operationID)
			stopCalls.Add(1)
			return 0
		}, func() { wakeCalls.Add(1) })
	}()
	select {
	case accepted := <-confirmed:
		if !accepted {
			t.Fatal("first confirmation was rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("WM_ENDSESSION confirmation waited for the blocked prepare callback")
	}
	if wakeCalls.Load() != 1 || stopCalls.Load() != 0 {
		t.Fatalf("pre-release effects wake=%d stop=%d, want wake=1 stop=0", wakeCalls.Load(), stopCalls.Load())
	}

	close(prepareRelease)
	prepared := <-prepareResult
	if !prepared.trackerInvoked || !prepared.externalInvoked || !prepared.succeeded || prepared.trackerAllowed || !prepared.owner.matches(generation, 5101) || prepared.conflictingOwner != nil || prepared.ownerPublished || prepared.resultEvidenceAllowed || prepared.ownerSuccessorAllowed {
		t.Fatalf("late prepare result = %+v", prepared)
	}
	operationID, phase, exists := tracker.captureStateForGeneration(generation)
	if !exists || operationID != 0 || phase != captureGenerationPrepareInFlight {
		t.Fatalf("late prepare lifecycle successor exists=%v operation=%d phase=%d", exists, operationID, phase)
	}
	if stopCalls.Load() != 0 || stoppedOperation.Load() != 0 {
		t.Fatalf("late native owner started post-latch Stop calls=%d operation=%d, want 0/0", stopCalls.Load(), stoppedOperation.Load())
	}
	if owners.current() != nil {
		t.Fatal("late native owner remained published after confirmed shutdown")
	}
	if shutdown.confirmedCapture.Load() != nil {
		t.Fatal("confirmation fabricated a capture owner before prepare returned")
	}
	successors := newR5SuccessorCounters()
	if prepared.dispatchResultEvidence(&shutdown, successors.callbacks()...) || prepared.dispatchOwnerSuccessor(&shutdown, successors.callbacks()...) {
		t.Fatal("late prepare admitted production result/owner successor callbacks")
	}
	successors.assertZero(t)
}

func TestR5F12ConfirmedShutdownDoesNotWaitForActivationAndSuppressesSuccessor(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 5202)
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}

	activationEntered := make(chan struct{})
	activationRelease := make(chan struct{})
	activationResult := make(chan captureActivationCoordinatorResult, 1)
	var stopCalls, wakeCalls atomic.Int32
	var mu sync.Mutex
	var order []string
	record := func(value string) {
		mu.Lock()
		order = append(order, value)
		mu.Unlock()
	}
	go func() {
		activationResult <- runCaptureActivationOwned(tracker, &owners, &shutdown, generation, 5202, func() {
			close(activationEntered)
			<-activationRelease
		}, func(operationID uint32) winprobe.HResult {
			if operationID != 5202 {
				t.Errorf("post-activation shutdown stop operation=%d, want 5202", operationID)
			}
			record("stop")
			stopCalls.Add(1)
			return 0
		})
	}()
	<-activationEntered

	confirmed := make(chan bool, 1)
	go func() {
		confirmed <- adapter.confirm(func(operationID uint32) winprobe.HResult {
			if operationID != 5202 {
				t.Errorf("confirmed shutdown stopped operation=%d, want 5202", operationID)
			}
			record("stop")
			stopCalls.Add(1)
			return 0
		}, func() {
			record("wake")
			wakeCalls.Add(1)
		})
	}()
	select {
	case accepted := <-confirmed:
		if !accepted {
			t.Fatal("first confirmation was rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("WM_ENDSESSION confirmation waited for the blocked activation callback")
	}
	mu.Lock()
	gotOrder := append([]string(nil), order...)
	mu.Unlock()
	if len(gotOrder) != 1 || gotOrder[0] != "wake" {
		t.Fatalf("confirmation order=%v, want nonblocking [wake] while activation owns the native call", gotOrder)
	}
	if stopCalls.Load() != 0 || wakeCalls.Load() != 1 {
		t.Fatalf("confirmation effects stop=%d wake=%d, want deferred 0/1", stopCalls.Load(), wakeCalls.Load())
	}

	close(activationRelease)
	activated := <-activationResult
	if !activated.trackerInvoked || !activated.externalInvoked || activated.continuationAllowed {
		t.Fatalf("activation result after confirmation = %+v", activated)
	}
	if stopCalls.Load() != 0 {
		t.Fatalf("activation return issued a post-latch deferred shutdown stop: %d", stopCalls.Load())
	}
	mu.Lock()
	gotOrder = append([]string(nil), order...)
	mu.Unlock()
	if len(gotOrder) != 1 || gotOrder[0] != "wake" {
		t.Fatalf("deferred shutdown order=%v, want OS handoff [wake]", gotOrder)
	}
	successors := newR5SuccessorCounters()
	if activated.dispatchContinuation(&shutdown, successors.callbacks()...) {
		t.Fatal("in-flight activation admitted production successor callbacks")
	}
	successors.assertZero(t)
}

func TestR5F13WaiterStaysAliveBetweenStopAndConfirmedLatch(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 5252)
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}

	stopEntered := make(chan struct{})
	stopRelease := make(chan struct{})
	confirmation := make(chan bool, 1)
	var wakeCalls atomic.Int32
	go func() {
		confirmation <- adapter.confirm(func(operationID uint32) winprobe.HResult {
			if operationID != 5252 {
				t.Errorf("stop operation=%d, want 5252", operationID)
			}
			close(stopEntered)
			<-stopRelease
			return 0
		}, func() { wakeCalls.Add(1) })
	}()
	<-stopEntered
	if !shutdown.isClosing() || shutdown.isConfirmed() {
		t.Fatalf("stop interval closing=%v confirmed=%v, want true/false", shutdown.isClosing(), shutdown.isConfirmed())
	}
	var ordinaryCalls, abruptCalls atomic.Int32
	if shutdown.runWaiterIteration([]func(){func() { ordinaryCalls.Add(1) }}, func() { abruptCalls.Add(1) }) {
		t.Fatal("waiter exited before the confirmed latch became visible")
	}
	if ordinaryCalls.Load() != 0 || abruptCalls.Load() != 0 {
		t.Fatalf("stop interval work ordinary=%d abrupt=%d, want 0/0", ordinaryCalls.Load(), abruptCalls.Load())
	}
	gate := confirmedShutdownMessageGate{shutdown: &shutdown}
	if result, suppressed := gate.enter(lifecycleWMQueryEndSession, func() { ordinaryCalls.Add(1) }); !suppressed || result != 1 {
		t.Fatalf("closing message gate query result=%d suppressed=%v, want 1/true", result, suppressed)
	}
	if result, suppressed := gate.enter(0x0111, func() { ordinaryCalls.Add(1) }); !suppressed || result != 0 {
		t.Fatalf("closing message gate command result=%d suppressed=%v, want 0/true", result, suppressed)
	}
	if ordinaryCalls.Load() != 0 {
		t.Fatal("wndproc application callback entered during stop-to-latch interval")
	}

	close(stopRelease)
	if accepted := <-confirmation; !accepted {
		t.Fatal("first confirmation was rejected")
	}
	if wakeCalls.Load() != 1 || !shutdown.isConfirmed() {
		t.Fatalf("post-stop latch confirmed=%v wake=%d", shutdown.isConfirmed(), wakeCalls.Load())
	}
	if !shutdown.runWaiterIteration([]func(){func() { ordinaryCalls.Add(1) }}, func() { abruptCalls.Add(1) }) {
		t.Fatal("waiter did not exit after confirmed shutdown")
	}
	if ordinaryCalls.Load() != 0 || abruptCalls.Load() != 1 {
		t.Fatalf("confirmed work ordinary=%d abrupt=%d, want 0/1", ordinaryCalls.Load(), abruptCalls.Load())
	}
	if !shutdown.runWaiterIteration(nil, func() { abruptCalls.Add(1) }) || abruptCalls.Load() != 1 {
		t.Fatalf("abrupt drain was not exactly once: %d", abruptCalls.Load())
	}
}

func TestR5F12ActivationWaitingForLifecycleGateCannotStartAfterConfirmation(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 5303)
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}

	gateEntered := make(chan struct{})
	gateRelease := make(chan struct{})
	gateDone := make(chan bool, 1)
	go func() {
		gateDone <- tracker.runGatedWork(func() {
			close(gateEntered)
			<-gateRelease
		})
	}()
	<-gateEntered
	var activationCalls atomic.Int32
	activationResult := make(chan captureActivationCoordinatorResult, 1)
	go func() {
		activationResult <- runCaptureActivationOwned(tracker, &owners, &shutdown, generation, 5303, func() {
			activationCalls.Add(1)
		}, func(uint32) winprobe.HResult { return 0 })
	}()
	deadline := time.Now().Add(time.Second)
	for owners.activationAttempts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if owners.activationAttempts.Load() == 0 {
		t.Fatal("activation did not reach the production lifecycle acquisition attempt")
	}

	var stopCalls, wakeCalls atomic.Int32
	if !adapter.confirm(func(operationID uint32) winprobe.HResult {
		if operationID != 5303 {
			t.Fatalf("stop operation=%d, want 5303", operationID)
		}
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("confirmation was rejected")
	}
	if stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("confirmation effects stop=%d wake=%d", stopCalls.Load(), wakeCalls.Load())
	}
	close(gateRelease)
	if !<-gateDone {
		t.Fatal("pre-confirmation lifecycle callback did not finish")
	}
	activated := <-activationResult
	if activated.externalInvoked || activated.continuationAllowed || activationCalls.Load() != 0 {
		t.Fatalf("post-confirmation activation started: result=%+v calls=%d", activated, activationCalls.Load())
	}
}

func TestR5F12ExactOwnerSnapshotIgnoresStaleClearAndStopsOnce(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 5404)
	if owners.clear(generation+1, 5404) || owners.clear(generation, 5405) {
		t.Fatal("stale generation or operation cleared the active owner")
	}
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}
	var stopCalls, wakeCalls atomic.Int32
	var stoppedOperation atomic.Uint32
	if !adapter.confirm(func(operationID uint32) winprobe.HResult {
		stoppedOperation.Store(operationID)
		stopCalls.Add(1)
		return 0
	}, func() {
		if stopCalls.Load() != 1 {
			t.Error("wake ran before the exact owner stop")
		}
		wakeCalls.Add(1)
	}) {
		t.Fatal("confirmation was rejected")
	}
	owner := shutdown.confirmedCapture.Load()
	if !owner.matches(generation, 5404) || stoppedOperation.Load() != 5404 || stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("exact owner binding owner=%+v stopped=%d stopCalls=%d wakeCalls=%d", owner, stoppedOperation.Load(), stopCalls.Load(), wakeCalls.Load())
	}
	if adapter.confirm(func(uint32) winprobe.HResult {
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("repeated confirmation was accepted")
	}
	if owner.requestShutdownStop(func(uint32) winprobe.HResult {
		stopCalls.Add(1)
		return 0
	}) {
		t.Fatal("exact owner accepted a duplicate shutdown stop")
	}
	if stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("idempotence effects stop=%d wake=%d", stopCalls.Load(), wakeCalls.Load())
	}
}

func TestR5F14ProductionSuccessorDispatcherRunsOnlyWhileGateIsOpen(t *testing.T) {
	t.Parallel()
	var shutdown abruptShutdownCoordinator
	result := captureActivationCoordinatorResult{trackerInvoked: true, externalInvoked: true, continuationAllowed: true}
	successors := newR5SuccessorCounters()
	if !result.dispatchContinuation(&shutdown, successors.callbacks()...) {
		t.Fatal("open production continuation gate rejected successor callbacks")
	}
	successors.assertEach(t, 1)
	if !shutdown.confirmAfterStop(nil, nil) {
		t.Fatal("failed to close continuation gate")
	}
	if result.dispatchContinuation(&shutdown, successors.callbacks()...) {
		t.Fatal("closed production continuation gate admitted successor callbacks")
	}
	successors.assertEach(t, 1)
}

type r5SuccessorCounters struct {
	activate atomic.Int32
	evidence atomic.Int32
	post     atomic.Int32
	release  atomic.Int32
	finalize atomic.Int32
}

func newR5SuccessorCounters() *r5SuccessorCounters {
	return &r5SuccessorCounters{}
}

func (c *r5SuccessorCounters) callbacks() []func() {
	return []func(){
		func() { c.activate.Add(1) },
		func() { c.evidence.Add(1) },
		func() { c.post.Add(1) },
		func() { c.release.Add(1) },
		func() { c.finalize.Add(1) },
	}
}

func (c *r5SuccessorCounters) assertZero(t *testing.T) {
	t.Helper()
	c.assertEach(t, 0)
}

func (c *r5SuccessorCounters) assertEach(t *testing.T, want int32) {
	t.Helper()
	if c.activate.Load() != want || c.evidence.Load() != want || c.post.Load() != want || c.release.Load() != want || c.finalize.Load() != want {
		t.Fatalf("successor callbacks activate=%d evidence=%d post=%d release=%d finalize=%d, want each %d", c.activate.Load(), c.evidence.Load(), c.post.Load(), c.release.Load(), c.finalize.Load(), want)
	}
}
