package main

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

// The numeric message values are part of the Win32 ABI. Keeping the mapping in
// a portable file lets the production decision table run in ordinary Go tests.
const (
	lifecycleWMQueryEndSession  uint32 = 0x0011
	lifecycleWMEndSession       uint32 = 0x0016
	lifecycleWMPowerBroadcast   uint32 = 0x0218
	lifecycleWMWTSSessionChange uint32 = 0x02B1

	lifecyclePBTAPMSuspend         uintptr = 0x0004
	lifecyclePBTAPMResumeSuspend   uintptr = 0x0007
	lifecyclePBTAPMResumeAutomatic uintptr = 0x0012
	lifecycleWTSSessionLock        uintptr = 0x0007
	lifecycleWTSSessionUnlock      uintptr = 0x0008
)

type lifecycleEdge string

const (
	lifecycleQuit             lifecycleEdge = "quit"
	lifecycleSystemShutdown   lifecycleEdge = "system_shutdown"
	lifecycleSuspend          lifecycleEdge = "suspend"
	lifecycleSessionLock      lifecycleEdge = "session_lock"
	lifecyclePermissionRevoke lifecycleEdge = "permission_revoke"
)

type lifecycleMode uint8

const (
	lifecycleReturnsIdle lifecycleMode = iota
	lifecycleGracefulExit
	lifecycleAbruptOSExit
)

func (m lifecycleMode) String() string {
	switch m {
	case lifecycleReturnsIdle:
		return "returns_idle"
	case lifecycleGracefulExit:
		return "graceful_exit"
	case lifecycleAbruptOSExit:
		return "abrupt_os_exit"
	default:
		return fmt.Sprintf("lifecycle_mode_%d", m)
	}
}

type lifecycleStage uint8

const (
	lifecycleSignalObserved lifecycleStage = iota + 1
	lifecycleStopRequested
	lifecycleCaptureTerminal
	lifecycleArtifactDisposed
	lifecycleCaptureReleased
	lifecyclePermissionUnsubscribed
	lifecycleHotkeyUnregistered
	lifecycleSessionNotificationUnregistered
	lifecycleHelperDestroyed
	lifecycleTrayIconRemoved
	lifecycleEvidenceSynced
	lifecycleIdle
	lifecycleAbruptHandoff
	lifecycleProcessExit
)

func (s lifecycleStage) String() string {
	switch s {
	case lifecycleSignalObserved:
		return "signal_observed"
	case lifecycleStopRequested:
		return "capture_stop_requested"
	case lifecycleCaptureTerminal:
		return "capture_settled"
	case lifecycleArtifactDisposed:
		return "temporary_artifact_disposed"
	case lifecycleCaptureReleased:
		return "capture_released"
	case lifecyclePermissionUnsubscribed:
		return "permission_subscription_unregistered"
	case lifecycleHotkeyUnregistered:
		return "hotkey_unregistered"
	case lifecycleSessionNotificationUnregistered:
		return "session_notification_unregistered"
	case lifecycleHelperDestroyed:
		return "capture_helper_destroyed"
	case lifecycleTrayIconRemoved:
		return "tray_icon_removed"
	case lifecycleEvidenceSynced:
		return "evidence_log_synced"
	case lifecycleIdle:
		return "idle"
	case lifecycleAbruptHandoff:
		return "os_exit_handoff"
	case lifecycleProcessExit:
		return "process_exit"
	default:
		return fmt.Sprintf("lifecycle_stage_%d", s)
	}
}

type lifecycleMessageAction uint8

const (
	lifecycleMessageNone lifecycleMessageAction = iota
	lifecycleMessageStop
	lifecycleMessageResume
	lifecycleMessageShutdownConfirmed
	lifecycleMessageShutdownCancelled
)

type lifecycleMessagePlan struct {
	Action lifecycleMessageAction
	Edge   lifecycleEdge
	Signal string
	Reason winprobe.CaptureReason
	Mode   lifecycleMode
}

func planLifecycleMessage(message uint32, wParam uintptr) (lifecycleMessagePlan, bool) {
	switch message {
	case lifecycleWMQueryEndSession:
		return lifecycleMessagePlan{Action: lifecycleMessageStop, Edge: lifecycleSystemShutdown, Signal: "WM_QUERYENDSESSION", Reason: winprobe.ReasonShutdown, Mode: lifecycleAbruptOSExit}, true
	case lifecycleWMEndSession:
		if wParam != 0 {
			return lifecycleMessagePlan{Action: lifecycleMessageShutdownConfirmed, Edge: lifecycleSystemShutdown, Signal: "WM_ENDSESSION(confirmed)", Reason: winprobe.ReasonShutdown, Mode: lifecycleAbruptOSExit}, true
		}
		return lifecycleMessagePlan{Action: lifecycleMessageShutdownCancelled, Edge: lifecycleSystemShutdown, Signal: "WM_ENDSESSION(cancelled)", Reason: winprobe.ReasonShutdown, Mode: lifecycleReturnsIdle}, true
	case lifecycleWMPowerBroadcast:
		switch wParam {
		case lifecyclePBTAPMSuspend:
			return lifecycleMessagePlan{Action: lifecycleMessageStop, Edge: lifecycleSuspend, Signal: "WM_POWERBROADCAST/PBT_APMSUSPEND", Reason: winprobe.ReasonSuspend, Mode: lifecycleReturnsIdle}, true
		case lifecyclePBTAPMResumeAutomatic:
			return lifecycleMessagePlan{Action: lifecycleMessageResume, Edge: lifecycleSuspend, Signal: "WM_POWERBROADCAST/PBT_APMRESUMEAUTOMATIC", Reason: winprobe.ReasonSuspend, Mode: lifecycleReturnsIdle}, true
		case lifecyclePBTAPMResumeSuspend:
			return lifecycleMessagePlan{Action: lifecycleMessageResume, Edge: lifecycleSuspend, Signal: "WM_POWERBROADCAST/PBT_APMRESUMESUSPEND", Reason: winprobe.ReasonSuspend, Mode: lifecycleReturnsIdle}, true
		}
	case lifecycleWMWTSSessionChange:
		switch wParam {
		case lifecycleWTSSessionLock:
			return lifecycleMessagePlan{Action: lifecycleMessageStop, Edge: lifecycleSessionLock, Signal: "WM_WTSSESSION_CHANGE/WTS_SESSION_LOCK", Reason: winprobe.ReasonLock, Mode: lifecycleReturnsIdle}, true
		case lifecycleWTSSessionUnlock:
			return lifecycleMessagePlan{Action: lifecycleMessageResume, Edge: lifecycleSessionLock, Signal: "WM_WTSSESSION_CHANGE/WTS_SESSION_UNLOCK", Reason: winprobe.ReasonLock, Mode: lifecycleReturnsIdle}, true
		}
	}
	return lifecycleMessagePlan{}, false
}

type lifecycleProgress struct {
	ID                  uint64
	Edge                lifecycleEdge
	Signal              string
	Signals             []string
	Reason              winprobe.CaptureReason
	Mode                lifecycleMode
	Stage               lifecycleStage
	CaptureExpected     bool
	CaptureGeneration   uint64
	CaptureOperationID  uint32
	RepeatedSignal      bool
	RepeatedSignalCount uint32
}

type lifecycleRun struct {
	lifecycleProgress
	last lifecycleStage
}

type captureGenerationPhase uint8

const (
	captureGenerationRequested captureGenerationPhase = iota + 1
	captureGenerationPermissionPending
	captureGenerationPrepareInFlight
	captureGenerationNativeOwned
	captureGenerationTerminal
	captureGenerationArtifactDisposed
	captureGenerationReleased
)

type captureGeneration struct {
	id          uint64
	operationID uint32
	phase       captureGenerationPhase
	invalidated bool
	settlement  captureSettlementFacts
}

// captureSettlementFacts is an observation ledger, not a lifecycle stage.
// Native settlement callbacks may race lifecycle stop publication, and release
// may be observed while artifact deletion is still pending. Keeping the facts
// independently lets every run bound to the generation replay only the ordered
// stages whose prerequisites have become true.
type captureSettlementFacts struct {
	terminal bool
	artifact bool
	released bool
}

type lifecycleStopPlan struct {
	Progress   lifecycleProgress
	First      bool
	Capture    uint32
	Generation uint64
	Phase      captureGenerationPhase
}

// lifecycleTracker is the production synchronization boundary for start gates,
// capture generations, terminal intent, and ordered evidence. A lifecycle run
// can only bind the exact generation visible while its gate is closed.
type lifecycleTracker struct {
	mu          sync.Mutex
	nextID      uint64
	nextGen     uint64
	nextRearm   uint64
	activeRearm uint64
	active      map[lifecycleEdge]*lifecycleRun
	capture     *captureGeneration
	generations map[uint64]*captureGeneration

	quitting          bool
	suspended         bool
	sessionLocked     bool
	shutdownPending   bool
	permissionBlocked bool
	quitIntentPending bool
	quitIntentStarted bool
}

func newLifecycleTracker() *lifecycleTracker {
	return &lifecycleTracker{
		active:      make(map[lifecycleEdge]*lifecycleRun),
		generations: make(map[uint64]*captureGeneration),
	}
}

func (t *lifecycleTracker) beginCaptureGeneration() (uint64, bool, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if reason := t.startBlockedReasonLocked(); reason != "" {
		return 0, false, reason
	}
	if t.capture != nil {
		return 0, false, "another capture generation is already active"
	}
	t.nextGen++
	t.capture = &captureGeneration{id: t.nextGen, phase: captureGenerationRequested}
	t.generations[t.nextGen] = t.capture
	return t.nextGen, true, ""
}

func (t *lifecycleTracker) startBlockedReasonLocked() string {
	switch {
	case t.quitting:
		return "graceful quit is pending"
	case t.shutdownPending:
		return "system shutdown is pending"
	case t.suspended:
		return "the app is suspended"
	case t.sessionLocked:
		return "the session is locked"
	case t.permissionBlocked:
		return "microphone permission is revoked"
	case t.activeRearm != 0:
		return "lifecycle rearm is pending"
	default:
		for _, run := range t.active {
			if run.Mode == lifecycleReturnsIdle && run.last < lifecycleIdle {
				return "lifecycle cleanup is pending"
			}
		}
		return ""
	}
}

func (t *lifecycleTracker) workAllowed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startBlockedReasonLocked() == ""
}

func (t *lifecycleTracker) runGatedWork(work func()) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.startBlockedReasonLocked() != "" {
		return false
	}
	work()
	return true
}

func (t *lifecycleTracker) beginRearm() (uint64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.activeRearm != 0 || t.capture != nil || t.rearmBlockedReasonLocked() != "" {
		return 0, false
	}
	t.nextRearm++
	t.activeRearm = t.nextRearm
	return t.activeRearm, true
}

func (t *lifecycleTracker) rearmBlockedReasonLocked() string {
	savedPermission := t.permissionBlocked
	savedRearm := t.activeRearm
	t.permissionBlocked = false
	t.activeRearm = 0
	reason := t.startBlockedReasonLocked()
	t.permissionBlocked = savedPermission
	t.activeRearm = savedRearm
	return reason
}

// runRearm atomically validates the waiter result, updates the permission gate,
// and keeps the rearm token as a start gate while UI-owned discovery/hotkey work
// runs. A failed work callback retains the token so the durable UI transition
// can retry; a stale callback cannot mutate permission state.
func (t *lifecycleTracker) runRearm(generation uint64, allowed bool, work func() bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if generation == 0 || t.activeRearm != generation || t.capture != nil || t.rearmBlockedReasonLocked() != "" {
		return false
	}
	t.permissionBlocked = !allowed
	if !allowed {
		t.activeRearm = 0
		return false
	}
	if work == nil || !work() {
		return false
	}
	t.activeRearm = 0
	return true
}

func (t *lifecycleTracker) rearmPending(generation uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return generation != 0 && t.activeRearm == generation
}

func (t *lifecycleTracker) permissionContinuationAllowed(generation uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.capture != nil && t.capture.id == generation && !t.capture.invalidated && t.startBlockedReasonLocked() == ""
}

func (t *lifecycleTracker) runPermissionRequest(generation uint64, start func() (uint32, bool)) (operationID uint32, succeeded, invoked bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.capture == nil || t.capture.id != generation || t.capture.invalidated || t.startBlockedReasonLocked() != "" {
		return 0, false, false
	}
	t.capture.phase = captureGenerationPermissionPending
	operationID, succeeded = start()
	if !succeeded {
		generation := t.capture.id
		t.capture.phase = captureGenerationReleased
		t.capture.settlement = captureSettlementFacts{terminal: true, artifact: true, released: true}
		t.capture = nil
		t.pruneGenerationLocked(generation)
	}
	return operationID, succeeded, true
}

func (t *lifecycleTracker) runCapturePrepare(generation uint64, prepare func() (uint32, bool)) (operationID uint32, succeeded, allowed, invoked bool) {
	return t.runCapturePrepareCommit(generation, func() (uint32, bool, bool) {
		operationID, succeeded := prepare()
		return operationID, succeeded, succeeded
	})
}

// runCapturePrepareCommit keeps native publication acceptance and the capture
// generation authoritative state in one lifecycle mutex transaction. A
// same-generation duplicate is rejected before the helper. A real successful
// loser behind a distinct/stale atomic incumbent remains native-owned in its
// generation until the waiter orphan obligation proves terminal and Release.
func (t *lifecycleTracker) runCapturePrepareCommit(generation uint64, prepare func() (operationID uint32, succeeded, commit bool)) (operationID uint32, succeeded, allowed, invoked bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.capture == nil || t.capture.id != generation || t.capture.invalidated || t.startBlockedReasonLocked() != "" {
		return 0, false, false, false
	}
	state := t.capture
	// Permission-ready is coalescible, and stale/duplicate window messages can
	// be delivered after the first helper result. A generation gets at most one
	// CapturePrepare invocation; a registry-backed native operation is never
	// created merely to lose publication behind its own incumbent.
	if state.phase >= captureGenerationPrepareInFlight {
		return 0, false, false, false
	}
	previousOperationID := state.operationID
	previousPhase := state.phase
	state.phase = captureGenerationPrepareInFlight
	operationID, succeeded, commit := prepare()
	if succeeded && !commit {
		if previousOperationID == 0 {
			// Publication can lose to a stale/distinct atomic incumbent after the
			// helper has already registered this operation. Keep the rejected
			// generation native-owned until its waiter orphan obligation reaches
			// terminal and Release; no-native settlement would be false.
			state.operationID = operationID
			state.phase = captureGenerationNativeOwned
			for _, run := range t.active {
				if run.CaptureGeneration == generation {
					run.CaptureOperationID = operationID
				}
			}
		} else {
			state.operationID = previousOperationID
			state.phase = previousPhase
		}
		return operationID, true, true, true
	}
	if !succeeded {
		if previousOperationID != 0 {
			state.operationID = previousOperationID
			state.phase = previousPhase
			return operationID, false, false, true
		}
		generation := state.id
		state.phase = captureGenerationReleased
		state.settlement = captureSettlementFacts{terminal: true, artifact: true, released: true}
		if t.capture == state {
			t.capture = nil
		}
		t.pruneGenerationLocked(generation)
		return operationID, false, false, true
	}
	state.operationID = operationID
	state.phase = captureGenerationNativeOwned
	for _, run := range t.active {
		if run.CaptureGeneration == generation {
			run.CaptureOperationID = operationID
		}
	}
	return operationID, true, true, true
}

func (t *lifecycleTracker) runCaptureActivation(generation uint64, operationID uint32, activate func()) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.capture == nil || t.capture.id != generation || t.capture.operationID != operationID || t.capture.phase != captureGenerationNativeOwned || t.capture.invalidated || t.startBlockedReasonLocked() != "" {
		return false
	}
	activate()
	return true
}

func (t *lifecycleTracker) generationInvalidated(generation uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.capture == nil || t.capture.id != generation || t.capture.invalidated || t.startBlockedReasonLocked() != ""
}

func (t *lifecycleTracker) hasCaptureGeneration() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.capture != nil
}

func (t *lifecycleTracker) permissionQueryFailureRequiresStop(accessChangedSignal, operationOwned bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return accessChangedSignal || operationOwned || t.capture != nil
}

func (t *lifecycleTracker) permissionIsBlocked() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.permissionBlocked
}

func (t *lifecycleTracker) phaseForGeneration(generation uint64) captureGenerationPhase {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.phaseForGenerationLocked(generation)
}

func (t *lifecycleTracker) captureStateForGeneration(generation uint64) (uint32, captureGenerationPhase, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.generations[generation]
	if state == nil {
		return 0, 0, false
	}
	return state.operationID, state.phase, true
}

func (t *lifecycleTracker) cancelCaptureGeneration(generation uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.generations[generation]
	if state == nil || state.operationID != 0 {
		return
	}
	state.phase = captureGenerationReleased
	state.settlement = captureSettlementFacts{terminal: true, artifact: true, released: true}
	t.replayCaptureFactsLocked(generation)
	if t.capture == state {
		t.capture = nil
	}
	t.pruneGenerationLocked(generation)
}

func (t *lifecycleTracker) beginLifecycle(edge lifecycleEdge, signal string, reason winprobe.CaptureReason, mode lifecycleMode) lifecycleStopPlan {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeGateLocked(edge)
	if run := t.active[edge]; run != nil {
		t.recordSignalLocked(run, signal)
		return lifecycleStopPlan{Progress: cloneProgress(run.lifecycleProgress), Capture: run.CaptureOperationID, Generation: run.CaptureGeneration, Phase: t.phaseForGenerationLocked(run.CaptureGeneration)}
	}
	t.nextID++
	progress := lifecycleProgress{ID: t.nextID, Edge: edge, Signal: signal, Signals: []string{signal}, Reason: reason, Mode: mode, Stage: lifecycleSignalObserved}
	phase := captureGenerationPhase(0)
	if t.capture != nil && t.capture.phase < captureGenerationReleased {
		progress.CaptureExpected = true
		progress.CaptureGeneration = t.capture.id
		progress.CaptureOperationID = t.capture.operationID
		phase = t.capture.phase
		t.capture.invalidated = true
	}
	run := &lifecycleRun{lifecycleProgress: progress, last: lifecycleSignalObserved}
	t.active[edge] = run
	return lifecycleStopPlan{Progress: cloneProgress(progress), First: true, Capture: progress.CaptureOperationID, Generation: progress.CaptureGeneration, Phase: phase}
}

// beginLifecycleStop closes the production start gate and binds the exact
// generation before invoking the nonblocking native stop seam. Evidence I/O is
// deliberately outside this method and therefore cannot delay gate invalidation
// or CaptureStop after an OS/permission signal.
func (t *lifecycleTracker) beginLifecycleStop(edge lifecycleEdge, signal string, reason winprobe.CaptureReason, mode lifecycleMode, stop func(uint64, uint32) winprobe.HResult) (lifecycleStopPlan, winprobe.HResult) {
	plan := t.beginLifecycle(edge, signal, reason, mode)
	// A requested generation has no native or temporary-artifact owner. Settle
	// it in the in-memory ledger immediately after closing the gate so a blocked
	// evidence writer cannot leave the generation alive. The bound run replays
	// these facts after stop_requested is published.
	if plan.Generation != 0 && plan.Capture == 0 && plan.Phase == captureGenerationRequested {
		t.cancelCaptureGeneration(plan.Generation)
	}
	if plan.Capture == 0 || stop == nil {
		return plan, 0
	}
	return plan, stop(plan.Generation, plan.Capture)
}

func (t *lifecycleTracker) beginGracefulQuit(signal string) lifecycleStopPlan {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.quitting = true
	if !t.quitIntentStarted {
		t.quitIntentPending = true
	}
	return t.beginLifecycleLocked(lifecycleQuit, signal, winprobe.ReasonCancel, lifecycleGracefulExit)
}

func (t *lifecycleTracker) beginLifecycleLocked(edge lifecycleEdge, signal string, reason winprobe.CaptureReason, mode lifecycleMode) lifecycleStopPlan {
	t.closeGateLocked(edge)
	if run := t.active[edge]; run != nil {
		t.recordSignalLocked(run, signal)
		return lifecycleStopPlan{Progress: cloneProgress(run.lifecycleProgress), Capture: run.CaptureOperationID, Generation: run.CaptureGeneration, Phase: t.phaseForGenerationLocked(run.CaptureGeneration)}
	}
	t.nextID++
	progress := lifecycleProgress{ID: t.nextID, Edge: edge, Signal: signal, Signals: []string{signal}, Reason: reason, Mode: mode, Stage: lifecycleSignalObserved}
	phase := captureGenerationPhase(0)
	if t.capture != nil && t.capture.phase < captureGenerationReleased {
		progress.CaptureExpected = true
		progress.CaptureGeneration = t.capture.id
		progress.CaptureOperationID = t.capture.operationID
		phase = t.capture.phase
		t.capture.invalidated = true
	}
	t.active[edge] = &lifecycleRun{lifecycleProgress: progress, last: lifecycleSignalObserved}
	return lifecycleStopPlan{Progress: cloneProgress(progress), First: true, Capture: progress.CaptureOperationID, Generation: progress.CaptureGeneration, Phase: phase}
}

func (t *lifecycleTracker) closeGateLocked(edge lifecycleEdge) {
	t.activeRearm = 0
	switch edge {
	case lifecycleQuit:
		t.quitting = true
	case lifecycleSystemShutdown:
		t.shutdownPending = true
	case lifecycleSuspend:
		t.suspended = true
	case lifecycleSessionLock:
		t.sessionLocked = true
	case lifecyclePermissionRevoke:
		t.permissionBlocked = true
	}
}

func (t *lifecycleTracker) consumeQuitIntent() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.quitIntentPending || t.quitIntentStarted {
		return false
	}
	t.quitIntentPending = false
	t.quitIntentStarted = true
	return true
}

func (t *lifecycleTracker) markShutdownConfirmed() {
	t.mu.Lock()
	t.shutdownPending = true
	t.activeRearm = 0
	t.mu.Unlock()
}

func (t *lifecycleTracker) cancelShutdown(signal string) (lifecycleProgress, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	run := t.active[lifecycleSystemShutdown]
	if run == nil {
		return lifecycleProgress{}, fmt.Errorf("no active lifecycle run for %s", lifecycleSystemShutdown)
	}
	if run.last >= lifecycleHotkeyUnregistered {
		return lifecycleProgress{}, fmt.Errorf("lifecycle %s has already committed its exit/idle disposition", lifecycleSystemShutdown)
	}
	t.recordSignalLocked(run, signal)
	run.Mode = lifecycleReturnsIdle
	t.shutdownPending = false
	return cloneProgress(run.lifecycleProgress), nil
}

func (t *lifecycleTracker) resume(edge lifecycleEdge) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch edge {
	case lifecycleSuspend:
		t.suspended = false
	case lifecycleSessionLock:
		t.sessionLocked = false
	}
}

func (t *lifecycleTracker) setPermissionAllowed(allowed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.permissionBlocked = !allowed
}

func (t *lifecycleTracker) recordSignalLocked(run *lifecycleRun, signal string) {
	run.Signal = signal
	run.Signals = append(run.Signals, signal)
	run.RepeatedSignal = true
	run.RepeatedSignalCount++
}

func (t *lifecycleTracker) phaseForGenerationLocked(generation uint64) captureGenerationPhase {
	if state := t.generations[generation]; state != nil {
		return state.phase
	}
	return 0
}

// observe is retained for focused ordering tests and native-terminal recovery.
// Production lifecycle entry points use beginLifecycle/beginGracefulQuit.
func (t *lifecycleTracker) observe(edge lifecycleEdge, signal string, reason winprobe.CaptureReason, mode lifecycleMode, captureExpected bool) lifecycleProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	if run := t.active[edge]; run != nil {
		t.recordSignalLocked(run, signal)
		return cloneProgress(run.lifecycleProgress)
	}
	t.nextID++
	progress := lifecycleProgress{ID: t.nextID, Edge: edge, Signal: signal, Signals: []string{signal}, Reason: reason, Mode: mode, Stage: lifecycleSignalObserved, CaptureExpected: captureExpected}
	t.active[edge] = &lifecycleRun{lifecycleProgress: progress, last: lifecycleSignalObserved}
	return cloneProgress(progress)
}

func (t *lifecycleTracker) advance(edge lifecycleEdge, stage lifecycleStage) (lifecycleProgress, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.advanceLocked(edge, stage)
}

func (t *lifecycleTracker) advanceLocked(edge lifecycleEdge, stage lifecycleStage) (lifecycleProgress, bool, error) {
	run := t.active[edge]
	if run == nil {
		return lifecycleProgress{}, false, fmt.Errorf("no active lifecycle run for %s", edge)
	}
	if stage <= run.last {
		progress := cloneProgress(run.lifecycleProgress)
		progress.Stage = stage
		return progress, false, nil
	}
	if err := validateLifecycleAdvance(run, stage); err != nil {
		return lifecycleProgress{}, false, err
	}
	run.last = stage
	run.Stage = stage
	progress := cloneProgress(run.lifecycleProgress)
	if stage == lifecycleIdle || stage == lifecycleAbruptHandoff || stage == lifecycleProcessExit {
		generation := run.CaptureGeneration
		delete(t.active, edge)
		t.pruneGenerationLocked(generation)
	}
	return progress, true, nil
}

type lifecycleAdvance struct {
	Progress lifecycleProgress
	Stage    lifecycleStage
	Changed  bool
	Err      error
}

func (t *lifecycleTracker) advanceCaptureGeneration(generation uint64, stage lifecycleStage) []lifecycleAdvance {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.generations[generation]
	if state == nil {
		return nil
	}
	switch stage {
	case lifecycleCaptureTerminal:
		state.settlement.terminal = true
	case lifecycleArtifactDisposed:
		state.settlement.artifact = true
	case lifecycleCaptureReleased:
		state.settlement.released = true
		state.operationID = 0
	default:
		return []lifecycleAdvance{{Stage: stage, Err: fmt.Errorf("stage %s is not a capture-settlement observation", stage)}}
	}
	state.recomputeSettlementPhase()
	if state.phase == captureGenerationReleased && t.capture == state {
		t.capture = nil
	}
	results := t.replayCaptureFactsLocked(generation)
	t.pruneGenerationLocked(generation)
	return results
}

func (g *captureGeneration) recomputeSettlementPhase() {
	if !g.settlement.terminal {
		return
	}
	g.phase = captureGenerationTerminal
	if !g.settlement.artifact {
		return
	}
	g.phase = captureGenerationArtifactDisposed
	if g.settlement.released {
		g.phase = captureGenerationReleased
	}
}

func (t *lifecycleTracker) replayCaptureGeneration(generation uint64) []lifecycleAdvance {
	t.mu.Lock()
	defer t.mu.Unlock()
	results := t.replayCaptureFactsLocked(generation)
	t.pruneGenerationLocked(generation)
	return results
}

func (t *lifecycleTracker) replayCaptureFactsLocked(generation uint64) []lifecycleAdvance {
	state := t.generations[generation]
	if state == nil {
		return nil
	}
	runs := t.matchingRunsLocked(func(run *lifecycleRun) bool { return run.CaptureGeneration == generation })
	results := make([]lifecycleAdvance, 0, len(runs)*3)
	for _, snapshot := range runs {
		run := t.active[snapshot.Edge]
		if run == nil || run.last < lifecycleStopRequested {
			continue
		}
		stages := []struct {
			observed bool
			stage    lifecycleStage
		}{
			{state.settlement.terminal, lifecycleCaptureTerminal},
			{state.settlement.artifact, lifecycleArtifactDisposed},
			{state.settlement.released, lifecycleCaptureReleased},
		}
		for _, fact := range stages {
			if !fact.observed {
				break
			}
			if run.last >= fact.stage {
				continue
			}
			progress, changed, err := t.advanceLocked(snapshot.Edge, fact.stage)
			results = append(results, lifecycleAdvance{Progress: progress, Stage: fact.stage, Changed: changed, Err: err})
			if err != nil {
				break
			}
		}
	}
	return results
}

func (t *lifecycleTracker) pruneGenerationLocked(generation uint64) {
	if generation == 0 {
		return
	}
	state := t.generations[generation]
	if state == nil || !state.settlement.released || t.capture == state {
		return
	}
	for _, run := range t.active {
		if run.CaptureGeneration == generation && run.last < lifecycleCaptureReleased {
			return
		}
	}
	delete(t.generations, generation)
}

func validateLifecycleAdvance(run *lifecycleRun, stage lifecycleStage) error {
	require := func(prerequisite lifecycleStage) error {
		if run.last < prerequisite {
			return fmt.Errorf("lifecycle %s cannot advance from %s to %s before %s", run.Edge, run.last, stage, prerequisite)
		}
		return nil
	}
	switch stage {
	case lifecycleStopRequested:
		return require(lifecycleSignalObserved)
	case lifecycleCaptureTerminal:
		if !run.CaptureExpected {
			return fmt.Errorf("lifecycle %s did not own an active capture generation", run.Edge)
		}
		return require(lifecycleStopRequested)
	case lifecycleArtifactDisposed:
		return require(lifecycleCaptureTerminal)
	case lifecycleCaptureReleased:
		return require(lifecycleArtifactDisposed)
	case lifecyclePermissionUnsubscribed:
		if run.Mode != lifecycleGracefulExit {
			return fmt.Errorf("lifecycle %s does not tear down the permission subscription while returning idle", run.Edge)
		}
		if run.CaptureExpected {
			return require(lifecycleCaptureReleased)
		}
		return require(lifecycleStopRequested)
	case lifecycleHotkeyUnregistered:
		if run.Mode == lifecycleGracefulExit {
			return require(lifecyclePermissionUnsubscribed)
		}
		if run.Mode == lifecycleAbruptOSExit {
			return require(lifecycleStopRequested)
		}
		if run.CaptureExpected {
			return require(lifecycleCaptureReleased)
		}
		return require(lifecycleStopRequested)
	case lifecycleSessionNotificationUnregistered:
		if run.Mode != lifecycleGracefulExit {
			return fmt.Errorf("lifecycle %s cannot unregister session notifications before returning idle", run.Edge)
		}
		return require(lifecycleHotkeyUnregistered)
	case lifecycleHelperDestroyed:
		if run.Mode != lifecycleGracefulExit {
			return fmt.Errorf("lifecycle %s cannot destroy the helper on an idle/abrupt path", run.Edge)
		}
		return require(lifecycleSessionNotificationUnregistered)
	case lifecycleTrayIconRemoved:
		if run.Mode != lifecycleGracefulExit {
			return fmt.Errorf("lifecycle %s cannot remove the tray icon on an idle/abrupt path", run.Edge)
		}
		return require(lifecycleHelperDestroyed)
	case lifecycleEvidenceSynced:
		return require(lifecycleTrayIconRemoved)
	case lifecycleIdle:
		if run.Mode != lifecycleReturnsIdle {
			return fmt.Errorf("lifecycle %s is not an idle-return path", run.Edge)
		}
		return require(lifecycleHotkeyUnregistered)
	case lifecycleAbruptHandoff:
		if run.Mode != lifecycleAbruptOSExit {
			return fmt.Errorf("lifecycle %s is not an abrupt OS-exit path", run.Edge)
		}
		return require(lifecycleHotkeyUnregistered)
	case lifecycleProcessExit:
		if run.Mode != lifecycleGracefulExit {
			return fmt.Errorf("lifecycle %s is not a graceful-exit path", run.Edge)
		}
		return require(lifecycleEvidenceSynced)
	default:
		return fmt.Errorf("unsupported lifecycle stage %d", stage)
	}
}

func (t *lifecycleTracker) captureRuns() []lifecycleProgress {
	return t.matchingRuns(func(run *lifecycleRun) bool {
		return run.CaptureExpected && run.last >= lifecycleStopRequested && run.last < lifecycleCaptureReleased
	})
}

func (t *lifecycleTracker) idleCleanupRuns() []lifecycleProgress {
	return t.matchingRuns(func(run *lifecycleRun) bool {
		if run.Mode != lifecycleReturnsIdle || run.last >= lifecycleHotkeyUnregistered {
			return false
		}
		if run.CaptureExpected {
			return run.last >= lifecycleCaptureReleased
		}
		return run.last >= lifecycleStopRequested
	})
}

func (t *lifecycleTracker) activeRun(edge lifecycleEdge) (lifecycleProgress, bool) {
	runs := t.matchingRuns(func(run *lifecycleRun) bool { return run.Edge == edge })
	if len(runs) == 0 {
		return lifecycleProgress{}, false
	}
	return runs[0], true
}

func (t *lifecycleTracker) matchingRuns(match func(*lifecycleRun) bool) []lifecycleProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.matchingRunsLocked(match)
}

func (t *lifecycleTracker) matchingRunsLocked(match func(*lifecycleRun) bool) []lifecycleProgress {
	runs := make([]lifecycleProgress, 0, len(t.active))
	for _, run := range t.active {
		if match(run) {
			runs = append(runs, cloneProgress(run.lifecycleProgress))
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs
}

func cloneProgress(progress lifecycleProgress) lifecycleProgress {
	progress.Signals = append([]string(nil), progress.Signals...)
	return progress
}

// processExitCoordinator keeps the watchdog armed until posting WM_QUIT is the
// only remaining action. No log or filesystem call is allowed in force().
type processExitCoordinator struct{ state atomic.Uint32 }

const (
	processExitRunning uint32 = iota
	processExitGracefulPending
	processExitForceCommitted
	processExitQuitCommitted
)

func (c *processExitCoordinator) beginGraceful() bool {
	return c.state.CompareAndSwap(processExitRunning, processExitGracefulPending)
}

func (c *processExitCoordinator) force(exit func()) bool {
	if !c.state.CompareAndSwap(processExitGracefulPending, processExitForceCommitted) {
		return false
	}
	exit()
	return true
}

func (c *processExitCoordinator) commitQuit(postQuit func()) bool {
	if !c.state.CompareAndSwap(processExitGracefulPending, processExitQuitCommitted) {
		return false
	}
	postQuit()
	return true
}

func (c *processExitCoordinator) load() uint32 { return c.state.Load() }

type helperLifetime struct{ initialized atomic.Bool }

func (h *helperLifetime) markInitialized() { h.initialized.Store(true) }
func (h *helperLifetime) clear()           { h.initialized.Store(false) }
func (h *helperLifetime) isInitialized() bool {
	return h.initialized.Load()
}

type permissionQueryRole uint8

const (
	permissionQueryUI permissionQueryRole = iota
	permissionQueryWaiter
)

type permissionQueryCoordinator struct{ mu sync.Mutex }

func (q *permissionQueryCoordinator) run(role permissionQueryRole, query func() (winprobe.PermissionStatus, winprobe.HResult)) (winprobe.PermissionStatus, winprobe.HResult, bool) {
	if role != permissionQueryWaiter {
		return winprobe.PermissionUnknown, 0, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	status, hr := query()
	return status, hr, true
}

type retryTimerDecision uint8

const (
	retryTimerScheduled retryTimerDecision = iota
	retryTimerForceExit
	retryTimerGracefulExit
)

func decideRetryTimer(timerID uintptr, terminal bool) retryTimerDecision {
	if timerID != 0 {
		return retryTimerScheduled
	}
	if terminal {
		return retryTimerForceExit
	}
	return retryTimerGracefulExit
}

type evidenceRetryBudget struct {
	attempts uint32
	limit    uint32
}

func newEvidenceRetryBudget(limit uint32) evidenceRetryBudget {
	return evidenceRetryBudget{limit: limit}
}

func (b *evidenceRetryBudget) recordFailure() (retry bool, attempts uint32) {
	b.attempts++
	return b.attempts < b.limit, b.attempts
}

func (b *evidenceRetryBudget) reset() { b.attempts = 0 }

// ownedResource clears ownership only after the production delete callback
// succeeds, making one-shot Win32 cleanup failures retryable.
type ownedResource struct {
	mu    sync.Mutex
	owned bool
}

func (r *ownedResource) setOwned(owned bool) {
	r.mu.Lock()
	r.owned = owned
	r.mu.Unlock()
}

func (r *ownedResource) release(remove func() bool) (released, called bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.owned {
		return true, false
	}
	if !remove() {
		return false, true
	}
	r.owned = false
	return true, true
}

func (r *ownedResource) isOwned() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.owned
}
