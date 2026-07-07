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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	protocol "relux.works/duet/pulsar-win/wire"
)

var version = "dev" // set via -ldflags "-X main.version=..."

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

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dir := *configDir
	if dir == "" {
		var err error
		dir, err = DefaultConfigDir()
		if err != nil {
			log.Error("config dir", "err", err)
			os.Exit(1)
		}
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
			creds.OrbitID, creds.Slot, filepath.Join(dir, credentialsFileName))
		return
	}

	run(dir, log)
}

func run(dir string, log *slog.Logger) {
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
		// Unpaired is a legal state with a human answer, not a stack trace (R1).
		fmt.Fprintln(os.Stderr, "Pulsar не сопряжён с координатором.")
		fmt.Fprintln(os.Stderr, "Получи код у бота (/pair) и запусти:")
		fmt.Fprintln(os.Stderr, "  pulsar-win.exe --pair КОД")
		os.Exit(2)
	}
	if err := ValidateCredentials(*creds); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	stop := make(chan struct{})
	ring := NewRing(cfg.RingBufferMS * sampleRate * channels / 1000)

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
	ws := NewWSClient(creds.WSURL, Identity{
		NodeID:           creds.Slot,
		Token:            creds.Token,
		AppVersion:       version,
		LibrespotVersion: sup.Version,
	}, log)

	gain := NewGain()
	engine := NewEngine(ring, gain)
	cache, err := NewVoiceCache(cfg.CacheDir, creds.Token, 0 /* default 2 GiB */, log)
	if err != nil {
		// Voice inserts degrade to media_download_failed errors; music works.
		log.Error("voice cache unavailable", "err", err)
	}

	player := NewPlayer(daemon, ring, engine, cache, ws.Clock(), ws.Send, cfg.OutputLatencyOffsetMS, log)
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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")
	close(stop)
	events.Stop()
	ws.Stop()
	sup.Stop()
	player.Close()
	gain.Close()
}
