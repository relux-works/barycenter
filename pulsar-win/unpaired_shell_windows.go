//go:build windows

package main

func runUnpairedShell(dir, coordinatorBase string) (paired, supported bool) {
	var didPair bool
	shell := NewWindowsShell(preferredWindowsShellLocale(), func() ShellSnapshot {
		return ShellSnapshot{
			Connection: ShellUnpaired, Recording: ShellRecordingUnavailable,
			DND: ShellDNDAllowAll, Volume: 80,
		}
	}, ShellActions{
		Create: func() { openURL(uiBotURL) },
		Join:   func() { openURL(uiBotURL) },
	})
	state := &TrayState{Shell: shell}
	state.Connected = func() bool { return false }
	state.OnRePair = func() {
		if _, err := showOnboardingWindow(dir, coordinatorBase); err == nil {
			didPair = true
			requestTrayLoopExit()
		}
	}
	state.OnQuit = func() {}
	awaitShutdown(state, make(chan struct{}))
	return didPair, true
}
