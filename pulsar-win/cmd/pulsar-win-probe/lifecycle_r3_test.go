package main

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestR3F1SettlementLedgerSurvivesEveryStopPublicationBoundary(t *testing.T) {
	t.Parallel()

	t.Run("all facts between registration and stop publication", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation := bindNativeCapture(t, tracker, 901)
		plan := tracker.beginLifecycle(lifecycleSessionLock, "WTS_SESSION_LOCK", winprobe.ReasonLock, lifecycleReturnsIdle)
		if plan.Generation != generation {
			t.Fatalf("bound generation = %d, want %d", plan.Generation, generation)
		}
		for _, stage := range []lifecycleStage{lifecycleCaptureTerminal, lifecycleArtifactDisposed, lifecycleCaptureReleased} {
			if advances := tracker.advanceCaptureGeneration(generation, stage); len(advances) != 0 {
				t.Fatalf("%s advanced before stop publication: %+v", stage, advances)
			}
		}
		if got := tracker.phaseForGeneration(generation); got != captureGenerationReleased {
			t.Fatalf("ledger phase = %v, want released", got)
		}
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
		advances := tracker.replayCaptureGeneration(generation)
		if len(advances) != 3 {
			t.Fatalf("replayed advances = %+v", advances)
		}
		run, _ := tracker.activeRun(lifecycleSessionLock)
		if run.Stage != lifecycleCaptureReleased {
			t.Fatalf("run stage = %s", run.Stage)
		}
	})

	for _, before := range []lifecycleStage{lifecycleCaptureTerminal, lifecycleArtifactDisposed} {
		before := before
		t.Run("fact before registration "+before.String(), func(t *testing.T) {
			tracker := newLifecycleTracker()
			generation := bindNativeCapture(t, tracker, 902)
			tracker.advanceCaptureGeneration(generation, lifecycleCaptureTerminal)
			if before == lifecycleArtifactDisposed {
				tracker.advanceCaptureGeneration(generation, lifecycleArtifactDisposed)
			}
			plan := tracker.beginLifecycle(lifecycleSuspend, "PBT_APMSUSPEND", winprobe.ReasonSuspend, lifecycleReturnsIdle)
			if plan.Generation != generation || plan.Phase < captureGenerationTerminal {
				t.Fatalf("registration snapshot = %+v", plan)
			}
			advanceRun(t, tracker, lifecycleSuspend, lifecycleStopRequested)
			tracker.replayCaptureGeneration(generation)
			run, _ := tracker.activeRun(lifecycleSuspend)
			want := before
			if run.Stage != want {
				t.Fatalf("run stage = %s, want %s", run.Stage, want)
			}
		})
	}

	t.Run("facts after stop publication", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation := bindNativeCapture(t, tracker, 903)
		tracker.beginLifecycle(lifecycleSuspend, "PBT_APMSUSPEND", winprobe.ReasonSuspend, lifecycleReturnsIdle)
		advanceRun(t, tracker, lifecycleSuspend, lifecycleStopRequested)
		settleGeneration(t, tracker, generation)
		run, _ := tracker.activeRun(lifecycleSuspend)
		if run.Stage != lifecycleCaptureReleased {
			t.Fatalf("run stage = %s", run.Stage)
		}
	})

	t.Run("native release before artifact cleanup remains lifecycle-owned", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation := bindNativeCapture(t, tracker, 906)
		tracker.advanceCaptureGeneration(generation, lifecycleCaptureTerminal)
		tracker.advanceCaptureGeneration(generation, lifecycleCaptureReleased)
		plan := tracker.beginLifecycle(lifecycleSessionLock, "lock-during-artifact-cleanup", winprobe.ReasonLock, lifecycleReturnsIdle)
		if !plan.Progress.CaptureExpected || plan.Generation != generation || plan.Capture != 0 {
			t.Fatalf("pending artifact generation binding = %+v", plan)
		}
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
		tracker.replayCaptureGeneration(generation)
		run, _ := tracker.activeRun(lifecycleSessionLock)
		if run.Stage != lifecycleCaptureTerminal {
			t.Fatalf("release incorrectly proved artifact disposal: %s", run.Stage)
		}
		tracker.advanceCaptureGeneration(generation, lifecycleArtifactDisposed)
		run, _ = tracker.activeRun(lifecycleSessionLock)
		if run.Stage != lifecycleCaptureReleased {
			t.Fatalf("artifact completion did not replay retained release: %s", run.Stage)
		}
	})
}

func TestR3F1GracefulQuitPermissionCancellationAndOverlappingEdgesReplayOneGeneration(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation, accepted, _ := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatal("capture generation rejected")
	}
	if _, succeeded, invoked := tracker.runPermissionRequest(generation, func() (uint32, bool) { return 77, true }); !invoked || !succeeded {
		t.Fatal("permission request did not enter production state")
	}
	quit := tracker.beginGracefulQuit("WM_CLOSE")
	lock := tracker.beginLifecycle(lifecycleSessionLock, "WTS_SESSION_LOCK", winprobe.ReasonLock, lifecycleReturnsIdle)
	if quit.Generation != generation || lock.Generation != generation {
		t.Fatalf("bindings quit=%d lock=%d generation=%d", quit.Generation, lock.Generation, generation)
	}
	tracker.cancelCaptureGeneration(generation)
	advanceRun(t, tracker, lifecycleQuit, lifecycleStopRequested)
	advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
	tracker.replayCaptureGeneration(generation)
	for _, edge := range []lifecycleEdge{lifecycleQuit, lifecycleSessionLock} {
		run, _ := tracker.activeRun(edge)
		if run.Stage != lifecycleCaptureReleased {
			t.Fatalf("%s stage = %s", edge, run.Stage)
		}
	}
}

func TestR3F1ReleasedGenerationCannotBeAdvancedByNPlusOne(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	first := bindNativeCapture(t, tracker, 904)
	tracker.beginLifecycle(lifecycleSessionLock, "lock", winprobe.ReasonLock, lifecycleReturnsIdle)
	for _, stage := range []lifecycleStage{lifecycleCaptureTerminal, lifecycleArtifactDisposed, lifecycleCaptureReleased} {
		tracker.advanceCaptureGeneration(first, stage)
	}
	advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
	tracker.replayCaptureGeneration(first)
	tracker.resume(lifecycleSessionLock)
	advanceRun(t, tracker, lifecycleSessionLock, lifecycleHotkeyUnregistered, lifecycleIdle)
	second := bindNativeCapture(t, tracker, 905)
	if second == first {
		t.Fatal("generation did not advance")
	}
	if advances := tracker.advanceCaptureGeneration(second, lifecycleCaptureTerminal); len(advances) != 0 {
		t.Fatalf("generation N+1 advanced an old run: %+v", advances)
	}
}

func TestR3F2DurableUITransitionsRetryUntilAcknowledged(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"no capture", "capture release", "artifact retry", "cancelled shutdown"} {
		scenario := scenario
		t.Run(scenario, func(t *testing.T) {
			coordinator := newUITransitionCoordinator(4)
			id := coordinator.publish(uiTransitionIdleCleanup, 0, winprobe.PermissionUnknown)
			attempts := 0
			for attempts < 2 {
				if coordinator.drive(func(uiTransition) bool { attempts++; return false }) {
					t.Fatal("transition escalated before retry budget")
				}
			}
			if coordinator.drive(func(got uiTransition) bool {
				attempts++
				return got.ID == id && got.Kind == uiTransitionIdleCleanup
			}) {
				t.Fatal("successful post escalated")
			}
			transition, consumed := coordinator.consume(uiTransitionIdleCleanup, id)
			if !consumed || transition.ID != id || attempts != 3 {
				t.Fatalf("consume=%v transition=%+v attempts=%d", consumed, transition, attempts)
			}
		})
	}

	t.Run("permission rearm payload survives post failure", func(t *testing.T) {
		coordinator := newUITransitionCoordinator(3)
		id := coordinator.publish(uiTransitionLifecycleRearm, 42, winprobe.PermissionAllowed)
		coordinator.drive(func(uiTransition) bool { return false })
		coordinator.drive(func(transition uiTransition) bool {
			return transition.ID == id && transition.Generation == 42 && transition.Status == winprobe.PermissionAllowed
		})
		transition, consumed := coordinator.consume(uiTransitionLifecycleRearm, id)
		if !consumed || transition.Generation != 42 || transition.Status != winprobe.PermissionAllowed {
			t.Fatalf("rearm transition = %+v consumed=%v", transition, consumed)
		}
	})
}

func TestR3F2DurableUITransitionHasBoundedGracefulEscalation(t *testing.T) {
	t.Parallel()
	coordinator := newUITransitionCoordinator(2)
	coordinator.publish(uiTransitionIdleCleanup, 0, winprobe.PermissionUnknown)
	if coordinator.drive(func(uiTransition) bool { return false }) {
		t.Fatal("first post failure escalated")
	}
	if !coordinator.drive(func(uiTransition) bool { return false }) {
		t.Fatal("bounded post failures did not escalate")
	}
	if coordinator.drive(func(uiTransition) bool { t.Fatal("escalated transition posted again"); return true }) {
		t.Fatal("transition escalated repeatedly")
	}
}

func TestR3F2SynchronousConsumeDuringPostCannotLoseIntent(t *testing.T) {
	t.Parallel()
	coordinator := newUITransitionCoordinator(3)
	id := coordinator.publish(uiTransitionIdleCleanup, 0, winprobe.PermissionUnknown)
	consumed := false
	if coordinator.drive(func(transition uiTransition) bool {
		if transition.ID != id {
			t.Fatalf("posted ID = %d, want %d", transition.ID, id)
		}
		_, consumed = coordinator.consume(uiTransitionIdleCleanup, transition.ID)
		return true
	}) {
		t.Fatal("successful synchronous dispatch escalated")
	}
	if !consumed {
		t.Fatal("in-flight exact-ID consumption was rejected")
	}
	if _, pending := coordinator.pendingTransition(uiTransitionIdleCleanup); pending {
		t.Fatal("finishPost recreated a synchronously consumed intent")
	}
	if coordinator.drive(func(uiTransition) bool { t.Fatal("consumed intent was posted twice"); return true }) {
		t.Fatal("consumed intent escalated")
	}
}

func TestR3F3PermissionQueryFailureDecisionFailsClosedForEveryOwnedState(t *testing.T) {
	t.Parallel()
	idle := newLifecycleTracker()
	if !idle.permissionQueryFailureRequiresStop(true, false) {
		t.Error("AccessChanged failure without capture did not fail closed")
	}
	if idle.permissionQueryFailureRequiresStop(false, false) {
		t.Error("defensive idle poll failure invented an owned capture")
	}
	if !idle.permissionQueryFailureRequiresStop(false, true) {
		t.Error("owned native/permission operation did not fail closed")
	}
	requested := newLifecycleTracker()
	requestedGeneration, accepted, _ := requested.beginCaptureGeneration()
	if !accepted || !requested.permissionQueryFailureRequiresStop(false, false) {
		t.Error("requested generation did not fail closed")
	}
	pending := newLifecycleTracker()
	pendingGeneration, accepted, _ := pending.beginCaptureGeneration()
	if !accepted {
		t.Fatal("pending generation rejected")
	}
	pending.runPermissionRequest(pendingGeneration, func() (uint32, bool) { return 1, true })
	if !pending.permissionQueryFailureRequiresStop(false, false) {
		t.Error("permission-pending generation did not fail closed")
	}
	preparing := newLifecycleTracker()
	prepareGeneration, accepted, _ := preparing.beginCaptureGeneration()
	if !accepted {
		t.Fatal("prepare generation rejected")
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	prepared := make(chan struct{})
	go func() {
		preparing.runCapturePrepare(prepareGeneration, func() (uint32, bool) {
			close(entered)
			<-release
			return 2, true
		})
		close(prepared)
	}()
	<-entered
	queryDecision := make(chan bool, 1)
	go func() { queryDecision <- preparing.permissionQueryFailureRequiresStop(false, false) }()
	select {
	case <-queryDecision:
		t.Fatal("permission decision crossed prepare ownership transition")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-prepared
	if !<-queryDecision {
		t.Error("prepared native generation did not fail closed")
	}
	requested.cancelCaptureGeneration(requestedGeneration)

	tracker := newLifecycleTracker()
	tracker.beginLifecycle(lifecyclePermissionRevoke, "query-failed", winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle)
	if !tracker.permissionIsBlocked() {
		t.Fatal("query failure did not close permission gate")
	}
	advanceRun(t, tracker, lifecyclePermissionRevoke, lifecycleStopRequested, lifecycleHotkeyUnregistered, lifecycleIdle)
	rearm, accepted := tracker.beginRearm()
	if !accepted || !tracker.runRearm(rearm, true, func() bool { return true }) || tracker.permissionIsBlocked() {
		t.Fatal("successful waiter query did not recover the gate through current rearm")
	}
}

func TestPermissionFailureEvidenceBoundsDuplicatePollFailures(t *testing.T) {
	t.Parallel()
	var evidence permissionFailureEvidence
	wrongThread := winprobe.HResultFromUintptr(uintptr(0x8001010e))
	if !evidence.shouldLog(wrongThread, false) {
		t.Fatal("first permission failure was suppressed")
	}
	for attempt := 0; attempt < 1000; attempt++ {
		if evidence.shouldLog(wrongThread, false) {
			t.Fatalf("duplicate defensive poll failure %d was logged", attempt)
		}
	}
	if !evidence.shouldLog(wrongThread, true) {
		t.Fatal("real AccessChanged failure was suppressed")
	}
	if !evidence.shouldLog(winprobe.HResultFromUintptr(uintptr(0x80070005)), false) {
		t.Fatal("changed HRESULT was suppressed")
	}
	evidence.reset()
	if !evidence.shouldLog(wrongThread, false) {
		t.Fatal("failure after successful reset was suppressed")
	}
}

func TestR3F9PermissionFailureStopsAndClosesGateBeforeBlockedEvidence(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation := bindNativeCapture(t, tracker, 909)
	var stops atomic.Int32
	plan, stopHR := tracker.beginLifecycleStop(
		lifecyclePermissionRevoke,
		"AppCapability.AccessChanged+CapPermissionCheck(query-failed)",
		winprobe.ReasonPermissionRevoke,
		lifecycleReturnsIdle,
		func(stopGeneration uint64, operationID uint32) winprobe.HResult {
			if stopGeneration != generation {
				t.Fatalf("stop generation = %d, want %d", stopGeneration, generation)
			}
			if operationID != 909 {
				t.Fatalf("stop operation = %d", operationID)
			}
			stops.Add(1)
			return 0
		},
	)
	if stopHR.Failed() || plan.Generation != generation || stops.Load() != 1 {
		t.Fatalf("plan=%+v stopHR=%s stops=%d", plan, stopHR.Hex(), stops.Load())
	}
	if _, accepted, reason := tracker.beginCaptureGeneration(); accepted || reason != "microphone permission is revoked" {
		t.Fatalf("post-revoke start accepted=%v reason=%q", accepted, reason)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	evidence := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		close(entered)
		<-release
		return nil
	}, func() error { return nil }, time.Second)
	logged := make(chan bool, 1)
	go func() { logged <- evidence.log(winprobe.LogEvent{Action: "permission_status_query"}) }()
	<-entered
	if stops.Load() != 1 || tracker.workAllowed() {
		t.Fatal("blocked evidence preceded fail-closed stop/gate")
	}
	close(release)
	if !<-logged {
		t.Fatal("released evidence write failed")
	}
}

func TestR3F91WaiterPermissionCheckFailureSettlesBeforeBlockedEvidence(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("requested generation rejected: %s", reason)
	}
	plan, stopHR := tracker.beginLifecycleStop(
		lifecyclePermissionRevoke,
		"CapPermissionCheck(explicit-record-query-failed)",
		winprobe.ReasonPermissionRevoke,
		lifecycleReturnsIdle,
		func(uint64, uint32) winprobe.HResult {
			t.Fatal("requested generation must not invoke native CaptureStop")
			return 0
		},
	)
	if stopHR.Failed() || plan.Generation != generation || plan.Phase != captureGenerationRequested {
		t.Fatalf("plan=%+v stopHR=%s", plan, stopHR.Hex())
	}
	if tracker.workAllowed() || tracker.phaseForGeneration(generation) != captureGenerationReleased {
		t.Fatalf("query failure did not gate and settle before evidence: allowed=%v phase=%d", tracker.workAllowed(), tracker.phaseForGeneration(generation))
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	evidence := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		close(entered)
		<-release
		return nil
	}, func() error { return nil }, time.Second)
	logged := make(chan bool, 1)
	go func() { logged <- evidence.log(winprobe.LogEvent{Action: "permission_check"}) }()
	<-entered
	if tracker.workAllowed() || tracker.phaseForGeneration(generation) != captureGenerationReleased {
		t.Fatal("blocked evidence held the failed permission generation open")
	}
	close(release)
	if !<-logged {
		t.Fatal("released evidence write failed")
	}

	if _, changed, err := tracker.advance(lifecyclePermissionRevoke, lifecycleStopRequested); err != nil || !changed {
		t.Fatalf("publish stop_requested: changed=%v err=%v", changed, err)
	}
	advances := tracker.replayCaptureGeneration(generation)
	if len(advances) != 3 || advances[2].Stage != lifecycleCaptureReleased || advances[2].Err != nil {
		t.Fatalf("settlement replay = %+v", advances)
	}
}

func TestR3F91PermissionRearmFailureClosesTokenBeforeEvidence(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	tracker.setPermissionAllowed(false)
	rearm, accepted := tracker.beginRearm()
	if !accepted {
		t.Fatal("rearm token rejected")
	}
	plan, stopHR := tracker.beginLifecycleStop(
		lifecyclePermissionRevoke,
		"CapPermissionCheck(lifecycle-rearm-query-failed)",
		winprobe.ReasonPermissionRevoke,
		lifecycleReturnsIdle,
		nil,
	)
	if stopHR.Failed() || plan.Capture != 0 || tracker.rearmPending(rearm) || !tracker.permissionIsBlocked() {
		t.Fatalf("plan=%+v stopHR=%s rearmPending=%v permissionBlocked=%v", plan, stopHR.Hex(), tracker.rearmPending(rearm), tracker.permissionIsBlocked())
	}
	var continuationCalls atomic.Int32
	if tracker.runRearm(rearm, true, func() bool {
		continuationCalls.Add(1)
		return true
	}) || continuationCalls.Load() != 0 {
		t.Fatal("failed rearm query allowed hotkey/discovery continuation")
	}
	if _, accepted, reason := tracker.beginCaptureGeneration(); accepted || reason != "microphone permission is revoked" {
		t.Fatalf("capture crossed failed rearm gate: accepted=%v reason=%q", accepted, reason)
	}
}

func TestR3F9RequiredEvidenceGateSuppressesStartContinuations(t *testing.T) {
	t.Parallel()
	prepareTracker := newLifecycleTracker()
	prepareGeneration, accepted, _ := prepareTracker.beginCaptureGeneration()
	if !accepted {
		t.Fatal("prepare generation rejected")
	}
	var prepares atomic.Int32
	if runAfterRequiredEvidence(func() bool { return false }, func() {
		prepareTracker.runCapturePrepare(prepareGeneration, func() (uint32, bool) {
			prepares.Add(1)
			return 1, true
		})
	}) {
		t.Fatal("failed evidence gate reported success")
	}
	if prepares.Load() != 0 {
		t.Fatal("failed evidence invoked CapturePrepare")
	}

	activationTracker := newLifecycleTracker()
	activationGeneration := bindNativeCapture(t, activationTracker, 910)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan bool, 1)
	var activations atomic.Int32
	go func() {
		done <- runAfterRequiredEvidence(func() bool {
			close(entered)
			<-release
			return false
		}, func() {
			activationTracker.runCaptureActivation(activationGeneration, 910, func() { activations.Add(1) })
		})
	}()
	<-entered
	if activations.Load() != 0 {
		t.Fatal("CaptureActivate ran while required evidence was blocked")
	}
	close(release)
	if <-done || activations.Load() != 0 {
		t.Fatal("failed stalled evidence invoked CaptureActivate")
	}
}

type injectedShortWriter struct{}

func (injectedShortWriter) Write(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	return len(buffer) / 2, io.ErrShortWrite
}

func TestR3F5JSONLoggerShortWriteReachesStickyProductionEvidenceSeam(t *testing.T) {
	t.Parallel()
	logger := winprobe.NewJSONLogger(injectedShortWriter{})
	coordinator := newEvidenceCoordinator(logger.Log, func() error { return nil }, time.Second)
	if coordinator.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "lifecycle_signal_observed"}) {
		t.Fatal("short JSONL write reported success")
	}
	if coordinator.healthy() || coordinator.sync() {
		t.Fatal("short JSONL write did not suppress clean sync")
	}
}

func TestR3F4HardDeadlineIsArmedBeforeBlockingEvidenceWrite(t *testing.T) {
	t.Parallel()
	var exit processExitCoordinator
	armed := make(chan func(), 1)
	exited := make(chan struct{}, 1)
	if !exit.beginGracefulWithDeadline(func(callback func()) { armed <- callback }, func() { exited <- struct{}{} }) {
		t.Fatal("graceful arbitration failed")
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	evidence := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		close(entered)
		<-release
		return nil
	}, func() error { return nil }, time.Second)
	logged := make(chan bool, 1)
	go func() {
		logged <- evidence.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "lifecycle_signal_observed"})
	}()
	<-entered
	callback := <-armed
	callback()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("hard exit did not win a blocked logger")
	}
	close(release)
	<-logged
}

func TestR3F5EvidenceFailureIsStickyAndSuppressesCleanClaims(t *testing.T) {
	t.Parallel()
	for _, stage := range []lifecycleStage{
		lifecycleSignalObserved,
		lifecycleStopRequested,
		lifecycleCaptureTerminal,
		lifecycleArtifactDisposed,
		lifecycleCaptureReleased,
		lifecycleHotkeyUnregistered,
		lifecycleEvidenceSynced,
		lifecycleProcessExit,
	} {
		stage := stage
		t.Run(stage.String(), func(t *testing.T) {
			var writes atomic.Int32
			coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error {
				writes.Add(1)
				return errors.New("injected short write")
			}, func() error { t.Fatal("sync ran after failed row"); return nil }, time.Second)
			if coordinator.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "lifecycle_" + stage.String()}) {
				t.Fatal("failed lifecycle row reported success")
			}
			if coordinator.healthy() || coordinator.sync() || coordinator.log(winprobe.LogEvent{Action: "lifecycle_evidence_synced"}) {
				t.Fatal("sticky failure allowed a clean evidence claim")
			}
			if writes.Load() != 1 {
				t.Fatalf("writes = %d, want exactly failed row", writes.Load())
			}
		})
	}

	t.Run("sync error", func(t *testing.T) {
		coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error { return nil }, func() error { return errors.New("sync failed") }, time.Second)
		if coordinator.sync() || coordinator.healthy() {
			t.Fatal("sync error was not sticky")
		}
	})

	t.Run("stall is bounded", func(t *testing.T) {
		release := make(chan struct{})
		coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error { <-release; return nil }, func() error { return nil }, 20*time.Millisecond)
		started := time.Now()
		if coordinator.log(winprobe.LogEvent{Action: "blocked"}) {
			t.Fatal("stalled write reported success")
		}
		if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
			t.Fatalf("bounded write took %s", elapsed)
		}
		close(release)
	})

	t.Run("nonblocking lifecycle row failure becomes sticky", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error {
			close(entered)
			<-release
			return errors.New("asynchronous write failed")
		}, func() error { return nil }, time.Second)
		if !coordinator.logAsync(winprobe.LogEvent{Action: "WM_QUERYENDSESSION"}) {
			t.Fatal("nonblocking row was not accepted into the production queue")
		}
		<-entered
		close(release)
		deadline := time.Now().Add(time.Second)
		for coordinator.healthy() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if coordinator.healthy() || coordinator.sync() {
			t.Fatal("asynchronous write failure allowed evidence sync")
		}
	})
}

func TestR3F5EvidenceQueueSaturationIsStickyAndSuppressesCleanClaims(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var processed atomic.Int32
	coordinator := newEvidenceCoordinator(func(winprobe.LogEvent) error {
		if processed.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	}, func() error {
		t.Fatal("sync ran after saturated evidence queue")
		return nil
	}, time.Second)

	if !coordinator.logAsync(winprobe.LogEvent{Action: "block-evidence-worker"}) {
		t.Fatal("first asynchronous row was not accepted")
	}
	<-entered
	capacity := cap(coordinator.operations)
	for index := 0; index < capacity; index++ {
		if !coordinator.logAsync(winprobe.LogEvent{Action: fmt.Sprintf("queued-%d", index)}) {
			t.Fatalf("queue rejected row %d before capacity %d", index, capacity)
		}
	}
	if coordinator.logAsync(winprobe.LogEvent{Action: "overflow"}) {
		t.Fatal("saturated evidence queue accepted an overflow row")
	}
	if coordinator.healthy() || coordinator.sync() || coordinator.log(winprobe.LogEvent{Action: "clean-claim"}) {
		t.Fatal("queue saturation allowed healthy/sync/clean claims")
	}

	close(release)
	wantProcessed := int32(1)
	deadline := time.Now().Add(time.Second)
	for processed.Load() < wantProcessed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if processed.Load() != wantProcessed {
		t.Fatalf("processed rows = %d, want only the in-flight row; queued successors must be discarded after saturation", processed.Load())
	}
	close(coordinator.operations)
}

func TestR3F7RearmValidationPermissionAndWorkAreOneStartGate(t *testing.T) {
	t.Parallel()

	t.Run("current rearm blocks capture until UI work succeeds", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, accepted := tracker.beginRearm()
		if !accepted {
			t.Fatal("rearm rejected")
		}
		if _, captureAccepted, reason := tracker.beginCaptureGeneration(); captureAccepted || reason != "lifecycle rearm is pending" {
			t.Fatalf("capture accepted=%v reason=%q", captureAccepted, reason)
		}
		var calls atomic.Int32
		if !tracker.runRearm(generation, true, func() bool { calls.Add(1); return true }) {
			t.Fatal("current rearm failed")
		}
		if calls.Load() != 1 {
			t.Fatalf("work calls = %d", calls.Load())
		}
		if _, captureAccepted, reason := tracker.beginCaptureGeneration(); !captureAccepted {
			t.Fatalf("capture remained blocked: %s", reason)
		}
	})

	t.Run("failed UI work retains token for retry", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, _ := tracker.beginRearm()
		if tracker.runRearm(generation, true, func() bool { return false }) || !tracker.rearmPending(generation) {
			t.Fatal("failed work did not retain current rearm")
		}
		if !tracker.runRearm(generation, true, func() bool { return true }) {
			t.Fatal("rearm retry failed")
		}
	})

	t.Run("stale result cannot mutate permission after terminal edge", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, _ := tracker.beginRearm()
		tracker.beginLifecycle(lifecycleSessionLock, "lock", winprobe.ReasonLock, lifecycleReturnsIdle)
		if tracker.runRearm(generation, false, func() bool { t.Fatal("stale work called"); return true }) {
			t.Fatal("stale rearm accepted")
		}
		if tracker.permissionIsBlocked() {
			t.Fatal("stale denied result mutated permission state")
		}
	})

	t.Run("denied current result closes permission gate", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, _ := tracker.beginRearm()
		if tracker.runRearm(generation, false, func() bool { return true }) || !tracker.permissionIsBlocked() {
			t.Fatal("denied current rearm did not close gate")
		}
	})
}

func TestR3F8ProductionCoordinatorsSurviveRepeatedLifecycleCycles(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	transitions := newUITransitionCoordinator(3)
	evidence := newEvidenceCoordinator(func(winprobe.LogEvent) error { return nil }, func() error { return nil }, time.Second)
	var hotkey ownedResource
	for cycle := 1; cycle <= 100; cycle++ {
		hotkey.setOwned(true)
		generation := bindNativeCapture(t, tracker, uint32(1000+cycle))
		tracker.beginLifecycle(lifecycleSessionLock, "lock", winprobe.ReasonLock, lifecycleReturnsIdle)
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
		settleGeneration(t, tracker, generation)
		id := transitions.publish(uiTransitionIdleCleanup, 0, winprobe.PermissionUnknown)
		if transitions.drive(func(uiTransition) bool { return true }) {
			t.Fatalf("cycle %d transition escalated", cycle)
		}
		if _, consumed := transitions.consume(uiTransitionIdleCleanup, id); !consumed {
			t.Fatalf("cycle %d cleanup message not consumed", cycle)
		}
		if released, _ := hotkey.release(func() bool { return true }); !released {
			t.Fatalf("cycle %d hotkey leaked", cycle)
		}
		if !evidence.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "lifecycle_hotkey_unregistered", Fields: map[string]any{"cycle": cycle}}) {
			t.Fatalf("cycle %d evidence failed", cycle)
		}
		tracker.resume(lifecycleSessionLock)
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleHotkeyUnregistered, lifecycleIdle)
		if tracker.hasCaptureGeneration() || hotkey.isOwned() {
			t.Fatalf("cycle %d retained capture or hotkey ownership", cycle)
		}
	}
}

func TestR3F7ConcurrentCaptureWaitsForRearmOwnership(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation, _ := tracker.beginRearm()
	entered := make(chan struct{})
	release := make(chan struct{})
	rearmDone := make(chan bool, 1)
	go func() {
		rearmDone <- tracker.runRearm(generation, true, func() bool {
			close(entered)
			<-release
			return true
		})
	}()
	<-entered
	type captureResult struct {
		accepted bool
	}
	captureDone := make(chan captureResult, 1)
	go func() {
		_, accepted, _ := tracker.beginCaptureGeneration()
		captureDone <- captureResult{accepted: accepted}
	}()
	select {
	case <-captureDone:
		t.Fatal("capture crossed the in-flight rearm boundary")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if !<-rearmDone {
		t.Fatal("rearm failed")
	}
	if result := <-captureDone; !result.accepted {
		t.Fatal("serialized capture was not accepted after rearm")
	}
}

func TestR3F2ConcurrentPublishCoalescesOneOwnedIntent(t *testing.T) {
	t.Parallel()
	coordinator := newUITransitionCoordinator(3)
	var ids sync.Map
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids.Store(coordinator.publish(uiTransitionIdleCleanup, 0, winprobe.PermissionUnknown), struct{}{})
		}()
	}
	wg.Wait()
	count := 0
	ids.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Fatalf("coalesced IDs = %d", count)
	}
}
