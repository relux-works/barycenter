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
# Sparkle compares CFBundleVersion: derive a clean semver and a monotonic build.
SHORT_VERSION="$(echo "$VERSION" | sed -E 's/^v//; s/[-+].*$//')"
BUILD_NUMBER="${BUILD_NUMBER:-$(git -C "$(dirname "$0")/.." rev-list --count HEAD 2>/dev/null || echo 1)}"
SPARKLE_PUBLIC_KEY="${SPARKLE_PUBLIC_KEY:-tx8SLAmqME/ldUthxRV5PFQiUt1MX65blT29cA8My1U=}"
SPARKLE_FEED_URL="${SPARKLE_FEED_URL:-https://github.com/relux-works/barycenter/releases/latest/download/appcast.xml}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT/.temp/build}"
# v2 naming (docs/v2-direction.md): the app is Pulsar. Old id
# works.relux.duet.nodeapp retired before anyone reached production.
BUNDLE_ID="works.relux.pulsar"

BIN="${NODEAPP_BIN:-$ROOT/node-app/.build/release/NodeApp}"
if [[ ! -x "$BIN" ]]; then
    echo "error: $BIN not built; run 'make build' first" >&2
    exit 1
fi

APP="$OUT_DIR/${APP_NAME:-NodeApp}.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/Audio"
cp "$BIN" "$APP/Contents/MacOS/NodeApp"
RECORDING_CUE="$ROOT/assets/audio/pulsar-recording-cue.wav"
RECORDING_CUE_SHA256="479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
if [[ ! -f "$RECORDING_CUE" ]] ||
   [[ "$(shasum -a 256 "$RECORDING_CUE" | awk '{print $1}')" != "$RECORDING_CUE_SHA256" ]]; then
  echo "FATAL: canonical recording cue is missing or has an unreviewed digest" >&2
  exit 1
fi
cp "$RECORDING_CUE" "$APP/Contents/Resources/Audio/pulsar-recording-cue.wav"
# App icon (assets/icon/Pulsar.icns, generated from the source artwork).
if [[ -f "$ROOT/assets/icon/Pulsar.icns" ]]; then
  cp "$ROOT/assets/icon/Pulsar.icns" "$APP/Contents/Resources/Pulsar.icns"
fi

# Sparkle is a dynamic framework (binary xcframework): ship it in
# Contents/Frameworks and point the executable's rpath there. Missing this
# = dyld crash at launch ("cannot be opened because of a problem").
SPARKLE_FW="$(dirname "$BIN")/Sparkle.framework"
if [[ ! -d "$SPARKLE_FW" ]]; then
  SPARKLE_FW="$ROOT/node-app/.build/artifacts/sparkle/Sparkle/Sparkle.xcframework/macos-arm64_x86_64/Sparkle.framework"
fi
if [[ -d "$SPARKLE_FW" ]]; then
  mkdir -p "$APP/Contents/Frameworks"
  cp -R "$SPARKLE_FW" "$APP/Contents/Frameworks/"
  install_name_tool -add_rpath "@executable_path/../Frameworks" "$APP/Contents/MacOS/NodeApp" 2>/dev/null || true
else
  echo "WARN: Sparkle.framework not found — updater will crash the app" >&2
fi

# R1: bundle the go-librespot daemon (relux-works fork) so the app is
# self-contained — no brew. Override with LIBRESPOT_BIN; falls back to the
# local fork build, then brew; warns if none found (dev-only bundle).
LIBRESPOT_BIN="${LIBRESPOT_BIN:-}"
if [[ -z "$LIBRESPOT_BIN" ]]; then
  for cand in "$ROOT/.temp/go-librespot-fork/daemon-fork"               /opt/homebrew/opt/go-librespot/bin/go-librespot; do
    [[ -x "$cand" ]] && LIBRESPOT_BIN="$cand" && break
  done
fi
if [[ -n "$LIBRESPOT_BIN" ]]; then
  if ! LC_ALL=C grep -a -q "PULSAR_ZEROCONF_HOST" "$LIBRESPOT_BIN"; then
    echo "FATAL: go-librespot binary does not support PULSAR_ZEROCONF_HOST; apply patches/go-librespot-pulsar-zeroconf-host.patch to the fork before bundling" >&2
    exit 1
  fi
  cp "$LIBRESPOT_BIN" "$APP/Contents/MacOS/go-librespot"
else
  echo "WARN: no go-librespot binary found to bundle (app will rely on brew)" >&2
fi

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleIdentifier</key><string>${BUNDLE_ID}</string>
  <key>CFBundleName</key><string>Pulsar</string>
  <key>CFBundleIconFile</key><string>Pulsar</string>
  <key>CFBundleDisplayName</key><string>Pulsar</string>
  <key>CFBundleExecutable</key><string>NodeApp</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleDevelopmentRegion</key><string>en</string>
  <key>CFBundleShortVersionString</key><string>${SHORT_VERSION}</string>
  <key>CFBundleVersion</key><string>${BUILD_NUMBER}</string>
  <key>SUFeedURL</key><string>${SPARKLE_FEED_URL}</string>
  <key>SUPublicEDKey</key><string>${SPARKLE_PUBLIC_KEY}</string>
  <!-- F4 silent auto-update: SUAutomaticallyUpdate alone does nothing without
       SUEnableAutomaticChecks — the app never checks the feed on its own and
       the user must "Check for Updates" by hand. Enable periodic checks (hourly)
       and silent install; skip Sparkle's first-run permission prompt. -->
  <key>SUEnableAutomaticChecks</key><true/>
  <key>SUAutomaticallyUpdate</key><true/>
  <key>SUScheduledCheckInterval</key><integer>3600</integer>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>NSPrincipalClass</key><string>NSApplication</string>
  <!-- Regular app (Dock icon and all): Airfoil lists only regular running
       apps as sources (spike S4); the node Macs are headless anyway. -->
  <key>NSAppleEventsUsageDescription</key>
  <string>NodeApp controls Airfoil to deliver audio to the home speakers.</string>
  <!-- Local Network (macOS 15+): the daemon advertises "Pulsar" over Bonjour so
       the phone's Spotify can find it as a speaker. Without NSBonjourServices the
       browse is blocked even after the user grants access; the usage string is
       the reason shown in the system prompt. Onboarding primes this deliberately
       (LocalNetworkProbe) so the prompt never appears out of context. -->
  <key>NSLocalNetworkUsageDescription</key>
  <string>Чтобы Spotify на телефоне видел этот компьютер как колонку «Pulsar».</string>
  <key>NSBonjourServices</key>
  <array><string>_spotify-connect._tcp</string></array>
</dict></plist>
PLIST

# Guard: the F4 silent-update bug was a MISSING plist key that nothing caught
# (SUAutomaticallyUpdate without SUEnableAutomaticChecks => never auto-checks).
# Fail the build if any required Sparkle auto-update key is absent, so it can
# never silently regress again.
PLIST_FILE="$APP/Contents/Info.plist"
for k in SUFeedURL SUPublicEDKey SUEnableAutomaticChecks SUAutomaticallyUpdate SUScheduledCheckInterval; do
  if ! /usr/libexec/PlistBuddy -c "Print :$k" "$PLIST_FILE" >/dev/null 2>&1; then
    echo "FATAL: Info.plist is missing required Sparkle key '$k' (silent auto-update would break — F4)" >&2
    exit 1
  fi
done
# Same guard for the Local Network keys: NSBonjourServices missing => the phone
# can't discover "Pulsar" even after the user grants access (the invisible-speaker
# class of bug), and it fails silently with no build error. Do not let it regress.
for k in NSLocalNetworkUsageDescription NSBonjourServices; do
  if ! /usr/libexec/PlistBuddy -c "Print :$k" "$PLIST_FILE" >/dev/null 2>&1; then
    echo "FATAL: Info.plist is missing required Local Network key '$k' (Spotify can't find the speaker)" >&2
    exit 1
  fi
done
# Content too, not just presence (PR #1): the declared service must be the one
# the phone browses. Index lookup, not text grep — PlistBuddy indentation is
# not a contract.
if [[ "$(/usr/libexec/PlistBuddy -c "Print :NSBonjourServices:0" "$PLIST_FILE" 2>/dev/null)" != "_spotify-connect._tcp" ]]; then
  echo "FATAL: NSBonjourServices must declare _spotify-connect._tcp (Spotify can't find the speaker)" >&2
  exit 1
fi

SIGN_CN="${SIGN_CN:-duet-nodeapp}"
KEYCHAIN="$HOME/Library/Keychains/duet-signing.keychain-db"
CERT_HASH="$(security find-certificate -c "$SIGN_CN" -Z 2>/dev/null | awk '/SHA-1/ {print $NF}' || true)"

if [[ -n "$CERT_HASH" ]]; then
    security unlock-keychain -p "duet-signing-local" "$KEYCHAIN" 2>/dev/null || true
    # Nested code first: the bundled daemon must carry its own signature.
    # SIGN_OPTS: dev identity keeps --timestamp=none; Developer ID (CI, R3)
    # needs hardened runtime + secure timestamp for notarization.
    SIGN_OPTS=(--timestamp=none)
    if [[ "${HARDENED:-}" == "1" ]]; then SIGN_OPTS=(--options runtime --timestamp); fi
    if [[ -x "$APP/Contents/MacOS/go-librespot" ]]; then
      codesign --force --sign "$CERT_HASH" --identifier "works.relux.pulsar.librespot" "${SIGN_OPTS[@]}" "$APP/Contents/MacOS/go-librespot"
    fi
    if [[ -d "$APP/Contents/Frameworks/Sparkle.framework" ]]; then
      codesign --force --deep --sign "$CERT_HASH" "${SIGN_OPTS[@]}" "$APP/Contents/Frameworks/Sparkle.framework"
    fi
    codesign --force --sign "$CERT_HASH" --identifier "$BUNDLE_ID" "${SIGN_OPTS[@]}" "$APP"
else
    echo "WARNING: identity '$SIGN_CN' not found (run scripts/setup-signing.sh);" >&2
    echo "         signing ad-hoc — TCC Automation will NOT survive updates (goal DoD-2)" >&2
    if [[ -x "$APP/Contents/MacOS/go-librespot" ]]; then
      codesign --force --sign - --identifier "works.relux.pulsar.librespot" "$APP/Contents/MacOS/go-librespot"
    fi
    if [[ -d "$APP/Contents/Frameworks/Sparkle.framework" ]]; then
      codesign --force --deep --sign - "$APP/Contents/Frameworks/Sparkle.framework"
    fi
    codesign --force --sign - --identifier "$BUNDLE_ID" "$APP"
fi

echo "built $APP (version $VERSION)"
codesign -dv "$APP" 2>&1 | grep -E "Identifier|Signature|Authority" || true
codesign -d -r- "$APP" 2>&1 | grep "designated" || true
