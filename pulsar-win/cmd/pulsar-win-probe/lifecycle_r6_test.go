package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestR6F15OpenGatePublicationConflictStopsUnpublishedOwnerOnce(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	const (
		activeOperation      = uint32(6101)
		unpublishedOperation = uint32(6102)
	)
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, activeOperation)
	{
		var helperCalls atomic.Int32
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			helperCalls.Add(1)
			return unpublishedOperation, true
		}, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
		if prepared.trackerInvoked || prepared.externalInvoked || helperCalls.Load() != 0 || owners.orphanCount() != 0 || owners.current().operationID != activeOperation {
			t.Fatalf("same-generation duplicate was not phase-rejected: prepared=%+v helper=%d orphans=%d active=%+v", prepared, helperCalls.Load(), owners.orphanCount(), owners.current())
		}
		var diagnostics, settlements atomic.Int32
		if !prepared.handleSuppressedPrepare(tracker, &owners, &shutdown, generation, func() { diagnostics.Add(1) }, func() { settlements.Add(1) }) || diagnostics.Load() != 1 || settlements.Load() != 0 {
			t.Fatalf("duplicate diagnostic=%d settlement=%d", diagnostics.Load(), settlements.Load())
		}
		return
	}
}

func TestR6F16SameGenerationConflictCannotEnterRollback(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	const (
		activeOperation      = uint32(6401)
		unpublishedOperation = uint32(6402)
	)
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, activeOperation)
	{
		var helperCalls atomic.Int32
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			helperCalls.Add(1)
			return unpublishedOperation, true
		}, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
		if prepared.trackerInvoked || prepared.externalInvoked || helperCalls.Load() != 0 || owners.orphanCount() != 0 || owners.current().operationID != activeOperation {
			t.Fatalf("same-generation rollback seam was not phase-rejected: prepared=%+v helper=%d orphans=%d active=%+v", prepared, helperCalls.Load(), owners.orphanCount(), owners.current())
		}
		return
	}
}

func TestR6F16DistinctUnpublishedGenerationMaySettleWithoutTouchingIncumbent(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	const (
		incumbentGeneration  = uint64(9001)
		incumbentOperation   = uint32(6301)
		unpublishedOperation = uint32(6302)
	)
	incumbent, conflict, published := owners.publish(&shutdown, incumbentGeneration, incumbentOperation, func(uint32) winprobe.HResult {
		t.Fatal("incumbent received shutdown stop while gate was open")
		return 1
	})
	if !published || conflict != nil || !incumbent.matches(incumbentGeneration, incumbentOperation) {
		t.Fatalf("incumbent publication owner=%+v conflict=%+v published=%v", incumbent, conflict, published)
	}
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin distinct generation: %s", reason)
	}
	var unpublishedStops atomic.Int32
	prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
		return unpublishedOperation, true
	}, func(uint32) winprobe.HResult {
		t.Fatal("unpublished operation received shutdown stop while gate was open")
		return 1
	}, func(operationID uint32) winprobe.HResult {
		if operationID != unpublishedOperation {
			t.Errorf("unpublished stop operation=%d, want %d", operationID, unpublishedOperation)
		}
		unpublishedStops.Add(1)
		return 0
	})
	if !prepared.owner.matches(generation, unpublishedOperation) || prepared.conflictingOwner != incumbent || prepared.ownerPublished || unpublishedStops.Load() != 1 {
		t.Fatalf("distinct conflict result=%+v stops=%d", prepared, unpublishedStops.Load())
	}
	if prepared.orphan == nil || owners.orphanCount() != 1 {
		t.Fatalf("distinct native loser did not retain waiter orphan ownership: prepared=%+v orphans=%d", prepared, owners.orphanCount())
	}
	if operationID, phase, exists := tracker.captureStateForGeneration(generation); !exists || operationID != unpublishedOperation || phase != captureGenerationNativeOwned {
		t.Fatalf("distinct native loser was falsely restored as ownerless: exists=%v operation=%d phase=%d", exists, operationID, phase)
	}
	drained := runCaptureOrphanDrain(prepared.orphan, func(operationID uint32) (winprobe.CaptureResult, winprobe.HResult) {
		return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
	}, func(operationID uint32) winprobe.HResult { return 0 })
	if !drained.TerminalObserved || !drained.Release.released() || !owners.completeOrphan(prepared.orphan) {
		t.Fatalf("distinct orphan drain=%+v", drained)
	}
	settleGeneration(t, tracker, generation)
	if _, _, exists := tracker.captureStateForGeneration(generation); exists || owners.orphanCount() != 0 {
		t.Fatalf("distinct orphan remained after exact terminal/release: exists=%v orphans=%d", exists, owners.orphanCount())
	}
	if owners.current() != incumbent || !incumbent.matches(incumbentGeneration, incumbentOperation) {
		t.Fatalf("distinct settlement disturbed incumbent: current=%+v", owners.current())
	}
}

func TestR6F17PrepareFailureEvidenceIsSeparateFromOwnerSuccessors(t *testing.T) {
	t.Parallel()

	t.Run("open gate logs one failure and starts no owner successor", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, accepted, reason := tracker.beginCaptureGeneration()
		if !accepted {
			t.Fatalf("begin capture: %s", reason)
		}
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			return 0, false
		}, func(uint32) winprobe.HResult {
			t.Fatal("failed prepare received shutdown stop")
			return 1
		}, func(uint32) winprobe.HResult {
			t.Fatal("failed prepare received unpublished-owner stop")
			return 1
		})
		if prepared.succeeded || !prepared.trackerInvoked || !prepared.externalInvoked || prepared.owner != nil || prepared.conflictingOwner != nil || prepared.ownerPublished || !prepared.resultEvidenceAllowed || prepared.ownerSuccessorAllowed {
			t.Fatalf("failed prepare result=%+v", prepared)
		}
		var evidence, ownerSuccessors, failureCallbacks atomic.Int32
		outcome := prepared.dispatchPostHelper(&shutdown, func() { ownerSuccessors.Add(1) }, func() bool {
			evidence.Add(1)
			return true
		}, func() { failureCallbacks.Add(1) })
		if outcome.ownerStatePublished || !outcome.evidenceAttempted || !outcome.evidencePassed || evidence.Load() != 1 || ownerSuccessors.Load() != 0 || failureCallbacks.Load() != 0 || owners.current() != nil {
			t.Fatalf("failed prepare outcome=%+v evidence=%d ownerSuccessors=%d failures=%d owner=%+v", outcome, evidence.Load(), ownerSuccessors.Load(), failureCallbacks.Load(), owners.current())
		}
	})

	t.Run("abrupt close suppresses pending failure evidence", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, accepted, reason := tracker.beginCaptureGeneration()
		if !accepted {
			t.Fatalf("begin capture: %s", reason)
		}
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			return 0, false
		}, func(uint32) winprobe.HResult { return 1 }, func(uint32) winprobe.HResult { return 1 })
		if !prepared.resultEvidenceAllowed {
			t.Fatalf("failure evidence was not initially admissible: %+v", prepared)
		}
		if !shutdown.confirmAfterStop(nil, nil) {
			t.Fatal("failed to close abrupt gate")
		}
		var evidence, ownerSuccessors, failureCallbacks atomic.Int32
		outcome := prepared.dispatchPostHelper(&shutdown, func() { ownerSuccessors.Add(1) }, func() bool {
			evidence.Add(1)
			return true
		}, func() { failureCallbacks.Add(1) })
		if outcome.ownerStatePublished || outcome.evidenceAttempted || outcome.evidencePassed || evidence.Load() != 0 || ownerSuccessors.Load() != 0 || failureCallbacks.Load() != 0 {
			t.Fatalf("abrupt close admitted failure completion: outcome=%+v evidence=%d owner=%d failures=%d", outcome, evidence.Load(), ownerSuccessors.Load(), failureCallbacks.Load())
		}
	})

	t.Run("failed duplicate preserves incumbent and logs only attempt", func(t *testing.T) {
		tracker := newLifecycleTracker()
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		const (
			activeOperation = uint32(6501)
			failedOperation = uint32(6502)
		)
		generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, activeOperation)
		var helperCalls atomic.Int32
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			helperCalls.Add(1)
			return failedOperation, false
		}, func(uint32) winprobe.HResult {
			t.Fatal("failed duplicate received shutdown stop")
			return 1
		}, func(uint32) winprobe.HResult {
			t.Fatal("failed duplicate received unpublished-owner stop")
			return 1
		})
		if prepared.operationID != 0 || prepared.succeeded || prepared.trackerInvoked || prepared.externalInvoked || prepared.owner != nil || prepared.resultEvidenceAllowed || prepared.ownerSuccessorAllowed || helperCalls.Load() != 0 {
			t.Fatalf("failed duplicate result=%+v", prepared)
		}
		if operationID, phase, exists := tracker.captureStateForGeneration(generation); !exists || operationID != activeOperation || phase != captureGenerationNativeOwned {
			t.Fatalf("failed duplicate changed tracker: exists=%v operation=%d phase=%d", exists, operationID, phase)
		}
		if !owners.current().matches(generation, activeOperation) {
			t.Fatalf("failed duplicate changed incumbent: %+v", owners.current())
		}
		var evidence, ownerSuccessors, failureCallbacks atomic.Int32
		outcome := prepared.dispatchPostHelper(&shutdown, func() { ownerSuccessors.Add(1) }, func() bool {
			evidence.Add(1)
			return true
		}, func() { failureCallbacks.Add(1) })
		if outcome.ownerStatePublished || outcome.evidenceAttempted || outcome.evidencePassed || evidence.Load() != 0 || ownerSuccessors.Load() != 0 || failureCallbacks.Load() != 0 {
			t.Fatalf("failed duplicate outcome=%+v evidence=%d ownerSuccessors=%d failures=%d", outcome, evidence.Load(), ownerSuccessors.Load(), failureCallbacks.Load())
		}
	})

	t.Run("published success keeps result then owner admissions", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, accepted, reason := tracker.beginCaptureGeneration()
		if !accepted {
			t.Fatalf("begin capture: %s", reason)
		}
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		const operationID = uint32(6601)
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			return operationID, true
		}, func(uint32) winprobe.HResult {
			t.Fatal("published success received premature shutdown stop")
			return 1
		}, func(uint32) winprobe.HResult {
			t.Fatal("published success received unpublished-owner stop")
			return 1
		})
		if !prepared.succeeded || !prepared.ownerPublished || !prepared.resultEvidenceAllowed || !prepared.ownerSuccessorAllowed || !prepared.owner.matches(generation, operationID) {
			t.Fatalf("published success result=%+v", prepared)
		}
		var order atomic.Int32
		if !prepared.dispatchResultEvidence(&shutdown, func() {
			if step := order.Add(1); step != 1 {
				t.Errorf("result evidence order=%d, want 1", step)
			}
		}) {
			t.Fatal("published success result evidence was suppressed")
		}
		if !prepared.dispatchOwnerSuccessor(&shutdown, func() {
			if step := order.Add(1); step != 2 {
				t.Errorf("owner successor order=%d, want 2", step)
			}
		}) {
			t.Fatal("published success owner successor was suppressed")
		}
		if order.Load() != 2 {
			t.Fatalf("published success callbacks=%d, want 2", order.Load())
		}
	})
}

func TestR6F18SuppressedDuplicatePreservesExactNativeIncumbent(t *testing.T) {
	t.Parallel()

	t.Run("same generation native incumbent is diagnostic only", func(t *testing.T) {
		tracker := newLifecycleTracker()
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		const activeOperation = uint32(6701)
		generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, activeOperation)
		var lifecycleStops atomic.Int32
		plan, stopHR := tracker.beginLifecycleStop(lifecycleSessionLock, "WTS_SESSION_LOCK-before-queued-duplicate", winprobe.ReasonLock, lifecycleReturnsIdle, func(stopGeneration uint64, operationID uint32) winprobe.HResult {
			if stopGeneration != generation {
				t.Errorf("lifecycle stop generation=%d, want %d", stopGeneration, generation)
			}
			if operationID != activeOperation {
				t.Errorf("lifecycle stop operation=%d, want incumbent %d", operationID, activeOperation)
			}
			lifecycleStops.Add(1)
			return 0
		})
		if stopHR.Failed() || plan.Generation != generation || plan.Capture != activeOperation || lifecycleStops.Load() != 1 {
			t.Fatalf("lifecycle stop plan=%+v hr=%s stops=%d", plan, stopHR.Hex(), lifecycleStops.Load())
		}
		advanceRun(t, tracker, lifecycleSessionLock, lifecycleStopRequested)

		var helperCalls atomic.Int32
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			helperCalls.Add(1)
			return 6702, true
		}, func(uint32) winprobe.HResult {
			t.Fatal("refused duplicate received shutdown stop")
			return 1
		}, func(uint32) winprobe.HResult {
			t.Fatal("refused duplicate received unpublished-owner stop")
			return 1
		})
		if prepared.trackerInvoked || prepared.externalInvoked || helperCalls.Load() != 0 {
			t.Fatalf("lifecycle-gated duplicate reached helper: result=%+v calls=%d", prepared, helperCalls.Load())
		}
		var diagnostics, falseSettlement atomic.Int32
		if !prepared.handleSuppressedPrepare(tracker, &owners, &shutdown, generation, func() {
			diagnostics.Add(1)
		}, func() {
			falseSettlement.Add(1)
		}) {
			t.Fatal("refused duplicate diagnostic was not admitted")
		}
		if diagnostics.Load() != 1 || falseSettlement.Load() != 0 {
			t.Fatalf("refused duplicate effects diagnostic=%d falseSettlement=%d", diagnostics.Load(), falseSettlement.Load())
		}
		if operationID, phase, exists := tracker.captureStateForGeneration(generation); !exists || operationID != activeOperation || phase != captureGenerationNativeOwned {
			t.Fatalf("refused duplicate changed incumbent tracker: exists=%v operation=%d phase=%d", exists, operationID, phase)
		}
		if run, ok := tracker.activeRun(lifecycleSessionLock); !ok || run.Stage != lifecycleStopRequested || run.CaptureOperationID != activeOperation {
			t.Fatalf("refused duplicate fabricated lifecycle settlement: run=%+v ok=%v", run, ok)
		}
		if !owners.current().matches(generation, activeOperation) {
			t.Fatalf("refused duplicate changed atomic incumbent: %+v", owners.current())
		}

		settleGeneration(t, tracker, generation)
		if run, ok := tracker.activeRun(lifecycleSessionLock); !ok || run.Stage != lifecycleCaptureReleased || run.CaptureOperationID != activeOperation {
			t.Fatalf("actual incumbent settlement did not complete normally: run=%+v ok=%v", run, ok)
		}
		owner := owners.matching(generation, activeOperation)
		if owner == nil || !owner.observeNativeTerminal() || !owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult { return 0 }).released() || !owners.clearReleased(owner) || owners.current() != nil {
			t.Fatal("matching actual release did not clear the incumbent")
		}
	})

	t.Run("distinct pre-native stale generation retains settlement", func(t *testing.T) {
		tracker := newLifecycleTracker()
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		generation, accepted, reason := tracker.beginCaptureGeneration()
		if !accepted {
			t.Fatalf("begin capture: %s", reason)
		}
		if _, succeeded, invoked := tracker.runPermissionRequest(generation, func() (uint32, bool) { return 1, true }); !succeeded || !invoked {
			t.Fatal("pre-native permission request was not established")
		}
		const unrelatedOperation = uint32(6801)
		unrelated, conflict, published := owners.publish(&shutdown, generation+100, unrelatedOperation, func(uint32) winprobe.HResult { return 1 })
		if !published || conflict != nil || !unrelated.matches(generation+100, unrelatedOperation) {
			t.Fatalf("unrelated incumbent publication owner=%+v conflict=%+v published=%v", unrelated, conflict, published)
		}
		plan, stopHR := tracker.beginLifecycleStop(lifecycleSuspend, "PBT_APMSUSPEND-pre-native", winprobe.ReasonSuspend, lifecycleReturnsIdle, func(uint64, uint32) winprobe.HResult {
			t.Fatal("pre-native lifecycle stop called native stop")
			return 1
		})
		if stopHR.Failed() || plan.Generation != generation || plan.Capture != 0 || plan.Phase != captureGenerationPermissionPending {
			t.Fatalf("pre-native lifecycle stop plan=%+v hr=%s", plan, stopHR.Hex())
		}
		advanceRun(t, tracker, lifecycleSuspend, lifecycleStopRequested)
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			t.Fatal("pre-native invalidated generation reached helper")
			return 0, false
		}, func(uint32) winprobe.HResult { return 1 }, func(uint32) winprobe.HResult { return 1 })
		var diagnostics, settlements atomic.Int32
		if !prepared.handleSuppressedPrepare(tracker, &owners, &shutdown, generation, func() {
			diagnostics.Add(1)
		}, func() {
			settlements.Add(1)
			settleGeneration(t, tracker, generation)
		}) {
			t.Fatal("distinct pre-native stale generation was not handled")
		}
		if diagnostics.Load() != 1 || settlements.Load() != 1 {
			t.Fatalf("distinct stale effects diagnostic=%d settlement=%d", diagnostics.Load(), settlements.Load())
		}
		if run, ok := tracker.activeRun(lifecycleSuspend); !ok || run.Stage != lifecycleCaptureReleased || run.CaptureOperationID != 0 {
			t.Fatalf("distinct stale generation did not settle: run=%+v ok=%v", run, ok)
		}
		if owners.current() != unrelated {
			t.Fatalf("distinct stale settlement disturbed unrelated incumbent: %+v", owners.current())
		}
	})
}

func newR6PublishedPrepare(t *testing.T, operationID uint32) (*lifecycleTracker, capturePrepareCoordinatorResult, *abruptShutdownCoordinator, *captureOwnershipCoordinator) {
	t.Helper()
	tracker := newLifecycleTracker()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin capture: %s", reason)
	}
	shutdown := &abruptShutdownCoordinator{}
	owners := &captureOwnershipCoordinator{}
	prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) {
		return operationID, true
	}, func(uint32) winprobe.HResult {
		t.Fatal("published prepare received premature shutdown stop")
		return 1
	}, func(uint32) winprobe.HResult {
		t.Fatal("published prepare received unpublished-owner stop")
		return 1
	})
	if !prepared.owner.matches(generation, operationID) || !prepared.ownerPublished || !prepared.resultEvidenceAllowed || !prepared.ownerSuccessorAllowed {
		t.Fatalf("published prepare result=%+v", prepared)
	}
	return tracker, prepared, shutdown, owners
}

func TestR6F19PublishedStatePrecedesBlockingResultEvidence(t *testing.T) {
	t.Parallel()

	t.Run("readiness consumed during blocked evidence sees exact state", func(t *testing.T) {
		const operationID = uint32(6901)
		_, prepared, shutdown, _ := newR6PublishedPrepare(t, operationID)
		var appOperation atomic.Uint32
		evidenceEntered := make(chan struct{})
		evidenceRelease := make(chan struct{})
		outcomeCh := make(chan capturePrepareCompletionOutcome, 1)
		var failureCallbacks atomic.Int32
		go func() {
			outcomeCh <- prepared.dispatchPostHelper(shutdown, func() {
				appOperation.Store(operationID)
			}, func() bool {
				close(evidenceEntered)
				<-evidenceRelease
				return true
			}, func() { failureCallbacks.Add(1) })
		}()
		<-evidenceEntered
		if observed := appOperation.Load(); observed != operationID {
			t.Fatalf("readiness consumed before exact app state publication: operation=%d", observed)
		}
		var activations atomic.Int32
		if appOperation.Load() == operationID {
			activations.Add(1)
		}
		close(evidenceRelease)
		outcome := <-outcomeCh
		if !outcome.ownerStatePublished || !outcome.evidenceAttempted || !outcome.evidencePassed || activations.Load() != 1 || failureCallbacks.Load() != 0 {
			t.Fatalf("blocked evidence outcome=%+v activations=%d failures=%d", outcome, activations.Load(), failureCallbacks.Load())
		}
	})

	t.Run("evidence failure stops exact owner once after state publication", func(t *testing.T) {
		const operationID = uint32(6902)
		_, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		var appOperation atomic.Uint32
		var stopCalls atomic.Int32
		var stoppedOperation atomic.Uint32
		outcome := prepared.dispatchPostHelper(shutdown, func() {
			appOperation.Store(operationID)
		}, func() bool {
			if appOperation.Load() != operationID {
				t.Error("evidence failure ran before exact app state publication")
			}
			return false
		}, func() {
			prepared.owner.requestStop(func(stopped uint32) winprobe.HResult {
				stoppedOperation.Store(stopped)
				stopCalls.Add(1)
				return 0
			})
		})
		if !outcome.ownerStatePublished || !outcome.evidenceAttempted || outcome.evidencePassed || appOperation.Load() != operationID || stoppedOperation.Load() != operationID || stopCalls.Load() != 1 {
			t.Fatalf("evidence failure outcome=%+v state=%d stopped=%d calls=%d", outcome, appOperation.Load(), stoppedOperation.Load(), stopCalls.Load())
		}
		var terminalFallbackStops atomic.Int32
		if result := requestCaptureStopOrReuse(prepared.owner, operationID, func(uint32) winprobe.HResult {
			terminalFallbackStops.Add(1)
			return 1
		}); !result.completed() || result.Result.Failed() || terminalFallbackStops.Load() != 0 {
			t.Fatalf("terminal cleanup duplicated exact stop: result=%+v fallbackStops=%d", result, terminalFallbackStops.Load())
		}
		if prepared.owner.requestShutdownStop(func(uint32) winprobe.HResult {
			stopCalls.Add(1)
			return 0
		}) {
			t.Fatal("evidence-failed owner accepted duplicate shutdown stop")
		}
		if stopCalls.Load() != 1 || owners.current() != prepared.owner {
			t.Fatalf("evidence-failure ownership calls=%d current=%+v", stopCalls.Load(), owners.current())
		}
	})
}

func TestR6F20StoppedOwnerRejectsQueuedActivation(t *testing.T) {
	t.Parallel()

	t.Run("evidence failure stop wins before queued readiness", func(t *testing.T) {
		const operationID = uint32(7001)
		tracker, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		var appOperation, stopCalls, intentEvidence, nativeActivations atomic.Int32
		evidenceEntered := make(chan struct{})
		evidenceRelease := make(chan struct{})
		outcomeCh := make(chan capturePrepareCompletionOutcome, 1)
		go func() {
			outcomeCh <- prepared.dispatchPostHelper(shutdown, func() {
				appOperation.Store(int32(operationID))
			}, func() bool {
				close(evidenceEntered)
				<-evidenceRelease
				return false
			}, func() {
				prepared.owner.requestStop(func(stopped uint32) winprobe.HResult {
					if stopped != operationID {
						t.Errorf("evidence failure stopped operation=%d, want %d", stopped, operationID)
					}
					stopCalls.Add(1)
					return 0
				})
			})
		}()
		<-evidenceEntered
		if appOperation.Load() != int32(operationID) {
			t.Fatal("queued readiness did not observe the published operation")
		}
		// Readiness is now queued but deliberately delivered only after the
		// required prepare row reports failure and claims the exact stop.
		close(evidenceRelease)
		outcome := <-outcomeCh
		if !outcome.ownerStatePublished || !outcome.evidenceAttempted || outcome.evidencePassed || stopCalls.Load() != 1 {
			t.Fatalf("evidence-failure outcome=%+v stops=%d", outcome, stopCalls.Load())
		}
		owner := admitCaptureActivationOwned(owners, shutdown, prepared.owner.generation, operationID, func(uint32) winprobe.HResult {
			stopCalls.Add(1)
			return 0
		})
		if owner != nil {
			intentEvidence.Add(1)
			result := runCaptureActivationAdmitted(tracker, owners, shutdown, owner, func() {}, func() {
				nativeActivations.Add(1)
			}, func(uint32) winprobe.HResult {
				stopCalls.Add(1)
				return 0
			})
			if result.trackerInvoked || result.externalInvoked {
				t.Fatalf("stopped owner admitted queued activation: %+v", result)
			}
		}
		if intentEvidence.Load() != 0 || nativeActivations.Load() != 0 {
			t.Fatalf("stopped owner successors intent=%d native=%d", intentEvidence.Load(), nativeActivations.Load())
		}
		var fallbackStops atomic.Int32
		plan, hr := tracker.beginLifecycleStop(lifecycleQuit, "evidence-failure-terminal-drain", winprobe.ReasonCancel, lifecycleGracefulExit, func(stopGeneration uint64, stopped uint32) winprobe.HResult {
			if stopGeneration != prepared.owner.generation {
				t.Errorf("terminal stop generation=%d, want %d", stopGeneration, prepared.owner.generation)
			}
			stop := requestCaptureStopOrReuse(prepared.owner, stopped, func(uint32) winprobe.HResult {
				fallbackStops.Add(1)
				return 1
			})
			if !stop.completed() {
				t.Errorf("recorded evidence-failure stop is not completed: %+v", stop)
			}
			return stop.Result
		})
		if hr.Failed() || plan.Capture != operationID || plan.Generation != prepared.owner.generation || fallbackStops.Load() != 0 {
			t.Fatalf("terminal drain plan=%+v hr=%s fallback=%d", plan, hr.Hex(), fallbackStops.Load())
		}
		advanceRun(t, tracker, lifecycleQuit, lifecycleStopRequested)
		settleGeneration(t, tracker, prepared.owner.generation)
		if run, ok := tracker.activeRun(lifecycleQuit); !ok || run.Stage != lifecycleCaptureReleased {
			t.Fatalf("terminal/release settlement run=%+v ok=%v", run, ok)
		}
		if prepared.owner.requestShutdownStop(func(uint32) winprobe.HResult {
			stopCalls.Add(1)
			return 0
		}) {
			t.Fatal("shutdown accepted a second stop for evidence-failed owner")
		}
		release := prepared.owner.requestRelease(captureReleaseAfterAcceptedStop, func(uint32) winprobe.HResult { return 0 })
		if !release.released() || !owners.clearReleased(prepared.owner) || owners.current() != nil || stopCalls.Load() != 1 {
			t.Fatalf("release cleanup owner=%+v stops=%d", owners.current(), stopCalls.Load())
		}
	})

	t.Run("native admission wins before evidence failure stop", func(t *testing.T) {
		const operationID = uint32(7002)
		tracker, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		evidenceEntered := make(chan struct{})
		evidenceRelease := make(chan struct{})
		activationEntered := make(chan struct{})
		activationRelease := make(chan struct{})
		outcomeCh := make(chan capturePrepareCompletionOutcome, 1)
		activationCh := make(chan captureActivationCoordinatorResult, 1)
		var statePublished, intentEvidence, nativeActivations, stopCalls atomic.Int32
		go func() {
			outcomeCh <- prepared.dispatchPostHelper(shutdown, func() {
				statePublished.Store(1)
			}, func() bool {
				close(evidenceEntered)
				<-evidenceRelease
				return false
			}, func() {
				prepared.owner.requestStop(func(stopped uint32) winprobe.HResult {
					if stopped != operationID || nativeActivations.Load() != 1 {
						t.Errorf("stop did not follow admitted native activation: operation=%d activations=%d", stopped, nativeActivations.Load())
					}
					stopCalls.Add(1)
					return 0
				})
			})
		}()
		<-evidenceEntered
		owner := admitCaptureActivationOwned(owners, shutdown, prepared.owner.generation, operationID, func(uint32) winprobe.HResult {
			stopCalls.Add(1)
			return 0
		})
		if owner != prepared.owner || statePublished.Load() != 1 {
			t.Fatalf("activation admission owner=%+v state=%d", owner, statePublished.Load())
		}
		intentEvidence.Add(1)
		go func() {
			activationCh <- runCaptureActivationAdmitted(tracker, owners, shutdown, owner, func() {}, func() {
				nativeActivations.Add(1)
				close(activationEntered)
				<-activationRelease
			}, func(uint32) winprobe.HResult {
				stopCalls.Add(1)
				return 0
			})
		}()
		<-activationEntered
		close(evidenceRelease)
		outcome := <-outcomeCh
		if outcome.evidencePassed || stopCalls.Load() != 0 || nativeActivations.Load() != 1 {
			t.Fatalf("activation-winner outcome=%+v stops=%d native=%d", outcome, stopCalls.Load(), nativeActivations.Load())
		}
		close(activationRelease)
		activated := <-activationCh
		if !activated.trackerInvoked || !activated.externalInvoked || intentEvidence.Load() != 1 || stopCalls.Load() != 1 {
			t.Fatalf("activation winner result=%+v intent=%d stops=%d", activated, intentEvidence.Load(), stopCalls.Load())
		}
		var duplicateStops atomic.Int32
		_ = requestCaptureStopOrReuse(prepared.owner, operationID, func(uint32) winprobe.HResult {
			duplicateStops.Add(1)
			return 1
		})
		if prepared.owner.requestShutdownStop(func(uint32) winprobe.HResult {
			duplicateStops.Add(1)
			return 0
		}) || duplicateStops.Load() != 0 || stopCalls.Load() != 1 {
			t.Fatalf("activation-winner duplicate stop=%d original=%d", duplicateStops.Load(), stopCalls.Load())
		}
	})
}

func TestR6F21PendingStopRetainsReleaseOwnership(t *testing.T) {
	t.Parallel()

	const operationID = uint32(7101)
	_, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
	stopEntered := make(chan struct{})
	stopRelease := make(chan struct{})
	winnerOutcome := make(chan captureStopOutcome, 1)
	var nativeStops, fallbackStops, finalizations, releases, wakes atomic.Int32
	go func() {
		winnerOutcome <- requestCaptureStopOrReuse(prepared.owner, operationID, func(stopped uint32) winprobe.HResult {
			if stopped != operationID {
				t.Errorf("winner stopped operation=%d, want %d", stopped, operationID)
			}
			nativeStops.Add(1)
			close(stopEntered)
			<-stopRelease
			return winprobe.HResult(-1)
		})
	}()
	<-stopEntered

	pending := runCaptureQueryFailureCleanup(prepared.owner, operationID, func(uint32) winprobe.HResult {
		fallbackStops.Add(1)
		return 0
	}, func() error {
		finalizations.Add(1)
		return nil
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !pending.Stop.pending() || pending.FinalizeAttempted || pending.ReleaseAttempted || nativeStops.Load() != 1 || fallbackStops.Load() != 0 || finalizations.Load() != 0 || releases.Load() != 0 {
		t.Fatalf("pending cleanup=%+v native=%d fallback=%d finalize=%d release=%d", pending, nativeStops.Load(), fallbackStops.Load(), finalizations.Load(), releases.Load())
	}
	terminal := requestCaptureStopOrReuse(prepared.owner, operationID, func(uint32) winprobe.HResult {
		fallbackStops.Add(1)
		return 0
	})
	if !terminal.pending() || fallbackStops.Load() != 0 {
		t.Fatalf("concurrent terminal stop=%+v fallback=%d", terminal, fallbackStops.Load())
	}

	adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
	confirmed := make(chan bool, 1)
	go func() {
		confirmed <- adapter.confirm(func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		}, func() { wakes.Add(1) })
	}()
	select {
	case accepted := <-confirmed:
		if !accepted {
			t.Fatal("confirmation rejected while ordinary stop was pending")
		}
	case <-time.After(time.Second):
		t.Fatal("confirmation waited for the in-flight ordinary stop")
	}
	if !shutdown.isConfirmed() || wakes.Load() != 1 || fallbackStops.Load() != 0 || finalizations.Load() != 0 || releases.Load() != 0 {
		t.Fatalf("confirmation confirmed=%v wakes=%d fallback=%d finalize=%d release=%d", shutdown.isConfirmed(), wakes.Load(), fallbackStops.Load(), finalizations.Load(), releases.Load())
	}

	close(stopRelease)
	winner := <-winnerOutcome
	if !winner.completed() || winner.Result != winprobe.HResult(-1) || nativeStops.Load() != 1 {
		t.Fatalf("winner result=%+v nativeStops=%d", winner, nativeStops.Load())
	}
	if shutdown.runOrdinary(func() {
		t.Fatal("confirmed shutdown admitted an ordinary cleanup retry")
	}) || fallbackStops.Load() != 0 || finalizations.Load() != 0 || releases.Load() != 0 {
		t.Fatalf("post-confirmation cleanup fallback=%d finalize=%d release=%d", fallbackStops.Load(), finalizations.Load(), releases.Load())
	}

	// A separate ordinary (non-confirmed) retry proves that pending retains
	// ownership only until the recorded result is visible, after which the real
	// artifact/native release path executes exactly once.
	const retryOperation = uint32(7102)
	_, retryPrepared, retryShutdown, retryOwners := newR6PublishedPrepare(t, retryOperation)
	retryStopEntered := make(chan struct{})
	retryStopRelease := make(chan struct{})
	retryWinner := make(chan captureStopOutcome, 1)
	go func() {
		retryWinner <- requestCaptureStopOrReuse(retryPrepared.owner, retryOperation, func(uint32) winprobe.HResult {
			nativeStops.Add(1)
			close(retryStopEntered)
			<-retryStopRelease
			return winprobe.HResult(-2)
		})
	}()
	<-retryStopEntered
	retained := runCaptureQueryFailureCleanup(retryPrepared.owner, retryOperation, func(uint32) winprobe.HResult {
		fallbackStops.Add(1)
		return 0
	}, func() error {
		finalizations.Add(1)
		return nil
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !retained.Stop.pending() || retained.FinalizeAttempted || retained.ReleaseAttempted {
		t.Fatalf("ordinary pending cleanup=%+v", retained)
	}
	close(retryStopRelease)
	if completed := <-retryWinner; !completed.completed() || completed.Result != winprobe.HResult(-2) {
		t.Fatalf("ordinary completed stop=%+v", completed)
	}
	var retry captureQueryFailureCleanupResult
	if !retryShutdown.runOrdinary(func() {
		retry = runCaptureQueryFailureCleanup(retryPrepared.owner, retryOperation, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		}, func() error {
			finalizations.Add(1)
			return nil
		}, func(released uint32) winprobe.HResult {
			if released != retryOperation {
				t.Errorf("released operation=%d, want %d", released, retryOperation)
			}
			releases.Add(1)
			return 0
		})
	}) {
		t.Fatal("ordinary retry was unexpectedly gated")
	}
	if !retry.Stop.completed() || retry.Stop.Result != winprobe.HResult(-2) || !retry.StructuralFailure || retry.FinalizeAttempted || retry.ReleaseAttempted || retry.Released {
		t.Fatalf("completed ordinary retry=%+v", retry)
	}
	if fallbackStops.Load() != 0 || nativeStops.Load() != 2 || finalizations.Load() != 0 || releases.Load() != 0 {
		t.Fatalf("retry counters native=%d fallback=%d finalize=%d release=%d", nativeStops.Load(), fallbackStops.Load(), finalizations.Load(), releases.Load())
	}
	if retryOwners.current() != retryPrepared.owner {
		t.Fatal("failed stop did not retain the exact owner for bounded escalation")
	}
}

func TestR6F22NativeCallOwnershipDefersStop(t *testing.T) {
	t.Parallel()

	t.Run("native admission defers query hotkey and confirmation stop", func(t *testing.T) {
		const operationID = uint32(7201)
		tracker, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		owner := admitCaptureActivationOwned(owners, shutdown, prepared.owner.generation, operationID, func(uint32) winprobe.HResult {
			t.Fatal("activation admission unexpectedly requested shutdown stop")
			return 0
		})
		if owner != prepared.owner {
			t.Fatalf("activation owner=%+v, want exact published owner", owner)
		}

		callOwned := make(chan struct{})
		allowNativeCall := make(chan struct{})
		activationResult := make(chan captureActivationCoordinatorResult, 1)
		var nativeActivations, nativeStops, fallbackStops, finalizations, releases, wakes atomic.Int32
		var orderMu sync.Mutex
		var order []string
		record := func(value string) {
			orderMu.Lock()
			order = append(order, value)
			orderMu.Unlock()
		}
		go func() {
			activationResult <- runCaptureActivationAdmitted(tracker, owners, shutdown, owner, func() {}, func() {
				// The callback is the real helper seam. Blocking before the fake
				// native call reproduces preemption immediately after owner admission.
				close(callOwned)
				<-allowNativeCall
				record("activate")
				nativeActivations.Add(1)
			}, func(uint32) winprobe.HResult {
				fallbackStops.Add(1)
				return 0
			})
		}()
		<-callOwned

		queryStop := requestCaptureStopOrReuse(owner, operationID, func(stopped uint32) winprobe.HResult {
			if stopped != operationID {
				t.Errorf("deferred stop operation=%d, want %d", stopped, operationID)
			}
			record("stop")
			nativeStops.Add(1)
			return winprobe.HResult(-3)
		})
		if !queryStop.pending() || nativeStops.Load() != 0 {
			t.Fatalf("query stop was not deferred: outcome=%+v nativeStops=%d", queryStop, nativeStops.Load())
		}
		hotkeyStop := requestCaptureStopOrReuse(owner, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		})
		if !hotkeyStop.pending() || fallbackStops.Load() != 0 {
			t.Fatalf("hotkey stop duplicated deferred owner: outcome=%+v fallback=%d", hotkeyStop, fallbackStops.Load())
		}
		pendingCleanup := runCaptureQueryFailureCleanup(owner, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		}, func() error {
			finalizations.Add(1)
			return nil
		}, func(uint32) winprobe.HResult {
			releases.Add(1)
			return 0
		})
		if !pendingCleanup.Stop.pending() || pendingCleanup.FinalizeAttempted || pendingCleanup.ReleaseAttempted || finalizations.Load() != 0 || releases.Load() != 0 {
			t.Fatalf("pre-activation cleanup=%+v finalize=%d release=%d", pendingCleanup, finalizations.Load(), releases.Load())
		}

		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		if !adapter.confirm(func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		}, func() { wakes.Add(1) }) {
			t.Fatal("confirmation was rejected")
		}
		if wakes.Load() != 1 || nativeStops.Load() != 0 || fallbackStops.Load() != 0 {
			t.Fatalf("confirmation waited/duplicated: wakes=%d nativeStops=%d fallback=%d", wakes.Load(), nativeStops.Load(), fallbackStops.Load())
		}

		close(allowNativeCall)
		activated := <-activationResult
		if !activated.trackerInvoked || !activated.externalInvoked {
			t.Fatalf("admitted activation result=%+v", activated)
		}
		orderMu.Lock()
		gotOrder := append([]string(nil), order...)
		orderMu.Unlock()
		if len(gotOrder) != 1 || gotOrder[0] != "activate" || nativeActivations.Load() != 1 || nativeStops.Load() != 0 {
			t.Fatalf("native order=%v activations=%d stops=%d", gotOrder, nativeActivations.Load(), nativeStops.Load())
		}
		stillPending := runCaptureQueryFailureCleanup(owner, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		}, func() error {
			finalizations.Add(1)
			return nil
		}, func(uint32) winprobe.HResult {
			releases.Add(1)
			return 0
		})
		if !stillPending.Stop.pending() || stillPending.FinalizeAttempted || stillPending.ReleaseAttempted || finalizations.Load() != 0 || releases.Load() != 0 {
			t.Fatalf("in-flight stop cleanup=%+v finalize=%d release=%d", stillPending, finalizations.Load(), releases.Load())
		}
		if result, ok := owner.completedStopResult(); ok || result != 0 || nativeStops.Load() != 0 || fallbackStops.Load() != 0 {
			t.Fatalf("post-latch deferred stop result=%s ok=%v native=%d fallback=%d", result.Hex(), ok, nativeStops.Load(), fallbackStops.Load())
		}
		if shutdown.runOrdinary(func() {
			t.Fatal("confirmed shutdown admitted release/finalization")
		}) || finalizations.Load() != 0 || releases.Load() != 0 {
			t.Fatalf("post-confirmation cleanup finalize=%d release=%d", finalizations.Load(), releases.Load())
		}
	})

	t.Run("stop first rejects activation before native call ownership", func(t *testing.T) {
		const operationID = uint32(7202)
		_, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		var stops, activationEvidence, nativeActivations atomic.Int32
		stop := requestCaptureStopOrReuse(prepared.owner, operationID, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		})
		if !stop.completed() || stops.Load() != 1 {
			t.Fatalf("stop-first outcome=%+v stops=%d", stop, stops.Load())
		}
		owner := admitCaptureActivationOwned(owners, shutdown, prepared.owner.generation, operationID, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		})
		if owner != nil {
			activationEvidence.Add(1)
			nativeActivations.Add(1)
		}
		if activationEvidence.Load() != 0 || nativeActivations.Load() != 0 || stops.Load() != 1 {
			t.Fatalf("stop-first successors evidence=%d native=%d stops=%d", activationEvidence.Load(), nativeActivations.Load(), stops.Load())
		}
	})
}

func TestR6F23AdmissionAlwaysCompletesOnClosingRace(t *testing.T) {
	t.Parallel()

	t.Run("close after admission before native call abandons and stops once", func(t *testing.T) {
		const operationID = uint32(7301)
		tracker, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		owner := admitCaptureActivationOwned(owners, shutdown, prepared.owner.generation, operationID, func(uint32) winprobe.HResult {
			t.Fatal("intent admission unexpectedly stopped owner")
			return 0
		})
		if owner != prepared.owner {
			t.Fatalf("intent owner=%+v, want exact published owner", owner)
		}
		admitted := make(chan struct{})
		releaseAdmission := make(chan struct{})
		activationResult := make(chan captureActivationCoordinatorResult, 1)
		var activations, stops, fallbackStops, finalizations, releases, wakes atomic.Int32
		go func() {
			activationResult <- runCaptureActivationAdmitted(tracker, owners, shutdown, owner, func() {
				close(admitted)
				<-releaseAdmission
			}, func() {
				activations.Add(1)
			}, func(uint32) winprobe.HResult {
				fallbackStops.Add(1)
				return 0
			})
		}()
		<-admitted

		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		confirmed := make(chan bool, 1)
		go func() {
			confirmed <- adapter.confirm(func(stopped uint32) winprobe.HResult {
				if stopped != operationID {
					t.Errorf("deferred confirmation stop operation=%d, want %d", stopped, operationID)
				}
				stops.Add(1)
				return winprobe.HResult(-4)
			}, func() { wakes.Add(1) })
		}()
		select {
		case accepted := <-confirmed:
			if !accepted {
				t.Fatal("confirmation rejected after native admission")
			}
		case <-time.After(time.Second):
			t.Fatal("confirmation waited for the post-admission barrier")
		}
		if wakes.Load() != 1 || stops.Load() != 0 || activations.Load() != 0 {
			t.Fatalf("pre-release confirmation wakes=%d stops=%d activations=%d", wakes.Load(), stops.Load(), activations.Load())
		}
		pending := runCaptureQueryFailureCleanup(owner, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		}, func() error {
			finalizations.Add(1)
			return nil
		}, func(uint32) winprobe.HResult {
			releases.Add(1)
			return 0
		})
		if !pending.Stop.pending() || pending.FinalizeAttempted || pending.ReleaseAttempted {
			t.Fatalf("post-admission pending cleanup=%+v", pending)
		}

		close(releaseAdmission)
		activated := <-activationResult
		if !activated.trackerInvoked || activated.externalInvoked || activated.continuationAllowed {
			t.Fatalf("abandoned activation result=%+v", activated)
		}
		if activations.Load() != 0 || stops.Load() != 0 || finalizations.Load() != 0 || releases.Load() != 0 {
			t.Fatalf("abandoned activation effects activations=%d stops=%d finalize=%d release=%d", activations.Load(), stops.Load(), finalizations.Load(), releases.Load())
		}
		stillPending := requestCaptureStopOrReuse(owner, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		})
		if !stillPending.pending() || fallbackStops.Load() != 0 {
			t.Fatalf("in-flight deferred stop=%+v fallback=%d", stillPending, fallbackStops.Load())
		}
		if result, ok := owner.completedStopResult(); ok || result != 0 || stops.Load() != 0 || fallbackStops.Load() != 0 {
			t.Fatalf("abandoned post-latch stop result=%s ok=%v stops=%d fallback=%d", result.Hex(), ok, stops.Load(), fallbackStops.Load())
		}
		if shutdown.runOrdinary(func() {
			t.Fatal("confirmed close admitted finalization/release")
		}) || finalizations.Load() != 0 || releases.Load() != 0 {
			t.Fatalf("post-close cleanup finalize=%d release=%d", finalizations.Load(), releases.Load())
		}
	})

	t.Run("closing before admission stops owner and invokes no activation", func(t *testing.T) {
		const operationID = uint32(7302)
		_, prepared, shutdown, owners := newR6PublishedPrepare(t, operationID)
		var stops, wakes, activations atomic.Int32
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		if !adapter.confirm(func(stopped uint32) winprobe.HResult {
			if stopped != operationID {
				t.Errorf("closing-first stop operation=%d, want %d", stopped, operationID)
			}
			stops.Add(1)
			return 0
		}, func() { wakes.Add(1) }) {
			t.Fatal("closing-first confirmation rejected")
		}
		owner := admitCaptureActivationOwned(owners, shutdown, prepared.owner.generation, operationID, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		})
		if owner != nil {
			activations.Add(1)
		}
		if stops.Load() != 1 || wakes.Load() != 1 || activations.Load() != 0 {
			t.Fatalf("closing-first effects stops=%d wakes=%d activations=%d", stops.Load(), wakes.Load(), activations.Load())
		}
	})
}

func TestR6F24ExactOwnerStopNeverFallsBackAfterRelease(t *testing.T) {
	t.Parallel()

	t.Run("lifecycle plan released before callback issues zero stale stop", func(t *testing.T) {
		tracker := newLifecycleTracker()
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		const operationID = uint32(7401)
		generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, operationID)
		var nativeStops atomic.Int32
		var stopOutcome captureStopOutcome
		plan, _ := tracker.beginLifecycleStop(lifecycleSuspend, "terminal-release-before-stop-callback", winprobe.ReasonSuspend, lifecycleReturnsIdle, func(stopGeneration uint64, stopped uint32) winprobe.HResult {
			if stopGeneration != generation || stopped != operationID {
				t.Errorf("bound stop generation/operation=%d/%d, want %d/%d", stopGeneration, stopped, generation, operationID)
			}
			owner := owners.matching(generation, operationID)
			if owner == nil || !owner.observeNativeTerminal() || !owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult { return 0 }).released() || !owners.clearReleased(owner) {
				t.Error("authoritative terminal release did not clear exact owner")
			}
			settleGeneration(t, tracker, generation)
			stopOutcome = requestCaptureStopForExactOwner(&owners, stopGeneration, stopped, func(uint32) winprobe.HResult {
				nativeStops.Add(1)
				return 0
			})
			return stopOutcome.Result
		})
		if plan.Generation != generation || plan.Capture != operationID || stopOutcome.State != captureStopNotRequested || nativeStops.Load() != 0 || owners.current() != nil {
			t.Fatalf("released-plan outcome plan=%+v stop=%+v native=%d owner=%+v", plan, stopOutcome, nativeStops.Load(), owners.current())
		}
	})

	t.Run("reused operation id cannot cross generations", func(t *testing.T) {
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		const operationID = uint32(7402)
		oldOwner, _, published := owners.publish(&shutdown, 91, operationID, func(uint32) winprobe.HResult { return 0 })
		if !published || oldOwner == nil || !oldOwner.observeNativeTerminal() || !oldOwner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult { return 0 }).released() || !owners.clearReleased(oldOwner) {
			t.Fatal("could not publish and release old generation")
		}
		newOwner, _, published := owners.publish(&shutdown, 92, operationID, func(uint32) winprobe.HResult { return 0 })
		if !published || newOwner == nil {
			t.Fatal("could not publish reused operation ID for new generation")
		}
		var staleStops atomic.Int32
		stale := requestCaptureStopForExactOwner(&owners, 91, operationID, func(uint32) winprobe.HResult {
			staleStops.Add(1)
			return 0
		})
		stalePointer := requestCaptureStopOrReuse(oldOwner, operationID, func(uint32) winprobe.HResult {
			staleStops.Add(1)
			return 0
		})
		if stale.State != captureStopNotRequested || stalePointer.State != captureStopNotRequested || staleStops.Load() != 0 || owners.current() != newOwner || newOwner.state.Load()&captureOwnerStopClaimed != 0 {
			t.Fatalf("stale generation outcomes exact=%+v pointer=%+v stops=%d current=%+v state=%d", stale, stalePointer, staleStops.Load(), owners.current(), newOwner.state.Load())
		}
	})

	t.Run("exact owner receives one stop and pending reuse", func(t *testing.T) {
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		const (
			generation  = uint64(93)
			operationID = uint32(7403)
		)
		owner, _, published := owners.publish(&shutdown, generation, operationID, func(uint32) winprobe.HResult { return 0 })
		if !published || owner == nil || !owner.admitActivationIntent() || !owner.admitNativeActivation() {
			t.Fatal("could not establish exact activation-owned operation")
		}
		var nativeStops, fallbackStops atomic.Int32
		first := requestCaptureStopForExactOwner(&owners, generation, operationID, func(uint32) winprobe.HResult {
			nativeStops.Add(1)
			return winprobe.HResult(-5)
		})
		second := requestCaptureStopForExactOwner(&owners, generation, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		})
		if !first.pending() || !second.pending() || nativeStops.Load() != 0 || fallbackStops.Load() != 0 {
			t.Fatalf("exact pending first=%+v second=%+v native=%d fallback=%d", first, second, nativeStops.Load(), fallbackStops.Load())
		}
		owner.completeNativeActivation(&shutdown)
		third := requestCaptureStopForExactOwner(&owners, generation, operationID, func(uint32) winprobe.HResult {
			fallbackStops.Add(1)
			return 0
		})
		if !third.completed() || third.Result != winprobe.HResult(-5) || nativeStops.Load() != 1 || fallbackStops.Load() != 0 {
			t.Fatalf("exact completion=%+v native=%d fallback=%d", third, nativeStops.Load(), fallbackStops.Load())
		}
	})
}

func TestR6F15AbruptCloseBeforeContinuationKeepsExactActiveOwner(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	const (
		activeOperation      = uint32(6201)
		unpublishedOperation = uint32(6202)
	)
	generation := bindOwnedNativeCapture(t, tracker, &owners, &shutdown, activeOperation)
	{
		var helperCalls, activeStops, wakes atomic.Int32
		prepared := runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			helperCalls.Add(1)
			return unpublishedOperation, true
		}, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
		if prepared.trackerInvoked || helperCalls.Load() != 0 || owners.orphanCount() != 0 {
			t.Fatalf("same-generation duplicate crossed the phase gate: prepared=%+v helper=%d orphans=%d", prepared, helperCalls.Load(), owners.orphanCount())
		}
		adapter := confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}
		if !adapter.confirm(func(operationID uint32) winprobe.HResult {
			if operationID != activeOperation {
				t.Errorf("confirmation stopped operation=%d, want %d", operationID, activeOperation)
			}
			activeStops.Add(1)
			return 0
		}, func() { wakes.Add(1) }) || activeStops.Load() != 1 || wakes.Load() != 1 || owners.current().operationID != activeOperation {
			t.Fatalf("confirmation activeStops=%d wakes=%d current=%+v", activeStops.Load(), wakes.Load(), owners.current())
		}
		return
	}
}
