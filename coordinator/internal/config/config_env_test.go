package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v2: env overrides for secrets (Coolify deployment).
func TestEnvOverrides(t *testing.T) {
	yml := `
listen: "0.0.0.0:8080"
db_path: /tmp/d.db
media_dir: /tmp/m
nodes:
  a: { token: "" }
  b: { token: "" }
telegram:
  bot_token: ""
`
	p := filepath.Join(t.TempDir(), "c.yml")
	os.WriteFile(p, []byte(yml), 0o600)

	t.Setenv("DUET_NODE_A_TOKEN", strings.Repeat("a", 64))
	t.Setenv("DUET_NODE_B_TOKEN", strings.Repeat("b", 64))
	t.Setenv("DUET_PUBLIC_URL", "https://barycenter.relux.works")
	t.Setenv("DUET_TELEGRAM_BOT_TOKEN", "123:ABC")
	t.Setenv("DUET_TELEGRAM_CHAT_ID", "-100500")
	t.Setenv("DUET_TELEGRAM_USERS", "111:a,222:b")

	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Nodes["a"].Token != strings.Repeat("a", 64) {
		t.Fatalf("token a not overridden")
	}
	if cfg.PublicURL != "https://barycenter.relux.works" {
		t.Fatalf("public url = %q", cfg.PublicURL)
	}
	if cfg.Telegram.ChatID != -100500 || cfg.Telegram.Users[111] != "a" || cfg.Telegram.Users[222] != "b" {
		t.Fatalf("telegram env not applied: %+v", cfg.Telegram)
	}
}

// Empty env + empty tokens must fail loudly (Coolify logs show the reason).
func TestContainerConfigFailsWithoutEnv(t *testing.T) {
	yml := "listen: \"0.0.0.0:8080\"\ndb_path: /tmp/d.db\nmedia_dir: /tmp/m\nnodes:\n  a: { token: \"\" }\n  b: { token: \"\" }\n"
	p := filepath.Join(t.TempDir(), "c.yml")
	os.WriteFile(p, []byte(yml), 0o600)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("want loud token error, got %v", err)
	}
}
