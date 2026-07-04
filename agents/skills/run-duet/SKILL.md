---
name: run-duet
description: Build, run, smoke-test and drive the duet stack (Barycenter coordinator + Pulsar macOS node + go-librespot). Use when asked to run duet, start the coordinator or Pulsar/NodeApp, smoke the stack, screenshot/verify playback, or reproduce the register/heartbeat cycle.
---

# Run duet (Barycenter + Pulsar)

One deployable stack: Go coordinator ("Barycenter") + macOS node app
("Pulsar", SwiftPM) which supervises its own go-librespot daemon. The agent
path is the smoke driver `agents/skills/run-duet/smoke.sh` — it boots an
ISOLATED sandbox (coordinator :8093, daemon :3680, node id `b`, clean
librespot config dir under `.temp/run-skill/`) so a developer stack on
:8091/:3678 keeps running untouched. All paths below are relative to the
repo root.

## Prerequisites (verified on this Mac)

```bash
brew install go-librespot   # daemon binary at /opt/homebrew/opt/go-librespot/bin/go-librespot
# Go 1.22+ and Swift 5.10+ toolchains (Xcode CLT) — both preinstalled here
```

## Build

Always from the **repo root** (the Makefile computes ROOT from `pwd`; running
`make` from a subdir scatters artifacts into `<subdir>/.temp` — this bit us
twice):

```bash
make build   # coordinator -> .temp/build/duet-coordinator, swift release build
make app     # wraps NodeApp into .temp/build/NodeApp.app (bundle id works.relux.pulsar, signed)
make test    # both sides: Go suites + 37 Swift tests (contract vs protocol/golden)
```

## Run — agent path (smoke driver)

```bash
agents/skills/run-duet/smoke.sh start    # boots sandbox, waits for node registration,
                                         # asserts /healthz shows "b":true
agents/skills/run-duet/smoke.sh status   # /healthz + last node log lines (heartbeat/clock)
agents/skills/run-duet/smoke.sh play [spotify:track:URI]  # drives the node's daemon API
agents/skills/run-duet/smoke.sh pause
agents/skills/run-duet/smoke.sh stop     # kills everything it started, leaves logs
```

Artifacts: `.temp/run-skill/` — `coordinator.log`, `node-b.log` (structured
JSON), `node-b.out`, configs, fifo. Registration proof is
`grep "welcome received" .temp/run-skill/node-b.log`.

`play` needs a Spotify Premium session in the sandbox: it exits 1 with
instructions until someone picks the sandbox device once in the phone's
Spotify app (same Wi-Fi). `persist_credentials: true` is rendered by the
node, so one login survives daemon restarts.

## Run — human/dev path

The long-lived dev stack (differs from sandbox: :8091/:3678, node id `a`,
default librespot config dir which already holds a real login):

```bash
.temp/build/duet-coordinator --config .temp/g1/coordinator.yml &   # :8091
open ~/duet/NodeApp.app        # reads ~/duet/node.yml; Dock icon "Pulsar"
# phone: Spotify -> devices -> "Pulsar A" -> play; audio exits the Mac's
# default output (direct mode; audio.output_device pins/auto-restores it)
```

## Gotchas (all hit live)

- **Makefile is CWD-sensitive** — build only from repo root (see above).
- **LaunchServices refuses to launch .app bundles from dot-directories**
  (`.temp/...` -> paramErr -50). Run the binary inside the bundle directly,
  or ditto the bundle to a visible path (`~/duet/NodeApp.app`) before `open`.
- **The FIFO reader must outlive playback**: killing whatever reads the pipe
  gives the daemon EPIPE ("output device failed: broken pipe") and the track
  dies. The node's reader loop handles this; don't attach throwaway readers
  to a live daemon's pipe.
- **go-librespot has no pacing** — a greedy pipe reader "plays" a track in
  seconds and cascades to the next. Tempo comes from the node's render
  callback; for standalone pipe experiments throttle to 352800 B/s.
- **Daemon /status returns an empty body before login** — parse defensively.
- **Playing a nonexistent uri** returns HTTP 500, emits a phantom `will_play`
  and nulls /status's track while old audio keeps flowing; only a successful
  play restores sanity.
- **Zeroconf credentials persist only with** `credentials.zeroconf.persist_credentials: true`
  (the node renders it); without it every daemon restart needs the phone again.
- **Port collisions**: dev daemon owns :3678 and the Spotify device name
  "Pulsar A"; the sandbox deliberately uses :3680/node `b`. Don't reuse the
  default config dir for experiments — two daemons over one config dir fight.
- Airfoil mode (`airfoil.enabled: true`) is Sequoia-only: on macOS 26 the
  capture drops audio (confirmed live); sandbox keeps it off.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `missing .temp/build/...` from smoke.sh | `make build && make app` from repo root |
| "FAIL: node never registered" | `tail .temp/run-skill/node-b.out` — config validation errors print there verbatim (they are human-readable by design) |
| healthz shows `"b":false` after start | coordinator died: `tail .temp/run-skill/coordinator.log`; usually a port clash on :8093 |
| Node exits instantly when double-clicked | no `~/duet/node.yml`; a GUI alert explains since v0.1 — run with `--config` or install the config |
| `play` says "no Spotify session" | expected in a fresh sandbox; do the one-time phone login for the sandbox device |
