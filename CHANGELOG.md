# Changelog

## v0.3.0-beta.26 — 2026-07-10

- Manual Spotify selections now keep the initiating Pulsar audible while the
  other homes seek and join it; no source-side pause/reload glitch.
- Stale coordinator-owned go-librespot loads can no longer masquerade as phone
  selections and resurrect an earlier album or context.
- A slow or restarting linked home catches up without stopping or skipping the
  healthy shared air; transient starvation recovery is less aggressive.
- Voice messages are FIFO by Telegram acceptance time across both linked
  Barycenters, identify their sender/target in the queue, and cannot be cut off
  by a phone track selection.
- Bot replies use human track and Barycenter names where metadata is available;
  Spotify link metadata is cached for later queue/status messages.
- macOS and Windows Pulsars advertise the additive
  `seamless_adoption_v1` capability, keeping rolling upgrades compatible with
  older nodes.

## Unreleased

Initial development toward v1.0.0 (spec v1.2, goal v1.0):

- Protocol v1: 25 golden JSON files, contract-tested from Go and Swift; `set_offset`/`offset_test` added beyond the spec ch. 8 catalog (documented in docs/protocol.md, proposed for spec v1.3).
- Coordinator (Go): ws-hub (token auth, 4401, last-write-wins, inline pong, offline detection), session FSM per spec 7.2 with 16 unit-tested scenarios, scheduler, SQLite persistence with restart-to-PAUSED rule, Telegram bot (all phase-1 commands), media pipeline (ffmpeg loudnorm to -14 LUFS, live-tested), authed /media endpoint, /healthz with version, retention sweep.
- NodeApp (Swift): SPSC lock-free ring with backpressure (drop forbidden), FIFO reader with lazy-open/EOF-reopen, AVAudioEngine graph (music + inserts + clicks, fade, (v/100)^2 volume), go-librespot supervisor (config render, backoff restarts) and API client (two-step load: play paused + seek), PlayerCore (resume_at via T_local, audible_position, ended after ring drain, starvation watch), clock sync (EMA, outlier rejection), voice LRU cache, .app bundle build with stable bundle id.
- Distribution: Makefile (test/build/app/release), deploy/ templates and idempotent installers, docs/runbook.md.
