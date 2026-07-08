package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalYML = `listen: "0.0.0.0:8080"
db_path: /tmp/d.db
media_dir: /tmp/m
nodes:
  a: { token: "" }
  b: { token: "" }
`

func writeYML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.yml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Ops finding 7.1: log level must be configurable, defaulting to info so prod
// stops running at hardcoded debug.
func TestLogLevelDefaultsToInfo(t *testing.T) {
	cfg, err := Load(writeYML(t, minimalYML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("default log_level = %q, want info", cfg.LogLevel)
	}
	if cfg.SlogLevel() != slog.LevelInfo {
		t.Fatalf("default SlogLevel = %v, want info", cfg.SlogLevel())
	}
}

func TestLogLevelFromYAML(t *testing.T) {
	cfg, err := Load(writeYML(t, minimalYML+"log_level: warn\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlogLevel() != slog.LevelWarn {
		t.Fatalf("yaml log_level warn -> %v", cfg.SlogLevel())
	}
}

// DUET_LOG_LEVEL overrides the yml (Coolify path) and is case-insensitive.
func TestLogLevelEnvOverride(t *testing.T) {
	t.Setenv("DUET_LOG_LEVEL", "DEBUG")
	cfg, err := Load(writeYML(t, minimalYML+"log_level: info\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlogLevel() != slog.LevelDebug {
		t.Fatalf("env DEBUG -> %v, want debug", cfg.SlogLevel())
	}
}

func TestLogLevelAllLevels(t *testing.T) {
	want := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for lvl, exp := range want {
		cfg, err := Load(writeYML(t, minimalYML+"log_level: "+lvl+"\n"))
		if err != nil {
			t.Fatalf("level %q: %v", lvl, err)
		}
		if got := cfg.SlogLevel(); got != exp {
			t.Fatalf("level %q -> %v, want %v", lvl, got, exp)
		}
	}
}

// A garbage level must fail loudly, not silently degrade (the F4-class trap).
func TestLogLevelInvalidRejected(t *testing.T) {
	_, err := Load(writeYML(t, minimalYML+"log_level: verbose\n"))
	if err == nil || !strings.Contains(err.Error(), "log_level") {
		t.Fatalf("invalid log_level must fail with a log_level error, got %v", err)
	}
}
