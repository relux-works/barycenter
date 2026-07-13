//go:build !windows

package main

func newPlatformRecoveryClipboardBackend() (recoveryClipboardBackend, error) {
	return nil, errClipboardUnsupported
}
