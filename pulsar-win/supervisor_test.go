//go:build !windows

// Supervisor tests run the portable process loop against a fake daemon
// script (unix-only shebang; Windows CI covers compilation via GOOS build).
package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRenderLibrespotConfig(t *testing.T) {
	got := RenderLibrespotConfig("Pulsar B", 3679, `\\.\pipe\LOCAL\pulsar-audio`)
	want := `# Rendered by Pulsar — do not edit; changes are overwritten on start.
device_name: "Pulsar B"
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
  port: 3679
audio_backend: pipe
audio_output_pipe: \\.\pipe\LOCAL\pulsar-audio
audio_output_pipe_format: f32le
external_volume: true
`
	if got != want {
		t.Fatalf("render mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestParseLibrespotVersion(t *testing.T) {
	cases := []struct {
		line string
		want string
		ok   bool
	}{
		{"time=x level=info msg=running go-librespot 0.7.4", "0.7.4", true},
		{"running go-librespot 9.9.9-test", "9.9.9-test", true},
		{"something else entirely", "", false},
		{"running go-librespot ", "", false},
	}
	for _, tc := range cases {
		got, ok := parseLibrespotVersion(tc.line)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("parse(%q) = (%q, %v), want (%q, %v)", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNextBackoffDoublesAndCaps(t *testing.T) {
	max := 30 * time.Second
	if d := nextBackoff(time.Second, max); d != 2*time.Second {
		t.Fatalf("1s -> %v, want 2s", d)
	}
	if d := nextBackoff(20*time.Second, max); d != max {
		t.Fatalf("20s -> %v, want cap %v", d, max)
	}
}

func TestSupervisorMissingBinary(t *testing.T) {
	sup := NewSupervisor(filepath.Join(t.TempDir(), "absent"), t.TempDir(), testLogger())
	if err := sup.Start("Pulsar A", 3678, `\\.\pipe\LOCAL\x`); err == nil {
		t.Fatal("missing binary must fail Start")
	}
}

// The full loop: config rendered, daemon spawned, version parsed from its
// output, crash triggers OnCrash and a backoff restart, Stop ends the loop.
func TestSupervisorSpawnRestartStop(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "spawns")
	script := filepath.Join(dir, "fake-librespot")
	// Each run logs the version line and appends one marker line, then exits
	// non-zero -> the supervisor must restart it after the backoff.
	body := "#!/bin/sh\necho \"running go-librespot 9.9.9-test\"\necho run >> \"" + marker + "\"\nexit 3\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	configDir := filepath.Join(dir, "librespot")
	sup := NewSupervisor(script, configDir, testLogger())
	sup.minBackoff = 10 * time.Millisecond
	sup.maxBackoff = 20 * time.Millisecond
	var crashes atomic.Int64
	sup.OnCrash = func() { crashes.Add(1) }

	if err := sup.Start("Pulsar A", 3678, `\\.\pipe\LOCAL\pulsar-audio`); err != nil {
		t.Fatal(err)
	}

	// config.yml must be the exact renderer output.
	cfg, err := os.ReadFile(filepath.Join(configDir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg) != RenderLibrespotConfig("Pulsar A", 3678, `\\.\pipe\LOCAL\pulsar-audio`) {
		t.Fatalf("config.yml is not the renderer output:\n%s", cfg)
	}

	waitFor(t, 5*time.Second, func() bool {
		raw, _ := os.ReadFile(marker)
		return strings.Count(string(raw), "run") >= 2 && // restarted at least once
			sup.Version() == "9.9.9-test" &&
			crashes.Load() >= 1
	}, "supervisor never restarted the daemon / parsed its version")

	sup.Stop()
	time.Sleep(50 * time.Millisecond) // let a possibly in-flight spawn settle
	raw, _ := os.ReadFile(marker)
	before := strings.Count(string(raw), "run")
	time.Sleep(100 * time.Millisecond)
	raw, _ = os.ReadFile(marker)
	if after := strings.Count(string(raw), "run"); after != before {
		t.Fatalf("supervisor kept spawning after Stop: %d -> %d", before, after)
	}
}
