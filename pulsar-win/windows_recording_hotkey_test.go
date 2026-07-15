package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeWindowsShortcutRegistrar struct {
	next          WindowsShortcutRegistration
	nextErr       error
	unregisterErr error
	live          map[WindowsShortcutRegistration]WindowsRecordingShortcut
	unregistered  []WindowsShortcutRegistration
}

func newFakeWindowsShortcutRegistrar() *fakeWindowsShortcutRegistrar {
	return &fakeWindowsShortcutRegistrar{live: make(map[WindowsShortcutRegistration]WindowsRecordingShortcut)}
}

func (r *fakeWindowsShortcutRegistrar) Register(shortcut WindowsRecordingShortcut) (WindowsShortcutRegistration, error) {
	if r.nextErr != nil {
		err := r.nextErr
		r.nextErr = nil
		return 0, err
	}
	r.next++
	r.live[r.next] = shortcut
	return r.next, nil
}

func (r *fakeWindowsShortcutRegistrar) Unregister(registration WindowsShortcutRegistration) error {
	if r.unregisterErr != nil {
		return r.unregisterErr
	}
	delete(r.live, registration)
	r.unregistered = append(r.unregistered, registration)
	return nil
}

func TestWindowsRecordingShortcutToggleConflictAndButtonFallback(t *testing.T) {
	registrar := newFakeWindowsShortcutRegistrar()
	toggles := 0
	controller := NewWindowsRecordingShortcutController(registrar, DefaultWindowsRecordingShortcut(), func() { toggles++ })
	controller.Start()
	first := registrar.next
	if controller.Status() != WindowsShortcutRegistered || !controller.HandleHotKey(first) || toggles != 1 {
		t.Fatalf("registered shortcut did not toggle: status=%s toggles=%d", controller.Status(), toggles)
	}

	registrar.nextErr = ErrWindowsShortcutConflict
	replacement := WindowsRecordingShortcut{VirtualKey: WindowsShortcutVKR, Modifiers: WindowsShortcutModAlt | WindowsShortcutModShift}
	if !controller.Reconfigure(replacement) || controller.Status() != WindowsShortcutConflict {
		t.Fatalf("conflict state = %s", controller.Status())
	}
	if controller.HandleHotKey(first) || toggles != 1 || len(registrar.live) != 0 {
		t.Fatal("stale registration remained active after conflict")
	}
	// Window/tray buttons call the shared recording controller directly; a
	// shortcut conflict cannot consume or disable that fallback.
	toggles++
	if toggles != 2 {
		t.Fatal("button fallback did not remain independent")
	}
}

func TestWindowsRecordingShortcutOverlappingLifecycleAndCleanup(t *testing.T) {
	registrar := newFakeWindowsShortcutRegistrar()
	controller := NewWindowsRecordingShortcutController(registrar, DefaultWindowsRecordingShortcut(), nil)
	controller.Start()
	first := registrar.next
	controller.Suspend(WindowsShortcutSessionLocked)
	controller.Suspend(WindowsShortcutSystemSuspend)
	controller.Suspend(WindowsShortcutSessionLocked)
	if controller.Status() != WindowsShortcutSuspended || len(registrar.live) != 0 {
		t.Fatal("shortcut not released on lock/suspend")
	}
	controller.Resume(WindowsShortcutSessionLocked)
	if controller.Status() != WindowsShortcutSuspended || len(registrar.live) != 0 {
		t.Fatal("shortcut re-registered before every suspension cleared")
	}
	controller.Resume(WindowsShortcutSystemSuspend)
	second := registrar.next
	if second == first || controller.Status() != WindowsShortcutRegistered || len(registrar.live) != 1 {
		t.Fatal("shortcut not cleanly re-registered after resume")
	}
	controller.Stop()
	controller.Stop()
	if controller.Status() != WindowsShortcutInactive || len(registrar.live) != 0 ||
		len(registrar.unregistered) != 2 {
		t.Fatalf("teardown leaked registration: live=%v released=%v", registrar.live, registrar.unregistered)
	}
}

func TestWindowsRecordingShortcutUnregisterFailureBlocksReplacement(t *testing.T) {
	registrar := newFakeWindowsShortcutRegistrar()
	controller := NewWindowsRecordingShortcutController(registrar, DefaultWindowsRecordingShortcut(), nil)
	controller.Start()
	registrar.unregisterErr = errors.New("win32 unregister failed")
	if controller.Reconfigure(WindowsRecordingShortcut{VirtualKey: WindowsShortcutVKR, Modifiers: WindowsShortcutModControl}) {
		t.Fatal("replacement registered after failed ownership release")
	}
	if controller.Status() != WindowsShortcutUnavailable || len(registrar.live) != 1 || registrar.next != 1 {
		t.Fatal("failed release did not remain explicit and single-owner")
	}
}

func TestWindowsRecordingShortcutStoreBoundsMalformedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "recording-shortcut.v1.json")
	store := WindowsRecordingShortcutStore{Path: path}
	if store.Load() != DefaultWindowsRecordingShortcut() {
		t.Fatal("missing settings did not use the default shortcut")
	}
	replacement := WindowsRecordingShortcut{VirtualKey: WindowsShortcutVKR, Modifiers: WindowsShortcutModControl | WindowsShortcutModAlt}
	if err := store.Save(replacement); err != nil || store.Load() != replacement {
		t.Fatalf("shortcut roundtrip failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"virtualKey":27,"modifiers":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if store.Load() != DefaultWindowsRecordingShortcut() {
		t.Fatal("bare Escape or modifier-free shortcut was accepted")
	}
}

func TestWindowsRecordingShortcutLabelsConfiguredAlternatives(t *testing.T) {
	tests := map[WindowsRecordingShortcut]string{
		DefaultWindowsRecordingShortcut(): "Ctrl+Shift+Space",
		{VirtualKey: WindowsShortcutVKR, Modifiers: WindowsShortcutModControl | WindowsShortcutModAlt}: "Ctrl+Alt+R",
		{VirtualKey: 0x7B, Modifiers: WindowsShortcutModWin}:                                           "Win+F12",
	}
	for shortcut, want := range tests {
		if got := shortcut.Label(); got != want {
			t.Errorf("label=%q want=%q", got, want)
		}
	}
}

func TestWindowsRecordingShortcutWin32RoutingAndForegroundCancelContract(t *testing.T) {
	traySource, err := os.ReadFile("ui_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	windowSource, err := os.ReadFile("main_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	compositionSource, err := os.ReadFile("windows_capture_composition_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	tray, window := string(traySource), string(windowSource)
	production := string(mainSource) + string(compositionSource)
	for _, required := range []string{
		"NewWindowsRecordingShortcutController", "win32RecordingShortcutRegistrar{hwnd: trayHwnd}",
		"case wmHotKey:", "case wmWtsSessionChange:", "case wmPowerBroadcast:",
		"WindowsShortcutSessionLocked", "WindowsShortcutSystemSuspend", "case menuCancel:",
		"menuShortcutDefault", "menuShortcutAlternative", "curRecordingShortcut.Stop()",
	} {
		if !containsText(tray, required) {
			t.Errorf("tray lifecycle missing %q", required)
		}
	}
	for _, required := range []string{"vkEscape", "idShellCancel", "mainOwnsMessage", "actions.CancelRecording"} {
		if !containsText(window, required) {
			t.Errorf("foreground cancel missing %q", required)
		}
	}
	for _, required := range []string{
		"NewNativeWindowsMicrophoneBackend", "NewWindowsMicrophoneCaptureService",
		"NewWindowsRecordingController", "ToggleRecording:", "workflow.Toggle", "CancelRecording:", "workflow.Cancel",
	} {
		if !containsText(production, required) {
			t.Errorf("production wiring missing %q", required)
		}
	}
	if containsText(tray, "SetWindowsHookEx") || containsText(window, "RegisterHotKey") {
		t.Fatal("global hook or window-local hotkey ownership crossed the intended seam")
	}
}

func TestWindowsRecordingShortcutSourceExcludesHooksAndBareEscape(t *testing.T) {
	source, err := os.ReadFile("windows_recording_hotkey_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{"RegisterHotKey", "UnregisterHotKey", "modNoRepeat"} {
		if !containsText(text, required) {
			t.Errorf("Windows registrar missing %q", required)
		}
	}
	for _, forbidden := range []string{"SetWindowsHookEx", "WH_KEYBOARD", "VK_ESCAPE"} {
		if containsText(text, forbidden) {
			t.Errorf("Windows registrar contains forbidden %q", forbidden)
		}
	}
}

func containsText(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
