//go:build windows

package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func runUnpairedShell(dir, coordinatorBase string) (paired, supported bool) {
	var didPair bool
	ring := NewRing(sampleRate * channels)
	gain := NewGain()
	engine := NewEngine(ring, gain)
	mixer := NewWindowsOverlayMediaClipMixer(engine)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	outputControl := newWindowsAudioOutputController(engine, nil, log)
	workflow := NewWindowsCaptureWorkflowController(NewWindowsRecordingController(nil), nil, nil)
	if configured, err := newWindowsCaptureWorkflow(dir, mixer, gain); err == nil {
		workflow = configured
	} else {
		log.Error("accountless local capture unavailable", "err", err)
	}
	shell := NewWindowsShell(preferredWindowsShellLocale(), func() ShellSnapshot {
		recording, recordingAvailable := workflow.Snapshot()
		local := workflow.LocalSnapshot()
		outputs, selectedOutput := outputControl.Snapshot()
		route := "Default output"
		if selectedOutput >= 0 && selectedOutput < len(outputs) {
			route = outputs[selectedOutput].Name
		}
		return ShellSnapshot{
			Connection: ShellUnpaired, Recording: recording, RecordingAvailable: recordingAvailable,
			RecordingShortcut: currentWindowsRecordingShortcutStatus(), RecordingShortcutKey: currentWindowsRecordingShortcut(),
			SelfTestAvailable: local.Available, SelfTestPhase: local.SelfTestPhase, SelfTestMeter: local.Meter,
			LocalDraftAvailable: local.DraftAvailable, LocalDraftName: local.DraftName, LocalFailure: local.Failure,
			RecordingDraftAvailable: local.RecordingDraftAvailable,
			CaptureInputs:           local.Inputs, SelectedCaptureInput: local.SelectedInput,
			AudioOutputs: outputs, SelectedAudioOutput: selectedOutput,
			RouteName: route, DND: ShellDNDAllowAll, Volume: 80,
		}
	}, ShellActions{
		Create: func() { openURL(uiBotURL) }, Join: func() { openURL(uiBotURL) },
		TryLocally: workflow.TryLocally, PlayBuiltinCue: workflow.PlayBuiltinCue,
		ChooseLocalFile: func() { workflow.ChooseFile(currentMainWindowOwner()) }, DeleteLocalDraft: workflow.DeleteLocalDraft,
		AcceptDroppedFile: workflow.AcceptBrokeredFile,
		SelectNextInput:   workflow.SelectNextInput, ToggleRecording: workflow.Toggle, CancelRecording: workflow.Cancel,
		SelectNextOutput: func() { go outputControl.SelectNext() },
	})
	shortcutStore := WindowsRecordingShortcutStore{Path: filepath.Join(dir, "recording-shortcut.v1.json")}
	state := &TrayState{Shell: shell, Recording: workflow, Shortcut: shortcutStore.Load(), ShortcutStore: shortcutStore}
	state.Connected = func() bool { return false }
	state.OnRePair = func() {
		if _, err := showOnboardingWindow(dir, coordinatorBase); err == nil {
			didPair = true
			requestTrayLoopExit()
		}
	}
	state.OnQuit = func() {}
	awaitShutdown(state, make(chan struct{}))
	workflow.Shutdown()
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = workflow.Wait(drain)
	cancel()
	workflow.ClosePlatform()
	outputControl.Close()
	gain.Close()
	return didPair, true
}
