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
		w = io.MultiWriter(os.Stderr, f)
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
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
		paired, supported := runUnpairedShell(dir, coordinatorBase)
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
	mediaMixer.BindInterruptController(player)
	recording := NewWindowsRecordingController(nil)
	if backend, captureErr := NewNativeWindowsMicrophoneBackend(); captureErr != nil {
		log.Error("microphone recording unavailable", "err", captureErr)
	} else if executable, executableErr := os.Executable(); executableErr != nil {
		log.Error("microphone recording unavailable", "err", executableErr)
	} else {
		cuePath := filepath.Join(filepath.Dir(executable), "Assets", "Audio", BuiltinRecordingCueFilename)
		if cueData, cueErr := os.ReadFile(cuePath); cueErr != nil || !ValidateBuiltinRecordingCue(cueData) {
			log.Error("microphone recording unavailable", "err", ErrRecordingCueUnavailable)
		} else {
			captureStore := NewCaptureMediaStore(filepath.Join(dir, "capture-media"))
			if _, recoveryErr := captureStore.Recover(); recoveryErr != nil {
				log.Error("microphone recording recovery failed", "err", recoveryErr)
			} else {
				localOutput := NewWindowsProductionLocalClipOutput(mediaMixer)
				captureService := NewWindowsMicrophoneCaptureService(
					backend, captureStore,
					WindowsLocalRecordingCuePlayer{Output: localOutput, CuePath: cuePath}, gain)
				recording = NewWindowsRecordingController(windowsMicrophoneRecordingCapture{service: captureService})
			}
		}
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
		if len(state.Speakers) > 0 {
			route = state.Speakers[0].Name
		}
		nowPlaying := ""
		if state.URI != nil {
			nowPlaying = *state.URI
		}
		presenceOnline, presenceTotal := 0, 0
		presenceAvailable := false
		if presence := player.LatestPresence(); presence != nil {
			presenceAvailable = true
			presenceTotal = len(presence.Nodes)
			for _, node := range presence.Nodes {
				if node.Online {
					presenceOnline++
				}
			}
		}
		shellStateMu.RLock()
		dnd := shellDND
		shellStateMu.RUnlock()
		recordingState, recordingAvailable := recording.Snapshot()
		return ShellSnapshot{
			Connection: connection, Identity: identityLine(*creds),
			PresenceOnline: presenceOnline, PresenceTotal: presenceTotal,
			PresenceAvailable: presenceAvailable, RouteName: route,
			NowPlaying: nowPlaying, PlaybackState: state.Playback,
			DND: dnd, Recording: recordingState,
			RecordingAvailable:   recordingAvailable,
			RecordingShortcut:    currentWindowsRecordingShortcutStatus(),
			RecordingShortcutKey: currentWindowsRecordingShortcut(),
			SelfTestAvailable:    false, Volume: state.Volume,
		}
	}, ShellActions{
		Create:          func() { openURL(uiBotURL) },
		Join:            func() { openURL(uiBotURL) },
		ToggleRecording: recording.Toggle,
		CancelRecording: recording.Cancel,
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
	})

	shortcutStore := WindowsRecordingShortcutStore{Path: filepath.Join(dir, "recording-shortcut.v1.json")}
	tray := &TrayState{
		Shell:         shell,
		Recording:     recording,
		Shortcut:      shortcutStore.Load(),
		ShortcutStore: shortcutStore,
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
	awaitShutdown(tray, quit)
	recording.Shutdown()
	drainContext, cancelRecordingDrain := context.WithTimeout(context.Background(), 5*time.Second)
	if drainErr := recording.Wait(drainContext); drainErr != nil {
		log.Error("microphone recording shutdown drain failed", "err", drainErr)
	}
	cancelRecordingDrain()

	log.Info("shutting down")
	close(stop)
	events.Stop()
	ws.Stop()
	sup.Stop()
	player.Close()
	gain.Close()
}
