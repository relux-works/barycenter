package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

type uiTransitionKind uint8

const (
	uiTransitionIdleCleanup uiTransitionKind = iota + 1
	uiTransitionLifecycleRearm
)

type uiTransition struct {
	ID         uint64
	Kind       uiTransitionKind
	Generation uint64
	Status     winprobe.PermissionStatus
}

type pendingUITransition struct {
	uiTransition
	dispatch  uiDispatchState
	attempts  uint32
	escalated bool
}

type uiDispatchState uint8

const (
	uiDispatchPending uiDispatchState = iota
	uiDispatchPosting
	uiDispatchQueued
)

// uiTransitionCoordinator owns waiter-to-UI work until the UI acknowledges the
// exact transition ID. PostMessageW failure therefore cannot discard cleanup or
// rearm work, while a successful post is not duplicated before consumption.
type uiTransitionCoordinator struct {
	mu      sync.Mutex
	nextID  uint64
	pending map[uiTransitionKind]*pendingUITransition
	limit   uint32
}

func newUITransitionCoordinator(limit uint32) *uiTransitionCoordinator {
	if limit == 0 {
		limit = 1
	}
	return &uiTransitionCoordinator{pending: make(map[uiTransitionKind]*pendingUITransition), limit: limit}
}

func (c *uiTransitionCoordinator) publish(kind uiTransitionKind, generation uint64, status winprobe.PermissionStatus) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current := c.pending[kind]; current != nil && current.Generation == generation && current.Status == status {
		return current.ID
	}
	c.nextID++
	c.pending[kind] = &pendingUITransition{uiTransition: uiTransition{ID: c.nextID, Kind: kind, Generation: generation, Status: status}}
	return c.nextID
}

// drive attempts each transition that has not reached the window queue. It
// returns true exactly once per transition after the bounded post-failure limit
// so production can escalate to graceful cleanup without losing ownership.
func (c *uiTransitionCoordinator) drive(post func(uiTransition) bool, operationGate ...func(func()) bool) (escalate bool) {
	for _, kind := range []uiTransitionKind{uiTransitionIdleCleanup, uiTransitionLifecycleRearm} {
		var transition uiTransition
		var claimed bool
		if !runOperationGate(operationGate, func() {
			transition, claimed = c.claimPost(kind)
		}) || !claimed {
			continue
		}
		posted := post(transition)
		var shouldEscalate bool
		if !runOperationGate(operationGate, func() {
			shouldEscalate = c.finishPost(transition, posted)
		}) {
			return escalate
		}
		if shouldEscalate {
			escalate = true
		}
	}
	return escalate
}

func (c *uiTransitionCoordinator) claimPost(kind uiTransitionKind) (uiTransition, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending[kind]
	if pending == nil || pending.dispatch != uiDispatchPending || pending.escalated {
		return uiTransition{}, false
	}
	pending.dispatch = uiDispatchPosting
	return pending.uiTransition, true
}

func (c *uiTransitionCoordinator) finishPost(transition uiTransition, posted bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending[transition.Kind]
	if pending == nil || pending.ID != transition.ID {
		return false
	}
	if posted {
		pending.dispatch = uiDispatchQueued
		return false
	}
	pending.dispatch = uiDispatchPending
	pending.attempts++
	if pending.attempts < c.limit || pending.escalated {
		return false
	}
	pending.escalated = true
	return true
}

func (c *uiTransitionCoordinator) consume(kind uiTransitionKind, id uint64) (uiTransition, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending[kind]
	if pending == nil || pending.ID != id || (pending.dispatch != uiDispatchPosting && pending.dispatch != uiDispatchQueued) {
		return uiTransition{}, false
	}
	transition := pending.uiTransition
	delete(c.pending, kind)
	return transition, true
}

func (c *uiTransitionCoordinator) pendingTransition(kind uiTransitionKind) (uiTransition, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending[kind]
	if pending == nil {
		return uiTransition{}, false
	}
	return pending.uiTransition, true
}

// abruptShutdownCoordinator is the production linearization point between
// confirmed WM_ENDSESSION and waiter-owned helper work. A callback admitted
// before confirmation may finish, but confirmation never waits for it and no
// later ordinary callback can start. The confirmed callback runs at most once.
type abruptShutdownCoordinator struct {
	closing          atomic.Bool
	confirmed        atomic.Bool
	abruptDispatched atomic.Bool
	confirmedCapture atomic.Pointer[captureOwnerSnapshot]
}

// confirmAfterStop preserves the Windows shutdown contract: the nonblocking
// stop request is issued before the monotonic confirmation latch and wake.
func (c *abruptShutdownCoordinator) confirmAfterStop(requestStop, wake func()) bool {
	if !c.closing.CompareAndSwap(false, true) {
		return false
	}
	if requestStop != nil {
		requestStop()
	}
	c.confirmed.Store(true)
	if wake != nil {
		wake()
	}
	return true
}

func (c *abruptShutdownCoordinator) isConfirmed() bool {
	return c.confirmed.Load()
}

func (c *abruptShutdownCoordinator) isClosing() bool {
	return c.closing.Load()
}

func (c *abruptShutdownCoordinator) runOrdinary(work func()) bool {
	if c.isClosing() {
		return false
	}
	if work != nil {
		work()
	}
	return true
}

// runOperation is the operation-level abrupt-shutdown permit. The acquire
// load that observes closing=false is the permit linearization point. The
// admitted callback may finish after confirmation, but every independently
// dangerous successor must obtain a new permit immediately before its own
// callback. Confirmation never waits for an admitted operation.
func (c *abruptShutdownCoordinator) runOperation(work func()) bool {
	return c.runOrdinary(work)
}

func runOperationGate(gates []func(func()) bool, operation func()) bool {
	if len(gates) != 0 && gates[0] != nil {
		return gates[0](operation)
	}
	if operation != nil {
		operation()
	}
	return true
}

func runAbruptOperation[T any](shutdown *abruptShutdownCoordinator, operation func() T) (T, bool) {
	var result T
	if shutdown == nil || operation == nil {
		return result, false
	}
	admitted := shutdown.runOperation(func() { result = operation() })
	return result, admitted
}

// receiveAbruptOperation gives each queue dequeue its own immediate pre-close
// permit. A waiter drain admitted earlier cannot consume a later command after
// confirmed shutdown closes the ordinary-work gate.
func receiveAbruptOperation[T any](shutdown *abruptShutdownCoordinator, queue <-chan T) (T, bool) {
	var value T
	if shutdown == nil || queue == nil {
		return value, false
	}
	received := false
	admitted := shutdown.runOperation(func() {
		select {
		case value = <-queue:
			received = true
		default:
		}
	})
	return value, admitted && received
}

// runFailClosedQueryOperations keeps cancel and release as two independent
// abrupt-shutdown operations. A pre-close cancel result never authorizes a
// Release that has not acquired its own permit.
func runFailClosedQueryOperations(
	gate func(func()) bool,
	cancel func() winprobe.HResult,
	release func() winprobe.HResult,
) (queryFailureOutcome, bool) {
	var outcome queryFailureOutcome
	if cancel != nil && !runOperationGate([]func(func()) bool{gate}, func() {
		outcome.cancelHR = cancel()
	}) {
		return outcome, false
	}
	if release != nil && !runOperationGate([]func(func()) bool{gate}, func() {
		outcome.releaseHR = release()
		outcome.released = outcome.releaseHR.Succeeded()
	}) {
		return outcome, false
	}
	return outcome, true
}

// runWaiterIteration checks confirmation before every ordinary drain, not the
// index returned by WaitForMultipleObjects. This gives confirmed shutdown
// priority when readiness events are coalesced or a lower-index event wins.
func (c *abruptShutdownCoordinator) runWaiterIteration(ordinary []func(), abrupt func()) bool {
	if c.dispatchAbrupt(abrupt) {
		return true
	}
	for _, work := range ordinary {
		if !c.runOrdinary(work) {
			return c.dispatchAbrupt(abrupt)
		}
	}
	return c.dispatchAbrupt(abrupt)
}

func (c *abruptShutdownCoordinator) dispatchAbrupt(abrupt func()) bool {
	if !c.isConfirmed() {
		return false
	}
	invoke := c.abruptDispatched.CompareAndSwap(false, true)
	if invoke && abrupt != nil {
		abrupt()
	}
	return true
}

// captureOwnerSnapshot is immutable after creation except for its one-shot
// native-stop claim. Keeping generation and operation in one object prevents
// shutdown and conflict disposal from pairing values from different capture
// generations, including owners that never become the active publication.
type captureOwnerSnapshot struct {
	generation    uint64
	operationID   uint32
	state         atomic.Uint32
	stopMu        sync.Mutex
	deferredStop  func(uint32) winprobe.HResult
	stopResult    atomic.Int32
	stopResultSet atomic.Bool
}

const captureReleaseBeforeTerminalHResult winprobe.HResult = -2147483634 // E_ILLEGAL_METHOD_CALL

const (
	captureOwnerStopClaimed uint32 = 1 << iota
	captureOwnerActivationIntentAdmitted
	captureOwnerNativeActivationAdmitted
	captureOwnerNativeActivationCompleted
	captureOwnerTerminalObserved
	captureOwnerReleaseAdmitted
	captureOwnerReleased
)

func (o *captureOwnerSnapshot) matches(generation uint64, operationID uint32) bool {
	return o != nil && o.generation == generation && o.operationID == operationID
}

func (o *captureOwnerSnapshot) requestStop(stop func(uint32) winprobe.HResult) bool {
	if !o.claimStop(stop) {
		return false
	}
	o.invokeClaimedStopAdmitted()
	return true
}

// claimStop publishes a durable unique Stop producer without invoking native
// code. Callers may therefore make an exact owner visible to another thread
// only after this method returns: a waiter that observes StopClaimed either
// sees the published result or knows this stored callback will publish it.
func (o *captureOwnerSnapshot) claimStop(stop func(uint32) winprobe.HResult) bool {
	if o == nil || o.operationID == 0 || stop == nil || !o.claimState(captureOwnerStopClaimed, captureOwnerStopClaimed|captureOwnerTerminalObserved|captureOwnerReleaseAdmitted|captureOwnerReleased) {
		return false
	}
	o.stopMu.Lock()
	o.deferredStop = stop
	o.stopMu.Unlock()
	return true
}

// invokeClaimedStop consumes a separately published producer only after its
// own immediate pre-close permit. The low-level admitted form is also used by
// requestStop, whose claim and invocation share either an ordinary operation
// permit or the special pre-latch confirmed-shutdown call.
func (o *captureOwnerSnapshot) invokeClaimedStop(shutdown *abruptShutdownCoordinator) bool {
	invoked := false
	if shutdown == nil || !shutdown.runOperation(func() { invoked = o.invokeClaimedStopAdmitted() }) {
		return false
	}
	return invoked
}

// invokeClaimedStopAdmitted consumes the stored producer exactly once when
// native activation is not in flight. Its callers already own the permit that
// authorizes this immediate callback.
func (o *captureOwnerSnapshot) invokeClaimedStopAdmitted() bool {
	if o == nil {
		return false
	}
	o.stopMu.Lock()
	state := o.state.Load()
	if state&captureOwnerStopClaimed == 0 || o.stopResultSet.Load() || (state&captureOwnerNativeActivationAdmitted != 0 && state&captureOwnerNativeActivationCompleted == 0) {
		o.stopMu.Unlock()
		return false
	}
	stop := o.deferredStop
	o.deferredStop = nil
	o.stopMu.Unlock()
	if stop == nil {
		return false
	}
	o.finishStop(stop)
	return true
}

func (o *captureOwnerSnapshot) finishStop(stop func(uint32) winprobe.HResult) {
	result := stop(o.operationID)
	o.stopResult.Store(int32(result))
	o.stopResultSet.Store(true)
}

// admitActivationIntent is the exact-owner linearization point between a
// queued readiness callback and every one-shot stop. A stop that wins first
// suppresses both activation intent evidence and the native activation. An
// intent that wins may still be stopped before native admission, but no second
// queued activation can enter.
func (o *captureOwnerSnapshot) admitActivationIntent() bool {
	return o != nil && o.operationID != 0 && o.claimState(captureOwnerActivationIntentAdmitted, captureOwnerStopClaimed|captureOwnerActivationIntentAdmitted|captureOwnerTerminalObserved|captureOwnerReleaseAdmitted|captureOwnerReleased)
}

// admitNativeActivation defines the second race boundary. If a stop was
// claimed while activation-intent evidence was being written, native
// activation is rejected. If native admission wins, the exact operation may
// enter its already-admitted callback and a later stop still remains one-shot.
func (o *captureOwnerSnapshot) admitNativeActivation() bool {
	if o == nil {
		return false
	}
	for {
		state := o.state.Load()
		if state&captureOwnerActivationIntentAdmitted == 0 || state&(captureOwnerStopClaimed|captureOwnerNativeActivationAdmitted|captureOwnerTerminalObserved|captureOwnerReleaseAdmitted|captureOwnerReleased) != 0 {
			return false
		}
		if o.state.CompareAndSwap(state, state|captureOwnerNativeActivationAdmitted) {
			return true
		}
	}
}

// completeNativeActivation closes the admission-to-call window before running
// a deferred exact-owner stop. requestStop never waits for this transition;
// abrupt confirmation can latch/wake while the activation callback owns the
// native call, and the winning stop is issued exactly once when that call
// returns.
func (o *captureOwnerSnapshot) completeNativeActivation(shutdown *abruptShutdownCoordinator) {
	if o == nil {
		return
	}
	o.stopMu.Lock()
	for {
		state := o.state.Load()
		if state&captureOwnerNativeActivationAdmitted == 0 || state&captureOwnerNativeActivationCompleted != 0 {
			o.stopMu.Unlock()
			return
		}
		if o.state.CompareAndSwap(state, state|captureOwnerNativeActivationCompleted) {
			break
		}
	}
	stop := o.deferredStop
	o.deferredStop = nil
	o.stopMu.Unlock()
	if stop != nil {
		// Activation owns only its already-admitted helper callback. A Stop
		// deferred behind that callback is a distinct native operation and must
		// acquire a fresh permit immediately before invocation. Confirmed
		// WM_ENDSESSION therefore abandons the stored producer to OS/process
		// teardown instead of starting new helper work after the latch.
		if shutdown != nil {
			shutdown.runOperation(func() { o.finishStop(stop) })
		}
	}
}

func (o *captureOwnerSnapshot) claimState(add, rejected uint32) bool {
	for {
		state := o.state.Load()
		if state&rejected != 0 {
			return false
		}
		if o.state.CompareAndSwap(state, state|add) {
			return true
		}
	}
}

func (o *captureOwnerSnapshot) requestShutdownStop(stop func(uint32) winprobe.HResult) bool {
	return o.requestStop(stop)
}

func (o *captureOwnerSnapshot) completedStopResult() (winprobe.HResult, bool) {
	if o == nil || !o.stopResultSet.Load() {
		return 0, false
	}
	return winprobe.HResult(o.stopResult.Load()), true
}

// observeNativeTerminal records authoritative helper terminal evidence before
// artifact finalization. A false return means an admitted Stop or native
// activation still owns a live producer; callers must not finalize or release
// until that producer has published its result/completion.
func (o *captureOwnerSnapshot) observeNativeTerminal() bool {
	if o == nil || o.operationID == 0 {
		return false
	}
	for {
		state := o.state.Load()
		if state&captureOwnerReleased != 0 {
			return false
		}
		if state&captureOwnerTerminalObserved == 0 {
			if !o.state.CompareAndSwap(state, state|captureOwnerTerminalObserved) {
				continue
			}
			state |= captureOwnerTerminalObserved
		}
		if state&captureOwnerNativeActivationAdmitted != 0 && state&captureOwnerNativeActivationCompleted == 0 {
			return false
		}
		if state&captureOwnerStopClaimed != 0 && !o.stopResultSet.Load() {
			return false
		}
		return true
	}
}

// finalizedReleaseAuthority prevents retry code from guessing why an artifact
// is finalized. Native terminal evidence is authoritative by itself; otherwise
// only an exact completed S_OK Stop from query-failure recovery permits retry.
// A failed, unexpected, or still-pending Stop never authorizes Release.
func (o *captureOwnerSnapshot) finalizedReleaseAuthority() (captureReleaseAuthority, bool) {
	if o == nil || o.operationID == 0 || o.state.Load()&captureOwnerReleased != 0 {
		return 0, false
	}
	if o.state.Load()&captureOwnerTerminalObserved != 0 {
		return captureReleaseAfterTerminal, true
	}
	result, completed := o.completedStopResult()
	if completed && result == 0 {
		return captureReleaseAfterAcceptedStop, true
	}
	return 0, false
}

type captureReleaseState uint8

const (
	captureReleaseNotAttempted captureReleaseState = iota
	captureReleasePending
	captureReleaseCompleted
)

type captureReleaseOutcome struct {
	State  captureReleaseState
	Result winprobe.HResult
}

func (o captureReleaseOutcome) pending() bool {
	return o.State == captureReleasePending
}

func (o captureReleaseOutcome) attempted() bool {
	return o.State == captureReleaseCompleted
}

func (o captureReleaseOutcome) released() bool {
	return o.State == captureReleaseCompleted && o.Result == 0
}

type captureReleaseAuthority uint8

const (
	captureReleaseAfterTerminal captureReleaseAuthority = iota + 1
	captureReleaseAfterAcceptedStop
)

// requestRelease owns both admission and the actual CaptureRelease invocation
// for this exact immutable owner. StopClaimed is published before a Stop call
// is selected, so release cannot overtake either an immediate or deferred Stop.
// Only exact S_OK transfers ownership; every other HRESULT leaves the same
// owner retryable and visible to the app.
func (o *captureOwnerSnapshot) requestRelease(authority captureReleaseAuthority, release func(uint32) winprobe.HResult) captureReleaseOutcome {
	if o == nil || o.operationID == 0 || release == nil {
		return captureReleaseOutcome{}
	}
	for {
		state := o.state.Load()
		if state&captureOwnerReleased != 0 {
			return captureReleaseOutcome{State: captureReleaseCompleted, Result: 0}
		}
		if state&captureOwnerReleaseAdmitted != 0 {
			return captureReleaseOutcome{State: captureReleasePending}
		}
		if state&captureOwnerNativeActivationAdmitted != 0 && state&captureOwnerNativeActivationCompleted == 0 {
			return captureReleaseOutcome{State: captureReleasePending}
		}
		switch authority {
		case captureReleaseAfterTerminal:
			if state&captureOwnerTerminalObserved == 0 {
				return captureReleaseOutcome{}
			}
			if state&captureOwnerStopClaimed != 0 && !o.stopResultSet.Load() {
				return captureReleaseOutcome{State: captureReleasePending}
			}
		case captureReleaseAfterAcceptedStop:
			if state&captureOwnerStopClaimed == 0 || !o.stopResultSet.Load() {
				return captureReleaseOutcome{State: captureReleasePending}
			}
			stopResult, _ := o.completedStopResult()
			if stopResult != 0 {
				return captureReleaseOutcome{}
			}
		default:
			return captureReleaseOutcome{}
		}
		if !o.state.CompareAndSwap(state, state|captureOwnerReleaseAdmitted) {
			continue
		}
		break
	}

	result := release(o.operationID)
	for {
		state := o.state.Load()
		next := state &^ captureOwnerReleaseAdmitted
		if result == 0 {
			next |= captureOwnerReleased
		}
		if o.state.CompareAndSwap(state, next) {
			break
		}
	}
	return captureReleaseOutcome{State: captureReleaseCompleted, Result: result}
}

type captureStopState uint8

const (
	captureStopNotRequested captureStopState = iota
	captureStopPending
	captureStopCompleted
)

type captureStopOutcome struct {
	State  captureStopState
	Result winprobe.HResult
}

func (o captureStopOutcome) completed() bool {
	return o.State == captureStopCompleted
}

func (o captureStopOutcome) pending() bool {
	return o.State == captureStopPending
}

// requestCaptureStopOrReuse never converts an in-flight exact-owner request
// into numeric S_OK. Callers must retain release/finalization ownership while
// State is pending and may proceed only after completion or independent native
// terminal evidence.
func requestCaptureStopOrReuse(owner *captureOwnerSnapshot, operationID uint32, stop func(uint32) winprobe.HResult) captureStopOutcome {
	if owner == nil || !owner.matches(owner.generation, operationID) || operationID == 0 || stop == nil {
		return captureStopOutcome{}
	}
	if owner.state.Load()&captureOwnerReleased != 0 {
		return captureStopOutcome{}
	}
	if result, ok := owner.completedStopResult(); ok {
		return captureStopOutcome{State: captureStopCompleted, Result: result}
	}
	if owner.requestStop(stop) {
		if result, ok := owner.completedStopResult(); ok {
			return captureStopOutcome{State: captureStopCompleted, Result: result}
		}
		return captureStopOutcome{State: captureStopPending}
	}
	if result, ok := owner.completedStopResult(); ok {
		return captureStopOutcome{State: captureStopCompleted, Result: result}
	}
	state := owner.state.Load()
	if state&captureOwnerStopClaimed != 0 {
		return captureStopOutcome{State: captureStopPending}
	}
	if state&(captureOwnerTerminalObserved|captureOwnerReleaseAdmitted|captureOwnerReleased) != 0 {
		return captureStopOutcome{}
	}
	return captureStopOutcome{}
}

func requestCaptureStopForExactOwner(
	owners *captureOwnershipCoordinator,
	generation uint64,
	operationID uint32,
	stop func(uint32) winprobe.HResult,
) captureStopOutcome {
	if owners == nil || generation == 0 || operationID == 0 {
		return captureStopOutcome{}
	}
	return requestCaptureStopOrReuse(owners.exactOwner(generation, operationID), operationID, stop)
}

type captureQueryFailureCleanupResult struct {
	Stop                  captureStopOutcome
	StructuralFailure     bool
	ReleaseAwaitsTerminal bool
	FinalizeAttempted     bool
	FinalizeError         error
	ReleaseAttempted      bool
	ReleaseResult         winprobe.HResult
	Released              bool
}

const captureInvalidHandleHResult winprobe.HResult = -2147024890 // HRESULT_FROM_WIN32(ERROR_INVALID_HANDLE)

type captureResultQueryFailureOutcome struct {
	InvalidNativeOwner bool
	Cleanup            captureQueryFailureCleanupResult
}

type requiredFailureContinuationOutcome struct {
	admitted          bool
	evidenceAttempted bool
	evidencePassed    bool
	escalated         bool
}

func runRequiredFailureContinuation(
	shutdown *abruptShutdownCoordinator,
	allowed bool,
	requiredEvidence func() bool,
	escalate func(),
) requiredFailureContinuationOutcome {
	var outcome requiredFailureContinuationOutcome
	if shutdown == nil || !allowed || requiredEvidence == nil {
		return outcome
	}
	outcome.admitted = shutdown.runOperation(func() {
		outcome.evidenceAttempted = true
		outcome.evidencePassed = requiredEvidence()
	})
	if !outcome.admitted || !outcome.evidencePassed || escalate == nil {
		return outcome
	}
	outcome.escalated = shutdown.runOperation(escalate)
	return outcome
}

func (r captureResultQueryFailureOutcome) handleInvalidNativeOwner(
	shutdown *abruptShutdownCoordinator,
	requiredEvidence func() bool,
	escalate func(),
) requiredFailureContinuationOutcome {
	if !r.InvalidNativeOwner {
		return requiredFailureContinuationOutcome{}
	}
	return runRequiredFailureContinuation(shutdown, true, requiredEvidence, escalate)
}

// runCaptureResultQueryFailure is the first failed CaptureGetResult boundary.
// ERROR_INVALID_HANDLE for the exact published nonzero owner is structural:
// an idempotent Stop/Release response cannot prove that operation ever existed,
// so no native or artifact cleanup claim is attempted here. The caller records
// required evidence and enters the existing bounded graceful-exit path.
func runCaptureResultQueryFailure(
	owner *captureOwnerSnapshot,
	operationID uint32,
	callResult winprobe.HResult,
	stop func(uint32) winprobe.HResult,
	finalize func() error,
	release func(uint32) winprobe.HResult,
	ordinaryGate ...func(func()) bool,
) captureResultQueryFailureOutcome {
	if operationID != 0 && (owner == nil || !owner.matches(owner.generation, operationID) || callResult == captureInvalidHandleHResult) {
		return captureResultQueryFailureOutcome{InvalidNativeOwner: true}
	}
	return captureResultQueryFailureOutcome{Cleanup: runCaptureQueryFailureCleanup(owner, operationID, stop, finalize, release, ordinaryGate...)}
}

// runCaptureQueryFailureCleanup is the production release gate for an
// unqueryable capture operation. A claimed but incomplete stop retains native
// and artifact ownership; cleanup retries after the exact stop result becomes
// observable. Independent native-terminal handling may use its own release
// path because that terminal evidence is authoritative.
func runCaptureQueryFailureCleanup(
	owner *captureOwnerSnapshot,
	operationID uint32,
	stop func(uint32) winprobe.HResult,
	finalize func() error,
	release func(uint32) winprobe.HResult,
	ordinaryGate ...func(func()) bool,
) captureQueryFailureCleanupResult {
	var result captureQueryFailureCleanupResult
	if !runOperationGate(ordinaryGate, func() {
		result.Stop = requestCaptureStopOrReuse(owner, operationID, stop)
	}) {
		return result
	}
	if !result.Stop.completed() {
		return result
	}
	if result.Stop.Result != 0 {
		result.StructuralFailure = true
		return result
	}
	if finalize != nil {
		if !runOperationGate(ordinaryGate, func() {
			result.FinalizeAttempted = true
			result.FinalizeError = finalize()
		}) {
			return result
		}
	}
	if release != nil {
		var releaseOutcome captureReleaseOutcome
		if !runOperationGate(ordinaryGate, func() {
			releaseOutcome = owner.requestRelease(captureReleaseAfterAcceptedStop, release)
		}) {
			return result
		}
		result.ReleaseAttempted = releaseOutcome.attempted()
		result.ReleaseResult = releaseOutcome.Result
		result.Released = releaseOutcome.released()
		if result.ReleaseAttempted && result.ReleaseResult == captureReleaseBeforeTerminalHResult {
			result.ReleaseAwaitsTerminal = true
		} else if result.ReleaseAttempted && !result.Released {
			result.StructuralFailure = true
		}
	}
	return result
}

// captureOwnershipCoordinator publishes the exact current native owner without
// taking lifecycleTracker.mu. Publication checks the abrupt start gate on both
// sides of the atomic store: confirmation either sees and stops the owner before
// its latch, or a late publisher observes the closed gate and hands the owner to
// OS/process teardown without starting another helper callback.
type captureOwnershipCoordinator struct {
	active             atomic.Pointer[captureOwnerSnapshot]
	activationAttempts atomic.Uint64
	orphanMu           sync.Mutex
	orphans            []*captureOrphanObligation
}

type captureOrphanObligation struct {
	owner            *captureOwnerSnapshot
	failureEscalated atomic.Bool
}

func (c *captureOwnershipCoordinator) retainOrphan(owner *captureOwnerSnapshot) *captureOrphanObligation {
	if c == nil || owner == nil || owner.operationID == 0 {
		return nil
	}
	c.orphanMu.Lock()
	defer c.orphanMu.Unlock()
	for _, existing := range c.orphans {
		if existing.owner == owner {
			return existing
		}
	}
	obligation := &captureOrphanObligation{owner: owner}
	c.orphans = append(c.orphans, obligation)
	return obligation
}

// publishOrphanStopProducer closes the orphan visibility race: the exact
// owner carries a stored, one-shot Stop producer before it enters the waiter-
// visible obligation list. Native Stop is deliberately invoked afterward so
// confirmed shutdown never waits for this ordinary helper callback.
func (c *captureOwnershipCoordinator) publishOrphanStopProducer(owner *captureOwnerSnapshot, stop func(uint32) winprobe.HResult) (*captureOrphanObligation, bool) {
	if c == nil || owner == nil || !owner.claimStop(stop) {
		return nil, false
	}
	obligation := c.retainOrphan(owner)
	if obligation == nil {
		return nil, false
	}
	return obligation, true
}

func (c *captureOwnershipCoordinator) orphanSnapshot() []*captureOrphanObligation {
	if c == nil {
		return nil
	}
	c.orphanMu.Lock()
	defer c.orphanMu.Unlock()
	return append([]*captureOrphanObligation(nil), c.orphans...)
}

func (c *captureOwnershipCoordinator) completeOrphan(obligation *captureOrphanObligation) bool {
	if c == nil || obligation == nil || obligation.owner == nil || obligation.owner.state.Load()&captureOwnerReleased == 0 {
		return false
	}
	c.orphanMu.Lock()
	defer c.orphanMu.Unlock()
	for index, current := range c.orphans {
		if current != obligation || current.owner != obligation.owner {
			continue
		}
		copy(c.orphans[index:], c.orphans[index+1:])
		c.orphans[len(c.orphans)-1] = nil
		c.orphans = c.orphans[:len(c.orphans)-1]
		return true
	}
	return false
}

func (c *captureOwnershipCoordinator) orphanCount() int {
	if c == nil {
		return 0
	}
	c.orphanMu.Lock()
	defer c.orphanMu.Unlock()
	return len(c.orphans)
}

type captureOrphanDrainResult struct {
	Stop              captureStopOutcome
	QueryAttempted    bool
	QueryResult       winprobe.CaptureResult
	QueryHResult      winprobe.HResult
	TerminalObserved  bool
	Release           captureReleaseOutcome
	StructuralFailure bool
}

// runCaptureOrphanDrain is the waiter-owned cleanup path for a successful
// CapturePrepare result that lost active publication. It has no artifact or
// activation successors: the exact loser must first expose its accepted Stop
// result, then native terminal state, then pass through its own Release gate.
func runCaptureOrphanDrain(
	obligation *captureOrphanObligation,
	query func(uint32) (winprobe.CaptureResult, winprobe.HResult),
	release func(uint32) winprobe.HResult,
	ordinaryGate ...func(func()) bool,
) captureOrphanDrainResult {
	var result captureOrphanDrainResult
	if obligation == nil || obligation.owner == nil || obligation.owner.operationID == 0 || query == nil || release == nil {
		result.StructuralFailure = true
		return result
	}
	owner := obligation.owner
	if owner.state.Load()&captureOwnerReleased != 0 {
		return result
	}
	if stopResult, ok := owner.completedStopResult(); ok {
		result.Stop = captureStopOutcome{State: captureStopCompleted, Result: stopResult}
		if stopResult != 0 {
			result.StructuralFailure = true
			return result
		}
	} else if owner.state.Load()&captureOwnerStopClaimed != 0 {
		result.Stop = captureStopOutcome{State: captureStopPending}
		return result
	} else {
		result.StructuralFailure = true
		return result
	}

	if !runOperationGate(ordinaryGate, func() {
		result.QueryAttempted = true
		result.QueryResult, result.QueryHResult = query(owner.operationID)
	}) {
		return result
	}
	if result.QueryHResult != 0 {
		result.StructuralFailure = true
		return result
	}
	if result.QueryResult.State < winprobe.CaptureStateStopped {
		return result
	}
	if !runOperationGate(ordinaryGate, func() {
		result.TerminalObserved = owner.observeNativeTerminal()
	}) || !result.TerminalObserved {
		return result
	}
	if !runOperationGate(ordinaryGate, func() {
		result.Release = owner.requestRelease(captureReleaseAfterTerminal, release)
	}) {
		return result
	}
	if result.Release.attempted() && !result.Release.released() {
		result.StructuralFailure = true
	}
	return result
}

func (c *captureOwnershipCoordinator) publish(
	shutdown *abruptShutdownCoordinator,
	generation uint64,
	operationID uint32,
	_ func(uint32) winprobe.HResult,
) (candidate, incumbent *captureOwnerSnapshot, published bool) {
	if generation == 0 || operationID == 0 {
		return nil, nil, false
	}
	owner := &captureOwnerSnapshot{generation: generation, operationID: operationID}
	publish := func() {
		for {
			if existing := c.active.Load(); existing != nil {
				incumbent = existing
				return
			}
			if c.active.CompareAndSwap(nil, owner) {
				break
			}
		}
		if shutdown != nil && shutdown.isClosing() {
			c.active.CompareAndSwap(owner, nil)
			return
		}
		published = true
	}
	if shutdown != nil && !shutdown.runOperation(publish) {
		return owner, nil, false
	}
	if shutdown == nil {
		publish()
	}
	return owner, incumbent, published
}

func (c *captureOwnershipCoordinator) current() *captureOwnerSnapshot {
	return c.active.Load()
}

func (c *captureOwnershipCoordinator) matching(generation uint64, operationID uint32) *captureOwnerSnapshot {
	owner := c.current()
	if owner.matches(generation, operationID) {
		return owner
	}
	return nil
}

func (c *captureOwnershipCoordinator) exactOwner(generation uint64, operationID uint32) *captureOwnerSnapshot {
	if owner := c.matching(generation, operationID); owner != nil {
		return owner
	}
	for _, obligation := range c.orphanSnapshot() {
		if obligation.owner.matches(generation, operationID) {
			return obligation.owner
		}
	}
	return nil
}

func (c *captureOwnershipCoordinator) clear(generation uint64, operationID uint32) bool {
	owner := c.matching(generation, operationID)
	return c.clearReleased(owner)
}

func (c *captureOwnershipCoordinator) clearReleased(owner *captureOwnerSnapshot) bool {
	if owner == nil || owner.state.Load()&captureOwnerReleased == 0 || c.active.Load() != owner {
		return false
	}
	return c.active.CompareAndSwap(owner, nil)
}

// confirmedShutdownMessageGate is the shared wndproc entry guard. Once the
// abrupt latch is set, the Windows message pump may continue dispatching queued
// messages, but application callbacks must not run. Only protocol-required
// return values are produced.
type confirmedShutdownMessageGate struct {
	shutdown *abruptShutdownCoordinator
}

func (g confirmedShutdownMessageGate) enter(message uint32, application func()) (uintptr, bool) {
	if g.shutdown == nil || !g.shutdown.isClosing() {
		if application != nil {
			application()
		}
		return 0, false
	}
	if message == lifecycleWMQueryEndSession || message == lifecycleWMPowerBroadcast {
		return 1, true
	}
	return 0, true
}

// confirmedShutdownAdapter binds the exact active capture generation and
// requests its nonblocking native stop before publishing confirmation. It does
// not advance lifecycle stages, replay settlement, write evidence, or release
// any resource; Windows and startup recovery own those abrupt boundaries.
type confirmedShutdownAdapter struct {
	shutdown *abruptShutdownCoordinator
	owners   *captureOwnershipCoordinator
}

func (a confirmedShutdownAdapter) confirm(stop func(uint32) winprobe.HResult, wake func()) bool {
	if a.shutdown == nil || a.owners == nil {
		return false
	}
	return a.shutdown.confirmAfterStop(func() {
		owner := a.owners.current()
		a.shutdown.confirmedCapture.Store(owner)
		if owner != nil {
			owner.requestShutdownStop(stop)
		}
	}, wake)
}

type capturePrepareCoordinatorResult struct {
	operationID           uint32
	owner                 *captureOwnerSnapshot
	conflictingOwner      *captureOwnerSnapshot
	orphan                *captureOrphanObligation
	succeeded             bool
	invalidSuccessfulID   bool
	trackerAllowed        bool
	trackerInvoked        bool
	externalInvoked       bool
	ownerPublished        bool
	resultEvidenceAllowed bool
	ownerSuccessorAllowed bool
}

func (r capturePrepareCoordinatorResult) handleInvalidSuccessfulResult(
	shutdown *abruptShutdownCoordinator,
	requiredEvidence func() bool,
	escalate func(),
) requiredFailureContinuationOutcome {
	if !r.invalidSuccessfulID {
		return requiredFailureContinuationOutcome{}
	}
	return runRequiredFailureContinuation(shutdown, r.resultEvidenceAllowed, requiredEvidence, escalate)
}

func (r capturePrepareCoordinatorResult) handleUnexpectedSuccessfulHResult(
	shutdown *abruptShutdownCoordinator,
	requiredEvidence func() bool,
	escalate func(),
) requiredFailureContinuationOutcome {
	return runRequiredFailureContinuation(shutdown, r.resultEvidenceAllowed, requiredEvidence, escalate)
}

func (r capturePrepareCoordinatorResult) dispatchResultEvidence(shutdown *abruptShutdownCoordinator, continuations ...func()) bool {
	return runCaptureContinuation(shutdown, r.resultEvidenceAllowed, continuations...)
}

func (r capturePrepareCoordinatorResult) dispatchOwnerSuccessor(shutdown *abruptShutdownCoordinator, continuations ...func()) bool {
	return runCaptureContinuation(shutdown, r.ownerSuccessorAllowed, continuations...)
}

type capturePrepareCompletionOutcome struct {
	ownerStatePublished bool
	evidenceAttempted   bool
	evidencePassed      bool
}

// dispatchPostHelper preserves the event-safe production order. A successful
// published prepare makes its exact app-side owner state observable before the
// result evidence callback can block, allowing an already-signaled auto-reset
// readiness event to find the operation. Failures skip owner state but retain
// their result row; unpublished successful operations admit neither path.
func (r capturePrepareCoordinatorResult) dispatchPostHelper(
	shutdown *abruptShutdownCoordinator,
	publishOwnerState func(),
	writeResultEvidence func() bool,
	onEvidenceFailure func(),
) capturePrepareCompletionOutcome {
	var outcome capturePrepareCompletionOutcome
	if r.succeeded {
		if !r.dispatchOwnerSuccessor(shutdown, func() {
			if publishOwnerState != nil {
				publishOwnerState()
				outcome.ownerStatePublished = true
			}
		}) {
			return outcome
		}
	}
	if !r.dispatchResultEvidence(shutdown, func() {
		outcome.evidenceAttempted = true
		if writeResultEvidence != nil {
			outcome.evidencePassed = writeResultEvidence()
		}
	}) {
		return outcome
	}
	if !outcome.evidencePassed && onEvidenceFailure != nil {
		shutdown.runOperation(onEvidenceFailure)
	}
	return outcome
}

// handleSuppressedPrepare is the production policy for a prepare refused
// before the helper callback. A queued duplicate can arrive after a lifecycle
// edge has already bound and stopped the exact native incumbent. That refusal
// is diagnostic only: it must not fabricate no-native settlement for the
// surviving owner. A generation with no exact native owner retains the
// existing fail-closed suppressed-settlement path.
func (r capturePrepareCoordinatorResult) handleSuppressedPrepare(
	lifecycle *lifecycleTracker,
	owners *captureOwnershipCoordinator,
	shutdown *abruptShutdownCoordinator,
	generation uint64,
	diagnose func(),
	settle func(),
) bool {
	if lifecycle == nil || owners == nil || shutdown == nil || generation == 0 || (r.trackerInvoked && r.externalInvoked) {
		return false
	}
	if diagnose != nil && !shutdown.runOperation(diagnose) {
		return false
	}
	type ownershipState struct {
		operationID uint32
		phase       captureGenerationPhase
		exists      bool
		incumbent   *captureOwnerSnapshot
	}
	state, admitted := runAbruptOperation(shutdown, func() ownershipState {
		operationID, phase, exists := lifecycle.captureStateForGeneration(generation)
		return ownershipState{operationID: operationID, phase: phase, exists: exists, incumbent: owners.current()}
	})
	if !admitted {
		return false
	}
	if (state.incumbent != nil && state.incumbent.generation == generation) || (state.exists && state.operationID != 0 && state.phase >= captureGenerationNativeOwned) {
		return true
	}
	return settle == nil || shutdown.runOperation(settle)
}

type captureOrphanCompletionResult struct {
	admitted        bool
	cleared         bool
	evidenceStarted bool
	evidencePassed  bool
	settled         bool
}

// completeCaptureOrphanDrain applies each post-Release successor under a fresh
// permit. A Release admitted before confirmation may publish its one-shot
// result, but it cannot use that stale permit to remove the orphan, emit PASS
// evidence, or settle lifecycle state after the latch.
func completeCaptureOrphanDrain(
	shutdown *abruptShutdownCoordinator,
	owners *captureOwnershipCoordinator,
	obligation *captureOrphanObligation,
	drain captureOrphanDrainResult,
	evidence func() bool,
	settle func(),
) captureOrphanCompletionResult {
	var result captureOrphanCompletionResult
	if shutdown == nil || owners == nil || obligation == nil || !drain.Release.released() {
		return result
	}
	result.cleared, result.admitted = runAbruptOperation(shutdown, func() bool {
		return owners.completeOrphan(obligation)
	})
	if !result.admitted || !result.cleared {
		return result
	}
	if evidence != nil {
		result.evidenceStarted = shutdown.runOperation(func() { result.evidencePassed = evidence() })
		if !result.evidenceStarted || !result.evidencePassed {
			return result
		}
	} else {
		result.evidencePassed = true
	}
	result.settled = settle == nil || shutdown.runOperation(settle)
	return result
}

// runCapturePrepareOwned is the production prepare seam. The helper callback
// remains under the accepted lifecycle ownership boundary, while exact native
// ownership is published atomically as soon as the helper returns an operation.
// A real publication loser receives a durable one-shot Stop claim before it is
// made waiter-visible; native Stop is invoked only after publication. It is not
// an ownerless failure and cannot be settled by the UI continuation.
func runCapturePrepareOwned(
	lifecycle *lifecycleTracker,
	owners *captureOwnershipCoordinator,
	shutdown *abruptShutdownCoordinator,
	generation uint64,
	prepare func() (uint32, bool),
	shutdownStop func(uint32) winprobe.HResult,
	unpublishedStop func(uint32) winprobe.HResult,
) capturePrepareCoordinatorResult {
	var result capturePrepareCoordinatorResult
	if lifecycle == nil || owners == nil || shutdown == nil || prepare == nil || shutdownStop == nil || unpublishedStop == nil || shutdown.isClosing() {
		return result
	}
	if !shutdown.runOperation(func() {
		result.operationID, result.succeeded, result.trackerAllowed, result.trackerInvoked = lifecycle.runCapturePrepareCommit(generation, func() (uint32, bool, bool) {
			if shutdown.isClosing() {
				return 0, false, false
			}
			result.externalInvoked = true
			operationID, succeeded := prepare()
			if succeeded && operationID == 0 {
				// A helper that reports success without a registry-backed operation
				// violates the native ABI contract. Treat it as fail-closed before
				// publication so no Stop/query/Release can ever receive ID zero and
				// the lifecycle generation is settled by the ordinary failure path.
				result.invalidSuccessfulID = true
				succeeded = false
			}
			if succeeded {
				owner, incumbent, published := owners.publish(shutdown, generation, operationID, shutdownStop)
				result.owner = owner
				result.conflictingOwner = incumbent
				result.ownerPublished = published
				if !published && owner != nil {
					// Publication and invocation are distinct ordinary operations. A
					// confirmed latch between them leaves the native loser to OS/startup
					// recovery and never inherits this transaction's stale permit.
					shutdown.runOperation(func() {
						result.orphan, _ = owners.publishOrphanStopProducer(owner, unpublishedStop)
					})
					owner.invokeClaimedStop(shutdown)
				}
			}
			return operationID, succeeded, result.ownerPublished
		}, shutdown.runOperation)
	}) {
		return result
	}
	result.resultEvidenceAllowed = result.trackerInvoked && result.externalInvoked && (!result.succeeded || result.ownerPublished) && !shutdown.isClosing()
	result.ownerSuccessorAllowed = result.trackerInvoked && result.externalInvoked && result.succeeded && result.trackerAllowed && result.ownerPublished && !shutdown.isClosing()
	return result
}

type captureActivationCoordinatorResult struct {
	trackerInvoked      bool
	externalInvoked     bool
	continuationAllowed bool
}

func (r captureActivationCoordinatorResult) dispatchContinuation(shutdown *abruptShutdownCoordinator, continuations ...func()) bool {
	return runCaptureContinuation(shutdown, r.trackerInvoked && r.externalInvoked, continuations...)
}

// runCaptureContinuation is the production admission seam for evidence, UI,
// and state work that follows an asynchronous helper callback. A callback that
// returns after confirmed shutdown receives no successor work.
func runCaptureContinuation(shutdown *abruptShutdownCoordinator, allowed bool, continuations ...func()) bool {
	if shutdown == nil || !allowed || len(continuations) == 0 {
		return false
	}
	for _, continuation := range continuations {
		if continuation != nil && !shutdown.runOperation(continuation) {
			return false
		}
	}
	return true
}

// admitCaptureActivationOwned claims the exact owner's queued-readiness slot
// before activation-intent evidence. This is the production boundary that
// keeps an evidence-failure stop from being followed by a stale activation.
func admitCaptureActivationOwned(
	owners *captureOwnershipCoordinator,
	shutdown *abruptShutdownCoordinator,
	generation uint64,
	operationID uint32,
	shutdownStop func(uint32) winprobe.HResult,
) *captureOwnerSnapshot {
	if owners == nil || shutdown == nil {
		return nil
	}
	var owner *captureOwnerSnapshot
	if !shutdown.runOperation(func() {
		owner = owners.matching(generation, operationID)
		if owner == nil || !owner.admitActivationIntent() {
			owner = nil
			return
		}
		owners.activationAttempts.Add(1)
	}) || owner == nil {
		return nil
	}
	if shutdown.isClosing() {
		owner.requestShutdownStop(shutdownStop)
		return nil
	}
	return owner
}

// runCaptureActivationAdmitted performs native admission inside the lifecycle
// callback for the exact owner whose intent was already admitted. A stop that
// won while intent evidence was in flight rejects the native callback. If the
// native admission won first, a later exact-owner stop is still one-shot.
func runCaptureActivationAdmitted(
	lifecycle *lifecycleTracker,
	owners *captureOwnershipCoordinator,
	shutdown *abruptShutdownCoordinator,
	owner *captureOwnerSnapshot,
	afterAdmission func(),
	activate func(),
	shutdownStop func(uint32) winprobe.HResult,
) captureActivationCoordinatorResult {
	var result captureActivationCoordinatorResult
	if lifecycle == nil || owners == nil || shutdown == nil || owner == nil || afterAdmission == nil || activate == nil || shutdown.isClosing() || owners.matching(owner.generation, owner.operationID) != owner {
		return result
	}
	shutdown.runOperation(func() {
		result.trackerInvoked = lifecycle.runCaptureActivation(owner.generation, owner.operationID, func() {
			if shutdown.isClosing() || owners.matching(owner.generation, owner.operationID) != owner || !owner.admitNativeActivation() {
				return
			}
			func() {
				defer owner.completeNativeActivation(shutdown)
				afterAdmission()
				if shutdown.isClosing() {
					owner.requestShutdownStop(shutdownStop)
					return
				}
				if !shutdown.runOperation(func() {
					result.externalInvoked = true
					activate()
				}) {
					owner.requestShutdownStop(shutdownStop)
				}
			}()
		})
	})
	result.continuationAllowed = result.trackerInvoked && result.externalInvoked && !shutdown.isClosing()
	if shutdown.isClosing() {
		if current := owners.matching(owner.generation, owner.operationID); current == owner {
			owner.requestShutdownStop(shutdownStop)
		}
	}
	return result
}

// runCaptureActivationOwned keeps the portable production seam used by
// abrupt-shutdown schedules. The window path admits before evidence and calls
// runCaptureActivationAdmitted directly.
func runCaptureActivationOwned(
	lifecycle *lifecycleTracker,
	owners *captureOwnershipCoordinator,
	shutdown *abruptShutdownCoordinator,
	generation uint64,
	operationID uint32,
	activate func(),
	shutdownStop func(uint32) winprobe.HResult,
) captureActivationCoordinatorResult {
	owner := admitCaptureActivationOwned(owners, shutdown, generation, operationID, shutdownStop)
	return runCaptureActivationAdmitted(lifecycle, owners, shutdown, owner, func() {}, activate, shutdownStop)
}

const (
	confirmedShutdownReadFrames = uint32(4096)
	confirmedShutdownMaxBatches = uint32(8)
)

type confirmedShutdownDrainResult struct {
	readBatches   uint32
	writtenFrames uint64
	lastHR        winprobe.HResult
	writeFailed   bool
}

// drainConfirmedShutdownBuffer is the only data path admitted after confirmed
// shutdown. It is capped, consumes only CaptureRead readiness, and appends to
// an existing writer through the explicitly no-sync callback.
func drainConfirmedShutdownBuffer(channels uint32, read func([]float32, uint32) (uint32, winprobe.HResult), appendFrames func([]float32, uint32) error) confirmedShutdownDrainResult {
	var result confirmedShutdownDrainResult
	if channels == 0 || channels > 8 || read == nil || appendFrames == nil {
		return result
	}
	for result.readBatches < confirmedShutdownMaxBatches {
		buffer := make([]float32, confirmedShutdownReadFrames*channels)
		frames, hr := read(buffer, confirmedShutdownReadFrames)
		result.lastHR = hr
		if hr == 1 || frames == 0 {
			break
		}
		result.readBatches++
		if hr.Failed() || frames > confirmedShutdownReadFrames {
			break
		}
		if err := appendFrames(buffer[:frames*channels], frames); err != nil {
			result.writeFailed = true
			break
		}
		result.writtenFrames += uint64(frames)
	}
	return result
}

type evidenceOperationKind uint8

const (
	evidenceOperationLog evidenceOperationKind = iota + 1
	evidenceOperationSync
)

type evidenceOperation struct {
	kind  evidenceOperationKind
	event winprobe.LogEvent
	reply chan error
}

// evidenceCoordinator serializes the real production logger and File.Sync seam
// behind a bounded acknowledgement. The first error or timeout is sticky: later
// code can still terminate under the hard deadline, but it cannot claim a clean
// evidence-synced exit after an ordered row was lost.
type evidenceCoordinator struct {
	operations  chan evidenceOperation
	timeout     time.Duration
	failed      atomic.Bool
	failureMu   sync.Mutex
	failure     error
	admissionMu sync.RWMutex
	admit       func(func()) bool
}

var (
	errEvidenceUnknownOperation = errors.New("unknown evidence operation")
	errEvidenceQueueSaturated   = errors.New("evidence operation queue saturated")
	errEvidenceEnqueueTimeout   = errors.New("evidence operation enqueue timed out")
	errEvidenceAckTimeout       = errors.New("evidence operation acknowledgement timed out")
	errEvidenceAbruptShutdown   = errors.New("evidence operation suppressed by confirmed shutdown")
)

func newEvidenceCoordinator(logFn func(winprobe.LogEvent) error, syncFn func() error, timeout time.Duration) *evidenceCoordinator {
	if timeout <= 0 {
		timeout = time.Second
	}
	c := &evidenceCoordinator{operations: make(chan evidenceOperation, 128), timeout: timeout}
	go func() {
		for operation := range c.operations {
			err := c.stickyFailure()
			if err == nil {
				known := true
				switch operation.kind {
				case evidenceOperationLog, evidenceOperationSync:
				default:
					known = false
					err = errEvidenceUnknownOperation
				}
				if known && !c.invokeAdmitted(func() {
					if operation.kind == evidenceOperationLog {
						err = logFn(operation.event)
					} else {
						err = syncFn()
					}
				}) {
					err = errEvidenceAbruptShutdown
				}
				if err != nil {
					err = c.markFailed(err)
				} else if sticky := c.stickyFailure(); sticky != nil {
					// Saturation or a caller timeout may have failed the
					// coordinator while this callback was in flight.
					err = sticky
				}
			}
			if operation.reply != nil {
				operation.reply <- err
			}
		}
	}()
	return c
}

func (c *evidenceCoordinator) bindAdmissionGate(admit func(func()) bool) {
	c.admissionMu.Lock()
	c.admit = admit
	c.admissionMu.Unlock()
}

func (c *evidenceCoordinator) invokeAdmitted(callback func()) bool {
	c.admissionMu.RLock()
	admit := c.admit
	c.admissionMu.RUnlock()
	if admit == nil {
		callback()
		return true
	}
	return admit(callback)
}

func (c *evidenceCoordinator) admissionOpen() bool {
	c.admissionMu.RLock()
	admit := c.admit
	c.admissionMu.RUnlock()
	return admit == nil || admit(nil)
}

func (c *evidenceCoordinator) log(event winprobe.LogEvent) bool {
	if !c.admissionOpen() {
		c.markFailed(errEvidenceAbruptShutdown)
		return false
	}
	return c.perform(evidenceOperation{kind: evidenceOperationLog, event: winprobe.SanitizeLogEvent(event), reply: make(chan error, 1)})
}

func (c *evidenceCoordinator) logAsync(event winprobe.LogEvent) bool {
	if !c.admissionOpen() {
		c.markFailed(errEvidenceAbruptShutdown)
		return false
	}
	if c.stickyFailure() != nil {
		return false
	}
	operation := evidenceOperation{kind: evidenceOperationLog, event: winprobe.SanitizeLogEvent(event)}
	select {
	case c.operations <- operation:
		return true
	default:
		c.markFailed(errEvidenceQueueSaturated)
		return false
	}
}

func (c *evidenceCoordinator) sync() bool {
	if !c.admissionOpen() {
		c.markFailed(errEvidenceAbruptShutdown)
		return false
	}
	if !c.healthy() {
		return false
	}
	return c.perform(evidenceOperation{kind: evidenceOperationSync, reply: make(chan error, 1)})
}

func (c *evidenceCoordinator) perform(operation evidenceOperation) bool {
	if !c.admissionOpen() {
		c.markFailed(errEvidenceAbruptShutdown)
		return false
	}
	if c.stickyFailure() != nil {
		return false
	}
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case c.operations <- operation:
	case <-timer.C:
		c.markFailed(errEvidenceEnqueueTimeout)
		return false
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(c.timeout)
	select {
	case err := <-operation.reply:
		if err != nil {
			c.markFailed(err)
			return false
		}
		return c.stickyFailure() == nil
	case <-timer.C:
		c.markFailed(errEvidenceAckTimeout)
		return false
	}
}

func (c *evidenceCoordinator) healthy() bool { return c != nil && !c.failed.Load() }

func (c *evidenceCoordinator) markFailed(err error) error {
	if err == nil {
		err = errors.New("evidence coordinator failed")
	}
	c.failureMu.Lock()
	if c.failure == nil {
		c.failure = err
		c.failed.Store(true)
	}
	sticky := c.failure
	c.failureMu.Unlock()
	return sticky
}

func (c *evidenceCoordinator) stickyFailure() error {
	if c == nil || !c.failed.Load() {
		return nil
	}
	c.failureMu.Lock()
	defer c.failureMu.Unlock()
	return c.failure
}

// beginGracefulWithDeadline commits exit arbitration and arms the hard callback
// before its caller performs any logger or filesystem operation.
func (c *processExitCoordinator) beginGracefulWithDeadline(arm func(func()), hardExit func()) bool {
	if !c.beginGraceful() {
		return false
	}
	arm(func() { c.force(hardExit) })
	return true
}

// runAfterRequiredEvidence is the production continuation gate for permission
// and capture starts. A required row must be acknowledged before the next
// helper call or UI message can be published.
func runAfterRequiredEvidence(record func() bool, continuation func()) bool {
	if record == nil || !record() {
		return false
	}
	if continuation != nil {
		continuation()
	}
	return true
}
