package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLivePTTFlagIsEnvOnlyAndDefaultsOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordinator.yml")
	yml := "listen: 127.0.0.1:8080\ndb_path: /tmp/live.db\nmedia_dir: /tmp/live-media\n"
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LivePTT {
		t.Fatal("live PTT defaulted on")
	}
	t.Setenv("DUET_LIVE_PTT", "1")
	cfg, err = Load(path)
	if err != nil || !cfg.LivePTT {
		t.Fatalf("env flag=%v err=%v", cfg.LivePTT, err)
	}
	if err := ValidatePreviousCoordinatorRollbackYAML(path); err != nil {
		t.Fatalf("env-only flag broke rollback YAML: %v", err)
	}
}
