//go:build !windows

package main

func newWindowsCaptureWorkflow(string, *WindowsOverlayMediaClipMixer, *Gain) (*WindowsCaptureWorkflowController, error) {
	return nil, ErrWindowsCaptureBackendFailure
}
