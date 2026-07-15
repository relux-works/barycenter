//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// newWindowsCaptureWorkflow binds the signed native helper, durable capture
// store, production mixer, cues, file intake and UI controller. It is shared
// by paired and accountless shells; neither branch adds a network dependency.
func newWindowsCaptureWorkflow(dir string, mixer *WindowsOverlayMediaClipMixer, gain *Gain) (*WindowsCaptureWorkflowController, error) {
	backendContract, err := NewNativeWindowsMicrophoneBackend()
	if err != nil {
		return nil, err
	}
	backend := backendContract.(*nativeWindowsMicrophoneBackend)
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cuePath := filepath.Join(filepath.Dir(executable), "Assets", "Audio", BuiltinRecordingCueFilename)
	cueData, err := os.ReadFile(cuePath)
	if err != nil || !ValidateBuiltinRecordingCue(cueData) {
		return nil, ErrRecordingCueUnavailable
	}
	store := NewCaptureMediaStore(filepath.Join(dir, "capture-media"))
	recovery, err := store.Recover()
	if err != nil {
		return nil, err
	}
	output := NewWindowsProductionLocalClipOutput(mixer)
	capture := NewWindowsMicrophoneCaptureService(backend, store, WindowsLocalRecordingCuePlayer{Output: output, CuePath: cuePath}, gain)
	recording := NewWindowsRecordingController(windowsMicrophoneRecordingCapture{service: capture})
	intake := NewWindowsShortAudioIntake(NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits()), store)
	selfTest, err := NewWindowsLocalSelfTestService(WindowsMicrophoneSelfTestCapture{Service: capture}, output, store, intake, cuePath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	inputs, _ := backend.Inputs(ctx)
	cancel()
	workflow := NewWindowsCaptureWorkflowController(recording, selfTest, inputs)
	workflow.ConfigureDraftBoundary(store, recovery.RetainedDrafts)
	workflow.SetOutgoingIntake(intake)
	workflow.SetPicker(func(ctx context.Context, owner uintptr) (WindowsBrokeredAudioFile, error) {
		return backend.PickAudio(ctx, windows.Handle(owner))
	})
	workflow.SetPlatformClose(backend.Close)
	return workflow, nil
}
