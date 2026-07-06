// go-librespot supervision — a port of LibrespotSupervisor.swift (spec 6.2
// item 1): render config.yml, run the bundled daemon as a child process,
// restart with exponential backoff 1,2,4..30 s, surface the daemon version
// from its startup log line.
//
// The daemon binary ships beside our exe (go-librespot.exe from the Windows
// fork); its config_dir lives under %APPDATA%\Pulsar\librespot.
package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// RenderLibrespotConfig renders the daemon config (spec A.2). Pure function
// for testability — mirror of the macOS LibrespotConfigRenderer.render, with
// the FIFO path replaced by the Windows named pipe.
func RenderLibrespotConfig(deviceName string, apiPort int, pipePath string) string {
	return fmt.Sprintf(`# Rendered by Pulsar — do not edit; changes are overwritten on start.
device_name: "%s"
device_type: speaker
credentials:
  type: zeroconf
  zeroconf:
    # Confirmed live (spike 2026-07-03): without this the zeroconf
    # session is memory-only and every daemon restart needs the phone.
    persist_credentials: true
server:
  enabled: true
  address: 127.0.0.1
  port: %d
audio_backend: pipe
audio_output_pipe: %s
audio_output_pipe_format: f32le
external_volume: true
`, deviceName, apiPort, pipePath)
}

// DefaultLibrespotBinary is the bundled daemon path: beside our own exe.
func DefaultLibrespotBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve own executable: %w", err)
	}
	name := "go-librespot"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

type Supervisor struct {
	binary    string
	configDir string
	log       *slog.Logger

	// OnCrash is called after each unexpected daemon exit
	// (spec 6.6: the node reports error(librespot_restart)).
	OnCrash func()

	// Backoff bounds are variables so tests can shrink them.
	minBackoff time.Duration
	maxBackoff time.Duration

	mu      sync.Mutex
	cmd     *exec.Cmd
	stopped bool
	version string
}

func NewSupervisor(binary, configDir string, log *slog.Logger) *Supervisor {
	return &Supervisor{
		binary:     binary,
		configDir:  configDir,
		log:        log,
		minBackoff: time.Second,
		maxBackoff: 30 * time.Second,
		stopped:    true,
		version:    "unknown",
	}
}

// Version is parsed from the daemon's "running go-librespot X.Y.Z" line;
// "unknown" until seen.
func (s *Supervisor) Version() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// Start renders config.yml and launches the spawn loop.
func (s *Supervisor) Start(deviceName string, apiPort int, pipeName string) error {
	if _, err := os.Stat(s.binary); err != nil {
		return fmt.Errorf("go-librespot binary not found at %s (must ship beside pulsar-win.exe): %w", s.binary, err)
	}
	if err := os.MkdirAll(s.configDir, 0o755); err != nil {
		return fmt.Errorf("create librespot config dir: %w", err)
	}
	cfg := RenderLibrespotConfig(deviceName, apiPort, pipeName)
	if err := os.WriteFile(filepath.Join(s.configDir, "config.yml"), []byte(cfg), 0o644); err != nil {
		return fmt.Errorf("write librespot config.yml: %w", err)
	}
	s.mu.Lock()
	s.stopped = false
	s.mu.Unlock()
	go s.runLoop()
	return nil
}

// Stop kills the daemon and stops the restart loop.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.stopped = true
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		// TODO(windows-hardening): a graceful shutdown channel (the fork could
		// watch its stdin or an event); Kill = TerminateProcess is abrupt but
		// deterministic, and the daemon persists credentials on disk anyway.
		cmd.Process.Kill()
	}
}

// SoftRestart kills the daemon; the supervisor loop restarts it (spec 6.6
// audio_starvation recovery).
func (s *Supervisor) SoftRestart() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		s.log.Info("librespot soft restart requested")
		cmd.Process.Kill()
	}
}

func (s *Supervisor) runLoop() {
	backoff := s.minBackoff
	for {
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		err := s.runOnce()
		s.mu.Lock()
		stopped := s.stopped
		s.mu.Unlock()
		if stopped {
			return
		}
		if err != nil {
			s.log.Warn("librespot exited", "err", err)
		} else {
			s.log.Warn("librespot exited", "code", 0)
		}
		if s.OnCrash != nil {
			s.OnCrash()
		}
		time.Sleep(backoff)
		backoff = nextBackoff(backoff, s.maxBackoff)
	}
}

// runOnce spawns the daemon and blocks until it exits.
func (s *Supervisor) runOnce() error {
	cmd := exec.Command(s.binary, "--config_dir", s.configDir)

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("spawn %s: %w", s.binary, err)
	}
	pw.Close() // child holds its own copy

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()
	s.log.Info("librespot started", "pid", cmd.Process.Pid)

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		s.log.Debug("librespot", "line", line)
		if v, ok := parseLibrespotVersion(line); ok {
			s.mu.Lock()
			s.version = v
			s.mu.Unlock()
		}
	}
	pr.Close()

	err = cmd.Wait()
	s.mu.Lock()
	if s.cmd == cmd {
		s.cmd = nil
	}
	s.mu.Unlock()
	return err
}

// parseLibrespotVersion extracts X.Y.Z from a "... running go-librespot X.Y.Z"
// log line (same match the macOS supervisor uses).
func parseLibrespotVersion(line string) (string, bool) {
	const marker = "running go-librespot "
	i := strings.Index(line, marker)
	if i < 0 {
		return "", false
	}
	v := strings.TrimSpace(line[i+len(marker):])
	if v == "" {
		return "", false
	}
	return v, true
}

func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}
