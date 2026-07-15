//go:build !windows

package main

func NewNativeWindowsMicrophoneBackend() (WindowsMicrophoneBackend, error) {
	return nil, ErrWindowsCaptureBackendFailure
}
