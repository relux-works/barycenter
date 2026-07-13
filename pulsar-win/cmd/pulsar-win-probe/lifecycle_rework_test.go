package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func bindNativeCapture(t *testing.T, tracker *lifecycleTracker, operationID uint32) uint64 {
	t.Helper()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin capture: %s", reason)
	}
	gotID, succeeded, allowed, invoked := tracker.runCapturePrepare(generation, func() (uint32, bool) {
		return operationID, true
	})
	if !invoked || !succeeded || !allowed || gotID != operationID {
		t.Fatalf("prepare = id:%d succeeded:%v allowed:%v invoked:%v", gotID, succeeded, allowed, invoked)
	}
	return generation
}

func advanceRun(t *testing.T, tracker *lifecycleTracker, edge lifecycleEdge, stages ...lifecycleStage) {
	t.Helper()
	for _, stage := range stages {
		if _, changed, err := tracker.advance(edge, stage); err != nil || !changed {
			t.Fatalf("advance %s/%s changed=%v err=%v", edge, stage, changed, err)
		}
	}
}

func settleGeneration(t *testing.T, tracker *lifecycleTracker, generation uint64) {
	t.Helper()
	for _, stage := range []lifecycleStage{lifecycleCaptureTerminal, lifecycleArtifactDisposed, lifecycleCaptureReleased} {
		advances := tracker.advanceCaptureGeneration(generation, stage)
		for _, advance := range advances {
			if advance.Err != nil {
				t.Fatalf("settle generation %d at %s: %v", generation, stage, advance.Err)
			}
		}
	}
}

func TestR1LifecycleRegistrationAndReleaseShareGenerationBoundary(t *testing.T) {
	t.Parallel()

	t.Run("release wins boundary", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation := bindNativeCapture(t, tracker, 101)
		settleGeneration(t, tracker, generation)
		plan := tracker.beginLifecycle(lifecycleSuspend, "suspend-after-release", winprobe.ReasonSuspend, lifecycleReturnsIdle)
		if plan.Progress.CaptureExpected || plan.Generation != 0 || plan.Capture != 0 {
			t.Fatalf("released generation was rebound: %+v", plan)
		}
	})

	t.Run("registration wins boundary", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation := bindNativeCapture(t, tracker, 102)
		registered := make(chan lifecycleStopPlan, 1)
		allowRegistration := make(chan struct{})
		go func() {
			<-allowRegistration
			registered <- tracker.beginLifecycle(lifecycleSuspend, "suspend-before-release", winprobe.ReasonSuspend, lifecycleReturnsIdle)
		}()
		close(allowRegistration)
		plan := <-registered
		if plan.Generation != generation || plan.Capture != 102 || !plan.Progress.CaptureExpected {
			t.Fatalf("registration did not bind exact generation: %+v", plan)
		}
		advanceRun(t, tracker, lifecycleSuspend, lifecycleStopRequested)
		settleGeneration(t, tracker, generation)
		if run, ok := tracker.activeRun(lifecycleSuspend); !ok || run.Stage != lifecycleCaptureReleased {
			t.Fatalf("registered run did not receive release: %+v ok=%v", run, ok)
		}
	})
}

func TestR1GenerationNPlusOneCannotSettleGenerationN(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation := bindNativeCapture(t, tracker, 201)
	plan := tracker.beginLifecycle(lifecycleSessionLock, "lock", winprobe.ReasonLock, lifecycleReturnsIdle)
	advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
	if advances := tracker.advanceCaptureGeneration(generation+1, lifecycleCaptureTerminal); len(advances) != 0 {
		t.Fatalf("unrelated generation advanced runs: %+v", advances)
	}
	run, _ := tracker.activeRun(lifecycleSessionLock)
	if run.Stage != lifecycleStopRequested || plan.Generation != generation {
		t.Fatalf("old run changed by generation N+1: %+v", run)
	}
	settleGeneration(t, tracker, generation)
}

func TestR1RegistrationReplaysAlreadyObservedGenerationState(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation := bindNativeCapture(t, tracker, 250)
	if advances := tracker.advanceCaptureGeneration(generation, lifecycleCaptureTerminal); len(advances) != 0 {
		t.Fatalf("terminal without a lifecycle run produced advances: %+v", advances)
	}
	plan := tracker.beginLifecycle(lifecycleSuspend, "suspend-after-terminal", winprobe.ReasonSuspend, lifecycleReturnsIdle)
	if plan.Generation != generation || plan.Phase != captureGenerationTerminal {
		t.Fatalf("registration did not snapshot terminal phase: %+v", plan)
	}
	advanceRun(t, tracker, lifecycleSuspend, lifecycleStopRequested)
	advances := tracker.advanceCaptureGeneration(generation, lifecycleCaptureTerminal)
	if len(advances) != 1 || !advances[0].Changed || advances[0].Err != nil {
		t.Fatalf("terminal replay = %+v", advances)
	}
	for _, stage := range []lifecycleStage{lifecycleArtifactDisposed, lifecycleCaptureReleased} {
		tracker.advanceCaptureGeneration(generation, stage)
	}
}

func TestR1OverlappingEdgesBindAndSettleOneGeneration(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation := bindNativeCapture(t, tracker, 301)
	suspend := tracker.beginLifecycle(lifecycleSuspend, "suspend", winprobe.ReasonSuspend, lifecycleReturnsIdle)
	lock := tracker.beginLifecycle(lifecycleSessionLock, "lock", winprobe.ReasonLock, lifecycleReturnsIdle)
	if suspend.Generation != generation || lock.Generation != generation {
		t.Fatalf("overlapping bindings = suspend:%d lock:%d want:%d", suspend.Generation, lock.Generation, generation)
	}
	advanceRun(t, tracker, lifecycleSuspend, lifecycleStopRequested)
	advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
	settleGeneration(t, tracker, generation)
	for _, edge := range []lifecycleEdge{lifecycleSuspend, lifecycleSessionLock} {
		run, ok := tracker.activeRun(edge)
		if !ok || run.Stage != lifecycleCaptureReleased {
			t.Fatalf("%s run = %+v ok=%v", edge, run, ok)
		}
	}
}

func TestR1QuitWithoutCaptureClosesCompetingStartGate(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	plan := tracker.beginGracefulQuit("WM_CLOSE")
	if plan.Progress.CaptureExpected {
		t.Fatalf("quit unexpectedly bound capture: %+v", plan)
	}
	if _, accepted, _ := tracker.beginCaptureGeneration(); accepted {
		t.Fatal("capture generation started after terminal intent")
	}
	if !tracker.consumeQuitIntent() || tracker.consumeQuitIntent() {
		t.Fatal("quit intent was not delivered exactly once")
	}
}

func TestR2StalePermissionAndCaptureContinuationsAreSuppressed(t *testing.T) {
	t.Parallel()
	edges := []struct {
		name   string
		edge   lifecycleEdge
		reason winprobe.CaptureReason
		mode   lifecycleMode
	}{
		{name: "quit", edge: lifecycleQuit, reason: winprobe.ReasonCancel, mode: lifecycleGracefulExit},
		{name: "suspend", edge: lifecycleSuspend, reason: winprobe.ReasonSuspend, mode: lifecycleReturnsIdle},
		{name: "lock", edge: lifecycleSessionLock, reason: winprobe.ReasonLock, mode: lifecycleReturnsIdle},
		{name: "shutdown", edge: lifecycleSystemShutdown, reason: winprobe.ReasonShutdown, mode: lifecycleAbruptOSExit},
	}
	for _, tc := range edges {
		tc := tc
		t.Run(tc.name+" permission-ready", func(t *testing.T) {
			tracker := newLifecycleTracker()
			generation, accepted, _ := tracker.beginCaptureGeneration()
			if !accepted {
				t.Fatal("capture request rejected before lifecycle edge")
			}
			if tc.edge == lifecycleQuit {
				tracker.beginGracefulQuit("quit")
			} else {
				tracker.beginLifecycle(tc.edge, tc.name, tc.reason, tc.mode)
			}
			var prepares atomic.Int32
			_, _, _, invoked := tracker.runCapturePrepare(generation, func() (uint32, bool) {
				prepares.Add(1)
				return 1, true
			})
			if invoked || prepares.Load() != 0 {
				t.Fatal("stale permission-ready invoked CapturePrepare")
			}
		})

		t.Run(tc.name+" capture-ready", func(t *testing.T) {
			tracker := newLifecycleTracker()
			generation := bindNativeCapture(t, tracker, 401)
			if tc.edge == lifecycleQuit {
				tracker.beginGracefulQuit("quit")
			} else {
				tracker.beginLifecycle(tc.edge, tc.name, tc.reason, tc.mode)
			}
			var activations atomic.Int32
			if tracker.runCaptureActivation(generation, 401, func() { activations.Add(1) }) || activations.Load() != 0 {
				t.Fatal("stale capture-ready invoked CaptureActivate")
			}
		})
	}
}

func TestR2ContinuationBeforeLifecycleMayInvokeExactlyOnce(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation, accepted, _ := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatal("capture request rejected")
	}
	var prepares, activations atomic.Int32
	operationID, succeeded, allowed, invoked := tracker.runCapturePrepare(generation, func() (uint32, bool) {
		prepares.Add(1)
		return 501, true
	})
	if !invoked || !succeeded || !allowed {
		t.Fatal("current permission continuation was suppressed")
	}
	if !tracker.runCaptureActivation(generation, operationID, func() { activations.Add(1) }) {
		t.Fatal("current capture continuation was suppressed")
	}
	if prepares.Load() != 1 || activations.Load() != 1 {
		t.Fatalf("prepare=%d activate=%d", prepares.Load(), activations.Load())
	}
}

func TestR3ShutdownQueryGateCancelAndConfirm(t *testing.T) {
	t.Parallel()

	t.Run("query blocks start then cancel permits start after cleanup", func(t *testing.T) {
		tracker := newLifecycleTracker()
		tracker.beginLifecycle(lifecycleSystemShutdown, "WM_QUERYENDSESSION", winprobe.ReasonShutdown, lifecycleAbruptOSExit)
		advanceRun(t, tracker, lifecycleSystemShutdown, lifecycleStopRequested)
		if _, accepted, _ := tracker.beginCaptureGeneration(); accepted {
			t.Fatal("query-end-session did not close start gate")
		}
		progress, err := tracker.cancelShutdown("WM_ENDSESSION(cancelled)")
		if err != nil || progress.Signal != "WM_ENDSESSION(cancelled)" {
			t.Fatalf("cancel = %+v err=%v", progress, err)
		}
		advanceRun(t, tracker, lifecycleSystemShutdown, lifecycleHotkeyUnregistered, lifecycleIdle)
		if _, accepted, reason := tracker.beginCaptureGeneration(); !accepted {
			t.Fatalf("start remained blocked after cancelled-shutdown cleanup: %s", reason)
		}
	})

	for _, withCapture := range []bool{false, true} {
		name := "without capture"
		if withCapture {
			name = "with capture"
		}
		t.Run("confirm "+name, func(t *testing.T) {
			tracker := newLifecycleTracker()
			if withCapture {
				bindNativeCapture(t, tracker, 601)
			}
			tracker.beginLifecycle(lifecycleSystemShutdown, "WM_QUERYENDSESSION", winprobe.ReasonShutdown, lifecycleAbruptOSExit)
			tracker.markShutdownConfirmed()
			tracker.beginLifecycle(lifecycleSystemShutdown, "WM_ENDSESSION(confirmed)", winprobe.ReasonShutdown, lifecycleAbruptOSExit)
			if _, accepted, _ := tracker.beginCaptureGeneration(); accepted {
				t.Fatal("confirmed shutdown reopened start gate")
			}
			run, _ := tracker.activeRun(lifecycleSystemShutdown)
			if run.Signal != "WM_ENDSESSION(confirmed)" || len(run.Signals) != 2 {
				t.Fatalf("confirmed signal history = %+v", run)
			}
		})
	}
}

func TestR4TerminalIntentSurvivesSaturatedOrdinaryQueueAndWakeFailure(t *testing.T) {
	t.Parallel()
	ordinary := make(chan struct{}, 2)
	ordinary <- struct{}{}
	ordinary <- struct{}{}
	tracker := newLifecycleTracker()
	tracker.beginGracefulQuit("first")
	tracker.beginGracefulQuit("repeated")
	// No ordinary queue write and no wake signal participates in delivery.
	if !tracker.consumeQuitIntent() {
		t.Fatal("terminal intent was lost behind saturated queue")
	}
	if tracker.consumeQuitIntent() {
		t.Fatal("repeated quit began cooperative cleanup twice")
	}
	if len(ordinary) != cap(ordinary) {
		t.Fatal("terminal delivery unexpectedly depended on ordinary queue capacity")
	}
}

func TestR5HardExitRemainsArmedAfterHelperDestroyAndDuringSyncStall(t *testing.T) {
	t.Parallel()
	var exit processExitCoordinator
	if !exit.beginGraceful() {
		t.Fatal("graceful exit did not begin")
	}
	var helper helperLifetime
	helper.markInitialized()
	helper.clear() // Successful CapDestroy must not defeat the watchdog.
	stalledSync := make(chan struct{})
	exited := make(chan struct{}, 1)
	go func() {
		<-stalledSync
	}()
	if !exit.force(func() { exited <- struct{}{} }) {
		t.Fatal("watchdog could not commit after helper destroy")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("hard-exit callback stalled behind evidence sync")
	}
	close(stalledSync)
	if exit.commitQuit(func() { t.Error("WM_QUIT committed after watchdog") }) {
		t.Fatal("graceful quit beat an already committed watchdog")
	}
}

func TestR5RetryTimerFailureHasImmediateFallback(t *testing.T) {
	t.Parallel()
	if got := decideRetryTimer(0, true); got != retryTimerForceExit {
		t.Fatalf("terminal timer failure = %v", got)
	}
	if got := decideRetryTimer(0, false); got != retryTimerGracefulExit {
		t.Fatalf("idle hotkey timer failure = %v", got)
	}
	if got := decideRetryTimer(1, true); got != retryTimerScheduled {
		t.Fatalf("successful SetTimer = %v", got)
	}
}

func TestR5RepeatedEvidenceSyncFailuresExhaustBoundedRetry(t *testing.T) {
	t.Parallel()
	budget := newEvidenceRetryBudget(3)
	for attempt := uint32(1); attempt <= 3; attempt++ {
		retry, gotAttempt := budget.recordFailure()
		if gotAttempt != attempt {
			t.Fatalf("attempt = %d, want %d", gotAttempt, attempt)
		}
		if retry != (attempt < 3) {
			t.Fatalf("attempt %d retry=%v", attempt, retry)
		}
	}
	budget.reset()
	if retry, attempt := budget.recordFailure(); !retry || attempt != 1 {
		t.Fatalf("reset budget retry=%v attempt=%d", retry, attempt)
	}
}

func TestR6RuntimePermissionQueriesAreWaiterOwnedAndSerialized(t *testing.T) {
	t.Parallel()
	var coordinator permissionQueryCoordinator
	if _, _, allowed := coordinator.run(permissionQueryUI, func() (winprobe.PermissionStatus, winprobe.HResult) {
		t.Fatal("UI permission query executed")
		return winprobe.PermissionAllowed, 0
	}); allowed {
		t.Fatal("UI permission query was accepted")
	}
	var active atomic.Int32
	var maximum atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, allowed := coordinator.run(permissionQueryWaiter, func() (winprobe.PermissionStatus, winprobe.HResult) {
				now := active.Add(1)
				for previous := maximum.Load(); now > previous && !maximum.CompareAndSwap(previous, now); previous = maximum.Load() {
				}
				time.Sleep(time.Millisecond)
				active.Add(-1)
				return winprobe.PermissionAllowed, 0
			})
			if !allowed {
				t.Error("waiter permission query rejected")
			}
		}()
	}
	wg.Wait()
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent permission queries = %d", maximum.Load())
	}
}

func TestR7HelperLifetimeIsRaceSafe(t *testing.T) {
	t.Parallel()
	var helper helperLifetime
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				helper.markInitialized()
				_ = helper.isInitialized()
				helper.clear()
			}
		}()
	}
	wg.Wait()
}

func TestR8RepeatedSignalsPersistLatestAndOrderedHistory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		edge    lifecycleEdge
		reason  winprobe.CaptureReason
		mode    lifecycleMode
		signals []string
	}{
		{name: "shutdown query then confirm", edge: lifecycleSystemShutdown, reason: winprobe.ReasonShutdown, mode: lifecycleAbruptOSExit, signals: []string{"WM_QUERYENDSESSION", "WM_ENDSESSION(confirmed)"}},
		{name: "repeated suspend", edge: lifecycleSuspend, reason: winprobe.ReasonSuspend, mode: lifecycleReturnsIdle, signals: []string{"PBT_APMSUSPEND", "PBT_APMSUSPEND(repeated)"}},
		{name: "repeated lock", edge: lifecycleSessionLock, reason: winprobe.ReasonLock, mode: lifecycleReturnsIdle, signals: []string{"WTS_SESSION_LOCK", "WTS_SESSION_LOCK(repeated)"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tracker := newLifecycleTracker()
			tracker.beginLifecycle(tc.edge, tc.signals[0], tc.reason, tc.mode)
			repeated := tracker.beginLifecycle(tc.edge, tc.signals[1], tc.reason, tc.mode)
			if repeated.Progress.Signal != tc.signals[1] || repeated.Progress.RepeatedSignalCount != 1 {
				t.Fatalf("latest signal not persisted: %+v", repeated.Progress)
			}
			if len(repeated.Progress.Signals) != len(tc.signals) {
				t.Fatalf("signals = %v", repeated.Progress.Signals)
			}
			for i := range tc.signals {
				if repeated.Progress.Signals[i] != tc.signals[i] {
					t.Fatalf("signals = %v", repeated.Progress.Signals)
				}
			}
		})
	}
}

func TestR9OwnedResourceRetriesDeleteBeforeClearingOwnership(t *testing.T) {
	t.Parallel()
	var resource ownedResource
	resource.setOwned(true)
	attempts := 0
	released, called := resource.release(func() bool {
		attempts++
		return false
	})
	if released || !called || !resource.isOwned() {
		t.Fatal("failed delete cleared tray ownership")
	}
	released, called = resource.release(func() bool {
		attempts++
		return true
	})
	if !released || !called || resource.isOwned() || attempts != 2 {
		t.Fatalf("retry released=%v called=%v owned=%v attempts=%d", released, called, resource.isOwned(), attempts)
	}
}

func TestR10ProductionCoordinatorSurvivesRepeatedStartStopCycles(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	for cycle := 1; cycle <= 100; cycle++ {
		generation := bindNativeCapture(t, tracker, uint32(700+cycle))
		tracker.beginLifecycle(lifecycleSessionLock, "lock", winprobe.ReasonLock, lifecycleReturnsIdle)
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)
		settleGeneration(t, tracker, generation)
		tracker.resume(lifecycleSessionLock)
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleHotkeyUnregistered, lifecycleIdle)
		if _, ok := tracker.activeRun(lifecycleSessionLock); ok {
			t.Fatalf("cycle %d left lifecycle run", cycle)
		}
	}
}
