package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestR4F10ConfirmedShutdownStopLatchAndWakeOrder(t *testing.T) {
	t.Parallel()
	var coordinator abruptShutdownCoordinator
	var mu sync.Mutex
	var order []string
	record := func(value string) func() {
		return func() {
			mu.Lock()
			order = append(order, value)
			mu.Unlock()
		}
	}
	if !coordinator.confirmAfterStop(record("CaptureRequestStop(shutdown)"), func() {
		if !coordinator.isConfirmed() {
			t.Fatal("shutdown wake preceded the monotonic confirmation latch")
		}
		record("SetEvent(shutdownEvent)")()
	}) {
		t.Fatal("first confirmation was not accepted")
	}
	mu.Lock()
	want := []string{"CaptureRequestStop(shutdown)", "SetEvent(shutdownEvent)"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		mu.Unlock()
		t.Fatalf("shutdown order = %v, want %v", order, want)
	}
	mu.Unlock()
	if coordinator.confirmAfterStop(record("duplicate-stop"), record("duplicate-wake")) {
		t.Fatal("repeated confirmed shutdown was accepted twice")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != len(want) {
		t.Fatalf("repeated confirmation added effects: %v", order)
	}
}

func TestR4F10ConfirmedShutdownWinsEveryCoalescedWaitIndex(t *testing.T) {
	t.Parallel()
	ordinaryActions := []string{
		"terminal-intent",
		"command",
		"permission-cancel",
		"permission-query-release",
		"permission-result-take-release",
		"enumeration-query-take-release",
		"default-device-query-release",
		"capture-result-finalize-release",
		"artifact-abort",
		"picker-result-take-release",
		"cleanup-ready",
		"ui-transition",
		"evidence-failure-drain",
		"CapDestroy",
		"clean-evidence-sync",
	}
	for readyIndex := range 8 {
		readyIndex := readyIndex
		t.Run(fmt.Sprintf("wait-index-%d", readyIndex), func(t *testing.T) {
			var coordinator abruptShutdownCoordinator
			var ordinaryCalls atomic.Int32
			ordinary := make([]func(), 0, len(ordinaryActions))
			for _, action := range ordinaryActions {
				action := action
				ordinary = append(ordinary, func() {
					ordinaryCalls.Add(1)
					t.Errorf("ordinary action %q ran after confirmed shutdown at wait index %d", action, readyIndex)
				})
			}

			stopCalled := false
			wakeCalled := false
			coordinator.confirmAfterStop(func() { stopCalled = true }, func() {
				if !stopCalled {
					t.Fatal("shutdown wake preceded stop")
				}
				wakeCalled = true
			})
			partialRecoveryOwned := true
			var bufferedReads, bufferedWrites, waiterExits atomic.Int32
			exited := coordinator.runWaiterIteration(ordinary, func() {
				bufferedReads.Add(1)
				bufferedWrites.Add(1)
				waiterExits.Add(1)
			})
			if !exited || !stopCalled || !wakeCalled {
				t.Fatalf("shutdown dispatch exited=%v stop=%v wake=%v", exited, stopCalled, wakeCalled)
			}
			if ordinaryCalls.Load() != 0 {
				t.Fatalf("ordinary calls = %d, want 0", ordinaryCalls.Load())
			}
			if bufferedReads.Load() != 1 || bufferedWrites.Load() != 1 || waiterExits.Load() != 1 || !partialRecoveryOwned {
				t.Fatalf("abrupt handoff read=%d write=%d exits=%d partialOwned=%v", bufferedReads.Load(), bufferedWrites.Load(), waiterExits.Load(), partialRecoveryOwned)
			}
			if !coordinator.runWaiterIteration(ordinary, func() { t.Fatal("abrupt callback ran twice") }) {
				t.Fatal("confirmed coordinator stopped reporting waiter exit")
			}
		})
	}
}

func TestR4F10OrdinaryPermitLinearizesAcrossConfirmationWithoutBlockingWndProc(t *testing.T) {
	t.Parallel()
	var coordinator abruptShutdownCoordinator
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan bool, 1)
	go func() {
		firstDone <- coordinator.runOrdinary(func() {
			close(entered)
			<-release
		})
	}()
	<-entered

	confirmed := make(chan struct{})
	go func() {
		coordinator.confirmAfterStop(nil, nil)
		close(confirmed)
	}()
	select {
	case <-confirmed:
	case <-time.After(time.Second):
		t.Fatal("confirmation waited for a pre-confirmation ordinary permit")
	}
	var lateCalls atomic.Int32
	if coordinator.runOrdinary(func() { lateCalls.Add(1) }) {
		t.Fatal("ordinary permit was admitted after confirmation")
	}
	if lateCalls.Load() != 0 {
		t.Fatal("post-confirmation ordinary work ran")
	}
	close(release)
	if !<-firstDone {
		t.Fatal("pre-confirmation permit was not allowed to finish")
	}
}

func TestR4W1ProductionAdapterAllowsOnlyBoundedBufferedAppendAfterConfirmation(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 4242)
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}

	ordinaryEntered := make(chan struct{})
	ordinaryRelease := make(chan struct{})
	ordinaryDone := make(chan bool, 1)
	go func() {
		ordinaryDone <- shutdown.runOrdinary(func() {
			close(ordinaryEntered)
			<-ordinaryRelease
		})
	}()
	<-ordinaryEntered
	evidenceEntered := make(chan struct{})
	evidenceRelease := make(chan struct{})
	var physicalLogs, physicalSyncs atomic.Int32
	evidence := newEvidenceCoordinator(func(event winprobe.LogEvent) error {
		physicalLogs.Add(1)
		close(evidenceEntered)
		<-evidenceRelease
		return nil
	}, func() error {
		physicalSyncs.Add(1)
		return nil
	}, 2*time.Second)
	evidence.bindAdmissionGate(shutdown.runOrdinary)
	inFlightLog := make(chan bool, 1)
	go func() {
		inFlightLog <- evidence.log(winprobe.LogEvent{Action: "pre-confirmation-in-flight"})
	}()
	<-evidenceEntered
	queuedLog := make(chan bool, 1)
	go func() { queuedLog <- evidence.log(winprobe.LogEvent{Action: "queued-post-confirmation-log"}) }()
	queuedSync := make(chan bool, 1)
	go func() { queuedSync <- evidence.sync() }()
	waitForEvidenceQueueLength(t, evidence, 2)

	var stopCalls, wakeCalls atomic.Int32
	if !adapter.confirm(func(operationID uint32) winprobe.HResult {
		if operationID != 4242 {
			t.Fatalf("stop operation = %d, want 4242", operationID)
		}
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("production adapter rejected first confirmation")
	}
	if stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("confirmation waited or reordered effects: stop=%d wake=%d", stopCalls.Load(), wakeCalls.Load())
	}
	owner := shutdown.confirmedCapture.Load()
	if !owner.matches(generation, 4242) || !shutdown.isClosing() {
		t.Fatalf("confirmed shutdown binding owner=%+v closing=%v", owner, shutdown.isClosing())
	}

	var forbiddenCalls atomic.Int32
	ordinary := make([]func(), 0, 9)
	for _, action := range []string{"log", "sync", "release", "result-take", "finalize", "abort", "cleanup-ready", "ui-transition", "CapDestroy"} {
		action := action
		ordinary = append(ordinary, func() {
			forbiddenCalls.Add(1)
			t.Errorf("post-confirmation ordinary callback %q started", action)
		})
	}
	var readCalls, appendCalls atomic.Int32
	var drain confirmedShutdownDrainResult
	if !shutdown.runWaiterIteration(ordinary, func() {
		drain = drainConfirmedShutdownBuffer(2, func(buffer []float32, maxFrames uint32) (uint32, winprobe.HResult) {
			call := readCalls.Add(1)
			if maxFrames != confirmedShutdownReadFrames || len(buffer) != int(confirmedShutdownReadFrames*2) {
				t.Fatalf("bounded read buffer len=%d max=%d", len(buffer), maxFrames)
			}
			if call == 1 {
				return 2, 0
			}
			return 0, 1
		}, func(samples []float32, frames uint32) error {
			appendCalls.Add(1)
			if frames != 2 || len(samples) != 4 {
				t.Fatalf("buffered append frames=%d samples=%d", frames, len(samples))
			}
			return nil
		})
	}) {
		t.Fatal("confirmed waiter iteration did not exit")
	}
	if forbiddenCalls.Load() != 0 || readCalls.Load() != 2 || appendCalls.Load() != 1 || drain.readBatches != 1 || drain.writtenFrames != 2 {
		t.Fatalf("post-confirm counters forbidden=%d read=%d append=%d drain=%+v", forbiddenCalls.Load(), readCalls.Load(), appendCalls.Load(), drain)
	}
	if adapter.confirm(func(uint32) winprobe.HResult {
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("repeated WM_ENDSESSION was not idempotent")
	}
	if stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("repeated confirmation added effects: stop=%d wake=%d", stopCalls.Load(), wakeCalls.Load())
	}
	queuedBeforeReject := len(evidence.operations)
	if evidence.logAsync(winprobe.LogEvent{Action: "late-signal-log"}) || evidence.log(winprobe.LogEvent{Action: "late-synchronous-log"}) || evidence.sync() {
		t.Fatal("evidence operation was accepted after confirmed shutdown")
	}
	if len(evidence.operations) != queuedBeforeReject {
		t.Fatalf("post-confirm evidence rejection changed queue length from %d to %d", queuedBeforeReject, len(evidence.operations))
	}
	if evidence.stickyFailure() != errEvidenceAbruptShutdown {
		t.Fatalf("abrupt evidence suppression error = %v", evidence.stickyFailure())
	}
	close(evidenceRelease)
	<-inFlightLog
	if <-queuedLog {
		t.Fatal("queued log callback survived confirmed shutdown")
	}
	if <-queuedSync {
		t.Fatal("queued sync callback survived confirmed shutdown")
	}
	waitForEvidenceQueueLength(t, evidence, 0)
	if physicalLogs.Load() != 1 || physicalSyncs.Load() != 0 || evidence.healthy() {
		t.Fatalf("evidence callbacks after confirmation: logs=%d syncs=%d healthy=%v", physicalLogs.Load(), physicalSyncs.Load(), evidence.healthy())
	}
	close(evidence.operations)
	close(ordinaryRelease)
	if !<-ordinaryDone {
		t.Fatal("pre-confirmation admitted drain did not finish")
	}
}

func TestR4W1ConfirmedShutdownBufferIsHardCapped(t *testing.T) {
	t.Parallel()
	var readCalls, appendCalls atomic.Int32
	result := drainConfirmedShutdownBuffer(1, func(buffer []float32, maxFrames uint32) (uint32, winprobe.HResult) {
		readCalls.Add(1)
		return 1, 0
	}, func(samples []float32, frames uint32) error {
		appendCalls.Add(1)
		return nil
	})
	if readCalls.Load() != int32(confirmedShutdownMaxBatches) || appendCalls.Load() != int32(confirmedShutdownMaxBatches) || result.readBatches != confirmedShutdownMaxBatches || result.writtenFrames != uint64(confirmedShutdownMaxBatches) {
		t.Fatalf("hard cap read=%d append=%d result=%+v", readCalls.Load(), appendCalls.Load(), result)
	}
}

func TestR4W2ProductionMessageGateSuppressesEveryQueuedApplicationClass(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 4343)
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}
	gate := confirmedShutdownMessageGate{shutdown: &shutdown}
	var preConfirmCalls atomic.Int32
	if result, suppressed := gate.enter(0x0111, func() { preConfirmCalls.Add(1) }); suppressed || result != 0 || preConfirmCalls.Load() != 1 {
		t.Fatalf("open message gate result=%d suppressed=%v calls=%d", result, suppressed, preConfirmCalls.Load())
	}

	var stopCalls, wakeCalls atomic.Int32
	if !adapter.confirm(func(operationID uint32) winprobe.HResult {
		if operationID != 4343 {
			t.Fatalf("stop operation = %d, want 4343", operationID)
		}
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("first confirmation was not accepted")
	}

	type effectCounters struct {
		log, sync, helper, release, destroy, ui, transition atomic.Int32
	}
	var effects effectCounters
	applicationEffect := func() {
		effects.log.Add(1)
		effects.sync.Add(1)
		effects.helper.Add(1)
		effects.release.Add(1)
		effects.destroy.Add(1)
		effects.ui.Add(1)
		effects.transition.Add(1)
	}
	tests := []struct {
		name       string
		message    uint32
		wantResult uintptr
	}{
		{name: "WM_COMMAND", message: 0x0111},
		{name: "WM_CLOSE", message: 0x0010},
		{name: "WM_HOTKEY", message: 0x0312},
		{name: "WM_TIMER", message: 0x0113},
		{name: "WM_APP tray", message: 0x8001},
		{name: "WM_APP devices", message: 0x8002},
		{name: "WM_APP permission", message: 0x8003},
		{name: "WM_APP capture ready", message: 0x8004},
		{name: "WM_APP capture started", message: 0x8005},
		{name: "WM_APP capture terminal", message: 0x8006},
		{name: "WM_APP picker", message: 0x8007},
		{name: "WM_APP cleanup", message: 0x8008},
		{name: "WM_APP lifecycle cleanup", message: 0x8009},
		{name: "WM_APP lifecycle rearm", message: 0x800a},
		{name: "WM_ENDSESSION cancelled", message: lifecycleWMEndSession},
		{name: "WM_ENDSESSION repeated confirmed", message: lifecycleWMEndSession},
		{name: "WM_WTSSESSION_CHANGE resume", message: lifecycleWMWTSSessionChange},
		{name: "WM_POWERBROADCAST resume", message: lifecycleWMPowerBroadcast, wantResult: 1},
		{name: "WM_QUERYENDSESSION", message: lifecycleWMQueryEndSession, wantResult: 1},
	}
	for _, tc := range tests {
		result, suppressed := gate.enter(tc.message, applicationEffect)
		if !suppressed || result != tc.wantResult {
			t.Errorf("%s result=%d suppressed=%v, want result=%d suppressed=true", tc.name, result, suppressed, tc.wantResult)
		}
	}
	if effects.log.Load() != 0 || effects.sync.Load() != 0 || effects.helper.Load() != 0 || effects.release.Load() != 0 || effects.destroy.Load() != 0 || effects.ui.Load() != 0 || effects.transition.Load() != 0 {
		t.Fatalf("post-confirm message effects log=%d sync=%d helper=%d release=%d destroy=%d ui=%d transition=%d", effects.log.Load(), effects.sync.Load(), effects.helper.Load(), effects.release.Load(), effects.destroy.Load(), effects.ui.Load(), effects.transition.Load())
	}
	if adapter.confirm(func(uint32) winprobe.HResult {
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("repeated confirmation was not idempotent")
	}
	if stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("confirmation counters stop=%d wake=%d, want 1/1", stopCalls.Load(), wakeCalls.Load())
	}
}

func TestR4W4LateNonWindowCallbacksAreSuppressedAfterConfirmation(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	bindOwnedNativeCapture(t, tracker, &owners, &shutdown, 4444)
	adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}
	var stopCalls, wakeCalls atomic.Int32
	if !adapter.confirm(func(uint32) winprobe.HResult {
		stopCalls.Add(1)
		return 0
	}, func() { wakeCalls.Add(1) }) {
		t.Fatal("confirmation failed")
	}

	type lateEffects struct {
		log, ui, watchdog, event, resource, lifecycle atomic.Int32
	}
	var effects lateEffects
	callback := func() {
		effects.log.Add(1)
		effects.ui.Add(1)
		effects.watchdog.Add(1)
		effects.event.Add(1)
		effects.resource.Add(1)
		effects.lifecycle.Add(1)
	}
	for _, source := range []string{"late SIGTERM", "late evidence failure", "GetMessage error/return", "graceful watchdog", "deferred local close"} {
		if shutdown.runOrdinary(callback) {
			t.Errorf("%s callback was admitted after confirmation", source)
		}
	}
	if effects.log.Load() != 0 || effects.ui.Load() != 0 || effects.watchdog.Load() != 0 || effects.event.Load() != 0 || effects.resource.Load() != 0 || effects.lifecycle.Load() != 0 {
		t.Fatalf("late callback effects log=%d ui=%d watchdog=%d event=%d resource=%d lifecycle=%d", effects.log.Load(), effects.ui.Load(), effects.watchdog.Load(), effects.event.Load(), effects.resource.Load(), effects.lifecycle.Load())
	}
	if stopCalls.Load() != 1 || wakeCalls.Load() != 1 {
		t.Fatalf("confirmation counters stop=%d wake=%d, want 1/1", stopCalls.Load(), wakeCalls.Load())
	}
}

func bindOwnedNativeCapture(t *testing.T, tracker *lifecycleTracker, owners *captureOwnershipCoordinator, shutdown *abruptShutdownCoordinator, operationID uint32) uint64 {
	t.Helper()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin capture: %s", reason)
	}
	prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) {
		return operationID, true
	}, func(uint32) winprobe.HResult {
		t.Fatal("shutdown stop ran before confirmation")
		return 1
	}, func(uint32) winprobe.HResult {
		t.Fatal("unpublished-owner stop ran for a published capture")
		return 1
	})
	if prepared.operationID != operationID || !prepared.owner.matches(generation, operationID) || prepared.conflictingOwner != nil || !prepared.succeeded || !prepared.trackerAllowed || !prepared.trackerInvoked || !prepared.externalInvoked || !prepared.ownerPublished || !prepared.resultEvidenceAllowed || !prepared.ownerSuccessorAllowed {
		t.Fatalf("owned prepare = %+v", prepared)
	}
	return generation
}

func TestR4F11FailedPrerequisiteSuppressesQueuedPassingRows(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var actions []string
	var syncCalls atomic.Int32
	coordinator := newEvidenceCoordinator(func(event winprobe.LogEvent) error {
		mu.Lock()
		actions = append(actions, event.Action)
		mu.Unlock()
		if event.Action == "lifecycle_signal_observed" {
			close(entered)
			<-release
			return errors.New("injected prerequisite write failure")
		}
		return nil
	}, func() error {
		syncCalls.Add(1)
		return nil
	}, time.Second)

	rowA := make(chan bool, 1)
	go func() {
		rowA <- coordinator.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "lifecycle_signal_observed"})
	}()
	<-entered
	if !coordinator.logAsync(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "lifecycle_hotkey_unregistered"}) {
		t.Fatal("passing row B was not queued behind the blocked prerequisite")
	}
	rowC := make(chan bool, 1)
	go func() {
		rowC <- coordinator.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "lifecycle_evidence_synced"})
	}()
	waitForEvidenceQueueLength(t, coordinator, 2)
	close(release)
	if <-rowA {
		t.Fatal("failed prerequisite row A reported success")
	}
	if <-rowC {
		t.Fatal("queued passing row C reported success")
	}
	waitForEvidenceQueueLength(t, coordinator, 0)
	mu.Lock()
	gotActions := append([]string(nil), actions...)
	mu.Unlock()
	if len(gotActions) != 1 || gotActions[0] != "lifecycle_signal_observed" {
		t.Fatalf("physically invoked evidence rows = %v, want prerequisite A only", gotActions)
	}
	if syncCalls.Load() != 0 || coordinator.healthy() || coordinator.sync() {
		t.Fatalf("failed prerequisite allowed sync/health: syncCalls=%d healthy=%v", syncCalls.Load(), coordinator.healthy())
	}
	close(coordinator.operations)
}

func TestR4F11SyncTimeoutSuppressesAlreadyQueuedPassingRows(t *testing.T) {
	t.Parallel()
	syncEntered := make(chan struct{})
	syncRelease := make(chan struct{})
	var logCalls atomic.Int32
	coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		logCalls.Add(1)
		return nil
	}, func() error {
		close(syncEntered)
		<-syncRelease
		return nil
	}, 30*time.Millisecond)

	syncResult := make(chan bool, 1)
	go func() { syncResult <- coordinator.sync() }()
	<-syncEntered
	if !coordinator.logAsync(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "lifecycle_hotkey_unregistered"}) {
		t.Fatal("passing row B was not queued behind blocked sync")
	}
	rowC := make(chan bool, 1)
	go func() {
		rowC <- coordinator.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "lifecycle_process_exit_ready"})
	}()
	waitForEvidenceQueueLength(t, coordinator, 2)
	if <-syncResult {
		t.Fatal("timed-out sync reported success")
	}
	close(syncRelease)
	if <-rowC {
		t.Fatal("passing row queued behind timed-out sync reported success")
	}
	waitForEvidenceQueueLength(t, coordinator, 0)
	if logCalls.Load() != 0 || coordinator.healthy() {
		t.Fatalf("queued rows reached logger after sync timeout: logCalls=%d healthy=%v", logCalls.Load(), coordinator.healthy())
	}
	close(coordinator.operations)
}

func TestR4F11UnknownEvidenceOperationFailsClosed(t *testing.T) {
	t.Parallel()
	var logCalls, syncCalls atomic.Int32
	coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		logCalls.Add(1)
		return nil
	}, func() error {
		syncCalls.Add(1)
		return nil
	}, time.Second)
	if coordinator.perform(evidenceOperation{kind: evidenceOperationKind(255), reply: make(chan error, 1)}) {
		t.Fatal("unknown operation received a successful acknowledgement")
	}
	if logCalls.Load() != 0 || syncCalls.Load() != 0 || coordinator.healthy() {
		t.Fatalf("unknown operation did not fail closed: log=%d sync=%d healthy=%v", logCalls.Load(), syncCalls.Load(), coordinator.healthy())
	}
	close(coordinator.operations)
}

func TestR4F11SuppressedSynchronousRepliesReceiveStableFailure(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	release := make(chan struct{})
	injected := errors.New("stable prerequisite failure")
	var calls atomic.Int32
	coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		calls.Add(1)
		close(entered)
		<-release
		return injected
	}, func() error { return nil }, time.Second)
	replyA := make(chan error, 1)
	replyB := make(chan error, 1)
	coordinator.operations <- evidenceOperation{kind: evidenceOperationLog, event: winprobe.LogEvent{Action: "lifecycle_signal_observed"}, reply: replyA}
	<-entered
	coordinator.operations <- evidenceOperation{kind: evidenceOperationLog, event: winprobe.LogEvent{Action: "lifecycle_process_exit_ready"}, reply: replyB}
	close(release)
	errA := <-replyA
	errB := <-replyB
	if errA == nil || errB == nil || errA != errB || errA != injected {
		t.Fatalf("synchronous replies A=%v B=%v, want the same injected sticky failure", errA, errB)
	}
	if calls.Load() != 1 {
		t.Fatalf("logger calls = %d, want prerequisite only", calls.Load())
	}
	close(coordinator.operations)
}

func waitForEvidenceQueueLength(t *testing.T, coordinator *evidenceCoordinator, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(coordinator.operations) != want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := len(coordinator.operations); got != want {
		t.Fatalf("evidence queue length = %d, want %d", got, want)
	}
}
