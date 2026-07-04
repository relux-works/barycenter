#!/bin/bash
# Wraps the NodeApp release binary into a minimal .app bundle (spec 6.3:
# Airfoil captures an application source by POSIX path; bare CLI capture is
# not guaranteed). Bundle id and Info.plist versioning per goal 3.1.
#
# Signing (goal DoD-2): uses the stable self-signed identity "duet-nodeapp"
# (scripts/setup-signing.sh) so the designated requirement — and therefore the
# TCC Automation grant — survives updates. Falls back to ad-hoc with a warning.
set -euo pipefail

VERSION="${VERSION:-0.1.0-dev}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT/.temp/build}"
# v2 naming (docs/v2-direction.md): the app is Pulsar. Old id
# works.relux.duet.nodeapp retired before anyone reached production.
BUNDLE_ID="works.relux.pulsar"

BIN="$ROOT/node-app/.build/release/NodeApp"
if [[ ! -x "$BIN" ]]; then
    echo "error: $BIN not built; run 'make build' first" >&2
    exit 1
fi

APP="$OUT_DIR/NodeApp.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp "$BIN" "$APP/Contents/MacOS/NodeApp"

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleIdentifier</key><string>${BUNDLE_ID}</string>
  <key>CFBundleName</key><string>Pulsar</string>
  <key>CFBundleDisplayName</key><string>Pulsar</string>
  <key>CFBundleExecutable</key><string>NodeApp</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleDevelopmentRegion</key><string>en</string>
  <key>CFBundleShortVersionString</key><string>${VERSION#v}</string>
  <key>CFBundleVersion</key><string>${VERSION#v}</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>NSPrincipalClass</key><string>NSApplication</string>
  <!-- Regular app (Dock icon and all): Airfoil lists only regular running
       apps as sources (spike S4); the node Macs are headless anyway. -->
  <key>NSAppleEventsUsageDescription</key>
  <string>NodeApp controls Airfoil to deliver audio to the home speakers.</string>
</dict></plist>
PLIST

SIGN_CN="duet-nodeapp"
KEYCHAIN="$HOME/Library/Keychains/duet-signing.keychain-db"
CERT_HASH="$(security find-certificate -c "$SIGN_CN" -Z 2>/dev/null | awk '/SHA-1/ {print $NF}' || true)"

if [[ -n "$CERT_HASH" ]]; then
    security unlock-keychain -p "duet-signing-local" "$KEYCHAIN" 2>/dev/null || true
    codesign --force --sign "$CERT_HASH" --identifier "$BUNDLE_ID" --timestamp=none "$APP"
else
    echo "WARNING: identity '$SIGN_CN' not found (run scripts/setup-signing.sh);" >&2
    echo "         signing ad-hoc — TCC Automation will NOT survive updates (goal DoD-2)" >&2
    codesign --force --sign - --identifier "$BUNDLE_ID" "$APP"
fi

echo "built $APP (version $VERSION)"
codesign -dv "$APP" 2>&1 | grep -E "Identifier|Signature|Authority" || true
codesign -d -r- "$APP" 2>&1 | grep "designated" || true
