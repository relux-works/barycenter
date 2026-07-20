package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestR6CapPermissionCheckCallSitesRemainWaiterOwned(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "main_windows.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"newProbeApp":              true, // Initialization before waiter ownership begins.
		"permissionCheckOperation": true, // Runtime helper call is centralized behind the waiter operation permit.
	}
	callSites := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "PermissionCheck" {
				return true
			}
			callSites++
			if !allowed[function.Name.Name] {
				t.Errorf("CapPermissionCheck is referenced from non-waiter runtime function %s", function.Name.Name)
			}
			return true
		})
	}
	if callSites != 2 {
		t.Fatalf("CapPermissionCheck call sites = %d, want one initialization plus one waiter-owned operation seam", callSites)
	}
	for _, caller := range []string{"drainCommands", "drainPermissionChange", "drainCapture"} {
		start := strings.Index(mustReadSource(t, "main_windows.go"), "func (a *probeApp) "+caller+"(")
		if start < 0 || !strings.Contains(mustReadSource(t, "main_windows.go")[start:], "permissionCheckOperation()") {
			t.Errorf("%s does not route runtime permission queries through permissionCheckOperation", caller)
		}
	}
}

func mustReadSource(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return normalizeSource(source)
}

func normalizeSource(source []byte) string {
	return strings.ReplaceAll(string(source), "\r\n", "\n")
}

func TestPackagedStartupUsesAuthoritativeAppContainerEvidencePath(t *testing.T) {
	t.Parallel()
	mainText := mustReadSource(t, "main_windows.go")
	for _, required := range []string{
		`winprobe.NewJSONLogger(logFile)`,
		`return filepath.Join(local, "PulsarProbe"), nil`,
	} {
		if !strings.Contains(mainText, required) {
			t.Errorf("packaged startup does not preserve %q", required)
		}
	}
	for _, forbidden := range []string{
		`io.MultiWriter(logFile, os.Stderr)`,
		`filepath.Join(local, "Packages", windows.UTF16ToString(buffer), "LocalState", "PulsarProbe")`,
	} {
		if strings.Contains(mainText, forbidden) {
			t.Errorf("packaged startup retains invalid path/logger wiring %q", forbidden)
		}
	}
}

func TestR3WindowsWiringUsesProductionLifecycleCoordinators(t *testing.T) {
	t.Parallel()
	mainSource, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	windowSource, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	coordinatorSource, err := os.ReadFile("coordinators.go")
	if err != nil {
		t.Fatal(err)
	}
	mainText := normalizeSource(mainSource)
	windowText := normalizeSource(windowSource)
	coordinatorText := normalizeSource(coordinatorSource)
	for _, required := range []string{
		"uiTransitions.publish(uiTransitionIdleCleanup",
		"uiTransitions.publish(uiTransitionLifecycleRearm",
		"uiTransitions.drive(",
		"lifecycle.permissionQueryFailureRequiresStop(",
		"CapPermissionCheck(explicit-record-query-failed)",
		"CapPermissionCheck(lifecycle-rearm-query-failed)",
		"exit.beginGracefulWithDeadline(",
		"evidence.logAsync(",
		"evidence.sync()",
		"lifecycle.replayCaptureGeneration(",
		"runAfterRequiredEvidence(",
		"shutdown.runWaiterIteration(",
		"drainConfirmedShutdownCapture",
		"a.drainCaptureOrphans",
		"runCapturePrepareOwned(a.lifecycle, &a.captureOwners, &a.shutdown",
		"admitCaptureActivationOwned(&a.captureOwners, &a.shutdown",
		"runCaptureActivationAdmitted(a.lifecycle, &a.captureOwners, &a.shutdown, owner",
		"runCaptureContinuation(&a.shutdown",
		"prepared.handleSuppressedPrepare(a.lifecycle, &a.captureOwners, &a.shutdown",
		"requestCaptureStopForExactOwner(&a.captureOwners, generation, operationID",
		"runCaptureResultQueryFailure(owner, id, callHR",
		"a.requestCaptureRelease(owner, captureReleaseAfterTerminal)",
		"a.captureOwners.clearReleased(owner)",
		"prepared.handleInvalidSuccessfulResult(&a.shutdown",
		"prepared.handleUnexpectedSuccessfulHResult(&a.shutdown",
		"a.recordRequiredStructuralFailure(",
		"return prepared.id, hr == 0",
		"owner.finalizedReleaseAuthority()",
		"a.captureOwners.orphanCount()",
		"a.requestOwnedCaptureStop(generation, capture, command.reason)",
		"a.requestOwnedCaptureStop(generation, capture, reason)",
		"a.shutdown.confirmedCapture.Load()",
		"writer.WriteBufferedFramesWithoutSync",
		"shutdown.isConfirmed()",
		"evidence.bindAdmissionGate(app.shutdown.runOperation)",
	} {
		if !strings.Contains(mainText, required) {
			t.Errorf("main_windows.go does not wire %q", required)
		}
	}
	for _, required := range []string{
		"owner.claimStop(stop)",
		"owner.invokeClaimedStop(shutdown)",
		"lifecycle.runCapturePrepareCommit(generation",
		"}, shutdown.runOperation)",
		"runCaptureContinuation(shutdown, r.resultEvidenceAllowed, continuations...)",
		"runCaptureContinuation(shutdown, r.ownerSuccessorAllowed, continuations...)",
		"owner.admitActivationIntent()",
		"owner.admitNativeActivation()",
		"defer owner.completeNativeActivation(shutdown)",
		"return captureStopOutcome{State: captureStopPending}",
		"if !result.Stop.completed()",
		"owner.requestRelease(captureReleaseAfterAcceptedStop, release)",
		"owners.publishOrphanStopProducer(owner, unpublishedStop)",
		"runCaptureOrphanDrain(",
		"runRequiredFailureContinuation(shutdown, true, requiredEvidence, escalate)",
	} {
		if !strings.Contains(coordinatorText, required) {
			t.Errorf("coordinators.go does not wire unpublished-owner invariant %q", required)
		}
	}
	conflictStart := strings.Index(mainText, "if prepared.succeeded && !prepared.ownerPublished {")
	if conflictStart < 0 {
		t.Fatal("capture-owner publication conflict settlement is absent")
	}
	conflictEnd := strings.Index(mainText[conflictStart:], "\n\tcompletion := prepared.dispatchPostHelper")
	if conflictEnd < 0 {
		t.Fatal("capture-owner publication conflict scope is malformed")
	}
	if strings.Contains(mainText[conflictStart:conflictStart+conflictEnd], "CaptureStop") {
		t.Fatal("capture-owner publication conflict still depends on a later UI-continuation stop")
	}
	if count := strings.Count(mainText, "prepared.dispatchPostHelper(&a.shutdown"); count != 1 {
		t.Fatalf("capture prepare completion dispatch sites=%d, want exactly 1", count)
	}
	if count := strings.Count(mainText, `Action: "capture_prepare"`); count != 1 {
		t.Fatalf("capture_prepare result evidence rows=%d, want exactly 1", count)
	}
	ownerState := strings.Index(mainText, "a.captureOp = id")
	resultEvidence := strings.Index(mainText, `Action: "capture_prepare"`)
	if ownerState < 0 || resultEvidence < 0 || ownerState > resultEvidence {
		t.Fatalf("published owner state is not ordered before result evidence: state=%d evidence=%d", ownerState, resultEvidence)
	}
	activateStart := strings.Index(mainText, "func (a *probeApp) activateCapture(")
	if activateStart < 0 {
		t.Fatal("activateCapture is absent")
	}
	activateBody := mainText[activateStart:]
	activationAdmission := strings.Index(activateBody, "owner := admitCaptureActivationOwned(")
	activationIntent := strings.Index(activateBody, `Action: "capture_activate_intent"`)
	nativeActivation := strings.Index(activateBody, "runCaptureActivationAdmitted(")
	if activationAdmission < 0 || activationIntent < 0 || nativeActivation < 0 || activationAdmission > activationIntent || activationIntent > nativeActivation {
		t.Fatalf("activation owner admission is not ordered before intent/native work: admission=%d intent=%d native=%d", activationAdmission, activationIntent, nativeActivation)
	}
	activationCoordinator := strings.Index(coordinatorText, "func runCaptureActivationAdmitted(")
	if activationCoordinator < 0 {
		t.Fatal("runCaptureActivationAdmitted is absent")
	}
	activationCoordinatorBody := coordinatorText[activationCoordinator:]
	deferredCompletion := strings.Index(activationCoordinatorBody, "defer owner.completeNativeActivation(shutdown)")
	postAdmissionSeam := strings.Index(activationCoordinatorBody, "afterAdmission()")
	postAdmissionClosing := -1
	if postAdmissionSeam >= 0 {
		if relative := strings.Index(activationCoordinatorBody[postAdmissionSeam:], "if shutdown.isClosing() {"); relative >= 0 {
			postAdmissionClosing = postAdmissionSeam + relative
		}
	}
	externalActivation := strings.Index(activationCoordinatorBody, "activate()")
	if deferredCompletion < 0 || postAdmissionSeam < 0 || postAdmissionClosing < 0 || externalActivation < 0 || deferredCompletion > postAdmissionSeam || postAdmissionSeam > postAdmissionClosing || postAdmissionClosing > externalActivation {
		t.Fatalf("native activation does not arm completion before post-admission close/call: completion=%d seam=%d closing=%d activate=%d", deferredCompletion, postAdmissionSeam, postAdmissionClosing, externalActivation)
	}
	for _, bypass := range []string{
		"hr := a.helper.CaptureStop(capture, command.reason)",
		"stopHR := a.helper.CaptureStop(id, winprobe.ReasonCancel)",
		"_ = a.helper.CaptureStop(id, winprobe.ReasonCancel)",
		"stopHR := a.helper.CaptureStop(id, winprobe.ReasonUserStop)",
		"return a.helper.CaptureStop(capture, reason)",
		"requestCaptureStopOrReuse(nil,",
	} {
		if strings.Contains(mainText, bypass) {
			t.Errorf("active-capture stop bypasses exact-owner one-shot seam: %q", bypass)
		}
	}
	if strings.Contains(coordinatorText, "return captureStopOutcome{State: captureStopCompleted, Result: stop(operationID)}") {
		t.Fatal("exact-owner stop coordinator still has an ownerless native fallback")
	}
	if strings.Contains(mainText, "a.helper.CaptureRelease(id)") {
		t.Fatal("CaptureRelease bypasses the exact-owner admission gate")
	}
	if !strings.Contains(coordinatorText, "owners.matching(generation, operationID)") || !strings.Contains(mainText, "func(generation uint64, capture uint32) winprobe.HResult") {
		t.Fatal("lifecycle stop callback does not preserve exact generation+operation identity")
	}
	drainCaptureStart := strings.Index(mainText, "func (a *probeApp) drainCapture() {")
	if drainCaptureStart < 0 {
		t.Fatal("drainCapture is absent")
	}
	drainCaptureBody := mainText[drainCaptureStart:]
	queryFailure := strings.Index(drainCaptureBody, "if callHR.Failed() {")
	if queryFailure < 0 {
		t.Fatal("capture query-failure branch is absent")
	}
	queryCleanupGate := strings.Index(drainCaptureBody[queryFailure:], "failure := runCaptureResultQueryFailure(")
	queryPendingReturn := strings.Index(drainCaptureBody[queryFailure:], "if !cleanup.Stop.completed() {")
	queryReleaseState := strings.Index(drainCaptureBody[queryFailure:], "recovery := queryFailureOutcome{")
	if queryCleanupGate < 0 || queryPendingReturn < queryCleanupGate || queryReleaseState < queryPendingReturn {
		t.Fatalf("capture query failure does not gate cleanup/release on completed stop: failure=%d cleanup=%d pending=%d release=%d", queryFailure, queryCleanupGate, queryPendingReturn, queryReleaseState)
	}
	permissionBranch := strings.Index(mainText, `case "permission_check", "permission_rearm":`)
	if permissionBranch < 0 {
		t.Fatal("waiter permission-query branch is absent")
	}
	permissionBranchText := mainText[permissionBranch:]
	permissionFailure := strings.Index(permissionBranchText, "if hr.Failed() {")
	permissionStop := strings.Index(permissionBranchText, "a.requestLifecycleStop(lifecyclePermissionRevoke")
	permissionLog := strings.Index(permissionBranchText, "a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioPermission")
	permissionContinue := strings.Index(permissionBranchText, "continue")
	if permissionFailure < 0 || permissionStop < permissionFailure || permissionLog < 0 || permissionStop >= permissionLog || permissionContinue < permissionLog {
		t.Fatal("waiter permission-query failure must gate/settle/stop before diagnostic evidence and continuation")
	}
	if count := strings.Count(mainText, "runAfterRequiredEvidence("); count < 6 {
		t.Errorf("required evidence gate production call sites = %d, want at least 6", count)
	}
	for _, required := range []string{
		"uiTransitions.consume(uiTransitionIdleCleanup",
		"uiTransitions.consume(uiTransitionLifecycleRearm",
		"confirmedShutdownAdapter{shutdown: &a.shutdown, owners: &a.captureOwners}",
		"adapter.confirm(func(capture uint32)",
	} {
		if !strings.Contains(windowText, required) {
			t.Errorf("window_windows.go does not wire %q", required)
		}
	}
	confirmedStart := strings.Index(windowText, "case lifecycleMessageShutdownConfirmed:")
	if confirmedStart < 0 {
		t.Fatal("confirmed WM_ENDSESSION branch is absent")
	}
	confirmedTail := windowText[confirmedStart:]
	confirmedEnd := strings.Index(confirmedTail, "\n\t\t}\n\t\tif message == wmQueryEndSession")
	if confirmedEnd < 0 {
		t.Fatal("confirmed WM_ENDSESSION branch boundary is absent")
	}
	confirmedBranch := confirmedTail[:confirmedEnd]
	for _, forbidden := range []string{"unregisterHotkey", "advanceLifecycle", "requestLifecycleStop", "replayCaptureSettlement", ".log(", "logNonblocking", "Sync(", "Release(", "Finalize(", "Abort(", "Destroy("} {
		if strings.Contains(confirmedBranch, forbidden) {
			t.Errorf("confirmed WM_ENDSESSION branch contains forbidden post-latch action %q", forbidden)
		}
	}
	if count := strings.Count(windowText, "confirmedShutdownMessageGate{shutdown: &a.shutdown}"); count != 2 {
		t.Fatalf("confirmed shutdown wndproc guards = %d, want one in each wndproc", count)
	}
	if count := strings.Count(coordinatorText, "if !c.admissionOpen() {"); count < 4 {
		t.Fatalf("evidence pre-enqueue admission checks = %d, want log/logAsync/sync/perform", count)
	}
	if !strings.Contains(coordinatorText, "if known && !c.invokeAdmitted(") {
		t.Fatal("evidence worker lacks immediate pre-callback shutdown admission")
	}
	if !strings.Contains(coordinatorText, "if g.shutdown == nil || !g.shutdown.isClosing()") {
		t.Fatal("wndproc entry gate does not suppress callbacks during the stop-to-confirmed interval")
	}
	if strings.Contains(coordinatorText, "c.dispatchAbrupt(abrupt)\n\t\t\treturn true") {
		t.Fatal("waiter can still exit while shutdown is closing but not confirmed")
	}
	adapterStart := strings.Index(coordinatorText, "func (a confirmedShutdownAdapter) confirm(")
	if adapterStart < 0 {
		t.Fatal("confirmed shutdown adapter is absent")
	}
	adapterTail := coordinatorText[adapterStart:]
	adapterEnd := strings.Index(adapterTail, "\n}\n")
	if adapterEnd < 0 {
		t.Fatal("confirmed shutdown adapter boundary is absent")
	}
	adapterBody := adapterTail[:adapterEnd]
	for _, forbidden := range []string{"lifecycle", "beginLifecycle", ".mu", "Lock(", "Log(", "Sync(", "Release("} {
		if strings.Contains(adapterBody, forbidden) {
			t.Errorf("confirmed shutdown adapter enters forbidden dependency %q", forbidden)
		}
	}
	requestQuit := strings.Index(mainText, "func (a *probeApp) requestGracefulQuitAdmitted(source string) {")
	if requestQuit < 0 {
		t.Fatal("requestGracefulQuitAdmitted is absent")
	}
	requestQuitGate := strings.Index(mainText[requestQuit:], "runAbruptOperation(&a.shutdown")
	requestQuitBody := strings.Index(mainText[requestQuit:], "a.lifecycle.beginGracefulQuit(source)")
	if requestQuitGate < 0 || requestQuitBody < 0 || requestQuitGate > requestQuitBody {
		t.Fatalf("graceful quit is not gated before lifecycle mutation: function=%d gate=%d body=%d", requestQuit, requestQuitGate, requestQuitBody)
	}
	if !strings.Contains(mainText, "time.AfterFunc(30*time.Second, func() { a.shutdown.runOperation(callback) })") {
		t.Fatal("graceful watchdog callback is not shutdown-gated")
	}
	runError := strings.Index(mainText, "if err := app.run(); err != nil {")
	if runError < 0 {
		t.Fatal("post-pump error branch is absent")
	}
	runErrorGate := strings.Index(mainText[runError:], "app.shutdown.runOperation(")
	runErrorLog := strings.Index(mainText[runError:], "app.log(")
	if runErrorGate < 0 || runErrorLog < 0 || runErrorGate > runErrorLog {
		t.Fatalf("post-pump error reporting is not gated: branch=%d gate=%d log=%d", runError, runErrorGate, runErrorLog)
	}
	closeResources := strings.Index(mainText, "func (a *probeApp) closeLocalResources() {")
	if closeResources < 0 {
		t.Fatal("closeLocalResources is absent")
	}
	closeGuard := strings.Index(mainText[closeResources:], "if a.shutdown.isConfirmed() {")
	closeHelper := strings.Index(mainText[closeResources:], "if a.helper != nil {")
	if closeGuard < 0 || closeHelper < 0 || closeGuard > closeHelper {
		t.Fatalf("deferred local cleanup is not suppressed before helper/resource work: function=%d guard=%d helper=%d", closeResources, closeGuard, closeHelper)
	}
	for _, functionName := range []string{"mainWindowProc", "hiddenWindowProc"} {
		start := strings.Index(windowText, "func "+functionName+"(")
		if start < 0 {
			t.Fatalf("%s is absent", functionName)
		}
		body := windowText[start:]
		guard := strings.Index(body, "confirmedShutdownMessageGate{shutdown: &a.shutdown}")
		dispatch := strings.Index(body, "switch message {")
		if guard < 0 || dispatch < 0 || guard > dispatch {
			t.Fatalf("%s does not guard confirmed shutdown before message dispatch: guard=%d switch=%d", functionName, guard, dispatch)
		}
	}
	if strings.Contains(mainText, "event.DeviceName = picked.Name") || strings.Contains(mainText, `"artifact": artifactPath`) || strings.Contains(mainText, `"path": outcome.Path`) {
		t.Fatal("Windows production logging still exposes a picker/artifact path or original filename")
	}

	stopIndex := strings.Index(mainText, "plan, stopHR := a.lifecycle.beginLifecycleStop(")
	observationIndex := strings.Index(mainText, "evidenceWritten := a.logLifecycleObservationWithMode(plan.Progress, signal, nonblockingEvidence)")
	if stopIndex < 0 || observationIndex < 0 || stopIndex > observationIndex {
		t.Fatalf("permission/OS fail-closed stop is not ordered before diagnostic I/O: stop=%d observation=%d", stopIndex, observationIndex)
	}
	deadlineIndex := strings.Index(mainText, "a.exit.beginGracefulWithDeadline(")
	if deadlineIndex < 0 {
		t.Fatal("graceful hard deadline wiring is absent")
	}
	quitLogIndex := strings.Index(mainText[deadlineIndex:], "a.logLifecycleObservation(plan.Progress, source)")
	if quitLogIndex < 0 {
		t.Fatal("graceful hard deadline is not visibly armed before quit logging")
	}
}

func TestR4ConfirmedShutdownDrainCannotEnterOrdinaryCleanupAPIs(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "main_windows.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"PermissionCheck":          true,
		"PermissionRequestResult":  true,
		"EnumerateDevicesResult":   true,
		"DefaultDeviceResult":      true,
		"CaptureResult":            true,
		"PickerResult":             true,
		"Finalize":                 true,
		"Abort":                    true,
		"Sync":                     true,
		"Destroy":                  true,
		"postTransition":           true,
		"postIdleLifecycleCleanup": true,
		"driveUITransitions":       true,
		"maybeCleanupReady":        true,
		"log":                      true,
		"logNonblocking":           true,
	}
	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "drainConfirmedShutdownCapture" {
			continue
		}
		found = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.SelectorExpr:
				name := target.Sel.Name
				if forbidden[name] || strings.HasSuffix(name, "Release") || strings.HasSuffix(name, "Take") {
					t.Errorf("confirmed shutdown drain calls forbidden ordinary API %s", name)
				}
			case *ast.Ident:
				if forbidden[target.Name] {
					t.Errorf("confirmed shutdown drain calls forbidden ordinary function %s", target.Name)
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("drainConfirmedShutdownCapture production function is absent")
	}
}
