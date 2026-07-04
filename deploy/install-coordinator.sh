#!/bin/bash
# Idempotent coordinator install/update on the VPS (goal 3.5). Run as root.
# Never overwrites an existing /etc/duet/coordinator.yml; SQLite state and
# media survive updates (goal §2.2).
#
# Usage: sudo ./install-coordinator.sh /path/to/duet-coordinator-vX.Y.Z-linux-amd64
set -euo pipefail

BIN="${1:?usage: install-coordinator.sh /path/to/duet-coordinator-vX.Y.Z-linux-amd64}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> duet coordinator install"
id -u duet >/dev/null 2>&1 || useradd --system --home /var/lib/duet --shell /usr/sbin/nologin duet

mkdir -p /etc/duet /var/lib/duet/media /var/lib/duet/backup
chown -R duet:duet /var/lib/duet

install -m 755 "$BIN" /usr/local/bin/duet-coordinator

if [[ ! -f /etc/duet/coordinator.yml ]]; then
    echo "==> first install: placing coordinator.yml template — EDIT IT (tokens, telegram, tailscale ip)"
    install -m 600 "$SCRIPT_DIR/coordinator.yml.template" /etc/duet/coordinator.yml
    chown duet:duet /etc/duet/coordinator.yml
else
    echo "==> keeping existing coordinator.yml"
fi

install -m 644 "$SCRIPT_DIR/duet-coordinator.service" /etc/systemd/system/duet-coordinator.service
systemctl daemon-reload
systemctl enable duet-coordinator
systemctl restart duet-coordinator
sleep 1
systemctl --no-pager --lines=5 status duet-coordinator || true
/usr/local/bin/duet-coordinator --version
