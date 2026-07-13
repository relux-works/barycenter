package main

import (
	"sync/atomic"
	"testing"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestR11LatePrepareHandsOffWithoutPostLatchStop(t *testing.T) {
	tracker := newLifecycleTracker()
	generation, accepted, reason := tracker.beginCaptureGeneration()
	if !accepted {
		t.Fatalf("begin capture: %s", reason)
	}
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	prepareEntered := make(chan struct{})
	allowPrepare := make(chan struct{})
	prepared := make(chan capturePrepareCoordinatorResult, 1)
	var stops atomic.Int32
	go func() {
		prepared <- runCapturePrepareOwned(tracker, &owners, &shutdown, generation, func() (uint32, bool) {
			close(prepareEntered)
			<-allowPrepare
			return 11001, true
		}, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		}, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		})
	}()
	<-prepareEntered
	if !(confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}).confirm(nil, nil) {
		t.Fatal("confirmation rejected")
	}
	close(allowPrepare)
	result := <-prepared
	if !result.succeeded || result.trackerAllowed || result.owner == nil || result.ownerPublished || result.orphan != nil {
		t.Fatalf("late prepare handoff=%+v", result)
	}
	operationID, phase, exists := tracker.captureStateForGeneration(generation)
	if !exists || operationID != 0 || phase != captureGenerationPrepareInFlight {
		t.Fatalf("late prepare published lifecycle successor exists=%v operation=%d phase=%d", exists, operationID, phase)
	}
	if stops.Load() != 0 || owners.current() != nil || owners.orphanCount() != 0 {
		t.Fatalf("post-latch effects stops=%d current=%+v orphans=%d", stops.Load(), owners.current(), owners.orphanCount())
	}
}

func TestR11OrphanPreInvocationSeamRejectsPostLatchStop(t *testing.T) {
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	incumbent, _, published := owners.publish(&shutdown, 12001, 12001, func(uint32) winprobe.HResult { return 0 })
	if !published || incumbent == nil {
		t.Fatal("could not publish incumbent")
	}
	orphan := &captureOwnerSnapshot{generation: 12002, operationID: 12002}
	var orphanStops atomic.Int32
	obligation, claimed := owners.publishOrphanStopProducer(orphan, func(uint32) winprobe.HResult {
		orphanStops.Add(1)
		return 0
	})
	if !claimed || obligation == nil {
		t.Fatal("could not publish orphan producer")
	}
	if !(confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}).confirm(func(uint32) winprobe.HResult { return 0 }, nil) {
		t.Fatal("confirmation rejected")
	}
	if orphan.invokeClaimedStop(&shutdown) {
		t.Fatal("confirmed shutdown admitted orphan Stop")
	}
	if orphanStops.Load() != 0 || owners.orphanCount() != 1 || owners.current() != incumbent {
		t.Fatalf("orphan effects stops=%d orphans=%d current=%+v", orphanStops.Load(), owners.orphanCount(), owners.current())
	}
}

func TestR11DeferredActivationStopRequiresFreshPermit(t *testing.T) {
	t.Run("confirmed handoff suppresses deferred Stop", func(t *testing.T) {
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		owner, _, published := owners.publish(&shutdown, 13001, 13001, func(uint32) winprobe.HResult { return 0 })
		if !published || owner == nil || !owner.admitActivationIntent() || !owner.admitNativeActivation() {
			t.Fatal("could not establish native-activation-owned capture")
		}
		var stops atomic.Int32
		if !(confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}).confirm(func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		}, nil) {
			t.Fatal("confirmation rejected")
		}
		owner.completeNativeActivation(&shutdown)
		if result, completed := owner.completedStopResult(); stops.Load() != 0 || completed || result != 0 {
			t.Fatalf("post-latch Stop calls=%d completed=%v result=%s", stops.Load(), completed, result.Hex())
		}
	})

	t.Run("ordinary completion invokes exactly once", func(t *testing.T) {
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		owner, _, published := owners.publish(&shutdown, 13002, 13002, func(uint32) winprobe.HResult { return 0 })
		if !published || owner == nil || !owner.admitActivationIntent() || !owner.admitNativeActivation() {
			t.Fatal("could not establish native-activation-owned capture")
		}
		var stops atomic.Int32
		if outcome := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult {
			stops.Add(1)
			return 0
		}); !outcome.pending() {
			t.Fatalf("deferred ordinary Stop=%+v", outcome)
		}
		owner.completeNativeActivation(&shutdown)
		if result, completed := owner.completedStopResult(); stops.Load() != 1 || !completed || result != 0 {
			t.Fatalf("ordinary Stop calls=%d completed=%v result=%s", stops.Load(), completed, result.Hex())
		}
	})
}

func TestR11EachWaiterDequeueRequiresFreshPermit(t *testing.T) {
	var shutdown abruptShutdownCoordinator
	queue := make(chan int, 2)
	queue <- 1
	queue <- 2
	first, received := receiveAbruptOperation(&shutdown, queue)
	if !received || first != 1 {
		t.Fatalf("first dequeue value=%d received=%v", first, received)
	}
	if !shutdown.confirmAfterStop(nil, nil) {
		t.Fatal("confirmation rejected")
	}
	if second, received := receiveAbruptOperation(&shutdown, queue); received || second != 0 || len(queue) != 1 {
		t.Fatalf("post-latch dequeue value=%d received=%v queued=%d", second, received, len(queue))
	}
}
