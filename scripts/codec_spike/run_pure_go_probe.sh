#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MODULE="$ROOT/scripts/codec_spike/purego_probe"
FIXTURES="$ROOT/acceptance/codec-spike/fixtures/smoke-v1"
OUTPUT="${1:-$ROOT/.temp/purego-probe}"
if command -v cygpath >/dev/null 2>&1; then OUTPUT="$(cygpath -u "$OUTPUT")"; fi
mkdir -p "$OUTPUT/cross"
OUTPUT="$(cd "$OUTPUT" && pwd)"

extension=""
if [[ "$(go env GOOS)" == "windows" ]]; then extension=".exe"; fi
native="$OUTPUT/purego-probe$extension"

(
  cd "$MODULE"
  CGO_ENABLED=0 go build -trimpath -o "$native" ./cmd/purego-probe
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o "$OUTPUT/cross/purego-probe-darwin-arm64" ./cmd/purego-probe
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o "$OUTPUT/cross/purego-probe-windows-amd64.exe" ./cmd/purego-probe
  CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -o "$OUTPUT/cross/purego-probe-windows-arm64.exe" ./cmd/purego-probe
)

"$native" -fixtures "$FIXTURES" -output "$OUTPUT/evidence.json"
python3 "$ROOT/scripts/codec_spike/validate_pure_go_probe.py" \
  --evidence "$OUTPUT/evidence.json" \
  --binary "$native" \
  --cross-directory "$OUTPUT/cross" \
  --receipt "$OUTPUT/receipt.json"

printf '%s\n' "$OUTPUT"
