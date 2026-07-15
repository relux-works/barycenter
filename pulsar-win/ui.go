// UI contract shared by the Win32 implementation (ui_windows.go) and the
// non-Windows stub (ui_stub.go). The portable parts (state, callbacks) live
// here so the darwin test build links; the Win32 plumbing is isolated.
package main

// TrayState is what the tray menu renders and acts on, read live on each open.
type TrayState struct {
	// Shell is the shared main-window/tray projection. Nil retains the legacy
	// onboarding-only tray for narrow tests and recovery paths.
	Shell *WindowsShell
	// Connected reports the coordinator link (heartbeat-backed).
	Connected func() bool
	// Identity is the "host · дом slot" line (empty while unpaired).
	Identity string
	// OnRePair opens the onboarding window to re-pair in place (F3).
	OnRePair func()
	// OnQuit tears the process down cleanly.
	OnQuit func()
}

// OnboardingResult carries the credentials a successful pairing produced.
type OnboardingResult struct {
	Creds Credentials
}

// pairAndSave runs the pairing exchange and persists credentials — the shared
// action behind both the window's submit button and the CLI --pair path.
func pairAndSave(dir, coordinatorBase, code string) (Credentials, error) {
	creds, err := Pair(coordinatorBase, normalizePairCode(code))
	if err != nil {
		return Credentials{}, err
	}
	if err := creds.Save(dir); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}
