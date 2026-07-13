package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is copied at runtime into the exact previous HEAD source tree by
// the current tagged integration test. It proves the predecessor can boot the
// rollback-neutral config while ignoring the current-only environment flag,
// and that persisting the current-only flag in YAML would break rollback.
func TestPreviousHeadRollbackConfigBootstrapContract(t *testing.T) {
	const neutral = `listen: "127.0.0.1:18080"
db_path: /tmp/previous-head.db
media_dir: /tmp/previous-head-media
log_level: warn
`
	dir := t.TempDir()
	path := filepath.Join(dir, "coordinator.yml")
	if err := os.WriteFile(path, []byte(neutral), 0o600); err != nil {
		t.Fatal(err)
	}

	// The old binary has no knowledge of this variable and must continue to
	// load the neutral file when a rollback starts before the environment has
	// been fully drained.
	t.Setenv("DUET_SELF_SERVICE_ONBOARDING", "1")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("predecessor rejected rollback-neutral config: %v", err)
	}
	if cfg.Listen != "127.0.0.1:18080" || cfg.DBPath != "/tmp/previous-head.db" ||
		cfg.MediaDir != "/tmp/previous-head-media" || cfg.LogLevel != "warn" {
		t.Fatalf("predecessor changed neutral config: %+v", cfg)
	}

	withCurrentOnlyKey := neutral + "self_service_onboarding: true\n"
	if err := os.WriteFile(path, []byte(withCurrentOnlyKey), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Load(path)
	if err == nil || !strings.Contains(err.Error(), "self_service_onboarding") {
		t.Fatalf("predecessor accepted current-only YAML key; rollback tripwire error=%v", err)
	}
}
