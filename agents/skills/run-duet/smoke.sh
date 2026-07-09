#!/bin/bash
# duet stack smoke driver: boots coordinator + Pulsar node (with its own
# go-librespot daemon) in an ISOLATED sandbox (ports 8093/3680, node id "b",
# isolated librespot config dir) so a dev stack on :8091/:3678 keeps running.
#
# Commands:
#   start    boot the sandbox stack, wait for node registration, assert /healthz
#   status   /healthz + key log lines
#   play     drive the node's daemon over its local API (needs a Spotify login
#            in the sandbox config dir — see SKILL.md; without it: clear error)
#   pause    pause via the daemon API
#   stop     tear everything down, leave no processes
#
# Artifacts land in .temp/run-skill/ (logs, configs, fifo).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SAND="$ROOT/.temp/run-skill"
COORD_PORT=8093
LS_PORT=3680
COORD_BIN="$ROOT/.temp/build/duet-coordinator"
NODE_BIN="$ROOT/.temp/build/NodeApp.app/Contents/MacOS/NodeApp"

need() { [[ -x "$1" ]] || { echo "missing $1 — run: make build && make app (from repo root)"; exit 1; }; }

prepare() {
    mkdir -p "$SAND/librespot" "$SAND/cache"
    [[ -p "$SAND/spotify.fifo" ]] || mkfifo "$SAND/spotify.fifo"
    if [[ ! -f "$SAND/coordinator.yml" ]]; then
        local TOKA TOKB
        TOKA=$(openssl rand -hex 32); TOKB=$(openssl rand -hex 32)
        cat > "$SAND/coordinator.yml" <<EOF
listen: "127.0.0.1:$COORD_PORT"
db_path: $SAND/duet.db
media_dir: $SAND/media
nodes:
  a: { token: "$TOKA" }
  b: { token: "$TOKB" }
telegram:
  bot_token: ""
EOF
        cat > "$SAND/node-b.yml" <<EOF
node_id: b
coordinator:
  url: ws://127.0.0.1:$COORD_PORT/ws
  token: "$TOKB"
audio:
  fifo_path: $SAND/spotify.fifo
  sample_rate: 44100
  format: f32le
  output_latency_offset_ms: 0
  ring_buffer_ms: 1000
airfoil:
  enabled: false
  app_path: /Applications/Airfoil.app
  speakers: []
  poll_s: 10
librespot:
  binary: /opt/homebrew/opt/go-librespot/bin/go-librespot
  api_port: $LS_PORT
  config_dir: $SAND/librespot
cache_dir: $SAND/cache
log:
  level: debug
  path: $SAND/node-b.log
EOF
    fi
}

start() {
    need "$COORD_BIN"; need "$NODE_BIN"
    prepare
    # A prior successful run must not satisfy the welcome grep for a new
    # process. Keep configs/credentials, but make registration evidence fresh.
    : > "$SAND/coordinator.log"
    : > "$SAND/node-b.log"
    : > "$SAND/node-b.out"
    "$COORD_BIN" --config "$SAND/coordinator.yml" > "$SAND/coordinator.log" 2>&1 &
    echo $! > "$SAND/coordinator.pid"
    sleep 1
    "$NODE_BIN" --config "$SAND/node-b.yml" > "$SAND/node-b.out" 2>&1 &
    echo $! > "$SAND/node.pid"

    for i in $(seq 1 20); do
        sleep 1
        if grep -q "welcome received" "$SAND/node-b.log" 2>/dev/null; then break; fi
        [[ $i == 20 ]] && { echo "FAIL: node never registered"; tail -5 "$SAND/node-b.log" "$SAND/coordinator.log"; exit 1; }
    done
    local HEALTH
    HEALTH=$(curl -s "http://127.0.0.1:$COORD_PORT/healthz")
    echo "$HEALTH"
    # /healthz is tenant-safe and exposes only the aggregate connection count;
    # this sandbox starts exactly one node, so one connected node proves b's
    # register/welcome cycle without depending on the retired per-slot shape.
    echo "$HEALTH" | grep -Eq '"nodes_connected":[[:space:]]*[1-9]' || { echo "FAIL: node b not online"; exit 1; }
    echo "OK: stack up (coordinator :$COORD_PORT, daemon :$LS_PORT). Logs: $SAND/"
}

status() {
    curl -s "http://127.0.0.1:$COORD_PORT/healthz" || echo "coordinator not running"
    echo
    tail -3 "$SAND/node-b.log" 2>/dev/null | cut -c1-140 || true
}

play() {
    local READY
    READY=$(curl -s -m 3 "http://127.0.0.1:$LS_PORT/" | grep -o 'true' || true)
    if [[ -z "$READY" ]]; then
        echo "daemon has no Spotify session in the sandbox."
        echo "Login once: pick the device in the phone's Spotify (same Wi-Fi); persist_credentials keeps it."
        exit 1
    fi
    local URI="${2:-spotify:track:4cOdK2wGLETKBW3PvgPWqT}"
    curl -s -X POST -H 'Content-Type: application/json' -d "{\"uri\":\"$URI\"}" "http://127.0.0.1:$LS_PORT/player/play" > /dev/null
    sleep 2
    curl -s "http://127.0.0.1:$LS_PORT/status" | python3 -c "import json,sys; d=json.load(sys.stdin); t=d.get('track') or {}; print('playing:', t.get('name'), '@', t.get('position'))"
}

pause() {
    curl -s -X POST "http://127.0.0.1:$LS_PORT/player/pause" > /dev/null && echo paused
}

stop() {
    for f in node coordinator; do
        [[ -f "$SAND/$f.pid" ]] && kill "$(cat "$SAND/$f.pid")" 2>/dev/null || true
        rm -f "$SAND/$f.pid"
    done
    sleep 1
    pkill -f "go-librespot.*run-skill" 2>/dev/null || true
    pkill -f "NodeApp --config $SAND" 2>/dev/null || true
    sleep 1
    if pgrep -f "run-skill" > /dev/null 2>&1; then
        pkill -9 -f "run-skill" 2>/dev/null || true
    fi
    echo "stopped; sandbox artifacts kept in $SAND"
}

case "${1:-}" in
    start) start ;;
    status) status ;;
    play) play "$@" ;;
    pause) pause ;;
    stop) stop ;;
    *) echo "usage: smoke.sh start|status|play [uri]|pause|stop"; exit 2 ;;
esac
