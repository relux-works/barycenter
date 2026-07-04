// duet-coordinator: session owner, queue, scheduler, Telegram bot, media
// (spec ch. 7). Persistence in SQLite (spec 5.3), media over authed HTTP
// (spec 10.3), no public ports — bind to the tailnet address only (spec 17).
package main

import (
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/spotify"
	"relux.works/duet/coordinator/internal/store"
)

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

	tokens := map[protocol.NodeID]string{
		protocol.NodeA: cfg.Nodes["a"].Token,
		protocol.NodeB: cfg.Nodes["b"].Token,
	}
	h := hub.New(log, tokens, time.Duration(cfg.Timings.OfflineAfterS)*time.Second)
	stop := make(chan struct{})
	go h.Run(stop)

	var tgBot *bot.Bot
	if cfg.TelegramEnabled() {
		tgBot = bot.New(bot.NewHTTPAPI(cfg.Telegram.BotToken), log, cfg.Telegram.Users, cfg.Telegram.ChatID)
		go tgBot.Run(stop)
	} else {
		log.Warn("telegram.bot_token is empty: bot disabled (dev mode), chat notifications go to the log")
	}

	sp := spotify.New(cfg.Spotify.ClientID, cfg.Spotify.ClientSecret)
	if sp == nil {
		log.Warn("spotify app credentials not set: playlist links will ask for setup (U10)")
	}

	l := newLoop(log, cfg, h, st, tgBot, sp)
	l.restore()
	go l.run(stop, h.Events)

	go retentionSweep(log, st, stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": version,
			"nodes":   h.Online(),
		})
	})
	mux.HandleFunc("/media/", mediaHandler(st, tokens))

	log.Info("listening", "addr", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Error("http server", "err", err)
		os.Exit(1)
	}
}

// mediaHandler serves GET /media/{id}.wav to authenticated nodes (spec 10.3).
func mediaHandler(st *store.Store, tokens map[protocol.NodeID]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		authorized := false
		for _, tok := range tokens {
			if subtle.ConstantTimeCompare([]byte(auth), []byte(tok)) == 1 {
				authorized = true
			}
		}
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
