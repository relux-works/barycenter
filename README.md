# duet

Shared Spotify session for two homes. One synchronized "broadcast" (same tracks, same position, <=150 ms skew) playing in two apartments in different cities, controlled by two people through a Telegram bot, with voice-message drops that play on air strictly after the current track ends.

Full specification (Russian, source of truth): [docs/spec.md](docs/spec.md), v1.1 of 2026-07-03.

## How it works (short)

- Each home has a Mac node: **go-librespot** (headless Spotify Connect client, own Premium account, PCM to a named pipe) -> **NodeApp** (Swift, AVAudioEngine: music + voice drops mixing) -> **Airfoil** (delivery to 1..N speakers of that home).
- A **coordinator** (Go, on a VPS inside a Tailscale tailnet) owns the session state machine and the queue, drives playback track-by-track on both nodes with clock-synced starts, runs the Telegram bot, and processes voice messages with ffmpeg (loudness-normalized to match music level).
- Audio never crosses between homes; only control messages and processed voice files travel over the tailnet.
- Modes: **shared** (one common queue, coordinator drives both nodes) and **solo** (each node is a regular Spotify Connect device; partner can still inject tracks and voice drops).

## Repository layout

| Path | Purpose |
|---|---|
| `docs/spec.md` | Specification v1.2 (source of truth, Russian) |
| `docs/goal.md` | Implementation goal: definition of done, gates G0-G4 |
| `docs/protocol.md` | Protocol v1 implementation notes (clarifications over spec ch. 8) |
| `docs/spike-report.md` | Phase 0 spike findings (in progress) |
| `protocol/golden/` | 23 golden JSON files: the wire contract, tested from both sides |
| `node-app/` | NodeApp: Swift/SwiftPM executable + NodeCore library for the home Macs |
| `coordinator/` | Coordinator: Go service for the VPS |
| `pulsar-win/` | Pulsar for Windows: Go shell skeleton (EPIC B blind build: portable parts unit-tested, WASAPI/named-pipe legs compile-gated via `GOOS=windows`; wire protocol mirrored from `coordinator/internal/protocol`, pinned by golden tests) |
| `scripts/` | build-app.sh (.app bundle), setup-signing.sh (stable TCC-safe identity), spike-live-probes.sh |
| `deploy/` | launchd plist + systemd unit + config templates + idempotent installers |
| `spike/` | Phase 0 prototypes (throwaway quality, findings promoted to spec/report) |
| `.temp/` | Task plan, logs, scratch artifacts (gitignored) |
| `UNRESOLVED_QUESTIONS.md` | Open decisions and their owners |

## Status

Gates (docs/goal.md §5): G0 spike: blocked on two user actions (zeroconf login, Airfoil trial install); **G1 contract & skeletons: exit criteria met and proven** (contract tests green both sides, NodeApp.app under launchd registers and heartbeats, /healthz serves version); G2 in progress. Details and evidence: `.temp/tasks.md`.

## Tools

| Tool | Used for | How to run | Artifacts |
|---|---|---|---|
| `make` | Entry point for everything | `make test` (both sides), `make build`, `make app`, `make release VERSION=vX.Y.Z` | dev builds in `.temp/build/`, distribution in `release/` (both gitignored) |
| `swift` (SwiftPM, 5.10+) | NodeApp and spike prototypes build/test | `swift build` / `swift test` in `node-app/` or `spike/...` | `.build/` (gitignored) |
| `go` (1.22+) | Coordinator build/test | `go build ./...` / `go test ./...` in `coordinator/` | binary in `.temp/build/` or `release/` |
| `go-librespot` | Headless Spotify Connect playback on nodes | `brew install go-librespot`; started/supervised by NodeApp in production; manual runs for spike: config under `.temp/spike/` | PCM FIFO, local HTTP+WS API on 127.0.0.1:3678 |
| `ffmpeg` | Voice message processing on coordinator (highpass, compressor, loudnorm) | see spec ch. 10 for the exact filter chain | WAV files in coordinator `media_dir` |
| `tailscale` | Mesh network between 2 Macs + VPS, MagicDNS names `node-a`/`node-b`/`coord`, Tailscale SSH | see spec ch. 11 |: |
| Airfoil (Rogue Amoeba) | Speaker delivery on each Mac, AppleScript-controlled | GUI + AppleScript via NodeApp; **license required per Mac** (trial injects noise after 10 min) | — |
| Telegram Bot API | User interface | long polling from coordinator; bot token via BotFather | — |

Verification/scratch outputs for tool checks live in `.temp/` (gitignored), per-task subdirectories.

<!-- relux-ecosystem:start -->

## About Relux Works

This project is part of the open-source ecosystem of
[Relux Works](https://relux.works), an AI-native software development studio.
We build fixed-price MVPs, rescue vibe-coded apps, run local AI inference, and
train teams to work with coding agents. Much of the infrastructure behind that
work is open source.

- Full catalog: [relux.works/en/open-source](https://relux.works/en/open-source/)
- Agentic enablement: [agent harnesses & team training](https://relux.works/en/agentic-enablement/)
- Hire us the agent-native way: point your assistant at `https://api.relux.works/mcp`
- Contact: ivan@relux.works

<!-- relux-ecosystem:end -->
