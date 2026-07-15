#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SOURCE_ARCHIVE=""
OUTPUT="$ROOT/.temp/codec-bundle-probe"
PLATFORM="darwin"
ARCH="$(uname -m)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source) SOURCE_ARCHIVE=$2; shift 2 ;;
    --output) OUTPUT=$2; shift 2 ;;
    --platform) PLATFORM=$2; shift 2 ;;
    --arch) ARCH=$2; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$SOURCE_ARCHIVE" || ! -f "$SOURCE_ARCHIVE" ]]; then
  echo "--source must name the pre-fetched FFmpeg 8.1.2 source archive" >&2
  exit 2
fi

SOURCE_ARCHIVE=$(cd "$(dirname "$SOURCE_ARCHIVE")" && pwd)/$(basename "$SOURCE_ARCHIVE")
if [[ "$OUTPUT" != /* ]]; then
  OUTPUT="$ROOT/$OUTPUT"
fi

EXPECTED=464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$SOURCE_ARCHIVE" | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 "$SOURCE_ARCHIVE" | awk '{print $1}')
fi
[[ "$ACTUAL" == "$EXPECTED" ]] || { echo "FFmpeg source digest mismatch" >&2; exit 1; }

rm -rf "$OUTPUT"
mkdir -p "$OUTPUT/source" "$OUTPUT/build" "$OUTPUT/prefix" "$OUTPUT/stage"
tar -xf "$SOURCE_ARCHIVE" -C "$OUTPUT/source" --strip-components=1

CONFIGURE=(
  --prefix="$OUTPUT/prefix"
  --disable-everything --disable-autodetect --disable-programs --disable-doc
  --disable-network --disable-static --enable-shared --enable-pic
  --disable-avdevice --disable-avfilter --disable-swscale
  --disable-gpl --disable-version3 --disable-nonfree
  --enable-protocol=file
  --enable-demuxer=aac,mov,mp3,ogg
  --enable-decoder=aac,mp3,opus
  --enable-parser=aac,mpegaudio,opus
  --enable-swresample
)

if [[ "$PLATFORM" == "darwin" ]]; then
  CONFIGURE+=(--cc=clang "--extra-cflags=-O2 -fno-common" --extra-ldflags=-Wl,-dead_strip)
  if [[ "$ARCH" == "x86_64" ]] && ! command -v nasm >/dev/null 2>&1; then
    CONFIGURE+=(--disable-x86asm)
  elif [[ "$ARCH" == "arm64" && "$(uname -m)" != "arm64" ]]; then
    CONFIGURE+=(--enable-cross-compile --target-os=darwin --arch=arm64
      --cc=xcrun\ -sdk\ macosx\ clang\ -arch\ arm64)
  fi
else
  echo "unsupported platform: $PLATFORM" >&2
  exit 2
fi

CLANG=(clang)
if [[ "$PLATFORM" == "darwin" && "$ARCH" == "arm64" && "$(uname -m)" != "arm64" ]]; then
  CLANG+=(-arch arm64)
fi

(
  cd "$OUTPUT/build"
  "$OUTPUT/source/configure" "${CONFIGURE[@]}"
  make -j"$(sysctl -n hw.logicalcpu 2>/dev/null || echo 2)"
  make install
)

FRAMEWORKS="$OUTPUT/stage/PulsarCodecProbe.app/Contents/Frameworks"
MACOS="$OUTPUT/stage/PulsarCodecProbe.app/Contents/MacOS"
mkdir -p "$FRAMEWORKS" "$MACOS"

for library in avutil swresample avcodec avformat; do
  link="$OUTPUT/prefix/lib/lib${library}.dylib"
  [[ -L "$link" ]] || { echo "missing shared library $link" >&2; exit 1; }
  major=$(basename "$(otool -D "$link" | tail -n 1)")
  cp -L "$link" "$FRAMEWORKS/$major"
done

for binary in "$FRAMEWORKS"/*.dylib; do
  name=$(basename "$binary")
  install_name_tool -id "@rpath/$name" "$binary"
  while IFS= read -r dependency; do
    dependency=$(printf '%s' "$dependency" | xargs)
    dependency=${dependency%% *}
    if [[ "$dependency" == "$OUTPUT/prefix/lib/"* ]]; then
      install_name_tool -change "$dependency" "@rpath/$(basename "$dependency")" "$binary"
    fi
  done < <(otool -L "$binary" | tail -n +2)
done

"${CLANG[@]}" -dynamiclib -O2 -fvisibility=hidden \
  -I"$OUTPUT/prefix/include" \
  -I"$ROOT/scripts/codec_spike/native" \
  "$ROOT/scripts/codec_spike/native/pulsar_codec_bridge.c" \
  -L"$OUTPUT/prefix/lib" -lavformat -lavcodec -lswresample -lavutil \
  -Wl,-rpath,@loader_path -Wl,-install_name,@rpath/libpulsar_codec_bridge.dylib \
  -o "$FRAMEWORKS/libpulsar_codec_bridge.dylib"

for dependency in $(otool -L "$FRAMEWORKS/libpulsar_codec_bridge.dylib" | tail -n +2 | awk '{print $1}'); do
  if [[ "$dependency" == "$OUTPUT/prefix/lib/"* ]]; then
    install_name_tool -change "$dependency" "@rpath/$(basename "$dependency")" \
      "$FRAMEWORKS/libpulsar_codec_bridge.dylib"
  fi
done

"${CLANG[@]}" -O2 -I"$ROOT/scripts/codec_spike/native" \
  "$ROOT/scripts/codec_spike/native/pulsar_codec_probe.c" \
  -L"$FRAMEWORKS" -lpulsar_codec_bridge \
  -Wl,-rpath,@executable_path/../Frameworks \
  -o "$MACOS/pulsar-codec-probe"

cat > "$OUTPUT/stage/PulsarCodecProbe.app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>pulsar-codec-probe</string>
<key>CFBundleIdentifier</key><string>live.barycenter.pulsar.codec-probe</string>
<key>CFBundleName</key><string>PulsarCodecProbe</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>1</string>
</dict></plist>
PLIST

for binary in "$FRAMEWORKS"/*.dylib; do codesign --force --sign - "$binary"; done
codesign --force --sign - "$MACOS/pulsar-codec-probe"
codesign --force --sign - "$OUTPUT/stage/PulsarCodecProbe.app"
codesign --verify --strict --deep --verbose=2 "$OUTPUT/stage/PulsarCodecProbe.app"

cp "$OUTPUT/build/ffbuild/config.mak" "$OUTPUT/config.mak"
python3 "$ROOT/scripts/codec_spike/inventory_bundled_probe.py" \
  --stage "$OUTPUT/stage/PulsarCodecProbe.app" \
  --config "$OUTPUT/config.mak" \
  --platform "macos-$ARCH" \
  --output "$OUTPUT/receipt.json"

echo "$OUTPUT/stage/PulsarCodecProbe.app"
