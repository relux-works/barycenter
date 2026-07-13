package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func confirmR10Bounded(t *testing.T, adapter confirmedShutdownAdapter, stop func(uint32) winprobe.HResult) {
	t.Helper()
	confirmed := make(chan bool, 1)
	go func() { confirmed <- adapter.confirm(stop, nil) }()
	select {
	case accepted := <-confirmed:
		if !accepted {
			t.Fatal("confirmed shutdown was not accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("confirmed shutdown waited for an admitted ordinary operation")
	}
}

func indexedR10Gate(shutdown *abruptShutdownCoordinator, before func(int)) func(func()) bool {
	var index atomic.Int32
	return func(operation func()) bool {
		current := int(index.Add(1))
		if before != nil {
			before(current)
		}
		return shutdown.runOperation(operation)
	}
}

func TestR10F28StopBoundariesRejectLaterFinalizeAndRelease(t *testing.T) {
	t.Run("query failure stop blocks then confirmation", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 1001, 10001)
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		stopEntered := make(chan struct{})
		allowStop := make(chan struct{})
		cleanupDone := make(chan captureQueryFailureCleanupResult, 1)
		var stops, finalizes, releases atomic.Int32
		go func() {
			cleanupDone <- runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult {
				stops.Add(1)
				close(stopEntered)
				<-allowStop
				return 0
			}, func() error {
				finalizes.Add(1)
				return nil
			}, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			}, shutdown.runOperation)
		}()
		<-stopEntered
		confirmR10Bounded(t, adapter, func(uint32) winprobe.HResult {
			t.Error("confirmation duplicated an already-claimed Stop")
			return 0
		})
		close(allowStop)
		cleanup := <-cleanupDone
		if !cleanup.Stop.completed() || cleanup.Stop.Result != 0 || stops.Load() != 1 || finalizes.Load() != 0 || releases.Load() != 0 {
			t.Fatalf("cleanup=%+v calls stop=%d finalize=%d release=%d", cleanup, stops.Load(), finalizes.Load(), releases.Load())
		}
	})

	t.Run("confirmation after stop publication before finalize permit", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 1002, 10002)
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		beforeFinalize := make(chan struct{})
		continueFinalize := make(chan struct{})
		gate := indexedR10Gate(shutdown, func(index int) {
			if index == 2 {
				close(beforeFinalize)
				<-continueFinalize
			}
		})
		var finalizes, releases atomic.Int32
		done := make(chan captureQueryFailureCleanupResult, 1)
		go func() {
			done <- runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult { return 0 }, func() error {
				finalizes.Add(1)
				return nil
			}, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			}, gate)
		}()
		<-beforeFinalize
		confirmR10Bounded(t, adapter, func(uint32) winprobe.HResult { return 0 })
		close(continueFinalize)
		cleanup := <-done
		if !cleanup.Stop.completed() || finalizes.Load() != 0 || releases.Load() != 0 || cleanup.FinalizeAttempted || cleanup.ReleaseAttempted {
			t.Fatalf("cleanup=%+v finalize=%d release=%d", cleanup, finalizes.Load(), releases.Load())
		}
	})
}

func TestR10F28FinalizeAndReleaseHaveIndependentPermits(t *testing.T) {
	t.Run("admitted finalize may finish but release is rejected", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 1003, 10003)
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		finalizeEntered := make(chan struct{})
		allowFinalize := make(chan struct{})
		var releases atomic.Int32
		done := make(chan captureQueryFailureCleanupResult, 1)
		go func() {
			done <- runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult { return 0 }, func() error {
				close(finalizeEntered)
				<-allowFinalize
				return nil
			}, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			}, shutdown.runOperation)
		}()
		<-finalizeEntered
		confirmR10Bounded(t, adapter, func(uint32) winprobe.HResult { return 0 })
		close(allowFinalize)
		cleanup := <-done
		if !cleanup.FinalizeAttempted || cleanup.ReleaseAttempted || releases.Load() != 0 {
			t.Fatalf("cleanup=%+v releases=%d", cleanup, releases.Load())
		}
	})

	t.Run("confirmation immediately before release permit", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 1004, 10004)
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		beforeRelease := make(chan struct{})
		continueRelease := make(chan struct{})
		gate := indexedR10Gate(shutdown, func(index int) {
			if index == 3 {
				close(beforeRelease)
				<-continueRelease
			}
		})
		var finalizes, releases atomic.Int32
		done := make(chan captureQueryFailureCleanupResult, 1)
		go func() {
			done <- runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult { return 0 }, func() error {
				finalizes.Add(1)
				return nil
			}, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			}, gate)
		}()
		<-beforeRelease
		confirmR10Bounded(t, adapter, func(uint32) winprobe.HResult { return 0 })
		close(continueRelease)
		cleanup := <-done
		if finalizes.Load() != 1 || releases.Load() != 0 || !cleanup.FinalizeAttempted || cleanup.ReleaseAttempted {
			t.Fatalf("cleanup=%+v finalize=%d release=%d", cleanup, finalizes.Load(), releases.Load())
		}
	})

	t.Run("admitted release returns once without post-latch successors", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 1005, 10005)
		adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
		releaseEntered := make(chan struct{})
		allowRelease := make(chan struct{})
		var releases atomic.Int32
		done := make(chan captureQueryFailureCleanupResult, 1)
		go func() {
			done <- runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult { return 0 }, func() error { return nil }, func(uint32) winprobe.HResult {
				releases.Add(1)
				close(releaseEntered)
				<-allowRelease
				return 0
			}, shutdown.runOperation)
		}()
		<-releaseEntered
		confirmR10Bounded(t, adapter, func(uint32) winprobe.HResult { return 0 })
		close(allowRelease)
		cleanup := <-done
		var ownerClears, settlements, posts, evidence atomic.Int32
		continued := runCaptureContinuation(shutdown, true,
			func() { ownerClears.Add(1) },
			func() { settlements.Add(1) },
			func() { posts.Add(1) },
			func() { evidence.Add(1) },
		)
		if !cleanup.Released || releases.Load() != 1 || continued || ownerClears.Load()+settlements.Load()+posts.Load()+evidence.Load() != 0 || owners.current() != owner {
			t.Fatalf("cleanup=%+v release=%d continued=%v successors=%d current=%+v", cleanup, releases.Load(), continued, ownerClears.Load()+settlements.Load()+posts.Load()+evidence.Load(), owners.current())
		}
	})
}

func TestR10F28AdmittedQueryOrReadCannotAuthorizeSuccessors(t *testing.T) {
	for _, operation := range []string{"CaptureGetResult", "CaptureRead"} {
		operation := operation
		t.Run(operation, func(t *testing.T) {
			owners, shutdown, _ := newR8Owner(t, 1006, 10006)
			adapter := confirmedShutdownAdapter{shutdown: shutdown, owners: owners}
			entered := make(chan struct{})
			allow := make(chan struct{})
			done := make(chan bool, 1)
			go func() {
				_, admitted := runAbruptOperation(shutdown, func() winprobe.HResult {
					close(entered)
					<-allow
					return 0
				})
				done <- admitted
			}()
			<-entered
			confirmR10Bounded(t, adapter, func(uint32) winprobe.HResult { return 0 })
			close(allow)
			if !<-done {
				t.Fatal("pre-close operation did not retain its permit")
			}
			var permission, artifact, release, evidence, ui atomic.Int32
			for _, successor := range []func(){
				func() { permission.Add(1) },
				func() { artifact.Add(1) },
				func() { release.Add(1) },
				func() { evidence.Add(1) },
				func() { ui.Add(1) },
			} {
				if shutdown.runOperation(successor) {
					t.Error("post-latch successor acquired a permit")
				}
			}
			if permission.Load()+artifact.Load()+release.Load()+evidence.Load()+ui.Load() != 0 {
				t.Fatal("post-latch successor callback ran")
			}
		})
	}
}

func TestR10F28RetryAndAllCallbackSeamsRespectConfirmation(t *testing.T) {
	t.Run("finalized release and artifact retry reject late operation", func(t *testing.T) {
		owners, shutdown, owner := newR8Owner(t, 1007, 10007)
		if stop := requestCaptureStopOrReuse(owner, owner.operationID, func(uint32) winprobe.HResult { return 0 }); !stop.completed() {
			t.Fatalf("stop=%+v", stop)
		}
		confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: shutdown, owners: owners}, func(uint32) winprobe.HResult { return 0 })
		var releases, aborts atomic.Int32
		if _, admitted := runAbruptOperation(shutdown, func() captureReleaseOutcome {
			return owner.requestRelease(captureReleaseAfterAcceptedStop, func(uint32) winprobe.HResult {
				releases.Add(1)
				return 0
			})
		}); admitted {
			t.Fatal("finalized-release retry was admitted after confirmation")
		}
		if _, admitted := runAbruptOperation(shutdown, func() error {
			aborts.Add(1)
			return nil
		}); admitted {
			t.Fatal("artifact cleanup retry was admitted after confirmation")
		}
		if releases.Load() != 0 || aborts.Load() != 0 || owners.current() != owner {
			t.Fatalf("release=%d abort=%d current=%+v", releases.Load(), aborts.Load(), owners.current())
		}
	})

	for _, seam := range []string{"permission-query", "artifact-write", "release", "evidence", "ui-post", "hotkey", "tray", "helper-destroy"} {
		seam := seam
		t.Run("bounded-"+seam, func(t *testing.T) {
			owners, shutdown, _ := newR8Owner(t, 1100, 11000)
			entered := make(chan struct{})
			allow := make(chan struct{})
			done := make(chan bool, 1)
			go func() {
				done <- shutdown.runOperation(func() {
					close(entered)
					<-allow
				})
			}()
			<-entered
			confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: shutdown, owners: owners}, func(uint32) winprobe.HResult { return 0 })
			close(allow)
			if !<-done {
				t.Fatalf("pre-close %s operation lost its permit", seam)
			}
		})
	}
}

func TestR10F28GracefulAndCancelledShutdownPathsRemainOperational(t *testing.T) {
	owners, shutdown, owner := newR8Owner(t, 1008, 10008)
	var mu sync.Mutex
	var order []string
	record := func(value string) {
		mu.Lock()
		order = append(order, value)
		mu.Unlock()
	}
	cleanup := runCaptureQueryFailureCleanup(owner, owner.operationID, func(uint32) winprobe.HResult {
		record("Stop")
		return 0
	}, func() error {
		record("Finalize")
		return nil
	}, func(uint32) winprobe.HResult {
		record("Release")
		return 0
	}, shutdown.runOperation)
	if !cleanup.Released || strings.Join(order, ",") != "Stop,Finalize,Release" || owners.current() != owner {
		t.Fatalf("cleanup=%+v order=%v current=%+v", cleanup, order, owners.current())
	}

	tracker := newLifecycleTracker()
	tracker.beginLifecycle(lifecycleSystemShutdown, "WM_QUERYENDSESSION", winprobe.ReasonShutdown, lifecycleAbruptOSExit)
	if _, err := tracker.cancelShutdown("WM_ENDSESSION(cancelled)"); err != nil {
		t.Fatal(err)
	}
	advanceRun(t, tracker, lifecycleSystemShutdown, lifecycleStopRequested, lifecycleHotkeyUnregistered, lifecycleIdle)
	if _, accepted, reason := tracker.beginCaptureGeneration(); !accepted {
		t.Fatalf("cancelled shutdown did not restore the ordinary start gate: %s", reason)
	}
}

func TestR10F28OperationIDReuseKeepsExactOwnerIdentity(t *testing.T) {
	owners, shutdown, oldOwner := newR8Owner(t, 1009, 10009)
	if !oldOwner.observeNativeTerminal() {
		t.Fatal("old owner did not accept terminal evidence")
	}
	oldRelease := oldOwner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult { return 0 })
	if !oldRelease.released() || !owners.clearReleased(oldOwner) {
		t.Fatalf("old release=%+v", oldRelease)
	}
	newOwner, incumbent, published := owners.publish(shutdown, 1010, oldOwner.operationID, func(uint32) winprobe.HResult { return 0 })
	if !published || incumbent != nil || newOwner == oldOwner {
		t.Fatalf("new owner=%+v incumbent=%+v published=%v", newOwner, incumbent, published)
	}
	var staleStops, staleReleases, newStops atomic.Int32
	if stop := requestCaptureStopOrReuse(oldOwner, oldOwner.operationID, func(uint32) winprobe.HResult {
		staleStops.Add(1)
		return 0
	}); stop.State != captureStopNotRequested {
		t.Fatalf("stale stop=%+v", stop)
	}
	if release := oldOwner.requestRelease(captureReleaseAfterTerminal, func(uint32) winprobe.HResult {
		staleReleases.Add(1)
		return 0
	}); !release.released() {
		t.Fatalf("stable old release=%+v", release)
	}
	confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: shutdown, owners: owners}, func(operationID uint32) winprobe.HResult {
		if operationID != newOwner.operationID {
			t.Errorf("confirmation stopped operation %d, want reused current %d", operationID, newOwner.operationID)
		}
		newStops.Add(1)
		return 0
	})
	if staleStops.Load() != 0 || staleReleases.Load() != 0 || newStops.Load() != 1 || owners.current() != newOwner {
		t.Fatalf("staleStop=%d staleRelease=%d newStop=%d current=%+v", staleStops.Load(), staleReleases.Load(), newStops.Load(), owners.current())
	}
}

func TestR10F30BlockedStructuralEvidenceCannotEscalateAfterLatch(t *testing.T) {
	var shutdown abruptShutdownCoordinator
	evidenceEntered := make(chan struct{})
	allowEvidence := make(chan struct{})
	done := make(chan requiredFailureContinuationOutcome, 1)
	var escalations atomic.Int32
	failure := captureResultQueryFailureOutcome{InvalidNativeOwner: true}
	go func() {
		done <- failure.handleInvalidNativeOwner(&shutdown, func() bool {
			close(evidenceEntered)
			<-allowEvidence
			return true
		}, func() { escalations.Add(1) })
	}()
	<-evidenceEntered
	confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: &shutdown, owners: &captureOwnershipCoordinator{}}, func(uint32) winprobe.HResult { return 0 })
	close(allowEvidence)
	outcome := <-done
	if !outcome.admitted || !outcome.evidenceAttempted || !outcome.evidencePassed || outcome.escalated || escalations.Load() != 0 {
		t.Fatalf("outcome=%+v escalations=%d", outcome, escalations.Load())
	}
}

func TestR10F31BlockedContinuationCannotSmuggleSuccessors(t *testing.T) {
	t.Run("prepare result evidence blocks failure callback", func(t *testing.T) {
		var shutdown abruptShutdownCoordinator
		evidenceEntered := make(chan struct{})
		allowEvidence := make(chan struct{})
		done := make(chan capturePrepareCompletionOutcome, 1)
		var failures atomic.Int32
		prepared := capturePrepareCoordinatorResult{resultEvidenceAllowed: true}
		go func() {
			done <- prepared.dispatchPostHelper(&shutdown, nil, func() bool {
				close(evidenceEntered)
				<-allowEvidence
				return false
			}, func() { failures.Add(1) })
		}()
		<-evidenceEntered
		confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: &shutdown, owners: &captureOwnershipCoordinator{}}, func(uint32) winprobe.HResult { return 0 })
		close(allowEvidence)
		outcome := <-done
		if !outcome.evidenceAttempted || outcome.evidencePassed || failures.Load() != 0 {
			t.Fatalf("outcome=%+v failures=%d", outcome, failures.Load())
		}
	})

	t.Run("diagnostic blocks lifecycle query and settlement", func(t *testing.T) {
		tracker := newLifecycleTracker()
		generation, accepted, reason := tracker.beginCaptureGeneration()
		if !accepted {
			t.Fatalf("begin generation: %s", reason)
		}
		var shutdown abruptShutdownCoordinator
		var owners captureOwnershipCoordinator
		diagnosticEntered := make(chan struct{})
		allowDiagnostic := make(chan struct{})
		done := make(chan bool, 1)
		var settlements atomic.Int32
		go func() {
			done <- (capturePrepareCoordinatorResult{}).handleSuppressedPrepare(tracker, &owners, &shutdown, generation, func() {
				close(diagnosticEntered)
				<-allowDiagnostic
			}, func() { settlements.Add(1) })
		}()
		<-diagnosticEntered
		confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}, func(uint32) winprobe.HResult { return 0 })
		close(allowDiagnostic)
		if <-done || settlements.Load() != 0 {
			t.Fatalf("suppressed prepare crossed latch: settlements=%d", settlements.Load())
		}
	})

	t.Run("blocked log-class continuation suppresses enqueue lifecycle UI and later log", func(t *testing.T) {
		var shutdown abruptShutdownCoordinator
		firstEntered := make(chan struct{})
		allowFirst := make(chan struct{})
		done := make(chan bool, 1)
		var enqueue, lifecycle, ui, laterLog atomic.Int32
		go func() {
			done <- runCaptureContinuation(&shutdown, true,
				func() {
					close(firstEntered)
					<-allowFirst
				},
				func() { enqueue.Add(1) },
				func() { lifecycle.Add(1) },
				func() { ui.Add(1) },
				func() { laterLog.Add(1) },
			)
		}()
		<-firstEntered
		confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: &shutdown, owners: &captureOwnershipCoordinator{}}, func(uint32) winprobe.HResult { return 0 })
		close(allowFirst)
		if <-done || enqueue.Load()+lifecycle.Load()+ui.Load()+laterLog.Load() != 0 {
			t.Fatalf("successors enqueue=%d lifecycle=%d ui=%d log=%d", enqueue.Load(), lifecycle.Load(), ui.Load(), laterLog.Load())
		}
	})
}

func TestR10F32OrphanReleaseCannotClearOrSettleAfterLatch(t *testing.T) {
	var shutdown abruptShutdownCoordinator
	var owners captureOwnershipCoordinator
	incumbent, _, published := owners.publish(&shutdown, 1200, 12000, func(uint32) winprobe.HResult { return 0 })
	if !published {
		t.Fatal("incumbent was not published")
	}
	orphan := &captureOwnerSnapshot{generation: 1201, operationID: 12001}
	if !orphan.requestStop(func(uint32) winprobe.HResult { return 0 }) {
		t.Fatal("orphan Stop was not accepted")
	}
	obligation := owners.retainOrphan(orphan)
	if obligation == nil {
		t.Fatal("orphan obligation was not retained")
	}
	releaseEntered := make(chan struct{})
	allowRelease := make(chan struct{})
	drainDone := make(chan captureOrphanDrainResult, 1)
	go func() {
		drainDone <- runCaptureOrphanDrain(obligation, func(uint32) (winprobe.CaptureResult, winprobe.HResult) {
			return winprobe.CaptureResult{State: winprobe.CaptureStateStopped}, 0
		}, func(uint32) winprobe.HResult {
			close(releaseEntered)
			<-allowRelease
			return 0
		}, shutdown.runOperation)
	}()
	<-releaseEntered
	confirmR10Bounded(t, confirmedShutdownAdapter{shutdown: &shutdown, owners: &owners}, func(operationID uint32) winprobe.HResult {
		if operationID != incumbent.operationID {
			t.Errorf("confirmation stopped %d, want incumbent %d", operationID, incumbent.operationID)
		}
		return 0
	})
	close(allowRelease)
	drain := <-drainDone
	var evidence, lifecycle, ui atomic.Int32
	completion := completeCaptureOrphanDrain(&shutdown, &owners, obligation, drain, func() bool {
		evidence.Add(1)
		return true
	}, func() {
		lifecycle.Add(1)
		ui.Add(1)
	})
	if !drain.Release.released() || completion.admitted || completion.cleared || completion.evidenceStarted || completion.settled {
		t.Fatalf("drain=%+v completion=%+v", drain, completion)
	}
	if owners.orphanCount() != 1 || owners.current() != incumbent || evidence.Load()+lifecycle.Load()+ui.Load() != 0 {
		t.Fatalf("orphans=%d current=%+v evidence=%d lifecycle=%d ui=%d", owners.orphanCount(), owners.current(), evidence.Load(), lifecycle.Load(), ui.Load())
	}
}

func TestR10CaptureReleaseCallSitesHaveExactOwnerAndOperationGates(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "main_windows.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	callsByFunction := map[string]int{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "CaptureRelease" {
				callsByFunction[function.Name.Name]++
			}
			return true
		})
	}
	want := map[string]int{"requestCaptureRelease": 1, "drainCaptureOrphans": 1, "drainCapture": 1}
	if len(callsByFunction) != len(want) {
		t.Fatalf("direct CaptureRelease functions=%v, want %v", callsByFunction, want)
	}
	for function, count := range want {
		if callsByFunction[function] != count {
			t.Errorf("%s direct CaptureRelease calls=%d, want %d", function, callsByFunction[function], count)
		}
	}
	source, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"func (a *probeApp) requestCaptureRelease(owner *captureOwnerSnapshot",
		"return runAbruptOperation(&a.shutdown, func() captureReleaseOutcome",
		"runCaptureOrphanDrain(obligation, a.helper.CaptureResult",
		"}, a.shutdown.runOperation)",
		"failure := runCaptureResultQueryFailure(owner, id, callHR",
		"release, releaseAdmitted := a.requestCaptureRelease(owner, captureReleaseAfterTerminal)",
		"release, admitted := a.requestCaptureRelease(owner, releaseAuthority)",
		"func (a *probeApp) log(event winprobe.LogEvent) bool",
		"written, admitted := runAbruptOperation(&a.shutdown",
		"func (a *probeApp) enqueue(command waiterCommand) bool",
		"queued, admitted := runAbruptOperation(&a.shutdown",
		"completion := completeCaptureOrphanDrain(&a.shutdown, &a.captureOwners",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("production CaptureRelease boundary lacks %q", required)
		}
	}
	coordinatorSource, err := os.ReadFile("coordinators.go")
	if err != nil {
		t.Fatal(err)
	}
	coordinatorText := string(coordinatorSource)
	for _, required := range []string{
		"outcome.escalated = shutdown.runOperation(escalate)",
		"shutdown.runOperation(onEvidenceFailure)",
		"func completeCaptureOrphanDrain(",
		"result.evidenceStarted = shutdown.runOperation",
		"result.settled = settle == nil || shutdown.runOperation(settle)",
	} {
		if !strings.Contains(coordinatorText, required) {
			t.Errorf("operation-level successor boundary lacks %q", required)
		}
	}
}
