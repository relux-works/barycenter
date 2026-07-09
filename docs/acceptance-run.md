# Acceptance Run — Phase 1 (spec ch. 18, goal DoD-8/DoD-10)

Scope: items 18.1-18.7 and 18.9-18.13 (18.8 is phase 2), plus the factual
clean-install run (goal 3.6). A row counts only with date, environment and an
observed result — no pre-ticking (goal invariant: progress needs evidence).

Status: **pending overall** — the formal run needs G0 closure (zeroconf login,
Airfoil trial) and G3 prerequisites (second Mac + account, VPS). Pre-verification
evidence recorded below where the mechanism was factually exercised on the dev
machine (2026-07-03); "pre-verified" is NOT a pass — every row is re-run live at G3.

## Pre-verification runs (dev machine, 2026-07-03)

Environment: MacBook arm64, macOS 26.5, NodeApp 0.1.0-dev (release zip),
duet-coordinator 0.1.0-dev, go-librespot 0.7.4, coordinator+2 nodes local
(127.0.0.1:8091/8092), Telegram disabled, Airfoil absent, Spotify not logged in.

| What | Result | Evidence |
|---|---|---|
| Runbook §1 node install from release zip | install < 5 s; template config rejected with named problems ("node_id is \"__NODE_ID__\"...", token length); after edit + kickstart: registered, healthz a:true | .temp/clean-install/install-01.log, duet/nodeapp.err.log |
| Runbook §7 update over installed node (twice) | "keeping existing node.yml (state preserved)", node re-registers after each update | .temp/clean-install/install-02-update.log |
| 18.5 protocol leg (kill -9 go-librespot) | supervisor caught exit, error(librespot_restart) reached coordinator, daemon restarted ~1 s (pid 59770->59810) | .temp/g2-sim/node-a.log, coordinator.log |
| 18.6 protocol leg (node network cut) | SIGKILL node b -> coordinator offline-detect ~12 s, healthz b:false; restart -> re-register -> b:true | .temp/g2-sim/coordinator.log |
| 18.10 logic | restart -> PAUSED, queue intact, /resume position from fresh heartbeats | coordinator/cmd/duet-coordinator/loop_test.go TestRestartRestoresPausedSession (green) |
| 18.11 logic | stranger user_id produces no event and no reply | internal/bot/bot_test.go TestStrangerSilentlyIgnored (green) |
| SIGTERM/launchd shutdown (found by sim) | NodeApp initially survived SIGTERM (signal source on main queue) — fixed (dedicated queue + watchdog); now exits <= 2.5 s | session log; fix in NodeApp/main.swift |
| Release from clean source copy (DoD-1a) | pristine copy (no .git/.temp/caches): make release = tests green + 3 artifacts | .temp/clean-clone/release/ |

## Formal acceptance environment (to fill at G3)

| Field | Value |
|---|---|
| Date | — |
| Coordinator | VPS/host, duet-coordinator version (`/healthz`) |
| Node A | Mac model, macOS, NodeApp version, go-librespot version, speakers |
| Node B | — |
| Network | tailnet RTT a<->coord, b<->coord (`/status`) |
| Airfoil | version, trial/licensed |

## Checklist

| # | Spec item | Result | Evidence (log path / journal quote / measurement) |
|---|---|---|---|
| 18.1 | Select a track on either Pulsar in Spotify (no bot link) -> both homes play from the reported position, journal skew <= 150 ms, click test simultaneous | pending | |
| 18.2 | Voice both: after current track, level within +-2 LU of -14 LUFS | pending | |
| 18.3 | Voice "лично": partner hears, sender waits, next track synced | pending | |
| 18.4 | /playnow /skip /pause /resume /queue /cancel /vol per ch. 9 | pending | |
| 18.5 | kill -9 go-librespot: sound back <= 15 s, element restarted, chat notified | pending | |
| 18.6 | 30 s network cut: both paused <= 15 s, /resume continues +-2 s | pending | |
| 18.7 | Mac reboot: node self-registers, playback resumes on command | pending | |
| 18.9 | Unavailable track: skipped with chat reason, session continues | pending | |
| 18.10 | Coordinator restart mid-PLAYING: PAUSED, queue intact, /resume works | pending | |
| 18.11 | Stranger user_id silently ignored | pending | |
| 18.12 | nmap from outside: zero public ports on VPS | pending | |
| 18.13 | Two speakers: in-home sync, unplug -> degraded + notice, auto-reconnect <= 60 s | pending | |
| CI | Clean install per runbook (fresh mac user + fresh VPS), time taken | pending | |
