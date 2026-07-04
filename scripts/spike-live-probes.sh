#!/bin/bash
# Spike remainder (G0): live go-librespot API probes after zeroconf login.
# Run while the daemon is up and logged in (runbook §3). Results land in
# .temp/spike/live-probes-<ts>.log — paste findings into docs/spike-report.md.
#
# Usage: scripts/spike-live-probes.sh [api_port] [track_uri]
set -uo pipefail

PORT="${1:-3678}"
TRACK="${2:-spotify:track:4cOdK2wGLETKBW3PvgPWqT}"   # any track both accounts can play
BAD_TRACK="spotify:track:0000000000000000000000"      # unavailable-track probe
BASE="http://127.0.0.1:$PORT"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG="$ROOT/.temp/spike/live-probes-$(date +%H%M%S).log"
mkdir -p "$ROOT/.temp/spike"

say() { echo "$@" | tee -a "$LOG"; }
probe() { # probe <label> <curl args...>
    local label="$1"; shift
    say ""
    say "=== $label ==="
    curl -sS -m 5 -w "\nHTTP %{http_code}\n" "$@" 2>&1 | tee -a "$LOG"
}

say "duet spike live probes — $(date) — port $PORT"
probe "S1 health" "$BASE/"
probe "S1 status (logged in?)" "$BASE/status"

say ""
say ">>> Watch WS events in another terminal:"
say ">>>   npx wscat -c ws://127.0.0.1:$PORT/events   (or: websocat)"
say ">>> note event names + arrival times at track boundaries"

probe "S1 two-step load: play paused" -X POST -H 'Content-Type: application/json' \
    -d "{\"uri\":\"$TRACK\",\"paused\":true}" "$BASE/player/play"
sleep 1
probe "S1 status after paused load (expect paused=true, track set)" "$BASE/status"
probe "S1 seek while paused -> 30s" -X POST -H 'Content-Type: application/json' \
    -d '{"position":30000,"relative":false}' "$BASE/player/seek"
sleep 0.5
probe "S1 status after seek (position ~30000?)" "$BASE/status"

say ""
say ">>> S2/R2: resume->first-audio jitter. Watch the FIFO in another terminal BEFORE resuming:"
say ">>>   cat '$ROOT/.temp/spike/spotify.fifo' | pv -bt > /dev/null    (pv shows first-byte time)"
read -rp ">>> press Enter to resume playback..."
date +%s%3N | tee -a "$LOG"
probe "S1 resume" -X POST "$BASE/player/resume"

say ""
read -rp ">>> let it play ~5s, then Enter to probe pause behavior (does FIFO stall or write silence?)..."
probe "S2 pause" -X POST "$BASE/player/pause"
say ">>> check the pv terminal: did byte flow STOP (expected) or continue with silence?"

read -rp ">>> Enter to continue..."
probe "S1 add_to_queue (solo_inject carrier)" -X POST -H 'Content-Type: application/json' \
    -d "{\"uri\":\"$TRACK\"}" "$BASE/player/add_to_queue"

probe "S1 unavailable track error shape" -X POST -H 'Content-Type: application/json' \
    -d "{\"uri\":\"$BAD_TRACK\",\"paused\":true}" "$BASE/player/play"
sleep 1
probe "S1 status after bad track" "$BASE/status"

probe "cleanup: stop" -X POST "$BASE/player/stop"

say ""
say "Remaining manual items for the report:"
say " - sample rate: is pipe output always 44100? (compare pv byte rate: 352800 B/s = 44.1k f32 stereo)"
say " - boundary events: gapless into next track? which events, what latency?"
say " - resume->first-byte jitter over ~10 runs (R2): repeat the resume block"
say "Log saved: $LOG"
