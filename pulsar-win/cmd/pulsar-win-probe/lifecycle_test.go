package main

import (
	"testing"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestLifecycleMessagePlansUseDocumentedSignals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		message uint32
		wParam  uintptr
		action  lifecycleMessageAction
		edge    lifecycleEdge
		reason  winprobe.CaptureReason
	}{
		{name: "shutdown query", message: lifecycleWMQueryEndSession, action: lifecycleMessageStop, edge: lifecycleSystemShutdown, reason: winprobe.ReasonShutdown},
		{name: "shutdown confirmed", message: lifecycleWMEndSession, wParam: 1, action: lifecycleMessageShutdownConfirmed, edge: lifecycleSystemShutdown, reason: winprobe.ReasonShutdown},
		{name: "shutdown cancelled", message: lifecycleWMEndSession, action: lifecycleMessageShutdownCancelled, edge: lifecycleSystemShutdown, reason: winprobe.ReasonShutdown},
		{name: "suspend", message: lifecycleWMPowerBroadcast, wParam: lifecyclePBTAPMSuspend, action: lifecycleMessageStop, edge: lifecycleSuspend, reason: winprobe.ReasonSuspend},
		{name: "automatic resume", message: lifecycleWMPowerBroadcast, wParam: lifecyclePBTAPMResumeAutomatic, action: lifecycleMessageResume, edge: lifecycleSuspend, reason: winprobe.ReasonSuspend},
		{name: "user resume", message: lifecycleWMPowerBroadcast, wParam: lifecyclePBTAPMResumeSuspend, action: lifecycleMessageResume, edge: lifecycleSuspend, reason: winprobe.ReasonSuspend},
		{name: "session lock", message: lifecycleWMWTSSessionChange, wParam: lifecycleWTSSessionLock, action: lifecycleMessageStop, edge: lifecycleSessionLock, reason: winprobe.ReasonLock},
		{name: "session unlock", message: lifecycleWMWTSSessionChange, wParam: lifecycleWTSSessionUnlock, action: lifecycleMessageResume, edge: lifecycleSessionLock, reason: winprobe.ReasonLock},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			plan, ok := planLifecycleMessage(tc.message, tc.wParam)
			if !ok {
				t.Fatal("message was not recognized")
			}
			if plan.Action != tc.action || plan.Edge != tc.edge || plan.Reason != tc.reason || plan.Signal == "" {
				t.Fatalf("plan = %+v", plan)
			}
		})
	}
	if _, ok := planLifecycleMessage(lifecycleWMPowerBroadcast, 0xdead); ok {
		t.Fatal("unknown power notification was treated as a lifecycle edge")
	}
}

func TestLifecycleTrackerOrdersCaptureCleanupAndGracefulExit(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	start := tracker.observe(lifecycleQuit, "tray_quit", winprobe.ReasonCancel, lifecycleGracefulExit, true)
	if start.ID == 0 || start.Stage != lifecycleSignalObserved {
		t.Fatalf("start = %+v", start)
	}
	want := []lifecycleStage{
		lifecycleStopRequested,
		lifecycleCaptureTerminal,
		lifecycleArtifactDisposed,
		lifecycleCaptureReleased,
		lifecyclePermissionUnsubscribed,
		lifecycleHotkeyUnregistered,
		lifecycleSessionNotificationUnregistered,
		lifecycleHelperDestroyed,
		lifecycleTrayIconRemoved,
		lifecycleEvidenceSynced,
		lifecycleProcessExit,
	}
	for _, stage := range want {
		progress, changed, err := tracker.advance(lifecycleQuit, stage)
		if err != nil || !changed || progress.Stage != stage || progress.ID != start.ID {
			t.Fatalf("advance(%s) = (%+v, %v, %v)", stage, progress, changed, err)
		}
	}
	if _, ok := tracker.activeRun(lifecycleQuit); ok {
		t.Fatal("graceful exit run survived process-exit evidence")
	}
}

func TestLifecycleTrackerOrdersGracefulExitWithoutCapture(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	tracker.observe(lifecycleQuit, "WM_CLOSE", winprobe.ReasonCancel, lifecycleGracefulExit, false)
	for _, stage := range []lifecycleStage{
		lifecycleStopRequested,
		lifecyclePermissionUnsubscribed,
		lifecycleHotkeyUnregistered,
		lifecycleSessionNotificationUnregistered,
		lifecycleHelperDestroyed,
		lifecycleTrayIconRemoved,
		lifecycleEvidenceSynced,
		lifecycleProcessExit,
	} {
		if _, changed, err := tracker.advance(lifecycleQuit, stage); err != nil || !changed {
			t.Fatalf("advance(%s) changed=%v err=%v", stage, changed, err)
		}
	}
}

func TestLifecycleCaptureSettlementAllowsHonestQueryFailureCleanup(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	tracker.observe(lifecyclePermissionRevoke, "AppCapability.AccessChanged", winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle, true)
	for _, stage := range []lifecycleStage{
		lifecycleStopRequested,
		lifecycleCaptureTerminal,
		lifecycleArtifactDisposed,
		lifecycleCaptureReleased,
		lifecycleHotkeyUnregistered,
		lifecycleIdle,
	} {
		if _, changed, err := tracker.advance(lifecyclePermissionRevoke, stage); err != nil || !changed {
			t.Fatalf("advance(%s) changed=%v err=%v", stage, changed, err)
		}
	}
}

func TestLifecycleTrackerRejectsUnprovenCleanup(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	tracker.observe(lifecyclePermissionRevoke, "AccessChanged", winprobe.ReasonPermissionRevoke, lifecycleReturnsIdle, true)
	if _, _, err := tracker.advance(lifecyclePermissionRevoke, lifecycleArtifactDisposed); err == nil {
		t.Fatal("artifact cleanup advanced before capture terminal")
	}
	if _, _, err := tracker.advance(lifecyclePermissionRevoke, lifecycleHotkeyUnregistered); err == nil {
		t.Fatal("hotkey cleanup advanced before capture release")
	}
	if _, _, err := tracker.advance(lifecyclePermissionRevoke, lifecycleHelperDestroyed); err == nil {
		t.Fatal("idle permission-revoke path destroyed the process helper")
	}
}

func TestLifecycleTrackerRepeatedCyclesAreIdempotent(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	for cycle := uint64(1); cycle <= 100; cycle++ {
		first := tracker.observe(lifecycleSessionLock, "WTS_SESSION_LOCK", winprobe.ReasonLock, lifecycleReturnsIdle, false)
		repeated := tracker.observe(lifecycleSessionLock, "WTS_SESSION_LOCK", winprobe.ReasonLock, lifecycleReturnsIdle, false)
		if repeated.ID != first.ID || !repeated.RepeatedSignal {
			t.Fatalf("cycle %d repeated signal = %+v, first = %+v", cycle, repeated, first)
		}
		for _, stage := range []lifecycleStage{lifecycleStopRequested, lifecycleHotkeyUnregistered, lifecycleIdle} {
			if _, changed, err := tracker.advance(lifecycleSessionLock, stage); err != nil || !changed {
				t.Fatalf("cycle %d advance(%s) changed=%v err=%v", cycle, stage, changed, err)
			}
		}
		if first.ID != cycle {
			t.Fatalf("cycle %d run id = %d", cycle, first.ID)
		}
	}
}

func TestAbruptShutdownDoesNotFabricateTerminalOrArtifactEvidence(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	tracker.observe(lifecycleSystemShutdown, "WM_QUERYENDSESSION", winprobe.ReasonShutdown, lifecycleAbruptOSExit, true)
	if _, _, err := tracker.advance(lifecycleSystemShutdown, lifecycleStopRequested); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tracker.advance(lifecycleSystemShutdown, lifecycleHotkeyUnregistered); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tracker.advance(lifecycleSystemShutdown, lifecycleAbruptHandoff); err != nil {
		t.Fatal(err)
	}
	if _, ok := tracker.activeRun(lifecycleSystemShutdown); ok {
		t.Fatal("abrupt OS handoff remained active")
	}
}

func TestCancelledSystemShutdownReturnsToOrderedIdleCleanup(t *testing.T) {
	t.Parallel()
	tracker := newLifecycleTracker()
	tracker.observe(lifecycleSystemShutdown, "WM_QUERYENDSESSION", winprobe.ReasonShutdown, lifecycleAbruptOSExit, true)
	if _, _, err := tracker.advance(lifecycleSystemShutdown, lifecycleStopRequested); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.cancelShutdown("WM_ENDSESSION(cancelled)"); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []lifecycleStage{
		lifecycleCaptureTerminal,
		lifecycleArtifactDisposed,
		lifecycleCaptureReleased,
		lifecycleHotkeyUnregistered,
		lifecycleIdle,
	} {
		if _, changed, err := tracker.advance(lifecycleSystemShutdown, stage); err != nil || !changed {
			t.Fatalf("advance(%s) changed=%v err=%v", stage, changed, err)
		}
	}
}
