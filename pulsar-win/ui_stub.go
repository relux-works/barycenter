//go:build !windows

// Non-Windows stub so the portable test suite links on darwin/linux. The GUI
// exists only on Windows; here the CLI --pair path and a signal wait stand in.
package main

import "errors"

// errNoGUI is returned by showOnboardingWindow off Windows so main falls back
// to the CLI onboarding message.
var errNoGUI = errors.New("onboarding window is available only on Windows; use --pair CODE")

// showOnboardingWindow: no window off Windows.
func showOnboardingWindow(dir, coordinatorBase string) (Credentials, error) {
	return Credentials{}, errNoGUI
}

// awaitShutdown blocks until the process should exit. Off Windows there is no
// tray: wait on the OS signal (preserves the dev-build behavior).
func awaitShutdown(state *TrayState, sig <-chan struct{}) {
	<-sig
}
