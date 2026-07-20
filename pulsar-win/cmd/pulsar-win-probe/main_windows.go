//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"relux.works/duet/pulsar-win/internal/winprobe"
)

const (
	probeSeconds             = 10
	minimumProbeMillis       = 250
	maxPickedBytes           = 50 << 20
	evidenceSyncRetries      = 50
	evidenceOperationTimeout = 2 * time.Second
	uiTransitionPostRetries  = 5
)

type recordMode int

const (
	recordDefault recordMode = iota
	recordSelected
)

type waiterCommand struct {
	kind       string
	reason     winprobe.CaptureReason
	source     winprobe.ProbeScenario
	generation uint64
}

type artifactCleanupRetry struct {
	writer             *winprobe.ArtifactWriter
	captureOperationID uint32
	captureGeneration  uint64
	captureReleased    bool
	fields             map[string]any
}

type probeApp struct {
	helper   *winprobe.Helper
	logFile  *os.File
	evidence *evidenceCoordinator

	hidden                windows.Handle
	main                  windows.Handle
	intro                 windows.Handle
	list                  windows.Handle
	status                windows.Handle
	recordDefaultControl  windows.Handle
	recordSelectedControl windows.Handle
	stopControl           windows.Handle
	pickerControl         windows.Handle
	hideControl           windows.Handle

	events                []windows.Handle
	permissionEvent       windows.Handle
	permissionChangeEvent windows.Handle
	enumerationEvent      windows.Handle
	defaultEvent          windows.Handle
	captureEvent          windows.Handle
	pickerEvent           windows.Handle
	commandEvent          windows.Handle
	shutdownEvent         windows.Handle

	commands               chan waiterCommand
	mu                     sync.Mutex
	permissionOp           uint32
	permissionGeneration   uint64
	enumerationOp          uint32
	defaultOp              uint32
	captureOp              uint32
	captureGeneration      uint64
	pickerOp               uint32
	devices                []winprobe.Device
	defaultDevice          string
	selected               int
	pendingMode            recordMode
	pendingDevice          string
	activatePosted         bool
	writer                 *winprobe.ArtifactWriter
	captureFormat          winprobe.CaptureFormat
	pickerWindow           pickerWindowState
	capturePrepareAt       time.Time
	captureTimeoutSent     bool
	permissionStopSent     bool
	permissionCancelSent   bool
	permissionMonitorReady bool
	captureFinalized       bool
	pickerProcessed        bool
	quitting               bool
	unsubscribed           bool
	waiterDone             chan struct{}
	helperLifetime         helperLifetime
	captureObservation     winprobe.CaptureObservation
	captureVisibility      winprobe.CaptureVisibilityEvidence
	captureDiagnostics     winprobe.CaptureDiagnostics
	frameLimiter           winprobe.FrameLimiter
	discardedCaptureFrames uint64
	lifecycle              *lifecycleTracker
	uiTransitions          *uiTransitionCoordinator
	shutdown               abruptShutdownCoordinator
	captureOwners          captureOwnershipCoordinator
	permissionQueries      permissionQueryCoordinator
	hotkeyRegistered       bool
	trayIcon               ownedResource
	wtsRegistered          bool
	permissionKnown        bool
	permissionStatus       winprobe.PermissionStatus
	exitEvidenceRecorded   bool
	evidenceRetries        evidenceRetryBudget
	artifactCleanup        *artifactCleanupRetry

	exit                   processExitCoordinator
	quittingAt             atomic.Int64
	evidenceFailurePending atomic.Bool
	evidenceFailureHandled atomic.Bool
}

func main() {
	runtime.LockOSThread()
	app, err := newProbeApp()
	if err != nil {
		messageBoxError("Pulsar Probe", err.Error())
		return
	}
	defer app.closeLocalResources()
	currentApp = app
	defer func() { currentApp = nil }()
	if err := app.run(); err != nil {
		if app.shutdown.runOperation(func() {
			app.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "probe_run", FailureCause: err.Error()})
		}) {
			app.shutdown.runOperation(func() { messageBoxError("Pulsar Probe", err.Error()) })
		}
	}
}

func newProbeApp() (*probeApp, error) {
	logDir, err := probeDataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "scenarios.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	// The packaged GUI process has no reliable stderr handle. The evidence file
	// is authoritative; an optional console mirror must never turn a durable
	// primary write into a startup failure.
	logger := winprobe.NewJSONLogger(logFile)
	app := &probeApp{
		logFile:         logFile,
		commands:        make(chan waiterCommand, 32),
		selected:        -1,
		waiterDone:      make(chan struct{}),
		lifecycle:       newLifecycleTracker(),
		uiTransitions:   newUITransitionCoordinator(uiTransitionPostRetries),
		evidenceRetries: newEvidenceRetryBudget(evidenceSyncRetries),
	}
	app.evidence = newEvidenceCoordinator(logger.Log, logFile.Sync, evidenceOperationTimeout)
	app.evidence.bindAdmissionGate(app.shutdown.runOperation)
	if !app.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultAttempt, Action: "helper_load", SelectedAPIPath: string(winprobe.LoaderPackaged)}) {
		app.closeLocalResources()
		return nil, fmt.Errorf("required startup evidence is unavailable")
	}
	helper, err := winprobe.LoadHelper()
	if err != nil {
		app.closeLocalResources()
		return nil, fmt.Errorf("load actual native helper: %w", err)
	}
	app.helper = helper
	version, size, hr := helper.Version()
	if hr.Failed() || version != winprobe.HelperABIVersion || size != winprobe.CaptureFormatStructSize {
		app.closeLocalResources()
		return nil, fmt.Errorf("helper ABI mismatch: call=%s version=%d struct=%d", hr.Hex(), version, size)
	}
	if !app.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultPass, Action: "helper_load", SelectedAPIPath: string(helper.LoadedVia), Fields: map[string]any{"coreABIVersion": version, "coreCaptureFormatSize": size, "probeDiagnosticsExtensionVersion": helper.DiagnosticsExtensionVersion, "probeDiagnosticsNegotiatedSeparately": true}}) {
		app.closeLocalResources()
		return nil, fmt.Errorf("required helper evidence is unavailable")
	}
	if hr = helper.Init(); hr.Failed() {
		app.closeLocalResources()
		return nil, fmt.Errorf("CapInit: %s", hr.Hex())
	}
	app.helperLifetime.markInitialized()
	permission, permissionHR := helper.PermissionCheck()
	if permissionHR.Failed() {
		permission = winprobe.PermissionUnknown
	} else {
		app.permissionKnown = true
		app.permissionStatus = permission
	}
	recoveryDir := filepath.Join(logDir, "evidence")
	recovery, recoveryErr := winprobe.RecoverArtifacts(recoveryDir, permission, minimumProbeMillis)
	if recoveryErr != nil {
		if !app.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "startup_recovery", PermissionStatus: permission.String(), HResult: permissionHR.Hex(), FailureCause: recoveryErr.Error()}) {
			app.closeLocalResources()
			return nil, fmt.Errorf("required recovery evidence is unavailable")
		}
	}
	for _, outcome := range recovery {
		if !app.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: outcome.Result, Action: "startup_recovery", PermissionStatus: permission.String(), HResult: permissionHR.Hex(), FailureCause: outcome.Cause, Fields: map[string]any{"sessionId": outcome.SessionID, "reason": outcome.Reason.String(), "frames": outcome.Frames, "artifactRetained": outcome.Path != ""}}) {
			app.closeLocalResources()
			return nil, fmt.Errorf("required recovery outcome evidence is unavailable")
		}
	}
	for i := 0; i < 8; i++ {
		manual := uint32(0)
		if i == 7 {
			manual = 1
		}
		event, eventErr := windows.CreateEvent(nil, manual, 0, nil)
		if eventErr != nil {
			app.closeLocalResources()
			return nil, eventErr
		}
		app.events = append(app.events, event)
	}
	app.permissionEvent = app.events[0]
	app.permissionChangeEvent = app.events[1]
	app.enumerationEvent = app.events[2]
	app.defaultEvent = app.events[3]
	app.captureEvent = app.events[4]
	app.pickerEvent = app.events[5]
	app.commandEvent = app.events[6]
	app.shutdownEvent = app.events[7]
	return app, nil
}

func probeDataDir() (string, error) {
	var chars uint32
	result, _, _ := pGetCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&chars)), 0)
	if uint32(result) == uint32(windows.ERROR_INSUFFICIENT_BUFFER) && chars > 1 {
		buffer := make([]uint16, chars)
		result, _, _ = pGetCurrentPackageFamilyName.Call(uintptr(unsafe.Pointer(&chars)), uintptr(unsafe.Pointer(&buffer[0])))
		if result != 0 {
			return "", fmt.Errorf("GetCurrentPackageFamilyName: win32=0x%08x", uint32(result))
		}
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			return "", fmt.Errorf("LOCALAPPDATA is empty inside package")
		}
		// LOCALAPPDATA is already the package's virtualized AppContainer root.
		// Appending Packages/<PFN>/LocalState duplicates the package path below
		// AC and makes the host-side evidence collector look in the wrong place.
		return filepath.Join(local, "PulsarProbe"), nil
	}
	if uint32(result) != winprobe.AppModelErrorNoPackage {
		return "", fmt.Errorf("GetCurrentPackageFamilyName sizing: win32=0x%08x", uint32(result))
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "PulsarProbe"), nil
}

func (a *probeApp) run() error {
	if err := a.createWindows(); err != nil {
		return err
	}
	if !a.evidence.healthy() {
		return fmt.Errorf("required window/tray startup evidence is unavailable")
	}
	a.registerSessionNotifications()
	if !a.evidence.healthy() {
		return fmt.Errorf("required session-notification evidence is unavailable")
	}
	if hr := a.helper.PermissionSubscribe(a.permissionChangeEvent); hr.Failed() {
		a.mu.Lock()
		a.permissionMonitorReady = false
		a.mu.Unlock()
		if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultBlocked, Action: "access_changed_subscribe", SelectedAPIPath: "AppCapability.AccessChanged", HResult: hr.Hex(), FailureCause: "permission revoke subscription unavailable"}) {
			return fmt.Errorf("required permission-subscription evidence is unavailable")
		}
	} else {
		a.mu.Lock()
		a.permissionMonitorReady = true
		a.mu.Unlock()
		if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultPass, Action: "access_changed_subscribe", SelectedAPIPath: "AppCapability.AccessChanged", HResult: hr.Hex()}) {
			return fmt.Errorf("required permission-subscription evidence is unavailable")
		}
	}
	visible := isWindowVisible(a.main)
	controlsResult := winprobe.ResultPass
	if !visible || !a.controlsCreated() {
		controlsResult = winprobe.ResultFail
	}
	if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: controlsResult, Action: "controls_ready", SelectedAPIPath: "hidden-top-level-window+visible-controls", WindowVisible: &visible, Fields: map[string]any{"allRequiredControlsCreated": a.controlsCreated()}}) {
		return fmt.Errorf("required controls evidence is unavailable")
	}
	a.registerHotkey("startup")
	if !a.evidence.healthy() {
		return fmt.Errorf("required hotkey evidence is unavailable")
	}
	go a.waiter()
	a.beginDiscovery()
	a.installSignalBridge()
	return pumpMessages()
}

func (a *probeApp) beginDiscovery() {
	if !runAfterRequiredEvidence(func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultAttempt, Action: "device_discovery_start_intent", SelectedAPIPath: "DeviceInformation.FindAllAsync+MediaDevice.GetDefaultAudioCaptureId"})
	}, nil) {
		a.requestGracefulQuit("required discovery intent evidence write failed")
		return
	}
	var events []winprobe.LogEvent
	invoked := a.lifecycle.runGatedWork(func() {
		events, _ = a.startDiscoveryOperations()
	})
	if !invoked {
		return
	}
	for _, event := range events {
		if !a.log(event) {
			a.requestGracefulQuit("required discovery result evidence write failed")
			return
		}
	}
}

func (a *probeApp) startDiscoveryOperations() ([]winprobe.LogEvent, bool) {
	a.mu.Lock()
	enumerationActive := a.enumerationOp != 0
	defaultActive := a.defaultOp != 0
	a.mu.Unlock()
	events := make([]winprobe.LogEvent, 0, 2)
	allStarted := true
	if !enumerationActive {
		type operationResult struct {
			id uint32
			hr winprobe.HResult
		}
		started, admitted := runAbruptOperation(&a.shutdown, func() operationResult {
			id, hr := a.helper.EnumerateDevices(a.enumerationEvent)
			return operationResult{id: id, hr: hr}
		})
		if !admitted {
			return events, false
		}
		id, hr := started.id, started.hr
		if hr.Succeeded() {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.enumerationOp = id
				a.mu.Unlock()
			}) {
				return events, false
			}
		} else {
			allStarted = false
		}
		events = append(events, winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: resultForHR(hr), Action: "enumerate_inputs", SelectedAPIPath: "DeviceInformation.FindAllAsync(AudioCapture)", HResult: hr.Hex()})
	}
	if !defaultActive {
		type operationResult struct {
			id uint32
			hr winprobe.HResult
		}
		started, admitted := runAbruptOperation(&a.shutdown, func() operationResult {
			id, hr := a.helper.DefaultDevice(0, a.defaultEvent)
			return operationResult{id: id, hr: hr}
		})
		if !admitted {
			return events, false
		}
		id, hr := started.id, started.hr
		if hr.Succeeded() {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.defaultOp = id
				a.mu.Unlock()
			}) {
				return events, false
			}
		} else {
			allStarted = false
		}
		events = append(events, winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: resultForHR(hr), Action: "default_input", SelectedAPIPath: "MediaDevice.GetDefaultAudioCaptureId(Default)", HResult: hr.Hex()})
	}
	return events, allStarted
}

func (a *probeApp) installSignalBridge() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		a.requestGracefulQuit("Ctrl-C/SIGTERM")
	}()
}

func (a *probeApp) waiter() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(a.waiterDone)
	waitFailureLogged := false
	for {
		// Notification events are coalescible readiness hints. A bounded poll also
		// closes the initiate/ID-publication race and drives the MTA-ready timeout.
		waitResult, waitErr := windows.WaitForMultipleObjects(a.events, false, 100)
		cleanupReady := false
		ordinary := []func(){
			func() {
				if waitErr != nil || waitResult == windows.WAIT_FAILED {
					if !waitFailureLogged {
						waitFailureLogged = true
						a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "waiter_wait", SelectedAPIPath: "WaitForMultipleObjects", FailureCause: fmt.Sprintf("wait failed: %v", waitErr), Fields: map[string]any{"waitResult": fmt.Sprintf("0x%08x", waitResult), "getLastError": fmt.Sprintf("0x%08x", win32ErrorCode(waitErr))}})
					}
					time.Sleep(100 * time.Millisecond)
				} else {
					waitFailureLogged = false
				}
			},
			a.drainTerminalIntent,
			a.drainCommands,
			a.cancelInvalidatedPermissionRequest,
			func() { a.drainPermissionChange(waitResult == windows.WAIT_OBJECT_0+1) },
			a.drainPermissionRequest,
			a.drainEnumeration,
			a.drainDefault,
			a.drainCaptureOrphans,
			a.drainCapture,
			a.drainPendingArtifactCleanup,
			a.drainPicker,
			a.driveUITransitions,
			a.drainEvidenceFailure,
			func() { cleanupReady = a.maybeCleanupReady() },
		}
		if a.shutdown.runWaiterIteration(ordinary, a.drainConfirmedShutdownCapture) {
			return
		}
		if cleanupReady {
			return
		}
	}
}

// drainConfirmedShutdownCapture performs the only waiter work admitted after
// the monotonic WM_ENDSESSION latch. It reads a fixed maximum of data already
// buffered by the native capture and appends it to the existing partial
// without sync, finalization, release, UI publication, or helper destruction.
func (a *probeApp) drainConfirmedShutdownCapture() {
	owner := a.shutdown.confirmedCapture.Load()
	if owner == nil {
		return
	}
	a.mu.Lock()
	id, generation, writer, format := a.captureOp, a.captureGeneration, a.writer, a.captureFormat
	a.mu.Unlock()
	if !owner.matches(generation, id) || writer == nil || format.Valid != 1 || format.Channels == 0 || format.Channels > 8 {
		return
	}
	drainConfirmedShutdownBuffer(format.Channels, func(buffer []float32, frames uint32) (uint32, winprobe.HResult) {
		return a.helper.CaptureRead(id, buffer, frames)
	}, writer.WriteBufferedFramesWithoutSync)
}

func (a *probeApp) requestOwnedCaptureStop(generation uint64, operationID uint32, reason winprobe.CaptureReason) captureStopOutcome {
	outcome, admitted := runAbruptOperation(&a.shutdown, func() captureStopOutcome {
		return requestCaptureStopForExactOwner(&a.captureOwners, generation, operationID, func(id uint32) winprobe.HResult {
			return a.helper.CaptureStop(id, reason)
		})
	})
	if !admitted {
		return captureStopOutcome{}
	}
	return outcome
}

func (a *probeApp) requestCaptureRelease(owner *captureOwnerSnapshot, authority captureReleaseAuthority) (captureReleaseOutcome, bool) {
	return runAbruptOperation(&a.shutdown, func() captureReleaseOutcome {
		return owner.requestRelease(authority, func(operationID uint32) winprobe.HResult {
			return a.helper.CaptureRelease(operationID)
		})
	})
}

func (a *probeApp) permissionCheckOperation() (winprobe.PermissionStatus, winprobe.HResult, bool) {
	type permissionResult struct {
		status winprobe.PermissionStatus
		hr     winprobe.HResult
	}
	result, admitted := runAbruptOperation(&a.shutdown, func() permissionResult {
		status, hr, _ := a.permissionQueries.run(permissionQueryWaiter, a.helper.PermissionCheck)
		return permissionResult{status: status, hr: hr}
	})
	return result.status, result.hr, admitted
}

func (a *probeApp) drainCaptureOrphans() {
	for _, obligation := range a.captureOwners.orphanSnapshot() {
		owner := obligation.owner
		result := runCaptureOrphanDrain(obligation, a.helper.CaptureResult, func(operationID uint32) winprobe.HResult {
			return a.helper.CaptureRelease(operationID)
		}, a.shutdown.runOperation)
		if a.shutdown.isClosing() {
			return
		}
		if result.StructuralFailure {
			escalate, admitted := runAbruptOperation(&a.shutdown, func() bool {
				return obligation.failureEscalated.CompareAndSwap(false, true)
			})
			if !admitted {
				return
			}
			if escalate {
				a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_orphan_cleanup_contract_failure", SelectedAPIPath: "waiter-owned-CaptureGetResult+exact-orphan-Release-gate", HResult: orphanFailureHResult(result).Hex(), FailureCause: "an unpublished native capture could not prove exact Stop, terminal, and Release prerequisites; its owner remains retained", Fields: map[string]any{"captureGeneration": owner.generation, "captureOperationId": owner.operationID, "stopState": result.Stop.State, "queryAttempted": result.QueryAttempted, "terminalObserved": result.TerminalObserved, "releaseAttempted": result.Release.attempted()}}, "unpublished capture cleanup contract failure")
			}
			continue
		}
		if !result.Release.attempted() {
			continue
		}
		completion := completeCaptureOrphanDrain(&a.shutdown, &a.captureOwners, obligation, result, func() bool {
			return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultPass, Action: "capture_orphan_released", SelectedAPIPath: "waiter-owned-CaptureGetResult(terminal)+exact-orphan-Release-gate", HResult: result.Release.Result.Hex(), Fields: map[string]any{"captureGeneration": owner.generation, "captureOperationId": owner.operationID, "activeOwnerUnchanged": a.captureOwners.current() != owner}})
		}, func() {
			a.settleReleasedOrphanGeneration(owner, result.QueryResult)
		})
		if !completion.admitted {
			return
		}
		if !completion.cleared {
			if obligation.failureEscalated.CompareAndSwap(false, true) {
				a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_orphan_clear_mismatch", SelectedAPIPath: "exact-orphan-owner-gate", FailureCause: "successful Release could not clear the same orphan obligation", Fields: map[string]any{"captureGeneration": owner.generation, "captureOperationId": owner.operationID}}, "unpublished capture owner clear invariant failed")
			}
			continue
		}
	}
}

func orphanFailureHResult(result captureOrphanDrainResult) winprobe.HResult {
	if result.Release.attempted() {
		return result.Release.Result
	}
	if result.QueryAttempted {
		return result.QueryHResult
	}
	if result.Stop.completed() {
		return result.Stop.Result
	}
	return captureInvalidHandleHResult
}

func (a *probeApp) settleReleasedOrphanGeneration(owner *captureOwnerSnapshot, terminal winprobe.CaptureResult) {
	if owner == nil {
		return
	}
	if incumbent := a.captureOwners.current(); incumbent != nil && incumbent.generation == owner.generation {
		return
	}
	fields := map[string]any{"captureGeneration": owner.generation, "captureOperationId": owner.operationID, "unpublishedNativeCapture": true, "activeOwnerUnaffected": true}
	a.advanceCaptureLifecycles(owner.generation, lifecycleCaptureTerminal, winprobe.ResultPass, "CaptureGetResult(unpublished-terminal)", terminal.Outcome, "", fields)
	a.advanceCaptureLifecycles(owner.generation, lifecycleArtifactDisposed, winprobe.ResultPass, "no-artifact-created-for-unpublished-capture", 0, "", fields)
	a.advanceCaptureLifecycles(owner.generation, lifecycleCaptureReleased, winprobe.ResultPass, "CaptureRelease(unpublished-exact-owner)", 0, "", fields)
	a.postIdleLifecycleCleanup()
}

func captureStopEvidence(stop captureStopOutcome, completedPath string) (winprobe.ProbeResult, string, string) {
	if stop.completed() {
		return resultForHR(stop.Result), completedPath, stop.Result.Hex()
	}
	if stop.pending() {
		return winprobe.ResultAttempt, completedPath + "(exact-owner-pending)", ""
	}
	return winprobe.ResultBlocked, completedPath + "(not-requested)", ""
}

func (a *probeApp) drainCommands() {
	for {
		command, received := receiveAbruptOperation(&a.shutdown, a.commands)
		if !received {
			return
		}
		a.mu.Lock()
		capture, generation := a.captureOp, a.captureGeneration
		a.mu.Unlock()
		switch command.kind {
		case "stop":
			if capture != 0 {
				stop := a.requestOwnedCaptureStop(generation, capture, command.reason)
				result, apiPath, hresult := captureStopEvidence(stop, "CaptureRequestStop")
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: result, Action: "capture_stop_request", SelectedAPIPath: apiPath, HResult: hresult, Fields: map[string]any{"reason": command.reason.String(), "stopPending": stop.pending()}})
				if command.source == winprobe.ScenarioHotkey {
					a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: result, Action: "hotkey_stop_accepted", SelectedAPIPath: "WM_HOTKEY+" + apiPath, HResult: hresult, Fields: map[string]any{"stopPending": stop.pending()}})
				}
			} else if command.source == winprobe.ScenarioHotkey {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultBlocked, Action: "hotkey_stop_accepted", SelectedAPIPath: "WM_HOTKEY+CaptureRequestStop", FailureCause: "no active capture operation"})
			}
		case "permission_check", "permission_rearm":
			status, hr, admitted := a.permissionCheckOperation()
			if !admitted || a.shutdown.isClosing() {
				return
			}
			if hr.Failed() {
				status = winprobe.PermissionUnknown
				signal := "CapPermissionCheck(explicit-record-query-failed)"
				if command.kind == "permission_rearm" {
					signal = "CapPermissionCheck(lifecycle-rearm-query-failed)"
				}
				// Gate invalidation, exact-generation no-native settlement, and
				// CaptureStop (when native ownership exists) must precede every
				// logger/filesystem operation. A failed query never publishes a
				// permission-ready or rearm continuation.
				a.requestLifecycleStop(lifecyclePermissionRevoke, signal, winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle)
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultFail, Action: command.kind, SelectedAPIPath: "CapPermissionCheck(waiter-owned)", PermissionStatus: status.String(), HResult: hr.Hex(), FailureCause: "permission status could not be queried; fail-closed lifecycle cleanup started before diagnostic evidence", Fields: map[string]any{"captureGeneration": command.generation}})
				continue
			}
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.permissionKnown = true
				a.permissionStatus = status
				a.mu.Unlock()
			}) {
				return
			}
			if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: resultForHR(hr), Action: command.kind, SelectedAPIPath: "CapPermissionCheck(waiter-owned)", PermissionStatus: status.String(), HResult: hr.Hex(), Fields: map[string]any{"captureGeneration": command.generation}}) {
				if command.kind == "permission_check" {
					a.settleOrCancelCaptureGeneration(command.generation, "permission_check_evidence_failed")
				}
				a.requestGracefulQuit("required permission query evidence write failed")
				continue
			}
			if command.kind == "permission_rearm" {
				a.publishRearmTransition(command.generation, status)
			} else if !a.postTransition(wmAppPermissionReady, uintptr(command.generation), uintptr(status), command.kind) {
				a.shutdown.runOperation(func() { a.lifecycle.cancelCaptureGeneration(command.generation) })
			}
		case "window_hidden":
			// Flush frames that may have been captured before the hide action,
			// then begin the hidden epoch. This deliberately prefers a false
			// negative over attributing pre-hide buffered audio to the interval.
			a.drainCapture()
			a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.captureVisibility.SetHidden(true)
				a.mu.Unlock()
			})
		case "window_shown":
			// End the hidden epoch before any later drain, so post-restore
			// frames cannot create a false hidden/capture overlap.
			a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.captureVisibility.SetHidden(false)
				a.mu.Unlock()
			})
		}
	}
}

func (a *probeApp) drainTerminalIntent() {
	consume, admitted := runAbruptOperation(&a.shutdown, a.lifecycle.consumeQuitIntent)
	if !admitted || !consume {
		return
	}
	type terminalSnapshot struct {
		capture, permission, enumeration, picker uint32
		generation                               uint64
	}
	snapshot, admitted := runAbruptOperation(&a.shutdown, func() terminalSnapshot {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.quitting = true
		return terminalSnapshot{capture: a.captureOp, generation: a.captureGeneration, permission: a.permissionOp, enumeration: a.enumerationOp, picker: a.pickerOp}
	})
	if !admitted {
		return
	}
	capture, generation := snapshot.capture, snapshot.generation
	permission, enumeration, picker := snapshot.permission, snapshot.enumeration, snapshot.picker
	if run, ok := a.lifecycle.activeRun(lifecycleQuit); ok {
		generation = run.CaptureGeneration
		if run.CaptureOperationID != 0 {
			capture = run.CaptureOperationID
		}
	}
	stop := captureStopOutcome{State: captureStopCompleted}
	if capture != 0 {
		stop = a.requestOwnedCaptureStop(generation, capture, winprobe.ReasonCancel)
	}
	if picker != 0 {
		_, _ = runAbruptOperation(&a.shutdown, func() winprobe.HResult { return a.helper.PickerCancel(picker) })
	}
	if permission != 0 {
		_, _ = runAbruptOperation(&a.shutdown, func() winprobe.HResult { return a.helper.PermissionRequestCancel(permission) })
	}
	if enumeration != 0 {
		_, _ = runAbruptOperation(&a.shutdown, func() winprobe.HResult { return a.helper.EnumerateDevicesCancel(enumeration) })
	}
	stopResult, stopPath, stopHResult := captureStopEvidence(stop, "CaptureRequestStop(cancel)")
	a.advanceLifecycleWithModeHResult(lifecycleQuit, lifecycleStopRequested, stopResult, stopPath+"+cooperative-operation-cancel", stopHResult, "", map[string]any{"captureActive": capture != 0, "captureOperationId": capture, "captureGeneration": generation, "captureStopState": stop.State, "pickerActive": picker != 0, "permissionRequestActive": permission != 0, "enumerationActive": enumeration != 0}, false)
	a.replayCaptureSettlement(generation, "quit_registration_replay")
	if generation != 0 && capture == 0 && permission == 0 {
		phase := a.lifecycle.phaseForGeneration(generation)
		if phase == captureGenerationRequested {
			a.settleSuppressedCaptureGeneration(generation, "quit_invalidated_before_native_capture")
		}
	}
}

func (a *probeApp) cancelInvalidatedPermissionRequest() {
	type cancelState struct {
		id         uint32
		generation uint64
		shouldSend bool
	}
	state, stateAdmitted := runAbruptOperation(&a.shutdown, func() cancelState {
		a.mu.Lock()
		id, generation := a.permissionOp, a.permissionGeneration
		alreadySent := a.permissionCancelSent
		a.mu.Unlock()
		if id == 0 || alreadySent || !a.lifecycle.generationInvalidated(generation) {
			return cancelState{}
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.permissionOp != id || a.permissionCancelSent {
			return cancelState{}
		}
		a.permissionCancelSent = true
		return cancelState{id: id, generation: generation, shouldSend: true}
	})
	if !stateAdmitted || !state.shouldSend {
		return
	}
	id, generation := state.id, state.generation
	hr, admitted := runAbruptOperation(&a.shutdown, func() winprobe.HResult {
		return a.helper.PermissionRequestCancel(id)
	})
	if !admitted || a.shutdown.isClosing() {
		return
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: resultForHR(hr), Action: "permission_request_cancel_for_lifecycle", SelectedAPIPath: "waiter-owned-CapPermissionRequestCancel", HResult: hr.Hex(), Fields: map[string]any{"captureGeneration": generation, "permissionOperationId": id}})
}

func (a *probeApp) drainPermissionChange(accessChangedSignal bool) {
	status, hr, admitted := a.permissionCheckOperation()
	if !admitted || a.shutdown.isClosing() {
		return
	}
	if hr.Failed() {
		a.mu.Lock()
		captureOwned := a.captureOp != 0 || a.permissionOp != 0
		a.mu.Unlock()
		failClosed := a.lifecycle.permissionQueryFailureRequiresStop(accessChangedSignal, captureOwned)
		if failClosed {
			signal := "CapPermissionCheck(runtime-query-failed-with-owned-generation)"
			if accessChangedSignal {
				signal = "AppCapability.AccessChanged+CapPermissionCheck(query-failed)"
			}
			a.requestLifecycleStop(lifecyclePermissionRevoke, signal, winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle)
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultFail, Action: "permission_status_query", SelectedAPIPath: "CapPermissionCheck(waiter-owned)", HResult: hr.Hex(), FailureCause: "permission status could not be queried", Fields: map[string]any{"accessChangedSignal": accessChangedSignal, "captureOrPermissionGenerationOwned": captureOwned, "failClosedCleanupStarted": failClosed}})
		return
	}
	type permissionTransition struct {
		capture     uint32
		alreadySent bool
		changed     bool
	}
	transition, transitionAdmitted := runAbruptOperation(&a.shutdown, func() permissionTransition {
		a.mu.Lock()
		defer a.mu.Unlock()
		capture := a.captureOp
		alreadySent := a.permissionStopSent
		changed := !a.permissionKnown || status != a.permissionStatus
		a.permissionKnown = true
		a.permissionStatus = status
		if status != winprobe.PermissionAllowed && status != winprobe.PermissionUnavailable && capture != 0 && !alreadySent {
			a.permissionStopSent = true
		}
		return permissionTransition{capture: capture, alreadySent: alreadySent, changed: changed}
	})
	if !transitionAdmitted {
		return
	}
	capture, alreadySent, changed := transition.capture, transition.alreadySent, transition.changed
	if status != winprobe.PermissionAllowed && status != winprobe.PermissionUnavailable && capture != 0 && !alreadySent {
		action := "permission_poll_during_capture"
		path := "AppCapability.CheckAccess(periodic-diagnostic-defense)"
		if accessChangedSignal {
			action = "permission_access_changed_during_capture"
			path = "AppCapability.AccessChanged+CheckAccess"
		}
		a.requestLifecycleStop(lifecyclePermissionRevoke, path, winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle)
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultBlocked, Action: action, SelectedAPIPath: path, PermissionStatus: status.String(), FailureCause: "microphone access is no longer allowed; ordered fail-closed cleanup started"})
		return
	}
	if status != winprobe.PermissionAllowed && status != winprobe.PermissionUnavailable && capture == 0 && changed {
		a.requestLifecycleStop(lifecyclePermissionRevoke, "AppCapability.AccessChanged+CheckAccess(no-active-capture)", winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle)
		return
	}
	if status == winprobe.PermissionAllowed && changed {
		type rearmResult struct {
			generation uint64
			accepted   bool
		}
		rearm, admitted := runAbruptOperation(&a.shutdown, func() rearmResult {
			generation, accepted := a.lifecycle.beginRearm()
			return rearmResult{generation: generation, accepted: accepted}
		})
		if admitted && rearm.accepted {
			a.publishRearmTransition(rearm.generation, status)
		}
	}
}

func (a *probeApp) drainPermissionRequest() {
	a.mu.Lock()
	id := a.permissionOp
	generation := a.permissionGeneration
	a.mu.Unlock()
	if id == 0 {
		return
	}
	type permissionRequestResult struct {
		state   int32
		status  winprobe.PermissionStatus
		outcome winprobe.HResult
		callHR  winprobe.HResult
	}
	query, admitted := runAbruptOperation(&a.shutdown, func() permissionRequestResult {
		state, status, outcome, callHR := a.helper.PermissionRequestResult(id)
		return permissionRequestResult{state: state, status: status, outcome: outcome, callHR: callHR}
	})
	if !admitted || a.shutdown.isClosing() {
		return
	}
	state, status, outcome, callHR := query.state, query.status, query.outcome, query.callHR
	disposition := classifyResultQuery(callHR, state)
	if disposition == resultQueryFailed {
		recovery, cleanupAdmitted := runFailClosedQueryOperations(
			a.shutdown.runOperation,
			func() winprobe.HResult { return a.helper.PermissionRequestCancel(id) },
			func() winprobe.HResult { return a.helper.PermissionRequestRelease(id) },
		)
		if !cleanupAdmitted || a.shutdown.isClosing() {
			return
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultFail, Action: "permission_request_query", SelectedAPIPath: "CapPermissionRequestResult+fail-closed-cancel/release", HResult: callHR.Hex(), FailureCause: "permission result query failed; zeroed outputs were ignored", Fields: map[string]any{"cancelHResult": recovery.cancelHR.Hex(), "releaseHResult": recovery.releaseHR.Hex(), "released": recovery.released}})
		if recovery.released {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				if a.permissionOp == id {
					a.permissionOp = 0
					a.permissionGeneration = 0
					a.permissionCancelSent = false
				}
				a.mu.Unlock()
			}) {
				return
			}
			a.settleOrCancelCaptureGeneration(generation, "permission_query_failed_after_lifecycle_invalidation")
		}
		return
	}
	if disposition == resultQueryPending {
		return
	}
	releaseHR, releaseAdmitted := runAbruptOperation(&a.shutdown, func() winprobe.HResult {
		return a.helper.PermissionRequestRelease(id)
	})
	if !releaseAdmitted || a.shutdown.isClosing() {
		return
	}
	if releaseHR.Failed() {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultFail, Action: "permission_request_release", SelectedAPIPath: "CapPermissionRequestRelease", HResult: releaseHR.Hex()})
		return
	}
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		if a.permissionOp == id {
			a.permissionOp = 0
			a.permissionGeneration = 0
			a.permissionCancelSent = false
		}
		a.mu.Unlock()
	}) {
		return
	}
	result := resultForHR(outcome)
	if state == 3 {
		result = winprobe.ResultDiscard
	}
	if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: result, Action: "permission_request_result", SelectedAPIPath: "AppCapability.RequestAccessAsync", PermissionStatus: status.String(), HResult: outcome.Hex()}) {
		a.settleOrCancelCaptureGeneration(generation, "permission_result_evidence_failed")
		a.requestGracefulQuit("required permission result evidence write failed")
		return
	}
	if state == 1 && status == winprobe.PermissionAllowed && a.lifecycle.permissionContinuationAllowed(generation) {
		if !a.postTransition(wmAppPermissionReady, uintptr(generation), uintptr(status), "permission_ready") {
			a.settleOrCancelCaptureGeneration(generation, "permission_ready_post_failed")
		}
		return
	}
	a.settleOrCancelCaptureGeneration(generation, "permission_completion_not_eligible_for_capture")
}

func (a *probeApp) drainEnumeration() {
	a.mu.Lock()
	id := a.enumerationOp
	a.mu.Unlock()
	if id == 0 {
		return
	}
	type enumerationResult struct {
		state   int32
		count   int32
		outcome winprobe.HResult
		callHR  winprobe.HResult
	}
	query, admitted := runAbruptOperation(&a.shutdown, func() enumerationResult {
		state, count, outcome, callHR := a.helper.EnumerateDevicesResult(id)
		return enumerationResult{state: state, count: count, outcome: outcome, callHR: callHR}
	})
	if !admitted || a.shutdown.isClosing() {
		return
	}
	state, count, outcome, callHR := query.state, query.count, query.outcome, query.callHR
	disposition := classifyResultQuery(callHR, state)
	if disposition == resultQueryFailed {
		recovery, cleanupAdmitted := runFailClosedQueryOperations(
			a.shutdown.runOperation,
			func() winprobe.HResult { return a.helper.EnumerateDevicesCancel(id) },
			func() winprobe.HResult { return a.helper.EnumerateDevicesRelease(id) },
		)
		if !cleanupAdmitted || a.shutdown.isClosing() {
			return
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "enumerate_inputs_query", SelectedAPIPath: "CapEnumerateDevicesResult+fail-closed-cancel/release", HResult: callHR.Hex(), FailureCause: "enumeration result query failed; zeroed outputs were ignored", Fields: map[string]any{"cancelHResult": recovery.cancelHR.Hex(), "releaseHResult": recovery.releaseHR.Hex(), "released": recovery.released}})
		if recovery.released {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				if a.enumerationOp == id {
					a.enumerationOp = 0
				}
				a.mu.Unlock()
			}) {
				return
			}
		}
		return
	}
	if disposition == resultQueryPending {
		return
	}
	validCount := count >= 0 && count <= 256
	capacity := int32(0)
	if validCount {
		capacity = count
	}
	devices := make([]winprobe.Device, 0, capacity)
	deviceInfoFailures := 0
	if state == 1 && outcome.Succeeded() {
		if !validCount {
			deviceInfoFailures++
		} else {
			for i := int32(0); i < count; i++ {
				type deviceInfoResult struct {
					device winprobe.Device
					hr     winprobe.HResult
				}
				info, infoAdmitted := runAbruptOperation(&a.shutdown, func() deviceInfoResult {
					device, hr := a.helper.DeviceInfo(id, i)
					return deviceInfoResult{device: device, hr: hr}
				})
				if !infoAdmitted || a.shutdown.isClosing() {
					return
				}
				device, hr := info.device, info.hr
				if hr.Succeeded() {
					devices = append(devices, device)
				} else {
					deviceInfoFailures++
					a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "enumerate_input_info", SelectedAPIPath: "CapGetDeviceInfo", HResult: hr.Hex(), Fields: map[string]any{"index": i}})
				}
			}
		}
	}
	releaseHR, releaseAdmitted := runAbruptOperation(&a.shutdown, func() winprobe.HResult {
		return a.helper.EnumerateDevicesRelease(id)
	})
	if !releaseAdmitted || a.shutdown.isClosing() {
		return
	}
	if releaseHR.Failed() {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "enumerate_inputs_release", SelectedAPIPath: "CapEnumerateDevicesRelease", HResult: releaseHR.Hex()})
		return
	}
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		a.enumerationOp = 0
		a.devices = devices
		if len(devices) > 0 {
			a.selected = 0
		}
		a.mu.Unlock()
	}) {
		return
	}
	event := winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: resultForHR(outcome), Action: "enumerate_inputs_result", SelectedAPIPath: "DeviceInformation.FindAllAsync(AudioCapture)+CapGetDeviceInfo", HResult: outcome.Hex(), Fields: map[string]any{"reportedCount": count, "returnedCount": len(devices), "deviceInfoFailures": deviceInfoFailures}}
	if deviceInfoFailures != 0 {
		event.Result = winprobe.ResultFail
		event.FailureCause = "one or more enumerated device identities could not be read"
	}
	if !a.log(event) {
		a.requestGracefulQuit("required enumeration evidence write failed")
		return
	}
	a.postTransition(wmAppDevicesReady, uintptr(id), 0, "devices_ready")
}

func (a *probeApp) drainDefault() {
	a.mu.Lock()
	id := a.defaultOp
	a.mu.Unlock()
	if id == 0 {
		return
	}
	type defaultDeviceResult struct {
		state    int32
		deviceID string
		outcome  winprobe.HResult
		callHR   winprobe.HResult
	}
	query, admitted := runAbruptOperation(&a.shutdown, func() defaultDeviceResult {
		state, deviceID, outcome, callHR := a.helper.DefaultDeviceResult(id)
		return defaultDeviceResult{state: state, deviceID: deviceID, outcome: outcome, callHR: callHR}
	})
	if !admitted || a.shutdown.isClosing() {
		return
	}
	state, deviceID, outcome, callHR := query.state, query.deviceID, query.outcome, query.callHR
	disposition := classifyResultQuery(callHR, state)
	if disposition == resultQueryFailed {
		recovery, cleanupAdmitted := runFailClosedQueryOperations(a.shutdown.runOperation, nil, func() winprobe.HResult { return a.helper.DefaultDeviceRelease(id) })
		if !cleanupAdmitted || a.shutdown.isClosing() {
			return
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "default_input_query", SelectedAPIPath: "CapGetDefaultDeviceResult+fail-closed-release", HResult: callHR.Hex(), FailureCause: "default-device result query failed; zeroed outputs were ignored", Fields: map[string]any{"releaseHResult": recovery.releaseHR.Hex(), "released": recovery.released}})
		if recovery.released {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				if a.defaultOp == id {
					a.defaultOp = 0
				}
				a.mu.Unlock()
			}) {
				return
			}
		}
		return
	}
	if disposition == resultQueryPending {
		return
	}
	releaseHR, releaseAdmitted := runAbruptOperation(&a.shutdown, func() winprobe.HResult {
		return a.helper.DefaultDeviceRelease(id)
	})
	if !releaseAdmitted || a.shutdown.isClosing() {
		return
	}
	if releaseHR.Failed() {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "default_input_release", SelectedAPIPath: "CapGetDefaultDeviceRelease", HResult: releaseHR.Hex()})
		return
	}
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		a.defaultOp = 0
		if state == 1 {
			a.defaultDevice = deviceID
		}
		a.mu.Unlock()
	}) {
		return
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: resultForHR(outcome), Action: "default_input_result", SelectedAPIPath: "MediaDevice.GetDefaultAudioCaptureId(Default)", DeviceID: deviceID, HResult: outcome.Hex()})
}

func (a *probeApp) drainCapture() {
	a.mu.Lock()
	id := a.captureOp
	generation := a.captureGeneration
	writer := a.writer
	finalized := a.captureFinalized
	a.mu.Unlock()
	if id == 0 {
		return
	}
	if finalized {
		owner := a.captureOwners.matching(generation, id)
		if owner == nil {
			a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_owner_mismatch", SelectedAPIPath: "exact-generation-operation-owner-gate", FailureCause: "finalized capture has no matching published native owner", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "capture release ownership invariant failed")
			return
		}
		releaseAuthority, releaseAuthorized := owner.finalizedReleaseAuthority()
		if !releaseAuthorized {
			a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_authority_missing", SelectedAPIPath: "exact-owner-finalized-release-authority", FailureCause: "neither native terminal evidence nor an exact completed S_OK Stop authorizes Release; ownership was retained", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "finalized capture has no valid release authority")
			return
		}
		release, admitted := a.requestCaptureRelease(owner, releaseAuthority)
		if !admitted || a.shutdown.isClosing() {
			return
		}
		if release.pending() {
			return
		}
		if !release.released() {
			if releaseAuthority == captureReleaseAfterAcceptedStop && release.Result == captureReleaseBeforeTerminalHResult {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultAttempt, Action: "capture_release_waiting_for_terminal", SelectedAPIPath: "exact-owner-gate+CaptureRelease(retry-after-S_OK-Stop)", HResult: release.Result.Hex(), FailureCause: "the nonblocking Stop is accepted but native terminal release is not ready; the exact owner remains retryable"})
				return
			}
			a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_retry", SelectedAPIPath: "exact-owner-gate+CaptureRelease", HResult: release.Result.Hex(), FailureCause: "CaptureRelease did not return the contractually required S_OK; native ownership was retained"}, "capture release ABI contract failure")
			return
		}
		cleared, clearAdmitted := runAbruptOperation(&a.shutdown, func() bool {
			return a.captureOwners.clearReleased(owner)
		})
		if !clearAdmitted {
			return
		}
		if !cleared {
			a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_clear_mismatch", SelectedAPIPath: "exact-generation-operation-owner-gate", FailureCause: "successful native release could not clear the same published owner", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "capture owner clear invariant failed")
			return
		}
		if !a.shutdown.runOperation(func() {
			a.mu.Lock()
			if a.captureOp == id {
				a.captureOp = 0
				a.captureGeneration = 0
				a.captureFinalized = false
			}
			a.mu.Unlock()
		}) {
			return
		}
		if a.markPendingArtifactCaptureReleased(id) {
			a.drainPendingArtifactCleanup()
		} else {
			a.advanceCaptureLifecycles(generation, lifecycleCaptureReleased, winprobe.ResultPass, "CaptureRelease", release.Result, "", map[string]any{"captureOperationId": id, "releaseRetry": true})
			a.postIdleLifecycleCleanup()
		}
		return
	}
	type captureResultCall struct {
		result winprobe.CaptureResult
		hr     winprobe.HResult
	}
	query, admitted := runAbruptOperation(&a.shutdown, func() captureResultCall {
		result, hr := a.helper.CaptureResult(id)
		return captureResultCall{result: result, hr: hr}
	})
	if !admitted {
		return
	}
	result, callHR := query.result, query.hr
	if callHR.Failed() {
		var finalizeArtifact func() error
		if writer != nil {
			finalizeArtifact = writer.Abort
		}
		owner := a.captureOwners.matching(generation, id)
		failure := runCaptureResultQueryFailure(owner, id, callHR, func(operationID uint32) winprobe.HResult {
			return a.helper.CaptureStop(operationID, winprobe.ReasonCancel)
		}, finalizeArtifact, func(operationID uint32) winprobe.HResult {
			return a.helper.CaptureRelease(operationID)
		}, a.shutdown.runOperation)
		if failure.InvalidNativeOwner {
			failure.handleInvalidNativeOwner(&a.shutdown, func() bool {
				return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_result_owner_contract_failure", SelectedAPIPath: "CaptureGetResult+exact-owner-structural-gate", HResult: callHR.Hex(), FailureCause: "the helper rejected its own published nonzero capture operation; terminal and release ownership remain retained", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id, "queryFailureCleanupAttempted": false}})
			}, func() {
				a.requestGracefulQuit("capture result owner ABI contract failure")
			})
			return
		}
		cleanup := failure.Cleanup
		if a.shutdown.isClosing() {
			return
		}
		if !cleanup.Stop.completed() {
			stopResult, stopPath, _ := captureStopEvidence(cleanup.Stop, "CaptureRequestStop(cancel)")
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: stopResult, Action: "capture_result_query_stop_pending", SelectedAPIPath: "CaptureGetResult(query-failed)+" + stopPath, HResult: callHR.Hex(), FailureCause: "capture result query failed while the exact-owner stop result is pending; artifact and native release remain owned for terminal-event retry", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id, "captureStopState": cleanup.Stop.State, "artifactCleanupDeferred": writer != nil, "releaseDeferred": true}})
			return
		}
		if cleanup.StructuralFailure && !cleanup.FinalizeAttempted {
			a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_stop_contract_failure", SelectedAPIPath: "CaptureGetResult(query-failed)+CaptureRequestStop(cancel)+exact-owner-gate", HResult: cleanup.Stop.Result.Hex(), FailureCause: "CaptureRequestStop did not return the contractually required S_OK; artifact and native ownership were retained", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id, "artifactFinalized": false, "nativeReleaseAttempted": false}}, "capture stop ABI contract failure")
			return
		}
		hiddenOverlap := false
		artifactCleanupErr := cleanup.FinalizeError
		artifactDiscarded := writer == nil || cleanup.FinalizeAttempted && artifactCleanupErr == nil
		if artifactCleanupErr != nil {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_query_artifact_abort", SelectedAPIPath: "fail-closed-artifact-delete+postcondition-check", FailureCause: artifactCleanupErr.Error()})
		}
		recovery := queryFailureOutcome{cancelHR: cleanup.Stop.Result, releaseHR: cleanup.ReleaseResult, released: cleanup.Released}
		structuralEvidencePassed := true
		if cleanup.StructuralFailure {
			structural := a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_contract_failure", SelectedAPIPath: "CaptureGetResult(query-failed)+exact-owner-Release-gate", HResult: cleanup.ReleaseResult.Hex(), FailureCause: "CaptureRelease did not return exact S_OK; native ownership remains retained", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "capture release ABI contract failure")
			structuralEvidencePassed = structural.evidencePassed
		}
		if a.shutdown.isClosing() {
			return
		}
		cleared := true
		clearAdmitted := true
		if recovery.released {
			cleared, clearAdmitted = runAbruptOperation(&a.shutdown, func() bool {
				return a.captureOwners.clearReleased(owner)
			})
		}
		if !clearAdmitted {
			return
		}
		if recovery.released && !cleared {
			a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_clear_mismatch", SelectedAPIPath: "exact-generation-operation-owner-gate", FailureCause: "successful query-recovery release could not clear the same published owner", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "capture owner clear invariant failed")
			return
		}
		if !a.shutdown.runOperation(func() {
			a.mu.Lock()
			hiddenOverlap = a.captureVisibility.FrameOverlap()
			a.writer = nil
			a.captureFormat = winprobe.CaptureFormat{}
			a.activatePosted = false
			a.captureObservation = winprobe.CaptureObservation{}
			a.captureVisibility.Reset()
			a.captureDiagnostics = winprobe.CaptureDiagnostics{}
			a.frameLimiter = winprobe.FrameLimiter{}
			a.discardedCaptureFrames = 0
			a.captureFinalized = !recovery.released
			if recovery.released && a.captureOp == id {
				a.captureOp = 0
				a.captureGeneration = 0
			}
			a.mu.Unlock()
		}) {
			return
		}
		if !structuralEvidencePassed {
			return
		}
		failureCause := "capture result query failed; zeroed outputs were ignored"
		if artifactDiscarded {
			failureCause += "; owned artifact paths were verified absent"
		} else {
			failureCause += "; owned artifact cleanup failed: " + artifactCleanupErr.Error()
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_result_query", SelectedAPIPath: "CaptureGetResult+fail-closed-stop/release", HResult: callHR.Hex(), FailureCause: failureCause, Fields: map[string]any{"stopHResult": recovery.cancelHR.Hex(), "releaseHResult": recovery.releaseHR.Hex(), "releaseAwaitsNativeTerminal": cleanup.ReleaseAwaitsTerminal, "released": recovery.released, "hiddenFrameOverlap": hiddenOverlap, "artifactDiscarded": artifactDiscarded}})
		a.advanceCaptureLifecycles(generation, lifecycleCaptureTerminal, winprobe.ResultFail, "CaptureGetResult(query-failed; terminal-unavailable)", callHR, failureCause, map[string]any{"captureOperationId": id, "terminalObserved": false})
		if artifactDiscarded {
			a.advanceCaptureLifecycles(generation, lifecycleArtifactDisposed, winprobe.ResultPass, "ArtifactWriter.Abort(fail-closed-postcondition)", callHR, "", map[string]any{"captureOperationId": id, "ownedTemporaryArtifactsAbsent": true})
		} else {
			a.queueArtifactCleanupRetry(writer, id, generation, recovery.released, map[string]any{"captureOperationId": id, "origin": "CaptureGetResult-query-failure", "cleanupError": artifactCleanupErr.Error()})
		}
		if recovery.released && artifactDiscarded {
			a.advanceCaptureLifecycles(generation, lifecycleCaptureReleased, winprobe.ResultPass, "CaptureRelease(fail-closed-query-recovery)", recovery.releaseHR, "", map[string]any{"captureOperationId": id})
			a.postIdleLifecycleCleanup()
		}
		return
	}
	type diagnosticsCall struct {
		diagnostics winprobe.CaptureDiagnostics
		hr          winprobe.HResult
	}
	diagnosticsResult, diagnosticsAdmitted := runAbruptOperation(&a.shutdown, func() diagnosticsCall {
		diagnostics, hr := a.helper.CaptureDiagnostics(id)
		return diagnosticsCall{diagnostics: diagnostics, hr: hr}
	})
	if !diagnosticsAdmitted || a.shutdown.isClosing() {
		return
	}
	diagnostics, diagnosticsHR := diagnosticsResult.diagnostics, diagnosticsResult.hr
	if diagnosticsHR.Failed() {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_diagnostics_query", SelectedAPIPath: "PulsarProbeCaptureGetDiagnosticsV1(private-extension-v1)", HResult: diagnosticsHR.Hex(), FailureCause: "native timestamp/cleanup evidence could not be retrieved"})
	} else {
		previous, admitted := runAbruptOperation(&a.shutdown, func() winprobe.CaptureDiagnostics {
			a.mu.Lock()
			defer a.mu.Unlock()
			previous := a.captureDiagnostics
			a.captureDiagnostics = diagnostics
			return previous
		})
		if !admitted {
			return
		}
		for _, event := range winprobe.CaptureDiagnosticEvents(previous, diagnostics) {
			a.log(event)
		}
	}
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		a.captureVisibility.ObserveState(result.State)
		a.mu.Unlock()
	}) {
		return
	}
	if result.State == winprobe.CaptureStatePreparing && result.Format.Ready == 0 {
		timedOut, admitted := runAbruptOperation(&a.shutdown, func() bool {
			a.mu.Lock()
			defer a.mu.Unlock()
			timedOut := !a.capturePrepareAt.IsZero() && time.Since(a.capturePrepareAt) >= 5*time.Second && !a.captureTimeoutSent
			if timedOut {
				a.captureTimeoutSent = true
			}
			return timedOut
		})
		if !admitted {
			return
		}
		if timedOut {
			stop := a.requestOwnedCaptureStop(generation, id, winprobe.ReasonCancel)
			_, stopPath, stopHResult := captureStopEvidence(stop, "CaptureRequestStop(cancel)")
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "mta_readiness_timeout", SelectedAPIPath: "CapturePrepare+CoInitializeEx(MTA)+" + stopPath, HResult: "0x800705b4", FailureCause: "capture MTA readiness was not published within five seconds", Fields: map[string]any{"stopRequestHResult": stopHResult, "stopPending": stop.pending()}})
		}
	}
	if result.State == winprobe.CaptureStatePreparing && result.Format.Ready == 1 {
		shouldPost, admitted := runAbruptOperation(&a.shutdown, func() bool {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.activatePosted {
				return false
			}
			a.activatePosted = true
			return true
		})
		if !admitted {
			return
		}
		if shouldPost && !a.postTransition(wmAppCaptureReady, uintptr(generation), uintptr(id), "capture_activate_ready") {
			_ = a.requestOwnedCaptureStop(generation, id, winprobe.ReasonCancel)
		}
	}
	startArtifact, observationAdmitted := runAbruptOperation(&a.shutdown, func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return writer == nil && a.captureObservation.Observe(result.State, result.Format)
	})
	if !observationAdmitted {
		return
	}
	if startArtifact {
		dir := filepath.Join(filepath.Dir(a.logFile.Name()), "evidence")
		type artifactCreateResult struct {
			writer *winprobe.ArtifactWriter
			err    error
		}
		created, createAdmitted := runAbruptOperation(&a.shutdown, func() artifactCreateResult {
			newWriter, err := winprobe.NewArtifactWriter(dir, fmt.Sprintf("capture-%d", time.Now().UnixMilli()), result.Format)
			return artifactCreateResult{writer: newWriter, err: err}
		})
		if !createAdmitted || a.shutdown.isClosing() {
			return
		}
		newWriter, err := created.writer, created.err
		if err != nil {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "artifact_create", FailureCause: err.Error()})
			_ = a.requestOwnedCaptureStop(generation, id, winprobe.ReasonCancel)
		} else {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.writer = newWriter
				a.captureFormat = result.Format
				a.frameLimiter = winprobe.FrameLimiter{LimitFrames: uint64(probeSeconds) * uint64(result.Format.SampleRate)}
				a.mu.Unlock()
			}) {
				return
			}
			writer = newWriter
			if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultPass, Action: "capture_started", SelectedAPIPath: "ActivateAudioInterfaceAsync+event-driven-WASAPI", HResult: callHR.Hex(), Fields: map[string]any{"sampleRate": result.Format.SampleRate, "channels": result.Format.Channels, "nativeSubtype": result.Format.NativeSubtype, "nativeBits": result.Format.NativeBits, "nativeValidBits": result.Format.NativeValidBits}}) {
				_ = a.requestOwnedCaptureStop(generation, id, winprobe.ReasonCancel)
				a.requestGracefulQuit("required capture-start evidence write failed")
				return
			}
			a.postTransition(wmAppCaptureStarted, uintptr(id), 0, "capture_started")
		}
	}
	channels := result.Format.Channels
	a.mu.Lock()
	if channels == 0 {
		channels = a.captureFormat.Channels
	}
	a.mu.Unlock()
	var discardedFrames uint64
	if channels != 0 {
		for {
			buffer := make([]float32, 4096*channels)
			type captureReadResult struct {
				frames uint32
				hr     winprobe.HResult
			}
			read, readAdmitted := runAbruptOperation(&a.shutdown, func() captureReadResult {
				frames, hr := a.helper.CaptureRead(id, buffer, 4096)
				return captureReadResult{frames: frames, hr: hr}
			})
			if !readAdmitted || a.shutdown.isClosing() {
				return
			}
			frames, hr := read.frames, read.hr
			if hr == 1 || frames == 0 {
				break
			}
			if hr.Failed() {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_read", SelectedAPIPath: "CaptureRead", HResult: hr.Hex()})
				_ = a.requestOwnedCaptureStop(generation, id, winprobe.ReasonCancel)
				break
			}
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				if isWindowVisible(a.main) {
					a.captureVisibility.SetHidden(false)
				}
				a.captureVisibility.ObserveFrames(frames)
				a.mu.Unlock()
			}) {
				return
			}
			if writer == nil {
				_, discard := winprobe.AccountCaptureRead(false, false, frames)
				discardedFrames += uint64(discard)
				continue
			}
			type frameAcceptance struct {
				writeFrames uint32
				requestStop bool
			}
			accepted, acceptanceAdmitted := runAbruptOperation(&a.shutdown, func() frameAcceptance {
				a.mu.Lock()
				defer a.mu.Unlock()
				writeFrames, requestStop := a.frameLimiter.Accept(frames)
				return frameAcceptance{writeFrames: writeFrames, requestStop: requestStop}
			})
			if !acceptanceAdmitted {
				return
			}
			writeFrames, requestStop := accepted.writeFrames, accepted.requestStop
			discardedFrames += uint64(frames - writeFrames)
			if writeFrames != 0 {
				writeErr, writeAdmitted := runAbruptOperation(&a.shutdown, func() error {
					return writer.WriteFrames(buffer[:writeFrames*channels], writeFrames)
				})
				if !writeAdmitted || a.shutdown.isClosing() {
					return
				}
				if writeErr != nil {
					a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "artifact_write_or_periodic_sync", SelectedAPIPath: "append-float32+FlushFileBuffers(sampleRate-frame-threshold)", FailureCause: writeErr.Error(), Fields: map[string]any{"framesWritten": writer.Frames(), "syncThresholdFrames": result.Format.SampleRate}})
					_ = a.requestOwnedCaptureStop(generation, id, winprobe.ReasonCancel)
					break
				}
			}
			if requestStop {
				stop := a.requestOwnedCaptureStop(generation, id, winprobe.ReasonUserStop)
				stopResult, stopPath, stopHResult := captureStopEvidence(stop, "CaptureRequestStop")
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: stopResult, Action: "probe_duration_limit", SelectedAPIPath: "whole-frame-clip+" + stopPath, HResult: stopHResult, Fields: map[string]any{"limitFrames": a.frameLimiter.LimitFrames, "stopPending": stop.pending()}})
			}
		}
	}
	totalDiscardedFrames, accountingAdmitted := runAbruptOperation(&a.shutdown, func() uint64 {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.discardedCaptureFrames += discardedFrames
		return a.discardedCaptureFrames
	})
	if !accountingAdmitted {
		return
	}
	if result.State < winprobe.CaptureStateStopped {
		return
	}
	owner := a.captureOwners.matching(generation, id)
	if owner == nil {
		a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_terminal_owner_mismatch", SelectedAPIPath: "CaptureGetResult(terminal)+exact-owner-gate", HResult: result.Outcome.Hex(), FailureCause: "native terminal evidence has no matching published owner", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "capture terminal ownership invariant failed")
		return
	}
	terminalObserved, terminalAdmission := runAbruptOperation(&a.shutdown, owner.observeNativeTerminal)
	if !terminalAdmission || !terminalObserved || a.shutdown.isClosing() {
		return
	}
	a.ensureTerminalLifecycle(result, generation)
	a.advanceCaptureLifecycles(generation, lifecycleCaptureTerminal, winprobe.ResultPass, "CaptureGetResult(terminal-after-native-cleanup)", result.Outcome, "", map[string]any{"captureOperationId": id, "terminalObserved": true, "terminalReason": result.Reason.String(), "terminalState": result.State})
	permission, permissionHR, permissionAdmitted := a.permissionCheckOperation()
	if !permissionAdmitted || a.shutdown.isClosing() {
		return
	}
	artifactPath := ""
	outcomeResult := winprobe.ResultFail
	var finalizeErr error
	a.mu.Lock()
	monitorReady := a.permissionMonitorReady
	observedCapturing := a.captureObservation.ObservedCapturing
	formatForArtifact := a.captureFormat
	hiddenFrameOverlap := a.captureVisibility.FrameOverlap()
	finalDiagnostics := a.captureDiagnostics
	a.mu.Unlock()
	if writer != nil {
		minFrames := uint64(formatForArtifact.SampleRate) * minimumProbeMillis / 1000
		type artifactFinalizeResult struct {
			path   string
			result winprobe.ProbeResult
			err    error
		}
		finalizedArtifact, finalizeAdmitted := runAbruptOperation(&a.shutdown, func() artifactFinalizeResult {
			path, result, err := writer.Finalize(result.Reason, result.Outcome, winprobe.PromotionContext{Permission: permission, PermissionMonitorReady: monitorReady}, minFrames)
			return artifactFinalizeResult{path: path, result: result, err: err}
		})
		if !finalizeAdmitted || a.shutdown.isClosing() {
			return
		}
		artifactPath, outcomeResult, finalizeErr = finalizedArtifact.path, finalizedArtifact.result, finalizedArtifact.err
		if finalizeErr != nil {
			cleanupErr, abortAdmitted := runAbruptOperation(&a.shutdown, writer.Abort)
			if !abortAdmitted || a.shutdown.isClosing() {
				return
			}
			if cleanupErr != nil {
				a.queueArtifactCleanupRetry(writer, id, generation, false, map[string]any{"captureOperationId": id, "origin": "ArtifactWriter.Finalize", "finalizeError": finalizeErr.Error(), "cleanupError": cleanupErr.Error()})
				finalizeErr = fmt.Errorf("%v; temporary artifact cleanup remains pending: %w", finalizeErr, cleanupErr)
			}
		}
	} else if !observedCapturing {
		finalizeErr = fmt.Errorf("terminal observed before positive CAPTURING evidence; buffered frames were drained and discarded")
	} else {
		finalizeErr = fmt.Errorf("CAPTURING was observed but no evidence writer was available")
	}
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		a.writer = nil
		a.captureFormat = winprobe.CaptureFormat{}
		a.activatePosted = false
		a.capturePrepareAt = time.Time{}
		a.captureTimeoutSent = false
		a.permissionStopSent = false
		a.captureFinalized = true
		a.captureObservation = winprobe.CaptureObservation{}
		a.captureVisibility.Reset()
		a.captureDiagnostics = winprobe.CaptureDiagnostics{}
		a.frameLimiter = winprobe.FrameLimiter{}
		a.discardedCaptureFrames = 0
		a.mu.Unlock()
	}) {
		return
	}
	event := winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: outcomeResult, Action: "capture_terminal", SelectedAPIPath: "CaptureGetResult+CaptureRead", PermissionStatus: permission.String(), HResult: result.Outcome.Hex(), Fields: map[string]any{"reason": result.Reason.String(), "artifactRetained": artifactPath != "", "permissionCheckHR": permissionHR.Hex(), "permissionMonitorReady": monitorReady, "discardedBufferedFrames": totalDiscardedFrames, "hiddenDuringCapture": hiddenFrameOverlap, "hiddenEvidenceBasis": "positive CAPTURING plus post-hide-drain frame overlap", "timestampErrorCount": finalDiagnostics.TimestampErrorCount, "cleanupReleaseBufferHResult": finalDiagnostics.CleanupReleaseBufferError.Hex(), "cleanupStopHResult": finalDiagnostics.CleanupStopError.Hex()}}
	if finalizeErr != nil {
		event.FailureCause = finalizeErr.Error()
	}
	a.log(event)
	artifactFields := map[string]any{"captureOperationId": id, "terminalReason": result.Reason.String(), "artifactRetained": artifactPath != "", "artifactResult": outcomeResult}
	artifactFailure := ""
	if finalizeErr != nil {
		artifactFailure = finalizeErr.Error()
	}
	a.mu.Lock()
	cleanupPending := a.artifactCleanup != nil && a.artifactCleanup.captureOperationID == id
	a.mu.Unlock()
	if !cleanupPending {
		a.advanceCaptureLifecycles(generation, lifecycleArtifactDisposed, outcomeResult, "ArtifactWriter.Finalize(fail-closed)", result.Outcome, artifactFailure, artifactFields)
	}
	a.postTransition(wmAppCaptureTerminal, uintptr(id), 0, "capture_terminal")
	release, releaseAdmitted := a.requestCaptureRelease(owner, captureReleaseAfterTerminal)
	if !releaseAdmitted || a.shutdown.isClosing() {
		return
	}
	if release.pending() {
		return
	}
	if !release.released() {
		a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release", SelectedAPIPath: "exact-owner-gate+CaptureRelease", HResult: release.Result.Hex(), FailureCause: "CaptureRelease did not return the contractually required S_OK; native ownership was retained"}, "capture release ABI contract failure")
		return
	}
	cleared, clearAdmitted := runAbruptOperation(&a.shutdown, func() bool {
		return a.captureOwners.clearReleased(owner)
	})
	if !clearAdmitted {
		return
	}
	if !cleared {
		a.recordRequiredStructuralFailure(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_release_clear_mismatch", SelectedAPIPath: "exact-generation-operation-owner-gate", FailureCause: "successful native release could not clear the same published owner", Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}}, "capture owner clear invariant failed")
		return
	}
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		if a.captureOp == id {
			a.captureOp = 0
			a.captureGeneration = 0
			a.captureFinalized = false
		}
		a.mu.Unlock()
	}) {
		return
	}
	if a.markPendingArtifactCaptureReleased(id) {
		a.drainPendingArtifactCleanup()
	} else {
		a.advanceCaptureLifecycles(generation, lifecycleCaptureReleased, winprobe.ResultPass, "CaptureRelease", release.Result, "", map[string]any{"captureOperationId": id})
		a.postIdleLifecycleCleanup()
	}
}

func (a *probeApp) drainPicker() {
	a.mu.Lock()
	id := a.pickerOp
	processed := a.pickerProcessed
	a.mu.Unlock()
	if id == 0 {
		return
	}
	if processed {
		releaseHR, releaseAdmitted := runAbruptOperation(&a.shutdown, func() winprobe.HResult {
			return a.helper.PickerRelease(id)
		})
		if !releaseAdmitted || a.shutdown.isClosing() {
			return
		}
		if releaseHR.Failed() {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: winprobe.ResultFail, Action: "picker_release_retry", SelectedAPIPath: "PickerRelease", HResult: releaseHR.Hex()})
			return
		}
		a.shutdown.runOperation(func() {
			a.mu.Lock()
			if a.pickerOp == id {
				a.pickerOp = 0
				a.pickerProcessed = false
			}
			a.mu.Unlock()
		})
		return
	}
	type pickerQueryResult struct {
		metadata winprobe.PickerResult
		required int32
		callHR   winprobe.HResult
	}
	query, admitted := runAbruptOperation(&a.shutdown, func() pickerQueryResult {
		metadata, required, callHR := a.helper.PickerResult(id, false, 0)
		return pickerQueryResult{metadata: metadata, required: required, callHR: callHR}
	})
	if !admitted || a.shutdown.isClosing() {
		return
	}
	metadata, required, callHR := query.metadata, query.required, query.callHR
	disposition := classifyResultQuery(callHR, metadata.State)
	if disposition == resultQueryFailed {
		recovery, cleanupAdmitted := runFailClosedQueryOperations(
			a.shutdown.runOperation,
			func() winprobe.HResult { return a.helper.PickerCancel(id) },
			func() winprobe.HResult { return a.helper.PickerRelease(id) },
		)
		if !cleanupAdmitted || a.shutdown.isClosing() {
			return
		}
		restoreHidden, stateAdmitted := runAbruptOperation(&a.shutdown, func() bool {
			a.mu.Lock()
			defer a.mu.Unlock()
			restoreHidden := a.pickerWindow.complete()
			a.pickerProcessed = !recovery.released
			if recovery.released && a.pickerOp == id {
				a.pickerOp = 0
			}
			return restoreHidden
		})
		if !stateAdmitted {
			return
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: winprobe.ResultFail, Action: "picker_result_query", SelectedAPIPath: "PickerGetResult+fail-closed-cancel/release", HResult: callHR.Hex(), FailureCause: "picker result query failed; zeroed outputs were ignored and any untaken handle is release-owned", Fields: map[string]any{"cancelHResult": recovery.cancelHR.Hex(), "releaseHResult": recovery.releaseHR.Hex(), "released": recovery.released, "restoreHidden": restoreHidden}})
		if restoreHidden {
			a.postTransition(wmAppPickerTerminal, uintptr(id), 1, "picker_query_failure_restore")
		}
		return
	}
	if disposition == resultQueryPending {
		return
	}
	event := winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Action: "picker_result", SelectedAPIPath: "FileOpenPicker+IStorageItemHandleAccess::Create", HResult: metadata.Outcome.Hex()}
	if metadata.State == 1 && metadata.Outcome.Succeeded() {
		taken, takeAdmitted := runAbruptOperation(&a.shutdown, func() pickerQueryResult {
			picked, ignored, takeHR := a.helper.PickerResult(id, true, required)
			return pickerQueryResult{metadata: picked, required: ignored, callHR: takeHR}
		})
		if !takeAdmitted || a.shutdown.isClosing() {
			return
		}
		picked, takeHR := taken.metadata, taken.callHR
		if takeHR.Failed() || !picked.HandleTaken || picked.Handle == windows.InvalidHandle {
			event.Result = winprobe.ResultFail
			event.FailureCause = "take-once readable handle transfer failed"
			event.Fields = map[string]any{"takeCallHResult": takeHR.Hex(), "handleTaken": picked.HandleTaken}
		} else {
			type pickedReadResult struct {
				hash      string
				bytesRead int64
				err       error
			}
			read, readAdmitted := runAbruptOperation(&a.shutdown, func() pickedReadResult {
				hash, bytesRead, err := winprobe.ReadAndHashPickedFile(handleReader{handle: picked.Handle}, maxPickedBytes)
				return pickedReadResult{hash: hash, bytesRead: bytesRead, err: err}
			})
			if !readAdmitted || a.shutdown.isClosing() {
				return
			}
			_, closeAdmitted := runAbruptOperation(&a.shutdown, func() error { return windows.CloseHandle(picked.Handle) })
			if !closeAdmitted || a.shutdown.isClosing() {
				return
			}
			hash, bytesRead, err := read.hash, read.bytesRead, read.err
			event.Fields = map[string]any{"metadataSize": picked.FileSize, "actualBytes": bytesRead, "sha256": hash}
			if err != nil {
				event.Result = winprobe.ResultFail
				event.FailureCause = err.Error()
			} else {
				event.Result = winprobe.ResultPass
			}
		}
	} else if metadata.State == 2 {
		event.Result = winprobe.ResultDiscard
	} else {
		event.Result = winprobe.ResultBlocked
		event.FailureCause = "brokered readable handle path failed"
	}
	restoreHidden := false
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		restoreHidden = a.pickerWindow.complete()
		a.pickerProcessed = true
		a.mu.Unlock()
	}) {
		return
	}
	a.log(event)
	a.postTransition(wmAppPickerTerminal, uintptr(id), boolToUintptr(restoreHidden), "picker_terminal")
	releaseHR, releaseAdmitted := runAbruptOperation(&a.shutdown, func() winprobe.HResult {
		return a.helper.PickerRelease(id)
	})
	if !releaseAdmitted || a.shutdown.isClosing() {
		return
	}
	if releaseHR.Failed() {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: winprobe.ResultFail, Action: "picker_release", SelectedAPIPath: "PickerRelease", HResult: releaseHR.Hex()})
		return
	}
	a.shutdown.runOperation(func() {
		a.mu.Lock()
		if a.pickerOp == id {
			a.pickerOp = 0
			a.pickerProcessed = false
		}
		a.mu.Unlock()
	})
}

func (a *probeApp) maybeCleanupReady() bool {
	a.mu.Lock()
	quitting := a.quitting
	empty := a.permissionOp == 0 && a.enumerationOp == 0 && a.defaultOp == 0 && a.captureOp == 0 && a.pickerOp == 0 && a.artifactCleanup == nil
	unsubscribed := a.unsubscribed
	a.mu.Unlock()
	if !quitting || !empty || a.captureOwners.current() != nil || a.captureOwners.orphanCount() != 0 {
		return false
	}
	if !unsubscribed {
		hr, admitted := runAbruptOperation(&a.shutdown, a.helper.PermissionUnsubscribe)
		if !admitted || a.shutdown.isClosing() {
			return false
		}
		if hr.Failed() {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultBlocked, Action: "access_changed_unsubscribe", SelectedAPIPath: "AppCapability.AccessChanged(revoke-token)", HResult: hr.Hex(), FailureCause: "permission subscription is still owned; cleanup remains blocked"})
			return false
		}
		if !a.shutdown.runOperation(func() {
			a.mu.Lock()
			a.unsubscribed = true
			a.mu.Unlock()
		}) {
			return false
		}
		if !a.advanceLifecycle(lifecycleQuit, lifecyclePermissionUnsubscribed, winprobe.ResultPass, "CapPermissionUnsubscribe", hr, "", nil) {
			return false
		}
	}
	quiescent, admitted := runAbruptOperation(&a.shutdown, a.helper.IsQuiescent)
	if !admitted || a.shutdown.isClosing() || quiescent != 0 {
		return false
	}
	return a.postTransition(wmAppCleanupReady, 0, 0, "cleanup_ready")
}

func (a *probeApp) requestRecord(mode recordMode) bool {
	if a.evidence == nil || !a.evidence.healthy() {
		a.requestGracefulQuit("record suppressed because required evidence is unhealthy")
		return false
	}
	if a.captureOwners.current() != nil || a.captureOwners.orphanCount() != 0 {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultBlocked, Action: "record", FailureCause: "native capture ownership is still awaiting exact terminal release"})
		return false
	}
	a.mu.Lock()
	if a.captureOp != 0 || a.permissionOp != 0 || a.artifactCleanup != nil {
		a.mu.Unlock()
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultBlocked, Action: "record", FailureCause: "capture, permission request, or artifact cleanup is already active"})
		return false
	}
	inputMode := winprobe.InputDefault
	if mode == recordSelected {
		inputMode = winprobe.InputSelected
	}
	device, resolveErr := winprobe.ResolveInput(inputMode, a.defaultDevice, a.devices, a.selected)
	deviceID, deviceName := device.ID, device.Name
	a.mu.Unlock()
	if resolveErr != nil {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultBlocked, Action: "record", FailureCause: resolveErr.Error()})
		return false
	}
	generation, accepted, blockedReason := a.lifecycle.beginCaptureGeneration()
	if !accepted {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultBlocked, Action: "record", FailureCause: blockedReason})
		return false
	}
	a.mu.Lock()
	a.pendingMode, a.pendingDevice = mode, deviceID
	a.mu.Unlock()
	if !runAfterRequiredEvidence(func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultAttempt, Action: "explicit_record_permission_check_intent", SelectedAPIPath: "waiter-owned-CapPermissionCheck", DeviceID: deviceID, DeviceName: deviceName, Fields: map[string]any{"captureGeneration": generation}})
	}, nil) {
		a.lifecycle.cancelCaptureGeneration(generation)
		a.requestGracefulQuit("required record intent evidence write failed")
		return false
	}
	if !a.enqueue(waiterCommand{kind: "permission_check", generation: generation}) {
		a.lifecycle.cancelCaptureGeneration(generation)
		return false
	}
	return true
}

func (a *probeApp) handlePermissionChecked(generation uint64, status winprobe.PermissionStatus) {
	if !a.lifecycle.permissionContinuationAllowed(generation) {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultDiscard, Action: "stale_permission_continuation_suppressed", SelectedAPIPath: "generation+lifecycle-gate", PermissionStatus: status.String(), Fields: map[string]any{"captureGeneration": generation}})
		a.settleOrCancelCaptureGeneration(generation, "stale_waiter_permission_result")
		return
	}
	a.mu.Lock()
	deviceID := a.pendingDevice
	a.mu.Unlock()
	if status == winprobe.PermissionAllowed || status == winprobe.PermissionUnavailable || status == winprobe.PermissionPromptRequired {
		a.lifecycle.setPermissionAllowed(true)
	}
	permissionResult := winprobe.ResultAttempt
	if status == winprobe.PermissionAllowed {
		permissionResult = winprobe.ResultPass
	} else if status != winprobe.PermissionPromptRequired {
		permissionResult = winprobe.ResultBlocked
	}
	permissionEvent := winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: permissionResult, Action: "explicit_record_permission_check", SelectedAPIPath: "CapPermissionCheck(waiter-owned)", DeviceID: deviceID, PermissionStatus: status.String(), Fields: map[string]any{"captureGeneration": generation}}
	if status == winprobe.PermissionUnavailable {
		permissionEvent.SelectedAPIPath = "ActivateAudioInterfaceAsync-consent-fallback"
		permissionEvent.FailureCause = "AppCapability unavailable; artifact promotion remains blocked until deterministic WASAPI revoke evidence exists"
	}
	if !a.log(permissionEvent) {
		a.settleOrCancelCaptureGeneration(generation, "permission_check_evidence_failed_on_ui")
		a.requestGracefulQuit("required checked-permission evidence write failed")
		return
	}
	if status == winprobe.PermissionUnknown {
		a.lifecycle.cancelCaptureGeneration(generation)
		return
	}
	if status == winprobe.PermissionPromptRequired {
		if !runAfterRequiredEvidence(func() bool {
			return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultAttempt, Action: "permission_request_intent", SelectedAPIPath: "AppCapability.RequestAccessAsync", Fields: map[string]any{"captureGeneration": generation}})
		}, nil) {
			a.settleOrCancelCaptureGeneration(generation, "permission_request_intent_evidence_failed")
			a.requestGracefulQuit("required permission-request intent evidence write failed")
			return
		}
		var requestHR winprobe.HResult
		id, succeeded, invoked := a.lifecycle.runPermissionRequest(generation, func() (uint32, bool) {
			type requestResult struct {
				id uint32
				hr winprobe.HResult
			}
			request, admitted := runAbruptOperation(&a.shutdown, func() requestResult {
				id, hr := a.helper.PermissionRequest(a.permissionEvent)
				return requestResult{id: id, hr: hr}
			})
			if !admitted {
				return 0, false
			}
			requestHR = request.hr
			return request.id, requestHR.Succeeded()
		})
		if !invoked || a.shutdown.isClosing() {
			a.settleOrCancelCaptureGeneration(generation, "permission_request_suppressed_by_lifecycle_gate")
			return
		}
		if succeeded {
			if !a.shutdown.runOperation(func() {
				a.mu.Lock()
				a.permissionOp = id
				a.permissionGeneration = generation
				a.permissionCancelSent = false
				a.mu.Unlock()
			}) {
				return
			}
		}
		if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: resultForHR(requestHR), Action: "permission_request", SelectedAPIPath: "AppCapability.RequestAccessAsync", HResult: requestHR.Hex(), Fields: map[string]any{"captureGeneration": generation}}) {
			a.requestGracefulQuit("required permission-request evidence write failed")
			if !succeeded {
				a.lifecycle.cancelCaptureGeneration(generation)
			}
			return
		}
		if !succeeded {
			a.lifecycle.cancelCaptureGeneration(generation)
		}
		return
	}
	if status != winprobe.PermissionAllowed && status != winprobe.PermissionUnavailable {
		a.lifecycle.setPermissionAllowed(false)
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultBlocked, Action: "record_denied", PermissionStatus: status.String(), FailureCause: "microphone permission is not allowed"})
		a.lifecycle.cancelCaptureGeneration(generation)
		return
	}
	a.prepareCapture(generation)
}

func (a *probeApp) prepareCapture(generation uint64) bool {
	if a.shutdown.isClosing() {
		return false
	}
	a.mu.Lock()
	deviceID := a.pendingDevice
	mode := a.pendingMode
	a.mu.Unlock()
	path := "default"
	if mode == recordSelected {
		path = "selected"
	}
	if !runAfterRequiredEvidence(func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultAttempt, Action: "capture_prepare_intent", SelectedAPIPath: "CapturePrepare(" + path + ")", DeviceID: deviceID, Fields: map[string]any{"captureGeneration": generation}})
	}, nil) {
		if a.shutdown.isClosing() {
			return false
		}
		a.settleOrCancelCaptureGeneration(generation, "capture_prepare_intent_evidence_failed")
		a.requestGracefulQuit("required capture-prepare intent evidence write failed")
		return false
	}
	var hr winprobe.HResult
	prepared := runCapturePrepareOwned(a.lifecycle, &a.captureOwners, &a.shutdown, generation, func() (uint32, bool) {
		type prepareResult struct {
			id uint32
			hr winprobe.HResult
		}
		prepared, admitted := runAbruptOperation(&a.shutdown, func() prepareResult {
			id, result := a.helper.CapturePrepare(a.captureEvent)
			return prepareResult{id: id, hr: result}
		})
		if !admitted {
			return 0, false
		}
		hr = prepared.hr
		return prepared.id, hr == 0
	}, func(operationID uint32) winprobe.HResult {
		return a.helper.CaptureStop(operationID, winprobe.ReasonShutdown)
	}, func(operationID uint32) winprobe.HResult {
		return a.helper.CaptureStop(operationID, winprobe.ReasonCancel)
	})
	id := prepared.operationID
	if !prepared.trackerInvoked || !prepared.externalInvoked {
		prepared.handleSuppressedPrepare(a.lifecycle, &a.captureOwners, &a.shutdown, generation, func() {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultDiscard, Action: "stale_capture_prepare_suppressed", SelectedAPIPath: "generation+lifecycle-gate", DeviceID: deviceID, Fields: map[string]any{"captureGeneration": generation}})
		}, func() {
			a.settleOrCancelCaptureGeneration(generation, "stale_permission_ready_before_prepare")
		})
		return false
	}
	if prepared.invalidSuccessfulID {
		prepared.handleInvalidSuccessfulResult(&a.shutdown, func() bool {
			return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_prepare_contract_failure", SelectedAPIPath: "CapturePrepare+ABI-result-validation", HResult: hr.Hex(), FailureCause: "CapturePrepare returned a non-contractual success result; no zero-ID owner was published and lifecycle ownership was retained or settled fail-closed", Fields: map[string]any{"captureGeneration": generation, "operationIdNonzero": id != 0, "expectedHResult": "0x00000000"}})
		}, func() {
			a.requestGracefulQuit("capture prepare ABI contract failure")
		})
		return false
	}
	if hr.Succeeded() && hr != 0 {
		prepared.handleUnexpectedSuccessfulHResult(&a.shutdown, func() bool {
			return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultFail, Action: "capture_prepare_contract_failure", SelectedAPIPath: "CapturePrepare+ABI-result-validation", HResult: hr.Hex(), FailureCause: "CapturePrepare returned an unexpected success code; no new native owner was published and lifecycle ownership was retained or settled fail-closed", Fields: map[string]any{"captureGeneration": generation, "operationIdNonzero": id != 0, "expectedHResult": "0x00000000"}})
		}, func() {
			a.requestGracefulQuit("capture prepare ABI contract failure")
		})
		return false
	}
	if prepared.succeeded && !prepared.ownerPublished {
		// A real successful loser owns a registry-backed native operation. Its
		// one-shot Stop was claimed at the helper-result seam; the waiter-owned
		// orphan obligation must observe terminal and Release before this
		// generation can be settled. Never fabricate no-native settlement here.
		return false
	}
	completion := prepared.dispatchPostHelper(&a.shutdown, func() {
		a.mu.Lock()
		a.captureOp = id
		a.captureGeneration = generation
		a.activatePosted = false
		a.capturePrepareAt = time.Now()
		a.captureTimeoutSent = false
		a.permissionStopSent = false
		a.captureFinalized = false
		a.mu.Unlock()
	}, func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: resultForHR(hr), Action: "capture_prepare", SelectedAPIPath: "CapturePrepare(" + path + ")", DeviceID: deviceID, HResult: hr.Hex(), Fields: map[string]any{"captureGeneration": generation}})
	}, func() {
		if a.shutdown.isClosing() {
			return
		}
		if prepared.succeeded && prepared.owner != nil {
			_, _ = runAbruptOperation(&a.shutdown, func() bool {
				return prepared.owner.requestStop(func(operationID uint32) winprobe.HResult {
					return a.helper.CaptureStop(operationID, winprobe.ReasonCancel)
				})
			})
		}
		if !a.shutdown.isClosing() {
			a.requestGracefulQuit("required capture-prepare evidence write failed")
		}
	})
	return prepared.succeeded && completion.ownerStatePublished && completion.evidencePassed && !a.shutdown.isClosing()
}

func (a *probeApp) activateCapture(generation uint64, id uint32) {
	if a.shutdown.isClosing() {
		if owner := a.captureOwners.matching(generation, id); owner != nil {
			owner.requestShutdownStop(func(operationID uint32) winprobe.HResult {
				return a.helper.CaptureStop(operationID, winprobe.ReasonShutdown)
			})
		}
		return
	}
	a.mu.Lock()
	if a.captureOp != id || a.captureGeneration != generation {
		a.mu.Unlock()
		return
	}
	deviceID := a.pendingDevice
	a.mu.Unlock()
	owner := admitCaptureActivationOwned(&a.captureOwners, &a.shutdown, generation, id, func(operationID uint32) winprobe.HResult {
		return a.helper.CaptureStop(operationID, winprobe.ReasonShutdown)
	})
	if owner == nil {
		return
	}
	if !runAfterRequiredEvidence(func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultAttempt, Action: "capture_activate_intent", SelectedAPIPath: "ActivateAudioInterfaceAsync", DeviceID: deviceID, Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}})
	}, nil) {
		if a.shutdown.isClosing() {
			return
		}
		_, _ = runAbruptOperation(&a.shutdown, func() bool {
			return owner.requestStop(func(operationID uint32) winprobe.HResult {
				return a.helper.CaptureStop(operationID, winprobe.ReasonCancel)
			})
		})
		a.requestGracefulQuit("required capture-activate intent evidence write failed")
		return
	}
	var hr winprobe.HResult
	activated := runCaptureActivationAdmitted(a.lifecycle, &a.captureOwners, &a.shutdown, owner, func() {}, func() {
		hr = a.helper.CaptureActivate(id, deviceID)
	}, func(operationID uint32) winprobe.HResult {
		return a.helper.CaptureStop(operationID, winprobe.ReasonShutdown)
	})
	if !activated.trackerInvoked || !activated.externalInvoked {
		runCaptureContinuation(&a.shutdown, true, func() {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultDiscard, Action: "stale_capture_activation_suppressed", SelectedAPIPath: "generation+operation+lifecycle-gate", DeviceID: deviceID, Fields: map[string]any{"captureGeneration": generation, "captureOperationId": id}})
		})
		return
	}
	evidencePassed := false
	if !activated.dispatchContinuation(&a.shutdown, func() {
		evidencePassed = a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: resultForHR(hr), Action: "capture_activate", SelectedAPIPath: "ActivateAudioInterfaceAsync", DeviceID: deviceID, HResult: hr.Hex()})
	}) {
		return
	}
	if !evidencePassed {
		_, _ = runAbruptOperation(&a.shutdown, func() bool {
			return owner.requestStop(func(operationID uint32) winprobe.HResult {
				return a.helper.CaptureStop(operationID, winprobe.ReasonCancel)
			})
		})
		a.requestGracefulQuit("required capture-activate evidence write failed")
		return
	}
	if hr.Failed() {
		a.enqueue(waiterCommand{kind: "stop", reason: winprobe.ReasonCancel})
	}
}

func (a *probeApp) requestPicker() {
	if !a.lifecycle.workAllowed() {
		return
	}
	if !runAfterRequiredEvidence(func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: winprobe.ResultAttempt, Action: "picker_open_intent", SelectedAPIPath: "FileOpenPicker(visible-main-HWND)"})
	}, nil) {
		a.requestGracefulQuit("required picker intent evidence write failed")
		return
	}
	hidden := false
	allowed := false
	started := a.shutdown.runOperation(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.pickerOp != 0 || a.quitting {
			return
		}
		hidden = !isWindowVisible(a.main)
		a.pickerWindow.begin(hidden)
		allowed = true
	})
	if !started || !allowed {
		return
	}
	if hidden {
		shown, admitted := runAbruptOperation(&a.shutdown, a.showMainWindow)
		if !admitted || !shown {
			return
		}
	}
	visible := isWindowVisible(a.main)
	if !visible {
		a.shutdown.runOperation(func() {
			a.mu.Lock()
			_ = a.pickerWindow.initiated(false)
			a.mu.Unlock()
		})
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: winprobe.ResultBlocked, Action: "picker_owner_restore", SelectedAPIPath: "ShowWindow(visible-main-HWND)", WindowVisible: &visible, FailureCause: "visible picker owner could not be restored"})
		return
	}
	foreground, foregroundAdmitted := runAbruptOperation(&a.shutdown, func() bool { return setForegroundWindow(a.main) })
	if !foregroundAdmitted {
		return
	}
	if !foreground {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: winprobe.ResultBlocked, Action: "picker_owner_foreground", SelectedAPIPath: "SetForegroundWindow(visible-main-HWND)", WindowVisible: &visible, FailureCause: "Windows rejected foreground activation; picker API result remains authoritative"})
	}
	var id uint32
	var hr winprobe.HResult
	restoreHidden := false
	invoked := a.lifecycle.runGatedWork(func() {
		type pickerOpenResult struct {
			id uint32
			hr winprobe.HResult
		}
		opened, admitted := runAbruptOperation(&a.shutdown, func() pickerOpenResult {
			id, hr := a.helper.PickerOpen(a.main, a.pickerEvent)
			return pickerOpenResult{id: id, hr: hr}
		})
		if !admitted {
			return
		}
		id, hr = opened.id, opened.hr
		a.shutdown.runOperation(func() {
			a.mu.Lock()
			if hr.Succeeded() {
				a.pickerOp = id
				a.pickerProcessed = false
			}
			restoreHidden = a.pickerWindow.initiated(hr.Succeeded())
			a.mu.Unlock()
		})
	})
	if a.shutdown.isClosing() {
		return
	}
	if !invoked {
		if !a.shutdown.runOperation(func() {
			a.mu.Lock()
			restoreHidden = a.pickerWindow.initiated(false)
			a.mu.Unlock()
		}) {
			return
		}
		if restoreHidden {
			a.shutdown.runOperation(func() { a.hideMainWindow("picker_lifecycle_gate_restore") })
		}
		return
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPicker, Result: resultForHR(hr), Action: "picker_open", SelectedAPIPath: "FileOpenPicker(visible-main-HWND)", HResult: hr.Hex(), WindowVisible: &visible})
	if restoreHidden {
		a.shutdown.runOperation(func() { a.hideMainWindow("picker_open_failure_restore") })
	}
}

func (a *probeApp) queueArtifactCleanupRetry(writer *winprobe.ArtifactWriter, captureOperationID uint32, captureGeneration uint64, captureReleased bool, fields map[string]any) {
	if writer == nil {
		return
	}
	published := false
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if pending := a.artifactCleanup; pending != nil {
			if pending.captureOperationID == captureOperationID {
				pending.captureReleased = pending.captureReleased || captureReleased
			}
			return
		}
		a.artifactCleanup = &artifactCleanupRetry{writer: writer, captureOperationID: captureOperationID, captureGeneration: captureGeneration, captureReleased: captureReleased, fields: fields}
		published = true
	}) {
		return
	}
	if published {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultBlocked, Action: "temporary_artifact_cleanup_pending", SelectedAPIPath: "ArtifactWriter.Abort(retryable-owned-path-cleanup)", FailureCause: "one or more owned temporary artifact paths could not yet be verified absent; lifecycle cleanup remains blocked", Fields: fields})
	}
}

func (a *probeApp) markPendingArtifactCaptureReleased(captureOperationID uint32) bool {
	marked := false
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.artifactCleanup == nil || a.artifactCleanup.captureOperationID != captureOperationID {
			return
		}
		a.artifactCleanup.captureReleased = true
		marked = true
	}) {
		return false
	}
	return marked
}

func (a *probeApp) drainPendingArtifactCleanup() {
	a.mu.Lock()
	pending := a.artifactCleanup
	a.mu.Unlock()
	if pending == nil {
		return
	}
	err, abortAdmitted := runAbruptOperation(&a.shutdown, pending.writer.Abort)
	if !abortAdmitted || a.shutdown.isClosing() {
		return
	}
	if err != nil {
		fields := make(map[string]any, len(pending.fields)+1)
		for key, value := range pending.fields {
			fields[key] = value
		}
		fields["cleanupError"] = err.Error()
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioCapture, Result: winprobe.ResultBlocked, Action: "temporary_artifact_cleanup_retry", SelectedAPIPath: "ArtifactWriter.Abort+owned-path-postcondition", FailureCause: "owned temporary artifacts are not yet verified absent", Fields: fields})
		return
	}
	cleared := false
	if !a.shutdown.runOperation(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.artifactCleanup != pending {
			return
		}
		a.artifactCleanup = nil
		cleared = true
	}) {
		return
	}
	if !cleared {
		return
	}
	captureReleased := pending.captureReleased
	a.advanceCaptureLifecycles(pending.captureGeneration, lifecycleArtifactDisposed, winprobe.ResultPass, "ArtifactWriter.Abort(retry-postcondition-satisfied)", 0, "", pending.fields)
	if captureReleased {
		a.advanceCaptureLifecycles(pending.captureGeneration, lifecycleCaptureReleased, winprobe.ResultPass, "CaptureRelease(already-observed-before-artifact-retry)", 0, "", map[string]any{"captureOperationId": pending.captureOperationID})
		a.postIdleLifecycleCleanup()
	}
}

func (a *probeApp) logLifecycleObservation(progress lifecycleProgress, signal string) bool {
	return a.logLifecycleObservationWithMode(progress, signal, false)
}

func (a *probeApp) logLifecycleObservationWithMode(progress lifecycleProgress, signal string, nonblocking bool) bool {
	fields := a.lifecycleFields(progress, nil)
	fields["repeatedSignal"] = progress.RepeatedSignal
	fields["repeatedSignalCount"] = progress.RepeatedSignalCount
	event := winprobe.LogEvent{
		Scenario:        winprobe.ScenarioWindow,
		Result:          winprobe.ResultAttempt,
		Action:          "lifecycle_" + lifecycleSignalObserved.String(),
		SelectedAPIPath: signal,
		Fields:          fields,
	}
	if nonblocking {
		return a.logNonblocking(event)
	}
	return a.log(event)
}

func (a *probeApp) lifecycleFields(progress lifecycleProgress, extra map[string]any) map[string]any {
	fields := map[string]any{
		"cleanupId":          progress.ID,
		"cleanupOrder":       int(progress.Stage),
		"cleanupStage":       progress.Stage.String(),
		"lifecycleEdge":      string(progress.Edge),
		"lifecycleMode":      progress.Mode.String(),
		"stopReason":         progress.Reason.String(),
		"captureExpected":    progress.CaptureExpected,
		"captureGeneration":  progress.CaptureGeneration,
		"captureOperationId": progress.CaptureOperationID,
		"observedOSSignal":   progress.Signal,
		"observedOSSignals":  append([]string(nil), progress.Signals...),
	}
	for key, value := range extra {
		fields[key] = value
	}
	return fields
}

func (a *probeApp) advanceLifecycle(edge lifecycleEdge, stage lifecycleStage, result winprobe.ProbeResult, apiPath string, hr winprobe.HResult, failure string, extra map[string]any) bool {
	return a.advanceLifecycleWithModeHResult(edge, stage, result, apiPath, hr.Hex(), failure, extra, false)
}

func (a *probeApp) advanceLifecycleWithMode(edge lifecycleEdge, stage lifecycleStage, result winprobe.ProbeResult, apiPath string, hr winprobe.HResult, failure string, extra map[string]any, nonblocking bool) bool {
	return a.advanceLifecycleWithModeHResult(edge, stage, result, apiPath, hr.Hex(), failure, extra, nonblocking)
}

func (a *probeApp) advanceLifecycleWithModeHResult(edge lifecycleEdge, stage lifecycleStage, result winprobe.ProbeResult, apiPath, hresult, failure string, extra map[string]any, nonblocking bool) bool {
	type lifecycleAdvanceResult struct {
		progress lifecycleProgress
		changed  bool
		err      error
	}
	advance, admitted := runAbruptOperation(&a.shutdown, func() lifecycleAdvanceResult {
		progress, changed, err := a.lifecycle.advance(edge, stage)
		return lifecycleAdvanceResult{progress: progress, changed: changed, err: err}
	})
	if !admitted {
		return false
	}
	progress, changed, err := advance.progress, advance.changed, advance.err
	if err != nil {
		event := winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "lifecycle_cleanup_order_rejected", SelectedAPIPath: apiPath, HResult: hresult, FailureCause: err.Error(), Fields: map[string]any{"lifecycleEdge": string(edge), "requestedStage": stage.String(), "requestedOrder": int(stage)}}
		if nonblocking {
			a.logNonblocking(event)
		} else {
			a.log(event)
		}
		return false
	}
	if !changed {
		return true
	}
	event := winprobe.LogEvent{
		Scenario:        winprobe.ScenarioWindow,
		Result:          result,
		Action:          "lifecycle_" + stage.String(),
		SelectedAPIPath: apiPath,
		HResult:         hresult,
		FailureCause:    failure,
		Fields:          a.lifecycleFields(progress, extra),
	}
	written := false
	if nonblocking {
		written = a.logNonblocking(event)
	} else {
		written = a.log(event)
	}
	if !written {
		if progress.Mode != lifecycleAbruptOSExit && (edge != lifecycleQuit || a.exit.load() == processExitRunning) {
			a.requestGracefulQuit("required lifecycle evidence write failed")
		}
		return false
	}
	return true
}

func (a *probeApp) advanceCaptureLifecycles(generation uint64, stage lifecycleStage, result winprobe.ProbeResult, apiPath string, hr winprobe.HResult, failure string, extra map[string]any) {
	advances, admitted := runAbruptOperation(&a.shutdown, func() []lifecycleAdvance {
		return a.lifecycle.advanceCaptureGeneration(generation, stage)
	})
	if !admitted {
		return
	}
	a.logCaptureLifecycleAdvances(advances, generation, result, apiPath, hr, failure, extra)
}

func (a *probeApp) logCaptureLifecycleAdvances(advances []lifecycleAdvance, generation uint64, result winprobe.ProbeResult, apiPath string, hr winprobe.HResult, failure string, extra map[string]any) {
	a.logCaptureLifecycleAdvancesWithMode(advances, generation, result, apiPath, hr, failure, extra, false)
}

func (a *probeApp) logCaptureLifecycleAdvancesWithMode(advances []lifecycleAdvance, generation uint64, result winprobe.ProbeResult, apiPath string, hr winprobe.HResult, failure string, extra map[string]any, nonblocking bool) {
	evidenceFailed := false
	abrupt := false
	for _, advance := range advances {
		abrupt = abrupt || advance.Progress.Mode == lifecycleAbruptOSExit
		if advance.Err != nil {
			event := winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "lifecycle_cleanup_order_rejected", SelectedAPIPath: apiPath, HResult: hr.Hex(), FailureCause: advance.Err.Error(), Fields: map[string]any{"lifecycleEdge": string(advance.Progress.Edge), "captureGeneration": generation, "requestedStage": advance.Stage.String(), "requestedOrder": int(advance.Stage)}}
			if nonblocking {
				a.logNonblocking(event)
			} else {
				a.log(event)
			}
			continue
		}
		if !advance.Changed {
			continue
		}
		event := winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: result, Action: "lifecycle_" + advance.Stage.String(), SelectedAPIPath: apiPath, HResult: hr.Hex(), FailureCause: failure, Fields: a.lifecycleFields(advance.Progress, extra)}
		written := false
		if nonblocking {
			written = a.logNonblocking(event)
		} else {
			written = a.log(event)
		}
		if !written {
			evidenceFailed = true
		}
	}
	if evidenceFailed && !abrupt {
		a.requestGracefulQuit("required capture lifecycle evidence write failed")
	}
}

func (a *probeApp) ensureTerminalLifecycle(result winprobe.CaptureResult, generation uint64) {
	var edge lifecycleEdge
	switch result.Reason {
	case winprobe.ReasonPermissionRevoke:
		edge = lifecyclePermissionRevoke
	case winprobe.ReasonShutdown:
		edge = lifecycleSystemShutdown
	case winprobe.ReasonSuspend:
		edge = lifecycleSuspend
	case winprobe.ReasonLock:
		edge = lifecycleSessionLock
	default:
		return
	}
	if _, ok := a.lifecycle.activeRun(edge); ok {
		return
	}
	mode := lifecycleReturnsIdle
	if edge == lifecycleSystemShutdown {
		mode = lifecycleAbruptOSExit
	}
	plan, admitted := runAbruptOperation(&a.shutdown, func() lifecycleStopPlan {
		return a.lifecycle.beginLifecycle(edge, "CaptureGetResult/native-terminal-reason", result.Reason, mode)
	})
	if !admitted {
		return
	}
	evidenceWritten := a.logLifecycleObservation(plan.Progress, plan.Progress.Signal)
	a.advanceLifecycle(edge, lifecycleStopRequested, winprobe.ResultPass, "native-capture-terminal-detection", result.Outcome, "", map[string]any{"stopAlreadySealedByNativeCapture": true})
	if !evidenceWritten && mode != lifecycleAbruptOSExit {
		a.requestGracefulQuit("lifecycle signal evidence write failed")
	}
}

func (a *probeApp) requestLifecycleStop(edge lifecycleEdge, signal string, reason winprobe.CaptureReason, mode lifecycleMode) {
	stop := captureStopOutcome{State: captureStopCompleted}
	type lifecycleStopResult struct {
		plan lifecycleStopPlan
		hr   winprobe.HResult
	}
	started, admitted := runAbruptOperation(&a.shutdown, func() lifecycleStopResult {
		plan, stopHR := a.lifecycle.beginLifecycleStop(edge, signal, reason, mode, func(generation uint64, capture uint32) winprobe.HResult {
			stop = a.requestOwnedCaptureStop(generation, capture, reason)
			return stop.Result
		})
		return lifecycleStopResult{plan: plan, hr: stopHR}
	})
	if !admitted {
		return
	}
	plan, stopHR := started.plan, started.hr
	if plan.Capture == 0 {
		stop.Result = stopHR
	}
	nonblockingEvidence := edge == lifecycleSystemShutdown || edge == lifecycleSuspend || edge == lifecycleSessionLock
	evidenceWritten := a.logLifecycleObservationWithMode(plan.Progress, signal, nonblockingEvidence)
	stopResult, stopPath, stopHResult := captureStopEvidence(stop, "CaptureRequestStop(nonblocking)")
	a.advanceLifecycleWithModeHResult(edge, lifecycleStopRequested, stopResult, stopPath, stopHResult, "", map[string]any{"captureActive": plan.Capture != 0, "captureOperationId": plan.Capture, "captureGeneration": plan.Generation, "captureStopState": stop.State}, nonblockingEvidence)
	a.replayCaptureSettlement(plan.Generation, "lifecycle_registration_replay", nonblockingEvidence)
	if mode == lifecycleReturnsIdle && (plan.Generation == 0 || a.lifecycle.phaseForGeneration(plan.Generation) == 0) {
		a.postIdleLifecycleCleanup()
	}
	if !evidenceWritten && mode != lifecycleAbruptOSExit {
		a.requestGracefulQuit("lifecycle signal evidence write failed")
	}
}

func (a *probeApp) replayCaptureSettlement(generation uint64, source string, nonblocking ...bool) {
	if generation == 0 {
		return
	}
	fields := map[string]any{"captureGeneration": generation, "replayedAfterLifecycleRegistration": true, "replaySource": source}
	async := len(nonblocking) != 0 && nonblocking[0]
	advances, admitted := runAbruptOperation(&a.shutdown, func() []lifecycleAdvance {
		return a.lifecycle.replayCaptureGeneration(generation)
	})
	if !admitted {
		return
	}
	a.logCaptureLifecycleAdvancesWithMode(advances, generation, winprobe.ResultPass, "generation-settlement-ledger-replay", 0, "", fields, async)
}

func (a *probeApp) settleOrCancelCaptureGeneration(generation uint64, source string) {
	if generation == 0 {
		return
	}
	for _, run := range a.lifecycle.captureRuns() {
		if run.CaptureGeneration == generation {
			a.settleSuppressedCaptureGeneration(generation, source)
			return
		}
	}
	a.shutdown.runOperation(func() { a.lifecycle.cancelCaptureGeneration(generation) })
}

func (a *probeApp) settleSuppressedCaptureGeneration(generation uint64, source string) {
	fields := map[string]any{"captureGeneration": generation, "nativeCaptureStarted": false, "temporaryArtifactsOwned": false, "suppressionSource": source}
	a.advanceCaptureLifecycles(generation, lifecycleCaptureTerminal, winprobe.ResultPass, "generation-gate/stale-continuation-suppressed", 0, "", fields)
	a.advanceCaptureLifecycles(generation, lifecycleArtifactDisposed, winprobe.ResultPass, "no-owned-temporary-artifact", 0, "", fields)
	a.advanceCaptureLifecycles(generation, lifecycleCaptureReleased, winprobe.ResultPass, "no-native-capture-operation-owned", 0, "", fields)
	a.postIdleLifecycleCleanup()
}

func (a *probeApp) postIdleLifecycleCleanup() {
	if len(a.lifecycle.idleCleanupRuns()) == 0 {
		return
	}
	if !a.shutdown.runOperation(func() {
		a.uiTransitions.publish(uiTransitionIdleCleanup, 0, winprobe.PermissionUnknown)
	}) {
		return
	}
	_, _ = runAbruptOperation(&a.shutdown, func() error { return windows.SetEvent(a.commandEvent) })
}

func (a *probeApp) publishRearmTransition(generation uint64, status winprobe.PermissionStatus) {
	if generation == 0 {
		return
	}
	if !a.shutdown.runOperation(func() {
		a.uiTransitions.publish(uiTransitionLifecycleRearm, generation, status)
	}) {
		return
	}
	_, _ = runAbruptOperation(&a.shutdown, func() error { return windows.SetEvent(a.commandEvent) })
}

func (a *probeApp) driveUITransitions() {
	escalate := a.uiTransitions.drive(func(transition uiTransition) bool {
		switch transition.Kind {
		case uiTransitionIdleCleanup:
			return a.postTransition(wmAppLifecycleIdleCleanup, uintptr(transition.ID), 0, "lifecycle_idle_cleanup")
		case uiTransitionLifecycleRearm:
			return a.postTransition(wmAppLifecycleRearm, uintptr(transition.ID), 0, "permission_rearm")
		default:
			return false
		}
	}, a.shutdown.runOperation)
	if escalate {
		a.requestGracefulQuit("durable lifecycle UI transition exceeded bounded PostMessage retries")
	}
}

func (a *probeApp) completeIdleLifecycleCleanups() {
	runs := a.lifecycle.idleCleanupRuns()
	if len(runs) == 0 {
		return
	}
	type unregisterResult struct {
		unregistered bool
		apiCalled    bool
		lastError    uint32
	}
	result, admitted := runAbruptOperation(&a.shutdown, func() unregisterResult {
		unregistered, apiCalled, lastError := a.unregisterHotkey("lifecycle_idle_cleanup")
		return unregisterResult{unregistered: unregistered, apiCalled: apiCalled, lastError: lastError}
	})
	if !admitted || a.shutdown.isClosing() {
		return
	}
	unregistered, apiCalled, lastError := result.unregistered, result.apiCalled, result.lastError
	if !unregistered {
		for _, run := range runs {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultBlocked, Action: "lifecycle_hotkey_unregister_retry_required", SelectedAPIPath: "UnregisterHotKey", FailureCause: "hotkey registration is still owned; idle cleanup cannot be asserted", Fields: a.lifecycleFields(run, map[string]any{"getLastError": fmt.Sprintf("0x%08x", lastError), "nextAction": "retry UnregisterHotKey on the UI thread and keep the signed hardware scenario blocked until it succeeds"})})
		}
		timer, timerAdmitted := runAbruptOperation(&a.shutdown, func() uintptr {
			timer, _, _ := pSetTimer.Call(uintptr(a.hidden), lifecycleRetryTimer, 100, 0)
			return timer
		})
		if !timerAdmitted {
			return
		}
		if decideRetryTimer(timer, false) == retryTimerGracefulExit {
			a.requestGracefulQuit("idle lifecycle hotkey retry timer unavailable")
		}
		return
	}
	allAdvanced := true
	for _, run := range runs {
		if !a.advanceLifecycle(run.Edge, lifecycleHotkeyUnregistered, winprobe.ResultPass, "UnregisterHotKey(hidden-top-level-HWND)", 0, "", map[string]any{"apiCalled": apiCalled}) {
			allAdvanced = false
			continue
		}
		if !a.advanceLifecycle(run.Edge, lifecycleIdle, winprobe.ResultPass, "idle-with-no-active-capture", 0, "", map[string]any{"temporaryArtifactsClosed": true, "hiddenCaptureSessionActive": false}) {
			allAdvanced = false
		}
	}
	if !allAdvanced {
		return
	}
	a.rearmAfterLifecycle("idle_cleanup_complete")
}

func (a *probeApp) observeLifecycleResume(edge lifecycleEdge, signal string) {
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "lifecycle_resume_observed", SelectedAPIPath: signal, Fields: map[string]any{"lifecycleEdge": string(edge), "observedOSSignal": signal, "autoRestartCapture": false}})
	a.rearmAfterLifecycle(signal)
}

func (a *probeApp) rearmAfterLifecycle(source string) {
	generation, accepted := a.lifecycle.beginRearm()
	if !accepted {
		return
	}
	if !a.enqueue(waiterCommand{kind: "permission_rearm", generation: generation}) {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultBlocked, Action: "lifecycle_rearm_permission_check_queue", SelectedAPIPath: "waiter-command", FailureCause: "permission rearm query could not be queued", Fields: map[string]any{"source": source, "rearmGeneration": generation}})
		a.requestGracefulQuit("permission rearm command queue saturated")
	}
}

func (a *probeApp) applyLifecycleRearm(generation uint64, status winprobe.PermissionStatus, source string) {
	allowed := status == winprobe.PermissionAllowed || status == winprobe.PermissionUnavailable
	if !runAfterRequiredEvidence(func() bool {
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultAttempt, Action: "lifecycle_rearm_apply_intent", SelectedAPIPath: "generation-bound-permission+discovery+hotkey", PermissionStatus: status.String(), Fields: map[string]any{"source": source, "rearmGeneration": generation}})
	}, nil) {
		a.requestGracefulQuit("required lifecycle-rearm intent evidence write failed")
		return
	}
	var discoveryEvents []winprobe.LogEvent
	accepted := a.lifecycle.runRearm(generation, allowed, func() bool {
		var discoveryStarted bool
		discoveryEvents, discoveryStarted = a.startDiscoveryOperations()
		if !discoveryStarted {
			return false
		}
		registered, admitted := runAbruptOperation(&a.shutdown, func() bool {
			return a.registerHotkey("lifecycle_rearm:" + source)
		})
		return admitted && registered
	})
	for _, event := range discoveryEvents {
		if !a.log(event) {
			a.requestGracefulQuit("required lifecycle-rearm discovery evidence write failed")
			return
		}
	}
	if !accepted {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission, Result: winprobe.ResultBlocked, Action: "lifecycle_rearm_permission_check", SelectedAPIPath: "CapPermissionCheck(waiter-owned)", PermissionStatus: status.String(), FailureCause: "permission is not available or the rearm continuation is stale; hotkey and new capture remain disabled", Fields: map[string]any{"source": source, "rearmGeneration": generation, "nextAction": "grant microphone permission, then wait for AccessChanged or restart the probe"}})
		if allowed && a.lifecycle.rearmPending(generation) {
			if pending, current := a.uiTransitions.pendingTransition(uiTransitionLifecycleRearm); !current || pending.Generation != generation {
				a.publishRearmTransition(generation, status)
			}
		}
		return
	}
}

func (a *probeApp) syncEvidenceBeforeExit() bool {
	if !a.evidence.healthy() {
		return false
	}
	if !a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "lifecycle_evidence_sync_begin", SelectedAPIPath: "scenarios.jsonl+File.Sync"}) || !a.evidence.sync() {
		return false
	}
	if !a.advanceLifecycle(lifecycleQuit, lifecycleEvidenceSynced, winprobe.ResultPass, "scenarios.jsonl+File.Sync", 0, "", nil) {
		return false
	}
	// Persist the evidence-synced record itself, not merely the records that
	// preceded it.
	if !a.evidence.sync() {
		return false
	}
	return true
}

func (a *probeApp) requestGracefulQuit(source string) {
	a.requestGracefulQuitAdmitted(source)
}

func (a *probeApp) recordRequiredStructuralFailure(event winprobe.LogEvent, quitReason string) requiredFailureContinuationOutcome {
	return runRequiredFailureContinuation(&a.shutdown, true, func() bool {
		return a.log(event)
	}, func() {
		a.requestGracefulQuit(quitReason)
	})
}

func (a *probeApp) requestGracefulQuitAdmitted(source string) {
	plan, admitted := runAbruptOperation(&a.shutdown, func() lifecycleStopPlan {
		return a.lifecycle.beginGracefulQuit(source)
	})
	if !admitted {
		return
	}
	began, deadlineAdmitted := runAbruptOperation(&a.shutdown, func() bool {
		return a.exit.beginGracefulWithDeadline(func(callback func()) {
			time.AfterFunc(30*time.Second, func() { a.shutdown.runOperation(callback) })
		}, func() { os.Exit(1) })
	})
	if !deadlineAdmitted {
		return
	}
	if began {
		if !a.shutdown.runOperation(func() { a.quittingAt.Store(time.Now().UnixMilli()) }) {
			return
		}
	}
	a.logLifecycleObservation(plan.Progress, source)
	if !began {
		_, _ = runAbruptOperation(&a.shutdown, func() error { return windows.SetEvent(a.commandEvent) })
		return
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "graceful_cleanup_watchdog_armed", SelectedAPIPath: "time.AfterFunc+processExitCoordinator", Fields: map[string]any{"hardDeadlineSeconds": 30, "watchdogRemainsArmedUntil": "WM_QUIT_commit", "hardExitPerformsNoBlockingLogOrSync": true}})
	wakeErr, wakeAdmitted := runAbruptOperation(&a.shutdown, func() error { return windows.SetEvent(a.commandEvent) })
	if wakeAdmitted && wakeErr != nil {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "terminal_intent_wake", SelectedAPIPath: "SetEvent(commandEvent)", FailureCause: wakeErr.Error(), Fields: map[string]any{"terminalIntentNonDroppable": true, "boundedWaiterPollMillis": 100}})
	}
}

func (a *probeApp) forceQuit() {
	a.commitForcedExit("explicit Force Quit")
}

func (a *probeApp) commitForcedExit(source string) {
	_ = source // The watchdog-arm evidence already records why this hard bound exists.
	a.exit.force(func() { os.Exit(1) })
}

func (a *probeApp) enqueue(command waiterCommand) bool {
	queued, admitted := runAbruptOperation(&a.shutdown, func() bool {
		select {
		case a.commands <- command:
			return true
		default:
			return false
		}
	})
	if !admitted {
		return false
	}
	if !queued {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "command_queue", FailureCause: "command queue full"})
		return false
	}
	wakeErr, admitted := runAbruptOperation(&a.shutdown, func() error { return windows.SetEvent(a.commandEvent) })
	if admitted && wakeErr != nil {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "command_event", SelectedAPIPath: "SetEvent", FailureCause: wakeErr.Error()})
		// The waiter polls every 100ms, so a queued command remains deliverable.
		return true
	}
	return true
}

func (a *probeApp) log(event winprobe.LogEvent) bool {
	event = normalizeLogEvent(event)
	written, admitted := runAbruptOperation(&a.shutdown, func() bool {
		return a.evidence != nil && a.evidence.log(event)
	})
	if !admitted {
		return false
	}
	if !written && !a.evidenceFailureHandled.Load() {
		a.shutdown.runOperation(func() { a.evidenceFailurePending.Store(true) })
	}
	return written
}

func (a *probeApp) logNonblocking(event winprobe.LogEvent) bool {
	event = normalizeLogEvent(event)
	written, admitted := runAbruptOperation(&a.shutdown, func() bool {
		return a.evidence != nil && a.evidence.logAsync(event)
	})
	if !admitted {
		return false
	}
	if !written && !a.evidenceFailureHandled.Load() {
		a.shutdown.runOperation(func() { a.evidenceFailurePending.Store(true) })
	}
	return written
}

func normalizeLogEvent(event winprobe.LogEvent) winprobe.LogEvent {
	if (event.Result == winprobe.ResultFail || event.Result == winprobe.ResultBlocked) && event.FailureCause == "" {
		if event.HResult != "" {
			event.FailureCause = "selected API path returned " + event.HResult
		} else {
			event.FailureCause = "scenario did not complete successfully"
		}
	}
	return event
}

func (a *probeApp) drainEvidenceFailure() {
	handle, admitted := runAbruptOperation(&a.shutdown, func() bool {
		failed := a.evidenceFailurePending.Swap(false) || (a.evidence != nil && !a.evidence.healthy())
		return failed && a.evidenceFailureHandled.CompareAndSwap(false, true)
	})
	if !admitted || !handle {
		return
	}
	a.requestGracefulQuit("evidence writer failed or exceeded its bounded acknowledgement deadline")
}

func (a *probeApp) postTransition(message uint32, wParam, lParam uintptr, action string) bool {
	type postResult struct {
		posted bool
		err    error
	}
	result, admitted := runAbruptOperation(&a.shutdown, func() postResult {
		posted, postErr := postMessage(a.hidden, message, wParam, lParam)
		return postResult{posted: posted, err: postErr}
	})
	if !admitted {
		return false
	}
	posted, postErr := result.posted, result.err
	if posted {
		return true
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "post_message_" + action, SelectedAPIPath: "PostMessageW", FailureCause: fmt.Sprintf("critical transition message could not be queued: %v", postErr), Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", win32ErrorCode(postErr))}})
	return false
}

func (a *probeApp) closeLocalResources() {
	if a.shutdown.isConfirmed() {
		// Confirmed WM_ENDSESSION is an abrupt OS-owned boundary. Ordinary
		// release, helper destruction, artifact cleanup, sync, and handle close
		// are intentionally left to process teardown and startup recovery.
		return
	}
	if a.helper != nil {
		a.mu.Lock()
		permissionSubscriptionOwned := a.permissionMonitorReady && !a.unsubscribed
		a.mu.Unlock()
		if permissionSubscriptionOwned {
			if hr := a.helper.PermissionUnsubscribe(); hr.Succeeded() {
				a.mu.Lock()
				a.unsubscribed = true
				a.mu.Unlock()
			}
		}
	}
	if a.hidden != 0 {
		if unregistered, _, lastError := a.unregisterHotkey("local_resource_cleanup"); !unregistered {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultFail, Action: "local_resource_hotkey_unregister", SelectedAPIPath: "UnregisterHotKey", FailureCause: "hotkey remained registered during local resource cleanup", Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", lastError)}})
		}
		if unregistered, _, lastError := a.unregisterSessionNotifications("local_resource_cleanup"); !unregistered {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "local_resource_session_notification_unregister", SelectedAPIPath: "WTSUnRegisterSessionNotification", FailureCause: "session notification registration remained owned during local resource cleanup", Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", lastError)}})
		}
		a.removeTrayIcon()
	}
	var destroyHelper func() bool
	if a.helper != nil && a.helperLifetime.isInitialized() {
		destroyHelper = func() bool {
			hr := a.helper.Destroy()
			if hr.Failed() {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "startup_or_abnormal_rollback", SelectedAPIPath: "CapDestroy-before-event-close", HResult: hr.Hex(), FailureCause: "helper refused rollback; Go-owned events are left for process teardown"})
				return false
			}
			a.helperLifetime.clear()
			return true
		}
	}
	var windowsToDestroy []func()
	if a.main != 0 {
		windowsToDestroy = append(windowsToDestroy, func() { destroyWindow(a.main); a.main = 0 })
	}
	if a.hidden != 0 {
		windowsToDestroy = append(windowsToDestroy, func() { destroyWindow(a.hidden); a.hidden = 0 })
	}
	eventsToClose := make([]func(), 0, len(a.events))
	for _, event := range a.events {
		event := event
		eventsToClose = append(eventsToClose, func() { _ = windows.CloseHandle(event) })
	}
	var closeLog func()
	if a.logFile != nil {
		closeLog = func() { _ = a.logFile.Close(); a.logFile = nil }
	}
	runStartupCleanup(destroyHelper, windowsToDestroy, eventsToClose, closeLog)
	a.events = nil
}

type handleReader struct{ handle windows.Handle }

func (r handleReader) Read(buffer []byte) (int, error) {
	var read uint32
	if err := windows.ReadFile(r.handle, buffer, &read, nil); err != nil {
		return int(read), err
	}
	if read == 0 {
		return 0, io.EOF
	}
	return int(read), nil
}

func resultForHR(hr winprobe.HResult) winprobe.ProbeResult {
	if hr.Failed() {
		return winprobe.ResultFail
	}
	return winprobe.ResultPass
}

func boolToUintptr(value bool) uintptr {
	if value {
		return 1
	}
	return 0
}
