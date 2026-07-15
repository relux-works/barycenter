#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUTPUT="${1:-$ROOT/.temp/macos-native-codec-probe}"
SOURCE="$ROOT/scripts/codec_spike/macos-native"
FIXTURES="$ROOT/acceptance/codec-spike/fixtures/smoke-v1"
CONTRACT="$ROOT/acceptance/codec-spike/macos-native-probe-v1.json"
APP="$OUTPUT/PulsarMacNativeCodecProbe.app"

rm -rf "$OUTPUT"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

xcrun swiftc \
  -O \
  -swift-version 5 \
  -framework AVFoundation \
  -framework AudioToolbox \
  -framework CoreMedia \
  -framework Security \
  -framework UniformTypeIdentifiers \
  "$SOURCE/MacNativeCodecProbe.swift" \
  -o "$APP/Contents/MacOS/pulsar-macos-native-codec-probe"

cp "$SOURCE/Info.plist" "$APP/Contents/Info.plist"
cp "$CONTRACT" "$APP/Contents/Resources/macos-native-probe-v1.json"
cp "$FIXTURES"/*.mp3 "$FIXTURES"/*.m4a "$FIXTURES"/*.aac "$FIXTURES"/*.ogg "$APP/Contents/Resources/"

codesign --force --sign - --options runtime --entitlements "$SOURCE/Probe.entitlements" "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
codesign -dv --verbose=4 "$APP" >"$OUTPUT/codesign-display.log" 2>&1

"$APP/Contents/MacOS/pulsar-macos-native-codec-probe" >"$OUTPUT/evidence.json"
python3 "$ROOT/scripts/codec_spike/validate_macos_native_probe.py" \
  --evidence "$OUTPUT/evidence.json" \
  --app "$APP" \
  --receipt "$OUTPUT/receipt.json"

printf '%s\n' "$OUTPUT"
