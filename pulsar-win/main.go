// Pulsar for Windows — shell skeleton (EPIC B, blind build).
//
// Wiring: config (%APPDATA%\Pulsar) -> pairing credentials -> WS client to
// Barycenter -> go-librespot supervisor -> named-pipe PCM -> ring -> WASAPI.
// Everything portable is unit-tested; the Windows-only audio legs compile
// for GOOS=windows and await the first hardware run (U-W1..U-W5 spikes).
//
// Usage:
//
//	pulsar-win.exe --pair CODE [--coordinator URL]   one-time pairing
//	pulsar-win.exe                                    normal run
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

var version = "dev" // set via -ldflags "-X main.version=..."

func init() {
	// Win32 message queues are per-OS-thread: a window must be created and
	// pumped on the SAME thread, and Go migrates goroutines between OS threads
	// at will — without the lock the onboarding window or the tray menu can
	// freeze intermittently on real hardware (H4). init() runs on the main
	// goroutine, so this pins main() (which owns both pumps) to the main
	// thread for the process lifetime. No-op cost on non-Windows builds.
	runtime.LockOSThread()
}

// newLogger writes debug logs to <dir>\pulsar.log (created if absent) and, for
// CLI runs that have one, to stderr. Without a console the file is the only
// diagnostic surface (GUI build has no stderr).
func newLogger(dir string) *slog.Logger {
	_ = os.MkdirAll(dir, 0o755)
	var w io.Writer = os.Stderr
	if f, err := os.OpenFile(filepath.Join(dir, "pulsar.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		w = bestEffortWriter{f, os.Stderr}
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// bestEffortWriter is intentionally different from io.MultiWriter: GUI builds
// can inherit an invalid stderr handle, and MultiWriter stops before reaching
// the file sink after the first write error. A log record succeeds when any
// configured sink accepted all of it, while CLI builds still mirror to stderr.
type bestEffortWriter []io.Writer

func (writers bestEffortWriter) Write(p []byte) (int, error) {
	var firstErr error
	wrote := false
	for _, writer := range writers {
		n, err := writer.Write(p)
		if n == len(p) && err == nil {
			wrote = true
			continue
		}
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = io.ErrShortWrite
			}
		}
	}
	if wrote {
		return len(p), nil
	}
	if firstErr == nil {
		firstErr = io.ErrClosedPipe
	}
	return 0, firstErr
}

func configureCrashOutput(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "pulsar-crash.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return debug.SetCrashOutput(file, debug.CrashOptions{})
}

func main() {
	pairCode := flag.String("pair", "", "one-time pairing code from the bot (/pair)")
	coordinator := flag.String("coordinator", "https://barycenter.relux.works", "coordinator base URL (pairing only)")
	configDir := flag.String("config-dir", "", `override the config directory (default: %APPDATA%\Pulsar)`)
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("pulsar-win", version)
		return
	}

	dir := *configDir
	if dir == "" {
		var err error
		dir, err = DefaultConfigDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config dir:", err)
			os.Exit(1)
		}
	}

	// The GUI build has no console (-H windowsgui), so stderr goes nowhere —
	// log to a file in the config dir (and to stderr too, for CLI runs).
	log := newLogger(dir)
	if err := configureCrashOutput(dir); err != nil {
		log.Error("configure crash output", "err", err)
	}

	if *pairCode != "" {
		creds, err := Pair(*coordinator, *pairCode)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := creds.Save(dir); err != nil {
			log.Error("save credentials", "err", err)
			os.Exit(1)
		}
		fmt.Printf("сопряжение готово: орбит %d, слот %s\nучётные данные: %s\n",
			creds.OrbitID, creds.Slot, filepath.Join(dir, protectedCredentialsFileName))
		return
	}

	run(dir, *coordinator, log)
}

func run(dir, coordinatorBase string, log *slog.Logger) {
	cfg, err := LoadConfig(dir)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if err := ValidateConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	creds, err := LoadCredentials(dir)
	if err != nil {
		log.Error("credentials", "err", err)
		os.Exit(1)
	}
	if creds == nil {
		// Unpaired Windows starts in the full shell rather than forcing a
		// Spotify-only pairing dialog. Create, Join, Try locally, Settings and
		// the honest unavailable states remain reachable; Connect in the tray
		// opens the existing code-entry onboarding window. Non-Windows dev builds
		// keep the CLI fallback.
		paired, supported := runUnpairedShell(dir, coordinatorBase, log)
		if !supported {
			fmt.Fprintln(os.Stderr, "Pulsar не сопряжён с координатором.")
			fmt.Fprintln(os.Stderr, "Получи код у бота (/pair) и запусти:")
			fmt.Fprintln(os.Stderr, "  pulsar-win.exe --pair КОД")
			os.Exit(2)
		}
		if !paired {
			return
		}
		creds, err = LoadCredentials(dir)
		if err != nil || creds == nil {
			log.Error("paired credentials unavailable after shell onboarding", "err", err)
			return
		}
	}
	if err := ValidateCredentials(*creds); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	stop := make(chan struct{})
	ring := NewRing(cfg.RingBufferMS * sampleRate * channels / 1000)
	gain := NewGain()
	engine := NewEngine(ring, gain)

	// Daemon supervision: bundled go-librespot.exe beside our own binary.
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "Pulsar " + strings.ToUpper(creds.Slot)
	}
	var sup *Supervisor
	if binary, err := DefaultLibrespotBinary(); err == nil {
		sup = NewSupervisor(binary, filepath.Join(dir, "librespot"), log)
	} else {
		log.Error("librespot binary path", "err", err)
		os.Exit(1)
	}

	daemon := NewDaemonClient(cfg.APIPort)
	capabilities := []string{protocol.CapabilitySeamlessAdoption}
	var mediaClips *MediaClipClient
	mediaMixer := NewWindowsOverlayMediaClipMixer(engine)
	mediaFetcher, mediaErr := NewAuthenticatedMediaClipFetcher(
		filepath.Join(cfg.CacheDir, "media-clips"), creds.Token, creds.WSURL)
	if mediaErr != nil {
		// Keep the existing player usable, but do not claim media_clip_v1 when
		// this installation cannot create its authenticated cache.
		log.Error("media clip hooks unavailable")
	} else {
		mediaClips = NewMediaClipClient(mediaFetcher, mediaMixer, log, nil)
		capabilities = append(capabilities, mediaClips.AdvertisedCapabilities()...)
	}
	ws := NewWSClient(creds.WSURL, Identity{
		NodeID:           creds.Slot,
		Token:            creds.Token,
		AppVersion:       version,
		LibrespotVersion: sup.Version,
		Capabilities:     capabilities,
	}, log)

	cache, err := NewVoiceCache(cfg.CacheDir, creds.Token, 0 /* default 2 GiB */, log)
	if err != nil {
		// Voice inserts degrade to media_download_failed errors; music works.
		log.Error("voice cache unavailable")
	}

	player := NewPlayer(daemon, ring, engine, cache, ws.Clock(), ws.Send, cfg.OutputLatencyOffsetMS, log)
	outputControl := newWindowsAudioOutputController(engine, player, log)
	mediaMixer.BindInterruptController(player)
	workflow := NewWindowsCaptureWorkflowController(NewWindowsRecordingController(nil), nil, nil)
	if configured, captureErr := newWindowsCaptureWorkflow(dir, mediaMixer, gain); captureErr != nil {
		log.Error("local capture workflow unavailable", "err", captureErr)
	} else {
		workflow = configured
	}
	workflow.ConfigureCaptureQuality(WindowsCaptureQualityRequest{
		Mode: WindowsCaptureQualityAuto, ProcessingRequested: true,
	}, func(state *protocol.CaptureQualityState) {
		if err := player.SetCaptureQualityState(state); err != nil {
			log.Error("capture quality state rejected")
		}
	})
	var phaseOne *WindowsPhaseOneComposition
	if configured, phaseErr := newProductionWindowsPhaseOneComposition(dir, workflow); phaseErr != nil {
		log.Error("Phase 1 app data unavailable")
	} else {
		phaseOne = configured
	}
	var soundboard *WindowsSoundboardComposition
	if configured, soundboardErr := newProductionWindowsSoundboardComposition(dir, workflow); soundboardErr != nil {
		log.Error("Windows soundboard unavailable")
	} else {
		soundboard = configured
	}
	var automationAdmin *WindowsAutomationAdmin
	if configured, automationErr := newProductionWindowsAutomationAdmin(dir); automationErr != nil {
		log.Error("Windows automation administration unavailable")
	} else {
		automationAdmin = configured
	}
	var airs *WindowsAirComposition
	if configured, airErr := newProductionWindowsAirComposition(dir); airErr != nil {
		log.Error("Air app data unavailable")
	} else {
		airs = configured
	}
	var identityRecovery *WindowsIdentityComposition
	if configured, identityErr := newProductionWindowsIdentityComposition(dir, coordinatorBase, nil); identityErr != nil {
		log.Error("Windows recovery export unavailable")
	} else {
		identityRecovery = configured
	}
	var targetsInbox *WindowsTargetsInboxComposition
	if configured, targetsErr := newProductionWindowsTargetsInboxComposition(dir, phaseOne); targetsErr != nil {
		log.Error("Phase 2 targets and inbox unavailable")
	} else {
		targetsInbox = configured
	}
	var streamTracks *WindowsStreamTrackComposition
	if configured, trackErr := newProductionWindowsStreamTrackComposition(dir, targetsInbox); trackErr != nil {
		log.Error("Phase 2 streamed-track UI unavailable")
	} else {
		streamTracks = configured
	}
	presenceStore := NewNodePresenceStore(filepath.Join(dir, "node-presence.v1.json"), log)
	player.ConfigureTransmissionHooks(mediaClips, presenceStore)
	player.Start()
	events := NewEventsClient(cfg.APIPort, player.HandleLibrespotEvent, log)
	sup.OnCrash = func() {
		ws.Send(protocol.TypeError, &protocol.ErrorPayload{
			Code: "librespot_restart", Message: "daemon exited, supervisor restarting",
		})
	}
	ws.OnMessage = func(env protocol.Envelope, payload any) {
		if w, ok := payload.(*protocol.WelcomePayload); ok {
			ws.MarkHealthy() // confirmed healthy exchange (spec 8.6)
			player.ApplyWelcome(w)
			return
		}
		player.Handle(env, payload)
	}
	ws.StateProvider = func() protocol.StatePayload {
		return player.StatePayload(ws.Clock().LastRTTMS())
	}
	ws.OnConnected = player.ResendLocalDND

	if err := sup.Start(deviceName, cfg.APIPort, cfg.PipeName); err != nil {
		// Protocol-level dev runs stay useful without the daemon; a real
		// install treats this as broken and the log says exactly why.
		log.Error("librespot supervisor not started", "err", err)
	}
	if err := startAudio(cfg.PipeName, ring, engine, player, log, stop); err != nil {
		// Same policy for the audio legs (always errors on non-Windows dev
		// builds via the stub). TODO(windows): decide fatal-vs-degraded after
		// the first hardware run.
		log.Error("audio not started", "err", err)
	}
	events.Start()
	ws.Start()
	log.Info("pulsar-win running", "version", version, "slot", creds.Slot,
		"orbit", creds.OrbitID, "device_name", deviceName, "pipe", cfg.PipeName)

	// Shutdown blocker: the tray message loop on Windows, an OS signal on dev
	// builds. Both end when the user quits.
	osSig := make(chan os.Signal, 1)
	signal.Notify(osSig, os.Interrupt, syscall.SIGTERM)
	quit := make(chan struct{})
	go func() { <-osSig; close(quit) }()

	var shellStateMu sync.RWMutex
	shellDND := ShellDNDAllowAll
	if current := presenceStore.CurrentLocalDND(); current != nil {
		shellDND = ShellDND(current.Mode)
	}
	shell := NewWindowsShell(preferredWindowsShellLocale(), func() ShellSnapshot {
		state := player.StatePayload(ws.Clock().LastRTTMS())
		connection := ShellReconnecting
		if ws.Healthy() {
			connection = ShellOnline
		}
		if state.Degraded {
			connection = ShellDegraded
		}
		route := ""
		outputs, selectedOutput := outputControl.Snapshot()
		if len(state.Speakers) > 0 {
			route = state.Speakers[0].Name
		}
		if selectedOutput >= 0 && selectedOutput < len(outputs) {
			route = outputs[selectedOutput].Name
		}
		nowPlaying := ""
		if state.URI != nil {
			nowPlaying = *state.URI
		}
		presenceOnline, presenceTotal, presenceReady := 0, 0, 0
		presenceAvailable := false
		if presence := player.LatestPresence(); presence != nil {
			presenceAvailable = true
			presenceTotal = len(presence.Nodes)
			for _, node := range presence.Nodes {
				if node.Online {
					presenceOnline++
					if node.OutputState == "ready" {
						presenceReady++
					}
				}
			}
		}
		shellStateMu.RLock()
		dnd := shellDND
		shellStateMu.RUnlock()
		recordingState, recordingAvailable := workflow.Snapshot()
		local := workflow.LocalSnapshot()
		snapshot := ShellSnapshot{
			Connection: connection, Identity: identityLine(*creds),
			PresenceOnline: presenceOnline, PresenceTotal: presenceTotal, PresenceReady: presenceReady,
			PresenceAvailable: presenceAvailable, RouteName: route,
			NowPlaying: nowPlaying, PlaybackState: state.Playback,
			DND: dnd, Recording: recordingState,
			RecordingAvailable:             recordingAvailable,
			RecordingShortcut:              currentWindowsRecordingShortcutStatus(),
			RecordingShortcutKey:           currentWindowsRecordingShortcut(),
			CaptureQualityMode:             local.CaptureQualityMode,
			CaptureQualityDegradedConsent:  local.CaptureQualityDegradedConsent,
			CaptureQualityBackendAvailable: local.CaptureQualityBackendAvailable,
			CaptureQualityState:            local.CaptureQualityState,
			SelfTestAvailable:              local.Available, SelfTestPhase: local.SelfTestPhase,
			SelfTestMeter: local.Meter, LocalDraftAvailable: local.DraftAvailable,
			LocalDraftName: local.DraftName, LocalFailure: local.Failure,
			RecordingDraftAvailable: local.RecordingDraftAvailable,
			CaptureInputs:           local.Inputs, SelectedCaptureInput: local.SelectedInput,
			AudioOutputs: outputs, SelectedAudioOutput: selectedOutput,
			Volume: state.Volume, IdentityOperation: ShellIdentityActive,
		}
		if phaseOne != nil {
			phaseOne.ApplyShellSnapshot(&snapshot)
		}
		if soundboard != nil {
			soundboard.ApplyShellSnapshot(&snapshot, currentWindowsSoundboardShortcutStates())
		} else {
			snapshot.SoundboardFailure = "credential_unavailable"
		}
		if automationAdmin != nil {
			automationAdmin.ApplyShellSnapshot(&snapshot)
		} else {
			snapshot.Automation.Failure = "credential_unavailable"
		}
		if airs != nil {
			airs.ApplyShellSnapshot(&snapshot)
		} else {
			snapshot.AirFailure = "credential_unavailable"
		}
		if identityRecovery != nil {
			identityRecovery.ApplyShellSnapshot(&snapshot)
		}
		if targetsInbox != nil {
			targetsInbox.ApplyShellSnapshot(&snapshot)
		} else {
			snapshot.TargetsInbox = TargetsInboxSnapshot{State: TargetsInboxCoordinatorError, StateLabel: targetsStateLabel(TargetsInboxCoordinatorError)}
			snapshot.TargetsInboxFailure = "credential_unavailable"
		}
		if streamTracks != nil {
			streamTracks.ApplyShellSnapshot(&snapshot)
		} else {
			snapshot.StreamTrack = StreamTrackSnapshot{State: TargetsInboxCoordinatorError, Failure: StreamTrackServiceUnavailable}
			snapshot.StreamTrackOutcome = "credential_unavailable"
		}
		return snapshot
	}, ShellActions{
		SaveRecovery: func(path string) {
			if identityRecovery != nil {
				identityRecovery.SaveRecovery(path)
			}
		},
		TryLocally:         workflow.TryLocally,
		PlayBuiltinCue:     workflow.PlayBuiltinCue,
		ChooseLocalFile:    func() { workflow.ChooseFile(currentMainWindowOwner()) },
		ChooseOutgoingFile: func() { workflow.ChooseOutgoingFile(currentMainWindowOwner()) },
		AcceptDroppedFile:  workflow.AcceptBrokeredFile,
		DeleteLocalDraft:   workflow.DeleteLocalDraft,
		SelectNextInput:    workflow.SelectNextInput,
		SelectNextOutput:   func() { go outputControl.SelectNext() },
		ToggleRecording:    workflow.Toggle,
		CancelRecording:    workflow.Cancel,
		SetCaptureQuality:  workflow.SetCaptureQuality,
		StopActiveCapture:  workflow.Cancel,
		SetDND: func(mode ShellDND) {
			if mode != ShellDNDAllowAll && mode != ShellDNDMessagesOnly {
				return
			}
			if err := player.SetLocalDND(string(mode), nil); err != nil {
				log.Error("shell DND update failed", "mode", mode)
				return
			}
			shellStateMu.Lock()
			shellDND = mode
			shellStateMu.Unlock()
		},
		SendSelectedDraft: func() {
			if phaseOne != nil {
				phaseOne.SendSelectedDraft()
			}
		},
		DeleteSelectedDraft: func() {
			if phaseOne != nil {
				phaseOne.DeleteSelectedDraft()
			}
		},
		SelectNextPhaseOneDraft: func() {
			if phaseOne != nil {
				phaseOne.SelectNextDraft()
			}
		},
		SelectNextPhaseOneRoute: func() {
			if phaseOne != nil {
				phaseOne.SelectNextRoute()
			}
		},
		SelectNextPhaseOneDelivery: func() {
			if phaseOne != nil {
				phaseOne.SelectNextDelivery()
			}
		},
		SelectNextHistoryItem: func() {
			if phaseOne != nil {
				phaseOne.SelectNextHistory()
			}
		},
		SelectNextReportReason: func() {
			if phaseOne != nil {
				phaseOne.SelectNextReportReason()
			}
		},
		DeleteSelectedHistoryItem: func() {
			if phaseOne != nil {
				phaseOne.DeleteSelectedHistoryItem()
			}
		},
		ReportSelectedHistoryItem: func(details string) {
			if phaseOne != nil {
				phaseOne.ReportSelectedHistoryItem(details)
			}
		},
		ReplaySelectedHistoryItem: func() {
			if phaseOne != nil {
				phaseOne.ReplaySelectedHistoryItem()
			}
		},
		BlockSelectedHistoryActor: func() {
			if phaseOne != nil {
				phaseOne.BlockSelectedHistoryActor()
			}
		},
		SelectNextSoundboardCue: func() {
			if soundboard != nil {
				soundboard.SelectNextCue()
			}
		},
		TriggerSelectedSoundboardCue: func() {
			if soundboard != nil {
				soundboard.TriggerSelected()
			}
		},
		SelectNextSoundboardRoute: func() {
			if soundboard != nil {
				soundboard.SelectNextRoute()
			}
		},
		SelectNextSoundboardDelivery: func() {
			if soundboard != nil {
				soundboard.SelectNextDelivery()
			}
		},
		ToggleSoundboardIncludeOrigin: func() {
			if soundboard != nil {
				soundboard.ToggleIncludeOrigin()
			}
		},
		DeleteSelectedSoundboardCue: func() {
			if soundboard != nil {
				soundboard.DeleteSelected()
			}
		},
		MoveSelectedSoundboardCue: func(delta int) {
			if soundboard != nil {
				soundboard.MoveSelected(delta)
			}
		},
		CycleSelectedSoundboardShortcut: func() {
			if soundboard != nil {
				soundboard.CycleSelectedShortcut()
			}
		},
		ChooseSoundboardFile: func() {
			if soundboard != nil {
				workflow.ChooseSoundboardFile(currentMainWindowOwner(), soundboard.AcceptBrokeredCue)
			}
		},
		RenameSelectedSoundboardCue: func(title string) {
			if soundboard != nil {
				soundboard.RenameSelected(title)
			}
		},
		RefreshTargetsInbox: func() {
			if targetsInbox != nil {
				targetsInbox.Refresh()
			}
		},
		SelectNextTargetAudience: func() {
			if targetsInbox != nil {
				targetsInbox.SelectNextAudience()
			}
		},
		SelectNextTarget: func() {
			if targetsInbox != nil {
				targetsInbox.SelectNextTarget()
			}
		},
		ToggleSelectedTarget: func() {
			if targetsInbox != nil {
				targetsInbox.ToggleSelectedTarget()
			}
		},
		ToggleTargetIncludeOrigin: func() {
			if targetsInbox != nil {
				targetsInbox.ToggleIncludeOrigin()
			}
		},
		SelectNextTargetsDelivery: func() {
			if targetsInbox != nil {
				targetsInbox.SelectNextDelivery()
			}
		},
		SendTargetsDraft: func() {
			if targetsInbox != nil {
				targetsInbox.SendSelectedDraft()
			}
		},
		SelectNextInboxItem: func() {
			if targetsInbox != nil {
				targetsInbox.SelectNextInbox()
			}
		},
		ReplaySelectedInbox: func() {
			if targetsInbox != nil {
				targetsInbox.ReplaySelectedInbox()
			}
		},
		DismissSelectedInbox: func() {
			if targetsInbox != nil {
				targetsInbox.DismissSelectedInbox()
			}
		},
		ReportSelectedInbox: func(details string) {
			if targetsInbox != nil {
				targetsInbox.ReportSelectedInbox(details)
			}
		},
		MuteSelectedInbox: func() {
			if targetsInbox != nil {
				targetsInbox.MuteSelectedInbox()
			}
		},
		LoadMoreInbox: func() {
			if targetsInbox != nil {
				targetsInbox.LoadMoreInbox()
			}
		},
		SelectNextTargetsHistory: func() {
			if targetsInbox != nil {
				targetsInbox.SelectNextHistory()
			}
		},
		DeleteSelectedTargetsHistory: func() {
			if targetsInbox != nil {
				targetsInbox.DeleteSelectedHistory()
			}
		},
		ReportSelectedTargetsHistory: func(details string) {
			if targetsInbox != nil {
				targetsInbox.ReportSelectedHistory(details)
			}
		},
		MuteSelectedTargetsHistory: func() {
			if targetsInbox != nil {
				targetsInbox.MuteSelectedHistory()
			}
		},
		LoadMoreTargetsHistory: func() {
			if targetsInbox != nil {
				targetsInbox.LoadMoreHistory()
			}
		},
		LoadMoreTargetReceipts: func() {
			if targetsInbox != nil {
				targetsInbox.LoadMoreReceipts()
			}
		},
		SelectNextTargetsReason: func() {
			if targetsInbox != nil {
				targetsInbox.SelectNextReason()
			}
		},
		ChooseStreamTrackFile: func() {
			if streamTracks != nil {
				workflow.ChooseStreamTrackFile(currentMainWindowOwner(), streamTracks.AcceptBrokeredFile)
			}
		},
		AcceptDroppedStreamTrack: func(file WindowsBrokeredAudioFile) {
			if streamTracks != nil {
				streamTracks.AcceptBrokeredFile(file)
			} else if file.Release != nil {
				file.Release()
			}
		},
		RefreshStreamTrack: func() {
			if streamTracks != nil {
				streamTracks.Refresh()
			}
		},
		AcceptStreamTrackPolicy: func() {
			if streamTracks != nil {
				streamTracks.AcceptPolicy()
			}
		},
		UploadStreamTrack: func() {
			if streamTracks != nil {
				streamTracks.Upload()
			}
		},
		DeleteStreamTrack: func(confirmed bool) {
			if streamTracks != nil {
				streamTracks.Delete(confirmed)
			}
		},
		SelectNextStreamTrackAudience: func() {
			if streamTracks != nil {
				streamTracks.SelectNextAudience()
			}
		},
		SelectNextStreamTrackTarget: func() {
			if streamTracks != nil {
				streamTracks.SelectNextTarget()
			}
		},
		ToggleStreamTrackTarget: func() {
			if streamTracks != nil {
				streamTracks.ToggleSelectedTarget()
			}
		},
		SelectNextStreamTrackInsertion: func() {
			if streamTracks != nil {
				streamTracks.SelectNextInsertion()
			}
		},
		RetryStreamTrack: func() {
			if streamTracks != nil {
				streamTracks.Retry()
			}
		},
		SelectNextAir: func() {
			if airs != nil {
				airs.SelectNextAir()
			}
		},
		CreateAir: func(title string) {
			if airs != nil {
				airs.Create(title)
			}
		},
		ConsumeAirInvite: func(code string) {
			if airs != nil {
				airs.ConsumeInvite(code)
			}
		},
		ConfirmAirJoin: func(activate bool) {
			if airs != nil {
				airs.ConfirmJoin(activate)
			}
		},
		DeclineAirJoin: func() {
			if airs != nil {
				airs.DeclineJoin()
			}
		},
		SelectNextAirInviteRole: func() {
			if airs != nil {
				airs.SelectNextInviteRole()
			}
		},
		IssueAirInvite: func() {
			if airs != nil {
				airs.IssueInvite()
			}
		},
		CopyAirInvite: func() {
			if airs != nil {
				if copyAirInviteToClipboard(airs.InviteCode()) {
					airs.setOutcome("clipboard_copied")
				} else {
					airs.setFailure("clipboard_failed")
				}
			}
		},
		HideAirInvite: func() {
			if airs != nil {
				airs.HideInvite()
			}
		},
		WithdrawAirInvite: func() {
			if airs != nil {
				airs.WithdrawInvite()
			}
		},
		RequestAirActivation: func() {
			if airs != nil {
				airs.RequestActivate()
			}
		},
		RequestAirLeave: func() {
			if airs != nil {
				airs.RequestLeave()
			}
		},
		RequestAirDissolve: func() {
			if airs != nil {
				airs.RequestDissolve()
			}
		},
		CycleAirPolicy: func() {
			if airs != nil {
				airs.CyclePolicy()
			}
		},
		ConfirmAirDisruptive: func() {
			if airs != nil {
				airs.ConfirmDisruptive()
			}
		},
		CancelAirDisruptive: func() {
			if airs != nil {
				airs.CancelDisruptive()
			}
		},
		RefreshAutomation: func() {
			if automationAdmin != nil {
				automationAdmin.Refresh()
			}
		},
		SelectNextAutomationSchedule: func() {
			if automationAdmin != nil {
				automationAdmin.SelectNextSchedule()
			}
		},
		SaveAutomationSchedule: func(name, timezone, weekdays, localTime, quiet string) {
			if automationAdmin != nil {
				automationAdmin.SaveSchedule(name, timezone, weekdays, localTime, quiet)
			}
		},
		RequestAutomationAction: func(action string) {
			if automationAdmin != nil {
				automationAdmin.Request(action)
			}
		},
		ConfirmAutomationAction: func(principalName string) {
			if automationAdmin != nil {
				automationAdmin.Confirm(principalName)
			}
		},
		CancelAutomationConfirmation: func() {
			if automationAdmin != nil {
				automationAdmin.CancelConfirmation()
			}
		},
		SelectNextAutomationPrincipal: func() {
			if automationAdmin != nil {
				automationAdmin.SelectNextPrincipal()
			}
		},
		SelectNextAutomationHistory: func() {
			if automationAdmin != nil {
				automationAdmin.SelectNextHistory()
			}
		},
		SaveAutomationFeature: func(timezone, quiet string) {
			if automationAdmin != nil {
				automationAdmin.SaveFeature(timezone, quiet)
			}
		},
		CopyAutomationSecret: func() {
			if automationAdmin != nil {
				automationAdmin.CopySecret()
			}
		},
		HideAutomationSecret: func() {
			if automationAdmin != nil {
				automationAdmin.HideSecret()
			}
		},
	})

	shortcutStore := WindowsRecordingShortcutStore{Path: filepath.Join(dir, "recording-shortcut.v1.json")}
	tray := &TrayState{
		Shell:         shell,
		Recording:     workflow,
		Shortcut:      shortcutStore.Load(),
		ShortcutStore: shortcutStore,
		Log:           log,
		Soundboard:    soundboard,
		Connected:     func() bool { return ws.Healthy() },
		Identity:      identityLine(*creds),
		OnRePair: func() {
			// Best-effort re-pair (F3): collect a fresh code, save, and exit so
			// the app relaunches paired. In-place restart is a follow-up
			// (UIPROBE). Off Windows this path is unreachable.
			if c, e := showOnboardingWindow(dir, coordinatorBase); e == nil {
				_ = c
				// L5: a bare os.Exit orphaned go-librespot.exe — it kept the
				// API port and the named pipe, so the user's relaunch found
				// the pipe busy and the supervisor restart-cycled. Stop the
				// daemon and the player before exiting.
				sup.Stop()
				player.Close()
				os.Exit(0)
			}
		},
		OnQuit: func() { close(quit) },
	}
	if soundboard != nil {
		tray.SoundboardPreferences = soundboard.Preferences()
	}
	awaitShutdown(tray, quit)
	if streamTracks != nil {
		streamTracks.Close()
	}
	if targetsInbox != nil {
		targetsInbox.Close()
	}
	if phaseOne != nil {
		phaseOne.Close()
	}
	if soundboard != nil {
		soundboard.Close()
	}
	if automationAdmin != nil {
		automationAdmin.Close()
	}
	if airs != nil {
		airs.Close()
	}
	if identityRecovery != nil {
		identityRecovery.Close()
	}
	workflow.Shutdown()
	drainContext, cancelRecordingDrain := context.WithTimeout(context.Background(), 5*time.Second)
	if drainErr := workflow.Wait(drainContext); drainErr != nil {
		log.Error("microphone recording shutdown drain failed", "err", drainErr)
	}
	cancelRecordingDrain()
	workflow.ClosePlatform()

	log.Info("shutting down")
	close(stop)
	events.Stop()
	ws.Stop()
	sup.Stop()
	outputControl.Close()
	player.Close()
	gain.Close()
}
