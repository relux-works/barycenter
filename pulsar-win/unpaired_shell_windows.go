//go:build windows

package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"time"
)

func runUnpairedShell(dir, coordinatorBase string, log *slog.Logger) (paired, supported bool) {
	return guardUnpairedShellStartup(log, func() { showFatalStartupError(dir) }, func() (bool, bool) {
		return runUnpairedShellInner(dir, coordinatorBase, log)
	})
}

func runUnpairedShellInner(dir, coordinatorBase string, log *slog.Logger) (paired, supported bool) {
	log.Info("unpaired shell startup", "stage", "begin")
	var didPair atomic.Bool
	ring := NewRing(sampleRate * channels)
	gain := NewGain()
	engine := NewEngine(ring, gain)
	mixer := NewWindowsOverlayMediaClipMixer(engine)
	outputControl := newWindowsAudioOutputController(engine, nil, log)
	log.Debug("unpaired shell startup", "stage", "audio_output_ready")
	workflow := NewWindowsCaptureWorkflowController(NewWindowsRecordingController(nil), nil, nil)
	if configured, err := newWindowsCaptureWorkflow(dir, mixer, gain); err == nil {
		workflow = configured
	} else {
		log.Error("accountless local capture unavailable", "err", err)
	}
	identity, identityErr := newProductionWindowsIdentityComposition(dir, coordinatorBase, func() {
		didPair.Store(true)
		requestTrayLoopExit()
	})
	if identityErr != nil {
		log.Error("self-service identity unavailable", "err", identityErr)
	}
	log.Debug("unpaired shell startup", "stage", "compositions_ready")
	shell := NewWindowsShell(preferredWindowsShellLocale(), func() ShellSnapshot {
		recording, recordingAvailable := workflow.Snapshot()
		local := workflow.LocalSnapshot()
		outputs, selectedOutput := outputControl.Snapshot()
		route := "Default output"
		if selectedOutput >= 0 && selectedOutput < len(outputs) {
			route = outputs[selectedOutput].Name
		}
		snapshot := ShellSnapshot{
			Connection: ShellUnpaired, Recording: recording, RecordingAvailable: recordingAvailable,
			RecordingShortcut: currentWindowsRecordingShortcutStatus(), RecordingShortcutKey: currentWindowsRecordingShortcut(),
			CaptureQualityMode:             local.CaptureQualityMode,
			CaptureQualityDegradedConsent:  local.CaptureQualityDegradedConsent,
			CaptureQualityBackendAvailable: local.CaptureQualityBackendAvailable,
			CaptureQualityState:            local.CaptureQualityState,
			SelfTestAvailable:              local.Available, SelfTestPhase: local.SelfTestPhase, SelfTestMeter: local.Meter,
			LocalDraftAvailable: local.DraftAvailable, LocalDraftName: local.DraftName, LocalFailure: local.Failure,
			RecordingDraftAvailable: local.RecordingDraftAvailable,
			CaptureInputs:           local.Inputs, SelectedCaptureInput: local.SelectedInput,
			AudioOutputs: outputs, SelectedAudioOutput: selectedOutput,
			RouteName: route, DND: ShellDNDAllowAll, Volume: 80,
		}
		if identity != nil {
			identity.ApplyShellSnapshot(&snapshot)
		} else {
			snapshot.IdentityOperation = ShellIdentityFailed
			snapshot.IdentityFailure = "identity_unavailable"
		}
		return snapshot
	}, ShellActions{
		Create: func(title string) {
			if identity != nil {
				identity.Create(title)
			}
		},
		Join: func(invite string) {
			if identity != nil {
				identity.Join(invite)
			}
		},
		SaveRecovery: func(path string) {
			if identity != nil {
				identity.SaveRecovery(path)
			}
		},
		TryLocally: workflow.TryLocally, PlayBuiltinCue: workflow.PlayBuiltinCue,
		ChooseLocalFile: func() { workflow.ChooseFile(currentMainWindowOwner()) }, DeleteLocalDraft: workflow.DeleteLocalDraft,
		AcceptDroppedFile: workflow.AcceptBrokeredFile,
		SelectNextInput:   workflow.SelectNextInput, ToggleRecording: workflow.Toggle, CancelRecording: workflow.Cancel,
		SetCaptureQuality: workflow.SetCaptureQuality, StopActiveCapture: workflow.Cancel,
		SelectNextOutput: func() { go outputControl.SelectNext() },
	})
	shortcutStore := WindowsRecordingShortcutStore{Path: filepath.Join(dir, "recording-shortcut.v1.json")}
	state := &TrayState{Shell: shell, Recording: workflow, Shortcut: shortcutStore.Load(), ShortcutStore: shortcutStore, Log: log}
	state.Connected = func() bool { return false }
	state.OnRePair = func() {
		if _, err := showOnboardingWindow(dir, coordinatorBase); err == nil {
			didPair.Store(true)
			requestTrayLoopExit()
		}
	}
	state.OnQuit = func() {}
	log.Debug("unpaired shell startup", "stage", "enter_tray_loop")
	awaitShutdown(state, make(chan struct{}))
	log.Info("unpaired shell stopped", "paired", didPair.Load())
	if identity != nil {
		identity.Close()
	}
	workflow.Shutdown()
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = workflow.Wait(drain)
	cancel()
	workflow.ClosePlatform()
	outputControl.Close()
	gain.Close()
	return didPair.Load(), true
}

func showFatalStartupError(dir string) {
	messageBox(0, "Pulsar couldn't start", "Pulsar hit an unexpected startup error. Diagnostic details were saved to:\n\n"+filepath.Join(dir, "pulsar.log"))
}
