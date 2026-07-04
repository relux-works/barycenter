#!/bin/bash
# Idempotent node install/update (goal 3.5): unpack NodeApp.app, place config
# template on first run (never overwrites an existing node.yml), install the
# LaunchAgent, (re)start it. Run as the autologin duet user on the home Mac.
#
# Usage: ./install-node.sh /path/to/NodeApp-vX.Y.Z.app.zip
set -euo pipefail

ZIP="${1:?usage: install-node.sh /path/to/NodeApp-vX.Y.Z.app.zip}"
DUET_HOME="${DUET_HOME:-$HOME/duet}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LABEL="works.relux.duet.nodeapp"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"

echo "==> duet node install into $DUET_HOME"
mkdir -p "$DUET_HOME/cache"

# Stop a running agent before replacing the app (update path, goal §2.2).
if launchctl print "gui/$(id -u)/$LABEL" >/dev/null 2>&1; then
    echo "==> stopping running NodeApp"
    launchctl bootout "gui/$(id -u)/$LABEL" || true
fi

echo "==> unpacking $(basename "$ZIP")"
rm -rf "$DUET_HOME/NodeApp.app"
ditto -x -k "$ZIP" "$DUET_HOME"
[[ -d "$DUET_HOME/NodeApp.app" ]] || { echo "error: zip did not contain NodeApp.app" >&2; exit 1; }

if [[ ! -f "$DUET_HOME/node.yml" ]]; then
    echo "==> first install: placing node.yml template — EDIT IT before the node can register"
    sed "s|__DUET_HOME__|$DUET_HOME|g" "$SCRIPT_DIR/node.yml.template" > "$DUET_HOME/node.yml"
    chmod 600 "$DUET_HOME/node.yml"
else
    echo "==> keeping existing node.yml (state preserved)"
fi

echo "==> installing LaunchAgent"
mkdir -p "$HOME/Library/LaunchAgents"
sed "s|__DUET_HOME__|$DUET_HOME|g" "$SCRIPT_DIR/works.relux.duet.nodeapp.plist.template" > "$PLIST"

launchctl bootstrap "gui/$(id -u)" "$PLIST"
echo "==> started; check: tail -f $DUET_HOME/nodeapp.log"
"$DUET_HOME/NodeApp.app/Contents/MacOS/NodeApp" --version
