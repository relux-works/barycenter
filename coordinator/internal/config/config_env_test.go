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
	t.Setenv("DUET_PROVIDERS", "1")

	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Providers {
		t.Fatal("DUET_PROVIDERS=1 must enable the provider layer")
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

// v2.1: empty tokens are legal (new installs mint everything via /create +
// /pair) — the container config boots with zero env. Garbage tokens still die.
func TestContainerConfigWithoutEnv(t *testing.T) {
	yml := "listen: \"0.0.0.0:8080\"\ndb_path: /tmp/d.db\nmedia_dir: /tmp/m\nnodes:\n  a: { token: \"\" }\n  b: { token: \"\" }\n"
	p := filepath.Join(t.TempDir(), "c.yml")
	os.WriteFile(p, []byte(yml), 0o600)
	if _, err := Load(p); err != nil {
		t.Fatalf("empty tokens must be legal in v2.1: %v", err)
	}
	bad := strings.Replace(yml, "a: { token: \"\" }", "a: { token: \"short\" }", 1)
	os.WriteFile(p, []byte(bad), 0o600)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "64 hex") {
		t.Fatalf("garbage token must fail loudly, got %v", err)
	}
}
