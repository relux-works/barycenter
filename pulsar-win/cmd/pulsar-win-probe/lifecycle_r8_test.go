package main

import (
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func newR8Owner(t *testing.T, generation uint64, operationID uint32) (*captureOwnershipCoordinator, *abruptShutdownCoordinator, *captureOwnerSnapshot) {
	t.Helper()
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	owner, incumbent, published := owners.publish(shutdown, generation, operationID, func(uint32) winprobe.HResult {
		t.Fatal("ordinary publication unexpectedly requested shutdown stop")
		return 0
	})
	if !published || owner == nil || incumbent != nil {
		t.Fatalf("publish owner=%+v incumbent=%+v published=%v", owner, incumbent, published)
	}
	return owners, shutdown, owner
}

func waitR8OwnerState(t *testing.T, owner *captureOwnerSnapshot, state uint32) {
	t.Helper()
	deadline := time.After(time.Second)
	for owner.state.Load()&state == 0 {
		select {
		case <-deadline:
			t.Fatalf("owner state %08b did not acquire %08b", owner.state.Load(), state)
		default:
			runtime.Gosched()
		}
	}
}

func TestR8F25ReleaseAfterStopClaimBeforeStopLockKeepsProducer(t *testing.T) {
	t.Parallel()
	owners, _, owner := newR8Owner(t, 801, 8001)

	owner.stopMu.Lock()
	stopResult := make(chan captureStopOutcome, 1)
	var stops, releases atomic.Int32
	go func() {
		stopResult <- requestCaptureStopOrReuse(owner, owner.operationID, func(operationID uint32) winprobe.HResult {
			if operationID != owner.operationID {
				t.Errorf("Stop operation=%d, want %d", operationID, owner.operationID)
			}
			stops.Add(1)
			return 0
		})
	}()
	waitR8OwnerState(t, owner, captureOwnerStopClaimed)

	pending := owner.requestRelease(captureReleaseAfterAcceptedStop, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !pending.pending() || releases.Load() != 0 || owner.state.Load()&captureOwnerReleased != 0 {
		t.Fatalf("pre-lock release=%+v releases=%d state=%08b", pending, releases.Load(), owner.state.Load())
	}
	owner.stopMu.Unlock()
	if stop := <-stopResult; !stop.completed() || stop.Result != 0 || stops.Load() != 1 {
		t.Fatalf("stop=%+v calls=%d", stop, stops.Load())
	}
	released := owner.requestRelease(captureReleaseAfterAcceptedStop, func(operationID uint32) winprobe.HResult {
		if operationID != owner.operationID {
			t.Errorf("Release operation=%d, want %d", operationID, owner.operationID)
		}
		releases.Add(1)
		return 0
	})
	if !released.released() || releases.Load() != 1 || !owners.clearReleased(owner) || owners.current() != nil {
		t.Fatalf("release=%+v calls=%d current=%+v", released, releases.Load(), owners.current())
	}
}

func TestR8F25ImmediateStopCompletesBeforeTerminalRelease(t *testing.T) {
	t.Parallel()
	owners, _, owner := newR8Owner(t, 802, 8002)
	stopEntered := make(chan struct{})
	allowStop := make(chan struct{})
	stopResult := make(chan captureStopOutcome, 1)
	var orderMu sync.Mutex
	var order []string
	var releases atomic.Int32
	record := func(value string) {
		orderMu.Lock()
		order = append(order, value)
		orderMu.Unlock()
	}
	go func() {
		stopResult <- requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult {
			record("Stop")
			close(stopEntered)
			<-allowStop
			return 0
		})
	}()
	<-stopEntered
	if owner.observeNativeTerminal() {
		t.Fatal("terminal path became ready while Stop result was in flight")
	}
	var fallbackStops atomic.Int32
	if reused := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult {
		fallbackStops.Add(1)
		return 0
	}); !reused.pending() || fallbackStops.Load() != 0 {
		t.Fatalf("terminal-observed stop reuse=%+v fallback=%d", reused, fallbackStops.Load())
	}
	if release := owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	}); !release.pending() || releases.Load() != 0 {
		t.Fatalf("in-flight release=%+v calls=%d", release, releases.Load())
	}
	close(allowStop)
	if stop := <-stopResult; !stop.completed() || stop.Result != 0 {
		t.Fatalf("stop=%+v", stop)
	}
	if !owner.observeNativeTerminal() {
		t.Fatal("terminal path did not become ready after Stop publication")
	}
	release := owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		record("Release")
		releases.Add(1)
		return 0
	})
	orderMu.Lock()
	gotOrder := append([]string(nil), order...)
	orderMu.Unlock()
	if !release.released() || len(gotOrder) != 2 || gotOrder[0] != "Stop" || gotOrder[1] != "Release" || !owners.clearReleased(owner) {
		t.Fatalf("release=%+v order=%v", release, gotOrder)
	}
}

func TestR8F25DeferredActivationStopCompletesBeforeRelease(t *testing.T) {
	t.Parallel()
	owners, _, owner := newR8Owner(t, 803, 8003)
	if !owner.admitActivationIntent() || !owner.admitNativeActivation() {
		t.Fatal("could not admit exact-owner native activation")
	}
	stopEntered := make(chan struct{})
	allowStop := make(chan struct{})
	var stops, releases atomic.Int32
	stop := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult {
		stops.Add(1)
		close(stopEntered)
		<-allowStop
		return 0
	})
	if !stop.pending() || stops.Load() != 0 || owner.observeNativeTerminal() {
		t.Fatalf("deferred stop=%+v calls=%d state=%08b", stop, stops.Load(), owner.state.Load())
	}
	activationCompleted := make(chan struct{})
	go func() {
		owner.completeNativeActivation()
		close(activationCompleted)
	}()
	<-stopEntered
	if release := owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	}); !release.pending() || releases.Load() != 0 {
		t.Fatalf("deferred in-flight release=%+v calls=%d", release, releases.Load())
	}
	close(allowStop)
	<-activationCompleted
	if result, ok := owner.completedStopResult(); !ok || result != 0 || stops.Load() != 1 || !owner.observeNativeTerminal() {
		t.Fatalf("stop result=%s ok=%v calls=%d", result.Hex(), ok, stops.Load())
	}
	release := owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !release.released() || releases.Load() != 1 || !owners.clearReleased(owner) {
		t.Fatalf("release=%+v calls=%d", release, releases.Load())
	}
}

func TestR8F25TerminalFirstMakesLaterStopStableNotRequested(t *testing.T) {
	t.Parallel()
	owners, _, owner := newR8Owner(t, 804, 8004)
	if !owner.observeNativeTerminal() {
		t.Fatal("terminal-first owner was not release-ready")
	}
	var stops, releases atomic.Int32
	release := owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !release.released() || !owners.clearReleased(owner) {
		t.Fatalf("terminal release=%+v", release)
	}
	for range 2 {
		stop := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		})
		if stop.State != captureStopNotRequested {
			t.Fatalf("late stop=%+v", stop)
		}
	}
	if stops.Load() != 0 || releases.Load() != 1 {
		t.Fatalf("stops=%d releases=%d", stops.Load(), releases.Load())
	}
}

func TestR8F27FailedOrUnexpectedStopCannotAuthorizeCleanup(t *testing.T) {
	t.Parallel()
	for _, stopResult := range []winprobe.HResult{-1, 1} {
		stopResult := stopResult
		t.Run(stopResult.Hex(), func(t *testing.T) {
			owners, _, owner := newR8Owner(t, uint64(8100+uint32(stopResult+2)), uint32(8100+stopResult+2))
			var stops, finalizes, releases atomic.Int32
			cleanup := runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult {
				stops.Add(1)
				return stopResult
			}, func() error {
				finalizes.Add(1)
				return nil
			}, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			})
			if !cleanup.Stop.completed() || cleanup.Stop.Result != stopResult || !cleanup.StructuralFailure || cleanup.FinalizeAttempted || cleanup.ReleaseAttempted || cleanup.Released {
				t.Fatalf("cleanup=%+v", cleanup)
			}
			if stops.Load() != 1 || finalizes.Load() != 0 || releases.Load() != 0 || owners.current() != owner || owner.state.Load()&captureOwnerReleased != 0 {
				t.Fatalf("calls stop=%d finalize=%d release=%d current=%+v state=%08b", stops.Load(), finalizes.Load(), releases.Load(), owners.current(), owner.state.Load())
			}
		})
	}
}

func TestR8F25ReleaseFailureRetainsExactOwnerUntilSuccessfulRetry(t *testing.T) {
	t.Parallel()
	for index, failedRelease := range []winprobe.HResult{-1, 1} {
		index, failedRelease := index, failedRelease
		t.Run(failedRelease.Hex(), func(t *testing.T) {
			owners, _, owner := newR8Owner(t, uint64(806+index), uint32(8006+index))
			if stop := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult { return 0 }); !stop.completed() || stop.Result != 0 {
				t.Fatalf("stop=%+v", stop)
			}
			var releases atomic.Int32
			first := owner.requestRelease(captureReleaseAfterAcceptedStop, func(uint32) winprobe.HResult {
				releases.Add(1)
				return failedRelease
			})
			if !first.attempted() || first.released() || owners.clearReleased(owner) || owners.current() != owner || owner.state.Load()&captureOwnerReleased != 0 {
				t.Fatalf("first=%+v calls=%d current=%+v state=%08b", first, releases.Load(), owners.current(), owner.state.Load())
			}
			second := owner.requestRelease(captureReleaseAfterAcceptedStop, func(operationID uint32) winprobe.HResult {
				if operationID != owner.operationID {
					t.Errorf("retry Release operation=%d, want %d", operationID, owner.operationID)
				}
				releases.Add(1)
				return 0
			})
			if !second.released() || releases.Load() != 2 || !owners.clearReleased(owner) || owners.current() != nil || owners.clearReleased(owner) {
				t.Fatalf("second=%+v calls=%d current=%+v", second, releases.Load(), owners.current())
			}
		})
	}
}

func TestR8F25ConfirmedShutdownDoesNotWaitOrAdmitReleaseSuccessors(t *testing.T) {
	t.Parallel()
	t.Run("in-flight Stop", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 807, 8007)
		stopEntered := make(chan struct{})
		allowStop := make(chan struct{})
		stopDone := make(chan captureStopOutcome, 1)
		go func() {
			stopDone <- requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult {
				close(stopEntered)
				<-allowStop
				return 0
			})
		}()
		<-stopEntered
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		confirmed := make(chan bool, 1)
		var wakes, duplicateStops, finalizes, releases atomic.Int32
		go func() {
			confirmed <- adapter.confirm(func(uint32) winprobe.HResult {
				duplicateStops.Add(1)
				return 0
			}, func() { wakes.Add(1) })
		}()
		select {
		case accepted := <-confirmed:
			if !accepted {
				t.Fatal("confirmation rejected")
			}
		case <-time.After(time.Second):
			t.Fatal("confirmed shutdown waited for in-flight Stop")
		}
		if shutdown.runOrdinary(func() {
			finalizes.Add(1)
			owner.requestRelease(captureReleaseAfterAcceptedStop, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			})
		}) {
			t.Fatal("confirmed shutdown admitted ordinary release successor")
		}
		if wakes.Load() != 1 || duplicateStops.Load() != 0 || finalizes.Load() != 0 || releases.Load() != 0 || owners.current() != owner {
			t.Fatalf("wake=%d duplicateStop=%d finalize=%d release=%d current=%+v", wakes.Load(), duplicateStops.Load(), finalizes.Load(), releases.Load(), owners.current())
		}
		close(allowStop)
		if stop := <-stopDone; !stop.completed() || stop.Result != 0 {
			t.Fatalf("stop=%+v", stop)
		}
	})

	t.Run("pre-confirmation Release may return but has no successor", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 808, 8018)
		if !owner.observeNativeTerminal() {
			t.Fatal("terminal owner was not release-ready")
		}
		releaseEntered := make(chan struct{})
		allowRelease := make(chan struct{})
		releaseDone := make(chan captureReleaseOutcome, 1)
		go func() {
			releaseDone <- owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
				close(releaseEntered)
				<-allowRelease
				return 0
			})
		}()
		<-releaseEntered
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		var wakes, stops, successors atomic.Int32
		if !adapter.confirm(func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		}, func() { wakes.Add(1) }) {
			t.Fatal("confirmation rejected while Release was in flight")
		}
		close(allowRelease)
		if release := <-releaseDone; !release.released() {
			t.Fatalf("pre-confirmation release=%+v", release)
		}
		if runCaptureContinuation(shutdown, true, func() { successors.Add(1) }) || stops.Load() != 0 || wakes.Load() != 1 || successors.Load() != 0 || owners.current() != owner {
			t.Fatalf("stop=%d wake=%d successor=%d current=%+v", stops.Load(), wakes.Load(), successors.Load(), owners.current())
		}
	})
}

func TestR8F25OperationIDReuseCannotReceiveOldDeferredWork(t *testing.T) {
	t.Parallel()
	owners, shutdown, oldOwner := newR8Owner(t, 808, 8008)
	if !oldOwner.admitActivationIntent() || !oldOwner.admitNativeActivation() {
		t.Fatal("could not admit old activation")
	}
	var oldStops, staleStops, oldReleases, newStops atomic.Int32
	if stop := requestCaptureStopOrReuse(oldOwner, oldOwner.operationID, func(uint32) winprobe.HResult {
		oldStops.Add(1)
		return 0
	}); !stop.pending() {
		t.Fatalf("old deferred stop=%+v", stop)
	}
	if oldOwner.observeNativeTerminal() || !oldOwner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		oldReleases.Add(1)
		return 0
	}).pending() || owners.clearReleased(oldOwner) {
		t.Fatal("old owner escaped while deferred Stop still had a live producer")
	}
	if candidate, incumbent, published := owners.publish(shutdown, 809, oldOwner.operationID, func(uint32) winprobe.HResult { return 0 }); published || candidate == nil || incumbent != oldOwner {
		t.Fatalf("reuse published before old release: candidate=%+v incumbent=%+v published=%v", candidate, incumbent, published)
	}
	oldOwner.completeNativeActivation()
	if result, ok := oldOwner.completedStopResult(); !ok || result != 0 || oldStops.Load() != 1 || !oldOwner.observeNativeTerminal() {
		t.Fatalf("old result=%s ok=%v stops=%d", result.Hex(), ok, oldStops.Load())
	}
	if release := oldOwner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		oldReleases.Add(1)
		return 0
	}); !release.released() || !owners.clearReleased(oldOwner) {
		t.Fatalf("old release=%+v calls=%d", release, oldReleases.Load())
	}
	newOwner, _, published := owners.publish(shutdown, 809, oldOwner.operationID, func(uint32) winprobe.HResult { return 0 })
	if !published || newOwner == nil {
		t.Fatal("could not publish reused operation ID")
	}
	if stale := requestCaptureStopOrReuse(oldOwner, oldOwner.operationID, func(uint32) winprobe.HResult {
		staleStops.Add(1)
		return 0
	}); stale.State != captureStopNotRequested {
		t.Fatalf("stale stop=%+v", stale)
	}
	if current := requestCaptureStopForExactOwner(owners, newOwner.generation, newOwner.operationID, func(uint32) winprobe.HResult {
		newStops.Add(1)
		return 0
	}); !current.completed() || current.Result != 0 {
		t.Fatalf("new stop=%+v", current)
	}
	if staleStops.Load() != 0 || newStops.Load() != 1 || owners.current() != newOwner {
		t.Fatalf("stale=%d new=%d current=%+v", staleStops.Load(), newStops.Load(), owners.current())
	}
}

func TestR8F26SuccessfulPrepareWithZeroIDFailsClosed(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin capture: %s", reason)
	}
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	var stops, evidence, escalation atomic.Int32
	prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) {
		return 0, true
	}, func(uint32) winprobe.HResult {
		stops.Add(1)
		return 0
	}, func(uint32) winprobe.HResult {
		stops.Add(1)
		return 0
	})
	if !prepared.trackerInvoked || !prepared.externalInvoked || !prepared.invalidSuccessfulID || prepared.succeeded || prepared.owner != nil || prepared.ownerPublished || owners.current() != nil || tracker.hasCaptureGeneration() {
		t.Fatalf("prepared=%+v current=%+v generationAlive=%v", prepared, owners.current(), tracker.hasCaptureGeneration())
	}
	policy := prepared.handleInvalidSuccessfulResult(shutdown, func() bool { evidence.Add(1); return true }, func() { escalation.Add(1) })
	if !policy.admitted || !policy.evidencePassed || !policy.escalated {
		t.Fatalf("invalid successful result policy=%+v", policy)
	}
	if stops.Load() != 0 || evidence.Load() != 1 || escalation.Load() != 1 {
		t.Fatalf("stops=%d evidence=%d escalation=%d", stops.Load(), evidence.Load(), escalation.Load())
	}
	if _, _, nextReason := tracker.beginCaptureGeneration(); nextReason != "" {
		t.Fatalf("invalid prepare stranded the next start gate: %s", nextReason)
	}
}

func TestR8F26InvalidNonzeroOwnerQueryFailsClosedWithoutCleanupClaims(t *testing.T) {
	t.Parallel()
	owners, shutdown, owner := newR8Owner(t, 810, 8010)
	var stops, finalizes, releases, evidence, escalation atomic.Int32
	failure := runCaptureResultQueryFailure(owner, owner.operationID, captureInvalidHandleHResult, func(uint32) winprobe.HResult {
		stops.Add(1)
		return 0
	}, func() error {
		finalizes.Add(1)
		return nil
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !failure.InvalidNativeOwner || failure.Cleanup.Stop.State != captureStopNotRequested || failure.Cleanup.FinalizeAttempted || failure.Cleanup.ReleaseAttempted {
		t.Fatalf("failure=%+v", failure)
	}
	policy := failure.handleInvalidNativeOwner(shutdown, func() bool { evidence.Add(1); return true }, func() { escalation.Add(1) })
	if !policy.admitted || !policy.evidencePassed || !policy.escalated {
		t.Fatalf("invalid owner policy=%+v", policy)
	}
	if stops.Load() != 0 || finalizes.Load() != 0 || releases.Load() != 0 || evidence.Load() != 1 || escalation.Load() != 1 || owners.current() != owner || owner.state.Load()&(captureOwnerTerminalObserved|captureOwnerReleaseAdmitted|captureOwnerReleased) != 0 {
		t.Fatalf("stop=%d finalize=%d release=%d evidence=%d escalation=%d current=%+v state=%08b", stops.Load(), finalizes.Load(), releases.Load(), evidence.Load(), escalation.Load(), owners.current(), owner.state.Load())
	}
}

func TestR8W1FinalizedRetryUsesExactRecordedAuthority(t *testing.T) {
	t.Parallel()

	t.Run("query failure Stop S_OK retries early Release on same owner", func(t *testing.T) {
		owners, _, owner := newR8Owner(t, 811, 8011)
		var stops, finalizes, releases, clears atomic.Int32
		first := runCaptureQueryFailureCleanup(owner, owner.operationID, func(operationID uint32) winprobe.HResult {
			if operationID != owner.operationID {
				t.Errorf("Stop operation=%d, want %d", operationID, owner.operationID)
			}
			stops.Add(1)
			return 0
		}, func() error {
			finalizes.Add(1)
			return nil
		}, func(operationID uint32) winprobe.HResult {
			if operationID != owner.operationID {
				t.Errorf("first Release operation=%d, want %d", operationID, owner.operationID)
			}
			releases.Add(1)
			return captureReleaseBeforeTerminalHResult
		})
		if !first.Stop.completed() || first.Stop.Result != 0 || !first.FinalizeAttempted || !first.ReleaseAttempted || !first.ReleaseAwaitsTerminal || first.StructuralFailure || first.Released {
			t.Fatalf("first cleanup=%+v", first)
		}
		authority, authorized := owner.finalizedReleaseAuthority()
		if !authorized || authority != captureReleaseAfterAcceptedStop {
			t.Fatalf("retry authority=%d authorized=%v", authority, authorized)
		}
		retry := owner.requestRelease(authority, func(operationID uint32) winprobe.HResult {
			if operationID != owner.operationID {
				t.Errorf("retry Release operation=%d, want %d", operationID, owner.operationID)
			}
			releases.Add(1)
			return 0
		})
		if retry.released() && owners.clearReleased(owner) {
			clears.Add(1)
		}
		if !retry.released() || stops.Load() != 1 || finalizes.Load() != 1 || releases.Load() != 2 || clears.Load() != 1 || owners.current() != nil {
			t.Fatalf("retry=%+v stop=%d finalize=%d release=%d clear=%d current=%+v", retry, stops.Load(), finalizes.Load(), releases.Load(), clears.Load(), owners.current())
		}
	})

	t.Run("terminal finalized retry never falls back to Stop authority", func(t *testing.T) {
		owners, _, owner := newR8Owner(t, 812, 8012)
		if !owner.observeNativeTerminal() {
			t.Fatal("terminal owner not ready")
		}
		var releases atomic.Int32
		first := owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
			releases.Add(1)
			return -1
		})
		if !first.attempted() || first.released() || owners.current() != owner {
			t.Fatalf("first terminal release=%+v current=%+v", first, owners.current())
		}
		authority, authorized := owner.finalizedReleaseAuthority()
		if !authorized || authority != captureReleaseAfterTerminal {
			t.Fatalf("terminal retry authority=%d authorized=%v", authority, authorized)
		}
		retry := owner.requestRelease(authority, func(uint32) winprobe.HResult {
			releases.Add(1)
			return 0
		})
		if !retry.released() || releases.Load() != 2 || !owners.clearReleased(owner) {
			t.Fatalf("terminal retry=%+v releases=%d", retry, releases.Load())
		}
	})

	t.Run("failed Stop never becomes finalized Release authority", func(t *testing.T) {
		_, _, owner := newR8Owner(t, 813, 8013)
		if stop := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult { return -1 }); !stop.completed() || stop.Result != -1 {
			t.Fatalf("failed stop=%+v", stop)
		}
		if authority, authorized := owner.finalizedReleaseAuthority(); authorized || authority != 0 {
			t.Fatalf("failed Stop authorized Release: authority=%d authorized=%v", authority, authorized)
		}
	})
}

func TestR8W2UnpublishedNativeOwnerDrainsOnWaiterWithoutTouchingIncumbent(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	const (
		incumbentGeneration = uint64(9901)
		incumbentOperation  = uint32(8201)
		orphanOperation     = uint32(8202)
	)
	incumbent, _, published := owners.publish(shutdown, incumbentGeneration, incumbentOperation, func(uint32) winprobe.HResult { return 0 })
	if !published || incumbent == nil {
		t.Fatal("could not publish incumbent A")
	}
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin orphan generation: %s", reason)
	}
	registry := map[uint32]bool{incumbentOperation: true, orphanOperation: true}
	var stops, queries, releases atomic.Int32
	prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) {
		return orphanOperation, true
	}, func(uint32) winprobe.HResult {
		t.Fatal("open-gate orphan received shutdown Stop")
		return -1
	}, func(operationID uint32) winprobe.HResult {
		if operationID != orphanOperation || !registry[operationID] {
			t.Errorf("orphan Stop operation=%d registry=%v", operationID, registry)
		}
		stops.Add(1)
		return 0
	})
	if !prepared.succeeded || prepared.ownerPublished || prepared.orphan == nil || prepared.conflictingOwner != incumbent || stops.Load() != 1 || owners.orphanCount() != 1 {
		t.Fatalf("prepared=%+v stops=%d orphans=%d", prepared, stops.Load(), owners.orphanCount())
	}
	if operationID, phase, exists := tracker.captureStateForGeneration(generation); !exists || operationID != orphanOperation || phase != captureGenerationNativeOwned {
		t.Fatalf("orphan generation exists=%v operation=%d phase=%d", exists, operationID, phase)
	}
	waiting := runCaptureOrphanDrain(prepared.orphan, func(operationID uint32) (winprobe.CaptureResult, winprobe.HResult) {
		queries.Add(1)
		return winprobe.CaptureResult{State: winprobe.CaptureStatePreparing}, 0
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if !waiting.QueryAttempted || waiting.TerminalObserved || waiting.Release.attempted() || releases.Load() != 0 {
		t.Fatalf("waiting drain=%+v releases=%d", waiting, releases.Load())
	}
	drained := runCaptureOrphanDrain(prepared.orphan, func(operationID uint32) (winprobe.CaptureResult, winprobe.HResult) {
		queries.Add(1)
		return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
	}, func(operationID uint32) winprobe.HResult {
		if operationID != orphanOperation || !registry[operationID] {
			t.Errorf("orphan Release operation=%d registry=%v", operationID, registry)
		}
		delete(registry, operationID)
		releases.Add(1)
		return 0
	})
	if !drained.TerminalObserved || !drained.Release.released() || !owners.completeOrphan(prepared.orphan) {
		t.Fatalf("drained=%+v", drained)
	}
	settleGeneration(t, tracker, generation)
	if owners.current() != incumbent || owners.orphanCount() != 0 || stops.Load() != 1 || queries.Load() != 2 || releases.Load() != 1 || !registry[incumbentOperation] {
		t.Fatalf("active=%+v orphans=%d stop=%d query=%d release=%d registry=%v", owners.current(), owners.orphanCount(), stops.Load(), queries.Load(), releases.Load(), registry)
	}
	if !incumbent.observeNativeTerminal() {
		t.Fatal("incumbent terminal was not ready")
	}
	if release := incumbent.requestRelease(captureReleaseAfterTerminal, func(operationID uint32) winprobe.HResult {
		delete(registry, operationID)
		return 0
	}); !release.released() || !owners.clearReleased(incumbent) || len(registry) != 0 || owners.current() != nil {
		t.Fatalf("incumbent release=%+v registry=%v active=%+v", release, registry, owners.current())
	}
}

func TestR8W2OrphanReleaseFailureRetriesExactOwnerAndID(t *testing.T) {
	t.Parallel()
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	incumbent, _, published := owners.publish(shutdown, 9910, 8210, func(uint32) winprobe.HResult { return 0 })
	if !published {
		t.Fatal("could not publish incumbent")
	}
	tracker := newLifecycleTracker()
	generation, _, _ := tracker.beginCaptureGeneration()
	prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) { return 8211, true }, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
	if prepared.orphan == nil {
		t.Fatal("forced conflict did not retain orphan")
	}
	var releases atomic.Int32
	first := runCaptureOrphanDrain(prepared.orphan, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
		return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
	}, func(operationID uint32) winprobe.HResult {
		if operationID != 8211 {
			t.Errorf("first Release operation=%d", operationID)
		}
		releases.Add(1)
		return -1
	})
	if !first.Release.attempted() || first.Release.released() || !first.StructuralFailure || owners.orphanCount() != 1 || owners.current() != incumbent {
		t.Fatalf("first=%+v orphans=%d active=%+v", first, owners.orphanCount(), owners.current())
	}
	second := runCaptureOrphanDrain(prepared.orphan, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
		return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
	}, func(operationID uint32) winprobe.HResult {
		if operationID != 8211 {
			t.Errorf("retry Release operation=%d", operationID)
		}
		releases.Add(1)
		return 0
	})
	if !second.Release.released() || releases.Load() != 2 || !owners.completeOrphan(prepared.orphan) || owners.current() != incumbent {
		t.Fatalf("second=%+v releases=%d active=%+v", second, releases.Load(), owners.current())
	}
	if !incumbent.observeNativeTerminal() || !incumbent.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult { return 0 }).released() || !owners.clearReleased(incumbent) {
		t.Fatal("could not release incumbent before operation-ID reuse")
	}
	newOwner, _, published := owners.publish(shutdown, 9911, 8211, func(uint32) winprobe.HResult { return 0 })
	if !published || newOwner == nil {
		t.Fatal("could not publish reused operation ID")
	}
	var staleQueries, staleReleases atomic.Int32
	stale := runCaptureOrphanDrain(prepared.orphan, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
		staleQueries.Add(1)
		return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
	}, func(uint32) winprobe.HResult {
		staleReleases.Add(1)
		return 0
	})
	if stale.QueryAttempted || stale.Release.attempted() || staleQueries.Load() != 0 || staleReleases.Load() != 0 || owners.current() != newOwner || newOwner.state.Load() != 0 {
		t.Fatalf("stale=%+v queries=%d releases=%d active=%+v newState=%08b", stale, staleQueries.Load(), staleReleases.Load(), owners.current(), newOwner.state.Load())
	}
}

func TestR8W2ConfirmedShutdownSuppressesOrphanQueryAndRelease(t *testing.T) {
	t.Parallel()
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	incumbent, _, published := owners.publish(shutdown, 9920, 8220, func(uint32) winprobe.HResult { return 0 })
	if !published {
		t.Fatal("could not publish incumbent")
	}
	tracker := newLifecycleTracker()
	generation, _, _ := tracker.beginCaptureGeneration()
	prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) { return 8221, true }, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
	if prepared.orphan == nil {
		t.Fatal("forced conflict did not retain orphan")
	}
	adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
	var incumbentStops, wakes, queries, releases atomic.Int32
	if !adapter.confirm(func(operationID uint32) winprobe.HResult {
		if operationID != incumbent.operationID {
			t.Errorf("confirmation Stop operation=%d, want %d", operationID, incumbent.operationID)
		}
		incumbentStops.Add(1)
		return 0
	}, func() { wakes.Add(1) }) {
		t.Fatal("confirmation rejected")
	}
	if shutdown.runOrdinary(func() {
		runCaptureOrphanDrain(prepared.orphan, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
			queries.Add(1)
			return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
		}, func(uint32) winprobe.HResult {
			releases.Add(1)
			return 0
		})
	}) {
		t.Fatal("confirmed shutdown admitted orphan drain")
	}
	if incumbentStops.Load() != 1 || wakes.Load() != 1 || queries.Load() != 0 || releases.Load() != 0 || owners.orphanCount() != 1 || owners.current() != incumbent {
		t.Fatalf("stop=%d wake=%d query=%d release=%d orphans=%d active=%+v", incumbentStops.Load(), wakes.Load(), queries.Load(), releases.Load(), owners.orphanCount(), owners.current())
	}
}

func TestR8W2ConfirmationDoesNotWaitForOrphanStopOrQuery(t *testing.T) {
	t.Parallel()

	t.Run("orphan Stop callback", func(t *testing.T) {
		owners := &captureOwnershipCoordinator{}
		shutdown := &abruptShutdownCoordinator{}
		incumbent, _, published := owners.publish(shutdown, 9930, 8230, func(uint32) winprobe.HResult { return 0 })
		if !published {
			t.Fatal("could not publish incumbent")
		}
		tracker := newLifecycleTracker()
		generation, _, _ := tracker.beginCaptureGeneration()
		stopEntered := make(chan struct{})
		allowStop := make(chan struct{})
		prepareDone := make(chan capturePrepareCoordinatorResult, 1)
		go func() {
			prepareDone <- runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) { return 8231, true }, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult {
				close(stopEntered)
				<-allowStop
				return 0
			})
		}()
		<-stopEntered
		if owners.orphanCount() != 1 {
			t.Fatalf("orphan was not published before Stop: %d", owners.orphanCount())
		}
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		confirmed := make(chan bool, 1)
		var activeStops, wakes atomic.Int32
		go func() {
			confirmed <- adapter.confirm(func(operationID uint32) winprobe.HResult {
				if operationID != incumbent.operationID {
					t.Errorf("confirmation Stop operation=%d", operationID)
				}
				activeStops.Add(1)
				return 0
			}, func() { wakes.Add(1) })
		}()
		select {
		case accepted := <-confirmed:
			if !accepted {
				t.Fatal("confirmation rejected")
			}
		case <-time.After(time.Second):
			t.Fatal("confirmation waited for orphan Stop")
		}
		close(allowStop)
		prepared := <-prepareDone
		if prepared.orphan == nil || activeStops.Load() != 1 || wakes.Load() != 1 || owners.orphanCount() != 1 {
			t.Fatalf("prepared=%+v activeStops=%d wakes=%d orphans=%d", prepared, activeStops.Load(), wakes.Load(), owners.orphanCount())
		}
	})

	t.Run("terminal query callback", func(t *testing.T) {
		owners := &captureOwnershipCoordinator{}
		shutdown := &abruptShutdownCoordinator{}
		incumbent, _, published := owners.publish(shutdown, 9940, 8240, func(uint32) winprobe.HResult { return 0 })
		if !published {
			t.Fatal("could not publish incumbent")
		}
		tracker := newLifecycleTracker()
		generation, _, _ := tracker.beginCaptureGeneration()
		prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) { return 8241, true }, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
		queryEntered := make(chan struct{})
		allowQuery := make(chan struct{})
		drainDone := make(chan captureOrphanDrainResult, 1)
		var releases atomic.Int32
		go func() {
			drainDone <- runCaptureOrphanDrain(prepared.orphan, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
				close(queryEntered)
				<-allowQuery
				return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
			}, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			}, shutdown.runOrdinary)
		}()
		<-queryEntered
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		var activeStops, wakes atomic.Int32
		if !adapter.confirm(func(operationID uint32) winprobe.HResult {
			if operationID != incumbent.operationID {
				t.Errorf("confirmation Stop operation=%d", operationID)
			}
			activeStops.Add(1)
			return 0
		}, func() { wakes.Add(1) }) {
			t.Fatal("confirmation rejected")
		}
		close(allowQuery)
		drained := <-drainDone
		if !drained.QueryAttempted || drained.TerminalObserved || drained.Release.attempted() || releases.Load() != 0 || activeStops.Load() != 1 || wakes.Load() != 1 {
			t.Fatalf("drained=%+v releases=%d activeStops=%d wakes=%d", drained, releases.Load(), activeStops.Load(), wakes.Load())
		}
	})
}

func TestR8W4OrphanVisibilityCarriesLiveStopProducer(t *testing.T) {
	t.Parallel()
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	incumbent, _, published := owners.publish(shutdown, 9950, 8250, func(uint32) winprobe.HResult { return 0 })
	if !published || incumbent == nil {
		t.Fatal("could not publish incumbent")
	}
	orphan := &captureOwnerSnapshot{generation: 9951, operationID: 8251}
	stopEntered := make(chan struct{})
	allowStop := make(chan struct{})
	var orderMu sync.Mutex
	var order []string
	obligation, claimed := owners.publishOrphanStopProducer(orphan, func(operationID uint32) winprobe.HResult {
		if operationID != orphan.operationID {
			t.Errorf("Stop operation=%d, want %d", operationID, orphan.operationID)
		}
		orderMu.Lock()
		order = append(order, "Stop")
		orderMu.Unlock()
		close(stopEntered)
		<-allowStop
		return 0
	})
	if !claimed || obligation == nil || owners.orphanCount() != 1 || orphan.state.Load()&captureOwnerStopClaimed == 0 {
		t.Fatalf("claimed=%v obligation=%+v orphans=%d state=%08b", claimed, obligation, owners.orphanCount(), orphan.state.Load())
	}

	var queries, releases atomic.Int32
	beforeInvocation := runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
		queries.Add(1)
		return winprobe.CaptureResult{}, 0
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if beforeInvocation.Stop.State != captureStopPending || beforeInvocation.StructuralFailure || beforeInvocation.QueryAttempted || beforeInvocation.Release.attempted() || queries.Load() != 0 || releases.Load() != 0 {
		t.Fatalf("pre-invocation drain=%+v queries=%d releases=%d", beforeInvocation, queries.Load(), releases.Load())
	}

	invoked := make(chan bool, 1)
	go func() { invoked <- orphan.invokeClaimedStop() }()
	<-stopEntered
	duringInvocation := runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
		queries.Add(1)
		return winprobe.CaptureResult{}, 0
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if duringInvocation.Stop.State != captureStopPending || duringInvocation.StructuralFailure || duringInvocation.QueryAttempted || duringInvocation.Release.attempted() || queries.Load() != 0 || releases.Load() != 0 {
		t.Fatalf("in-flight drain=%+v queries=%d releases=%d", duringInvocation, queries.Load(), releases.Load())
	}
	close(allowStop)
	if !<-invoked {
		t.Fatal("claimed Stop producer was not invoked")
	}

	drained := runCaptureOrphanDrain(obligation, func(operationID uint32) (winprobe.CaptureResult, winprobe.HResult) {
		if operationID != orphan.operationID {
			t.Errorf("query operation=%d, want %d", operationID, orphan.operationID)
		}
		queries.Add(1)
		orderMu.Lock()
		order = append(order, "QueryTerminal")
		orderMu.Unlock()
		return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
	}, func(operationID uint32) winprobe.HResult {
		if operationID != orphan.operationID {
			t.Errorf("Release operation=%d, want %d", operationID, orphan.operationID)
		}
		releases.Add(1)
		orderMu.Lock()
		order = append(order, "Release")
		orderMu.Unlock()
		return 0
	})
	if !drained.TerminalObserved || !drained.Release.released() || drained.StructuralFailure || queries.Load() != 1 || releases.Load() != 1 || !owners.completeOrphan(obligation) {
		t.Fatalf("drained=%+v queries=%d releases=%d orphans=%d", drained, queries.Load(), releases.Load(), owners.orphanCount())
	}
	orderMu.Lock()
	gotOrder := append([]string(nil), order...)
	orderMu.Unlock()
	if !reflect.DeepEqual(gotOrder, []string{"Stop", "QueryTerminal", "Release"}) {
		t.Fatalf("native order=%v", gotOrder)
	}
	if owners.current() != incumbent || incumbent.state.Load() != 0 {
		t.Fatalf("orphan cleanup touched incumbent: current=%+v state=%08b", owners.current(), incumbent.state.Load())
	}
}

func TestR8W4ConfirmationAtOrphanPreInvocationSeam(t *testing.T) {
	t.Parallel()
	owners := &captureOwnershipCoordinator{}
	shutdown := &abruptShutdownCoordinator{}
	incumbent, _, published := owners.publish(shutdown, 9960, 8260, func(uint32) winprobe.HResult { return 0 })
	if !published || incumbent == nil {
		t.Fatal("could not publish incumbent")
	}
	orphan := &captureOwnerSnapshot{generation: 9961, operationID: 8261}
	var orphanStops, activeStops, wakes, queries, releases atomic.Int32
	obligation, claimed := owners.publishOrphanStopProducer(orphan, func(operationID uint32) winprobe.HResult {
		if operationID != orphan.operationID {
			t.Errorf("orphan Stop operation=%d", operationID)
		}
		orphanStops.Add(1)
		return 0
	})
	if !claimed || obligation == nil {
		t.Fatal("orphan Stop producer was not published")
	}
	pending := runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
		queries.Add(1)
		return winprobe.CaptureResult{}, 0
	}, func(uint32) winprobe.HResult {
		releases.Add(1)
		return 0
	})
	if pending.Stop.State != captureStopPending || pending.StructuralFailure || pending.QueryAttempted {
		t.Fatalf("pre-confirmation drain=%+v", pending)
	}

	confirmed := make(chan bool, 1)
	go func() {
		confirmed <- (confirmedShutdownAdapter{shutdown: shutdown, owners: owners}).confirm(func(operationID uint32) winprobe.HResult {
			if operationID != incumbent.operationID {
				t.Errorf("active Stop operation=%d", operationID)
			}
			activeStops.Add(1)
			return 0
		}, func() { wakes.Add(1) })
	}()
	select {
	case accepted := <-confirmed:
		if !accepted {
			t.Fatal("confirmation rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("confirmation waited for unpublished orphan Stop invocation")
	}
	if !orphan.invokeClaimedStop() {
		t.Fatal("late orphan Stop producer was not invoked")
	}
	if shutdown.runOrdinary(func() {
		runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
			queries.Add(1)
			return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
		}, func(uint32) winprobe.HResult {
			releases.Add(1)
			return 0
		})
	}) {
		t.Fatal("confirmed shutdown admitted orphan query/release")
	}
	if activeStops.Load() != 1 || wakes.Load() != 1 || orphanStops.Load() != 1 || queries.Load() != 0 || releases.Load() != 0 || owners.orphanCount() != 1 || owners.current() != incumbent {
		t.Fatalf("activeStops=%d wakes=%d orphanStops=%d queries=%d releases=%d orphans=%d current=%+v", activeStops.Load(), wakes.Load(), orphanStops.Load(), queries.Load(), releases.Load(), owners.orphanCount(), owners.current())
	}
}

func TestR8W3RequiredStructuralEvidencePrecedesEscalation(t *testing.T) {
	t.Parallel()
	requireOrder := func(t *testing.T, label string, invoke func(required func() bool, escalate func()) requiredFailureContinuationOutcome) {
		t.Helper()
		var order []string
		outcome := invoke(func() bool {
			order = append(order, "required:"+label)
			return true
		}, func() {
			order = append(order, "escalate:"+label)
		})
		if !outcome.admitted || !outcome.evidenceAttempted || !outcome.evidencePassed || !outcome.escalated || len(order) != 2 || order[0] != "required:"+label || order[1] != "escalate:"+label {
			t.Fatalf("%s outcome=%+v order=%v", label, outcome, order)
		}
	}

	t.Run("CapturePrepare S_OK zero", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, _, _ := tracker.beginCaptureGeneration()
		owners := &captureOwnershipCoordinator{}
		shutdown := &abruptShutdownCoordinator{}
		prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) { return 0, true }, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
		if !prepared.invalidSuccessfulID {
			t.Fatal("zero-ID success was not classified")
		}
		requireOrder(t, "capture_prepare_contract_failure", func(required func() bool, escalate func()) requiredFailureContinuationOutcome {
			return prepared.handleInvalidSuccessfulResult(shutdown, required, escalate)
		})
	})

	t.Run("unexpected Prepare success HRESULT", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, _, _ := tracker.beginCaptureGeneration()
		owners := &captureOwnershipCoordinator{}
		shutdown := &abruptShutdownCoordinator{}
		prepared := runCapturePrepareOwned(tracker, owners, shutdown, generation, func() (uint32, bool) { return 8280, false }, func(uint32) winprobe.HResult { return 0 }, func(uint32) winprobe.HResult { return 0 })
		requireOrder(t, "capture_prepare_unexpected_success", func(required func() bool, escalate func()) requiredFailureContinuationOutcome {
			return prepared.handleUnexpectedSuccessfulHResult(shutdown, required, escalate)
		})
	})

	t.Run("invalid registered owner query", func(t *testing.T) {
		_, shutdown, owner := newR8Owner(t, 9950, 8250)
		failure := runCaptureResultQueryFailure(owner, owner.operationID, captureInvalidHandleHResult, func(uint32) winprobe.HResult { return 0 }, nil, func(uint32) winprobe.HResult { return 0 })
		if !failure.InvalidNativeOwner {
			t.Fatal("invalid owner query was not classified")
		}
		requireOrder(t, "capture_result_owner_contract_failure", func(required func() bool, escalate func()) requiredFailureContinuationOutcome {
			return failure.handleInvalidNativeOwner(shutdown, required, escalate)
		})
	})

	for _, scenario := range []struct {
		name  string
		drain func(*captureOrphanObligation) captureOrphanDrainResult
	}{
		{
			name: "orphan Stop failure",
			drain: func(obligation *captureOrphanObligation) captureOrphanDrainResult {
				return runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) { return winprobe.CaptureResult{}, 0 }, func(uint32) winprobe.HResult { return 0 })
			},
		},
		{
			name: "orphan query failure",
			drain: func(obligation *captureOrphanObligation) captureOrphanDrainResult {
				return runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
					return winprobe.CaptureResult{}, captureInvalidHandleHResult
				}, func(uint32) winprobe.HResult { return 0 })
			},
		},
		{
			name: "orphan Release failure",
			drain: func(obligation *captureOrphanObligation) captureOrphanDrainResult {
				return runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
					return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
				}, func(uint32) winprobe.HResult { return -1 })
			},
		},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			owner := &captureOwnerSnapshot{generation: 9960, operationID: 8260}
			obligation := &captureOrphanObligation{owner: owner}
			stopResult := winprobe.HResult(0)
			if scenario.name == "orphan Stop failure" {
				stopResult = -1
			}
			if !owner.requestStop(func(uint32) winprobe.HResult { return stopResult }) {
				t.Fatal("could not claim orphan Stop")
			}
			drained := scenario.drain(obligation)
			if !drained.StructuralFailure {
				t.Fatalf("drained=%+v", drained)
			}
			shutdown := &abruptShutdownCoordinator{}
			requireOrder(t, scenario.name, func(required func() bool, escalate func()) requiredFailureContinuationOutcome {
				return runRequiredFailureContinuation(shutdown, true, required, escalate)
			})
		})
	}

	t.Run("orphan clear mismatch", func(t *testing.T) {
		owner := &captureOwnerSnapshot{generation: 9970, operationID: 8270}
		if !owner.observeNativeTerminal() || !owner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult { return 0 }).released() {
			t.Fatal("could not establish released orphan")
		}
		wrongCoordinator := &captureOwnershipCoordinator{}
		obligation := &captureOrphanObligation{owner: owner}
		if wrongCoordinator.completeOrphan(obligation) {
			t.Fatal("wrong coordinator cleared orphan")
		}
		shutdown := &abruptShutdownCoordinator{}
		requireOrder(t, "capture_orphan_clear_mismatch", func(required func() bool, escalate func()) requiredFailureContinuationOutcome {
			return runRequiredFailureContinuation(shutdown, true, required, escalate)
		})
	})

	t.Run("evidence failure suppresses escalation", func(t *testing.T) {
		shutdown := &abruptShutdownCoordinator{}
		var escalations atomic.Int32
		outcome := runRequiredFailureContinuation(shutdown, true, func() bool { return false }, func() { escalations.Add(1) })
		if !outcome.admitted || !outcome.evidenceAttempted || outcome.evidencePassed || outcome.escalated || escalations.Load() != 0 {
			t.Fatalf("outcome=%+v escalations=%d", outcome, escalations.Load())
		}
	})

	t.Run("confirmation suppresses diagnostic and escalation", func(t *testing.T) {
		shutdown := &abruptShutdownCoordinator{}
		if !shutdown.confirmAfterStop(nil, nil) {
			t.Fatal("could not confirm shutdown")
		}
		var evidence, escalations atomic.Int32
		outcome := runRequiredFailureContinuation(shutdown, true, func() bool { evidence.Add(1); return true }, func() { escalations.Add(1) })
		if outcome.admitted || outcome.evidenceAttempted || outcome.escalated || evidence.Load() != 0 || escalations.Load() != 0 {
			t.Fatalf("outcome=%+v evidence=%d escalations=%d", outcome, evidence.Load(), escalations.Load())
		}
	})
}
