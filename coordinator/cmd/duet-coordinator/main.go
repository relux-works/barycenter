// duet-coordinator: session owner, queue, scheduler, Telegram bot, media
// (spec ch. 7). Persistence in SQLite (spec 5.3), media over authed HTTP
// (spec 10.3), no public ports — bind to the tailnet address only (spec 17).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/spotify"
	"relux.works/duet/coordinator/internal/store"
)

// rateLimiter is a tiny per-IP sliding-window limiter for the unauthenticated
// /pair endpoint (architecture: no throttle = code-spam / DB-exhaustion vector).
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64
	limit  int
	window int64 // ms
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]int64{}, limit: limit, window: window.Milliseconds()}
}

// allow reports whether this IP may proceed, pruning stale hits.
func (rl *rateLimiter) allow(ip string) bool {
	now := time.Now().UnixMilli()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cut := now - rl.window
	kept := rl.hits[ip][:0]
	for _, t := range rl.hits[ip] {
		if t > cut {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.limit {
		rl.hits[ip] = kept
		return false
	}
	rl.hits[ip] = append(kept, now)
	// Opportunistic map cleanup so idle IPs don't accumulate.
	if len(rl.hits) > 4096 {
		for k, v := range rl.hits {
			if len(v) == 0 || v[len(v)-1] < cut {
				delete(rl.hits, k)
			}
		}
	}
	return true
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimit wraps a handler, rejecting IPs over the limit with 429.
func rateLimit(rl *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// version is stamped by the release build: -ldflags "-X main.version=vX.Y.Z".
var version = "0.1.0-dev"

func main() {
	configPath := flag.String("config", "/etc/duet/coordinator.yml", "path to coordinator.yml")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	// Config snapshot without secrets (goal 3.5).
	log.Info("duet-coordinator starting",
		"version", version,
		"listen", cfg.Listen,
		"db_path", cfg.DBPath,
		"media_dir", cfg.MediaDir,
		"telegram_enabled", cfg.TelegramEnabled(),
		"ready_timeout_s", cfg.Timings.ReadyTimeoutS,
		"offline_after_s", cfg.Timings.OfflineAfterS,
		"media_preset", cfg.Media.Preset,
		"providers", cfg.Providers,
	)

	if err := os.MkdirAll(cfg.MediaDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create media_dir %s: %v\n", cfg.MediaDir, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create db directory for %s: %v\n", cfg.DBPath, err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()

	// Legacy migration (v2.1 M1): env/yml tokens+users become orbit #1 once,
	// so a pre-multi-tenant node keeps working without re-pairing.
	legacyTokens := map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token}
	if o, err := st.BootstrapLegacyOrbit(legacyTokens, cfg.Telegram.Users); err != nil {
		log.Error("legacy orbit bootstrap failed", "err", err)
	} else if o != nil {
		log.Info("legacy orbit bootstrapped from config", "orbit", o.ID)
	}

	lookup := func(token string) (int64, string, bool) {
		orbitID, slot, ok, err := st.LookupToken(token)
		if err != nil {
			log.Error("token lookup failed", "err", err)
			return 0, "", false
		}
		return orbitID, slot, ok
	}
	h := hub.New(log, lookup, time.Duration(cfg.Timings.OfflineAfterS)*time.Second)
	stop := make(chan struct{})
	go h.Run(stop)

	var tgBot *bot.Bot
	if cfg.TelegramEnabled() {
		tgBot = bot.New(bot.NewHTTPAPI(cfg.Telegram.BotToken), log)
		go tgBot.Run(stop)
	} else {
		log.Warn("telegram.bot_token is empty: bot disabled (dev mode), chat notifications go to the log")
	}

	sp := spotify.New(cfg.Spotify.ClientID, cfg.Spotify.ClientSecret)
	if sp == nil {
		log.Warn("spotify app credentials not set: playlist links will ask for setup (U10)")
	}

	l := newLoop(log, cfg, h, st, tgBot, sp)
	if cfg.Providers {
		// Provider layer (spec-providers, DUET_PROVIDERS=1): resolve cascade
		// clients; secrets stay in the environment, only presence is logged.
		l.resolveTrack = newResolveTrackFn(log, sp)
		log.Info("provider layer enabled",
			"yandex_token_set", os.Getenv("DUET_YANDEX_TOKEN") != "",
			"odesli_key_set", os.Getenv("DUET_ODESLI_KEY") != "")
	}
	l.warmup()
	go l.run(stop, h.Events)

	go retentionSweep(log, st, stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		orbits, _ := st.OrbitIDs()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":          "ok",
			"version":         version,
			"orbits":          len(orbits),
			"nodes_connected": h.Stats(),
		})
	})
	// /pair is unauthenticated (code-gated) — cap attempts per IP to blunt
	// brute-force of pairing codes and DB-exhaustion spam.
	pairLimiter := newRateLimiter(10, time.Minute)
	mux.HandleFunc("/pair", rateLimit(pairLimiter, pairHandler(log, st, cfg)))
	mux.HandleFunc("/media/", mediaHandler(st))

	log.Info("listening", "addr", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Error("http server", "err", err)
		os.Exit(1)
	}
}

// pairHandler is POST /pair {code} -> {orbit_id, slot, token, ws_url}
// (design §4: the app exchanges a bot-issued one-time code for credentials).
func pairHandler(log *slog.Logger, st *store.Store, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Code == "" {
			http.Error(w, "body must be {\"code\": \"...\"}", http.StatusBadRequest)
			return
		}
		orbitID, issuedBy, err := st.ConsumeInvite(strings.ToUpper(strings.TrimSpace(req.Code)), "pair")
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if orbitID == 0 {
			http.Error(w, "code invalid or expired", http.StatusForbidden)
			return
		}
		slot, token, err := st.PairSlot(orbitID, issuedBy)
		if err != nil {
			log.Error("pair slot failed", "orbit", orbitID, "err", err)
			http.Error(w, "orbit is full", http.StatusConflict)
			return
		}
		wsURL := "ws://" + cfg.Listen + "/ws"
		if cfg.PublicURL != "" {
			wsURL = strings.Replace(strings.TrimRight(cfg.PublicURL, "/"), "https://", "wss://", 1) + "/ws"
		}
		log.Info("node paired", "orbit", orbitID, "slot", slot)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"orbit_id": orbitID,
			"slot":     slot,
			"token":    token,
			"ws_url":   wsURL,
		})
	}
}

// mediaHandler serves GET /media/{id}.wav to authenticated nodes (spec 10.3):
// any valid (non-revoked) node token grants access.
func mediaHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		_, _, authorized, _ := st.LookupToken(auth)
		if !authorized {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/media/"), ".wav")
		rec, err := st.GetMedia(id)
		if err != nil || rec == nil || rec.Status != "ready" {
			http.NotFound(w, r)
			return
		}
		if rec.ExpiresAt < time.Now().UnixMilli() {
			http.NotFound(w, r) // spec 10.3: 404 after expires_at
			return
		}
		http.ServeFile(w, r, rec.PathWAV)
	}
}

// retentionSweep deletes voice WAVs past expires_at daily (spec 5.3).
func retentionSweep(log *slog.Logger, st *store.Store, stop <-chan struct{}) {
	sweep := func() {
		expired, err := st.ExpiredMedia(time.Now().UnixMilli())
		if err != nil {
			log.Error("retention sweep failed", "err", err)
			return
		}
		for _, m := range expired {
			if m.PathWAV != "" {
				os.Remove(m.PathWAV)
			}
			st.MarkMediaDeleted(m.ID)
		}
		if len(expired) > 0 {
			log.Info("retention sweep", "deleted", len(expired))
		}
	}
	sweep()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			sweep()
		}
	}
}
