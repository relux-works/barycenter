package config

import (
	"os"
	"path/filepath"
	"strconv"
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
	t.Setenv("DUET_SELF_SERVICE_ONBOARDING", "1")

	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Providers {
		t.Fatal("DUET_PROVIDERS=1 must enable the provider layer")
	}
	if !cfg.SelfServiceOnboarding {
		t.Fatal("DUET_SELF_SERVICE_ONBOARDING=1 must enable the actor resolver")
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

func TestSelfServiceOnboardingDefaultsOffAndLoadsFromYAML(t *testing.T) {
	yml := `
listen: "0.0.0.0:8080"
db_path: /tmp/d.db
media_dir: /tmp/m
self_service_onboarding: true
`
	p := filepath.Join(t.TempDir(), "c.yml")
	if err := os.WriteFile(p, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SelfServiceOnboarding {
		t.Fatal("self_service_onboarding YAML flag was ignored")
	}

	if err := os.WriteFile(p, []byte(strings.Replace(yml, "self_service_onboarding: true\n", "", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SelfServiceOnboarding {
		t.Fatal("self_service_onboarding must default off")
	}
}

// R8: the flag uses the documented 1/other/unset precedence. Loading must not
// rewrite or normalize the operator's YAML while applying an env override.
func TestSelfServiceOnboardingEnvironmentPrecedenceAndYAMLPreservation(t *testing.T) {
	tests := []struct {
		name      string
		yamlValue bool
		envValue  string
		want      bool
	}{
		{name: "unset preserves YAML true", yamlValue: true, envValue: "", want: true},
		{name: "unset preserves YAML false", yamlValue: false, envValue: "", want: false},
		{name: "one enables over YAML false", yamlValue: false, envValue: "1", want: true},
		{name: "zero disables", yamlValue: true, envValue: "0", want: false},
		{name: "off disables", yamlValue: true, envValue: "off", want: false},
		{name: "non one disables", yamlValue: true, envValue: "true", want: false},
		{name: "invalid disables", yamlValue: true, envValue: "not-a-boolean", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yml := "listen: \"0.0.0.0:8080\"\n" +
				"db_path: /tmp/d.db\n" +
				"media_dir: /tmp/m\n" +
				"self_service_onboarding: " + strconv.FormatBool(tc.yamlValue) + "\n"
			path := filepath.Join(t.TempDir(), "flag.yml")
			if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("DUET_SELF_SERVICE_ONBOARDING", tc.envValue)
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SelfServiceOnboarding != tc.want {
				t.Fatalf("flag = %v, want %v", cfg.SelfServiceOnboarding, tc.want)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != yml {
				t.Fatal("loading the flag rewrote the YAML source")
			}
		})
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
