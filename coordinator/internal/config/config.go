// Package config loads and validates coordinator.yml (spec 7.4, appendix A.3).
// Validation errors must be specific and human-readable (goal DoD-10).
package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Node struct {
	Token string `yaml:"token"`
}

type Telegram struct {
	BotToken string           `yaml:"bot_token"`
	ChatID   int64            `yaml:"chat_id"`
	Users    map[int64]string `yaml:"users"` // telegram user id -> "a"|"b"
}

type Timings struct {
	ReadyTimeoutS   int `yaml:"ready_timeout_s"`
	StartMarginMS   int `yaml:"start_margin_extra_ms"`
	OfflineAfterS   int `yaml:"offline_after_s"`
	NearEndMS       int `yaml:"near_end_ms"`
	HeartbeatEveryS int `yaml:"heartbeat_every_s"`
}

type Media struct {
	MaxVoiceS     int    `yaml:"max_voice_s"`
	RetentionDays int    `yaml:"retention_days"`
	Preset        string `yaml:"preset"`
}

// Spotify Web API app credentials (U10 playlist expansion only; optional —
// without them playlist links get a "configure the Spotify app" reply).
type Spotify struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type Config struct {
	Listen string `yaml:"listen"`
	// PublicURL: external base (https://barycenter.relux.works) used in URLs
	// handed to nodes (media downloads) when running behind a proxy (v2).
	PublicURL string          `yaml:"public_url"`
	DBPath    string          `yaml:"db_path"`
	MediaDir  string          `yaml:"media_dir"`
	Nodes     map[string]Node `yaml:"nodes"`
	Telegram  Telegram        `yaml:"telegram"`
	Timings   Timings         `yaml:"timings"`
	Media     Media           `yaml:"media"`
	Spotify   Spotify         `yaml:"spotify"`
	// Providers is the master switch of the multi-provider layer
	// (docs/spec-providers.md). Off = pre-provider behavior everywhere.
	Providers bool `yaml:"providers"`
}

var hexToken = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %s: %w", path, err)
	}
	cfg := defaults()
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("config: %s is not valid coordinator.yml: %w", path, err)
	}
	applyEnv(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv overrides secrets and per-install values from the environment —
// the v2 production path (Coolify): the image ships a neutral yml, env brings
// the rest. Every var is optional.
func applyEnv(c *Config) {
	set := func(dst *string, key string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	set(&c.Listen, "DUET_LISTEN")
	set(&c.PublicURL, "DUET_PUBLIC_URL")
	set(&c.DBPath, "DUET_DB_PATH")
	set(&c.MediaDir, "DUET_MEDIA_DIR")
	set(&c.Telegram.BotToken, "DUET_TELEGRAM_BOT_TOKEN")
	set(&c.Spotify.ClientID, "DUET_SPOTIFY_CLIENT_ID")
	set(&c.Spotify.ClientSecret, "DUET_SPOTIFY_CLIENT_SECRET")
	// DUET_PROVIDERS=1 turns the provider layer on (spec-providers); any
	// other non-empty value forces it off, unset keeps the yml value.
	if v := os.Getenv("DUET_PROVIDERS"); v != "" {
		c.Providers = v == "1"
	}

	if v := os.Getenv("DUET_NODE_A_TOKEN"); v != "" {
		n := c.Nodes["a"]
		n.Token = v
		if c.Nodes == nil {
			c.Nodes = map[string]Node{}
		}
		c.Nodes["a"] = n
	}
	if v := os.Getenv("DUET_NODE_B_TOKEN"); v != "" {
		n := c.Nodes["b"]
		n.Token = v
		if c.Nodes == nil {
			c.Nodes = map[string]Node{}
		}
		c.Nodes["b"] = n
	}
	if v := os.Getenv("DUET_TELEGRAM_CHAT_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.Telegram.ChatID = id
		}
	}
	// DUET_TELEGRAM_USERS="111111:a,222222:b"
	if v := os.Getenv("DUET_TELEGRAM_USERS"); v != "" {
		users := map[int64]string{}
		for _, pair := range strings.Split(v, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(parts) != 2 {
				continue
			}
			if id, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				users[id] = parts[1]
			}
		}
		if len(users) > 0 {
			c.Telegram.Users = users
		}
	}
}

func defaults() *Config {
	return &Config{
		Timings: Timings{
			ReadyTimeoutS:   8,
			StartMarginMS:   500,
			OfflineAfterS:   12,
			NearEndMS:       400,
			HeartbeatEveryS: 5,
		},
		Media: Media{MaxVoiceS: 180, RetentionDays: 30, Preset: "default"},
	}
}

func (c *Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if c.Listen == "" {
		add("listen is required (e.g. \"100.x.y.z:8080\", the tailscale address)")
	} else if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		add("listen %q is not host:port: %v", c.Listen, err)
	}
	if c.DBPath == "" {
		add("db_path is required (e.g. /var/lib/duet/duet.db)")
	}
	if c.MediaDir == "" {
		add("media_dir is required (e.g. /var/lib/duet/media)")
	}

	// v2.1 M1: node tokens/users in the config are LEGACY seeds — they only
	// feed the one-time orbit #1 bootstrap. New installs run with none and
	// mint everything through /create + /pair. A non-empty token that is not
	// 64-hex is always a mistake.
	for id, n := range c.Nodes {
		if id != "a" && id != "b" {
			add("nodes.%s: unknown node id, only a and b exist in legacy seeding", id)
			continue
		}
		if n.Token != "" && !hexToken.MatchString(n.Token) {
			add("nodes.%s.token must be 64 hex chars (32 random bytes), got %d chars", id, len(n.Token))
		}
	}
	if a, okA := c.Nodes["a"]; okA {
		if b, okB := c.Nodes["b"]; okB && a.Token == b.Token && a.Token != "" {
			add("nodes.a.token and nodes.b.token must differ")
		}
	}
	for uid, home := range c.Telegram.Users {
		if home != "a" && home != "b" {
			add("telegram.users.%d maps to %q, must be \"a\" or \"b\"", uid, home)
		}
	}

	if c.Media.Preset != "default" && c.Media.Preset != "radio" {
		add("media.preset %q is unknown, use \"default\" or \"radio\"", c.Media.Preset)
	}
	if c.Timings.ReadyTimeoutS <= 0 || c.Timings.OfflineAfterS <= 0 || c.Timings.StartMarginMS < 0 {
		add("timings must be positive (ready_timeout_s, offline_after_s) and non-negative (start_margin_extra_ms)")
	}

	if len(problems) > 0 {
		return fmt.Errorf("config invalid:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// TelegramEnabled reports whether the bot should start.
func (c *Config) TelegramEnabled() bool { return c.Telegram.BotToken != "" }
