#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
SOURCE_ARCHIVE=""
OUTPUT="$ROOT/.temp/codec-bundle-probe-windows"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source) SOURCE_ARCHIVE=$2; shift 2 ;;
    --output) OUTPUT=$2; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$SOURCE_ARCHIVE" || ! -f "$SOURCE_ARCHIVE" ]]; then
  echo "--source must name the pre-fetched FFmpeg 8.1.2 source archive" >&2
  exit 2
fi
SOURCE_ARCHIVE=$(cd "$(dirname "$SOURCE_ARCHIVE")" && pwd)/$(basename "$SOURCE_ARCHIVE")
if [[ "$OUTPUT" != /* ]]; then OUTPUT="$ROOT/$OUTPUT"; fi

EXPECTED=464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c
ACTUAL=$(sha256sum "$SOURCE_ARCHIVE" | awk '{print $1}')
[[ "$ACTUAL" == "$EXPECTED" ]] || { echo "FFmpeg source digest mismatch" >&2; exit 1; }

rm -rf "$OUTPUT"
mkdir -p "$OUTPUT/source" "$OUTPUT/build" "$OUTPUT/prefix" "$OUTPUT/stage"
tar -xf "$SOURCE_ARCHIVE" -C "$OUTPUT/source" --strip-components=1

(
  cd "$OUTPUT/build"
  "$OUTPUT/source/configure" \
    --prefix="$OUTPUT/prefix" \
    --target-os=mingw32 --arch=x86_64 --cc=gcc \
    --disable-everything --disable-autodetect --disable-programs --disable-doc \
    --disable-network --disable-static --enable-shared --enable-pic \
    --disable-avdevice --disable-avfilter --disable-swscale \
    --disable-gpl --disable-version3 --disable-nonfree \
    --enable-protocol=file \
    --enable-demuxer=aac,mov,mp3,ogg \
    --enable-decoder=aac,mp3,opus \
    --enable-parser=aac,mpegaudio,opus \
    --enable-swresample \
    --extra-cflags='-O2 -fno-common' \
    --extra-ldflags='-Wl,--dynamicbase -Wl,--nxcompat'
  make -j"${NUMBER_OF_PROCESSORS:-2}"
  make install
)

for library in avutil swresample avcodec avformat; do
  dll=$(find "$OUTPUT/prefix/bin" -maxdepth 1 -type f -iname "${library}-*.dll" | head -n 1)
  [[ -n "$dll" ]] || { echo "missing shared library $library" >&2; exit 1; }
  cp "$dll" "$OUTPUT/stage/"
done

gcc -shared -O2 -fvisibility=hidden -static-libgcc -DPULSAR_CODEC_BUILD \
  -I"$OUTPUT/prefix/include" -I"$ROOT/scripts/codec_spike/native" \
  "$ROOT/scripts/codec_spike/native/pulsar_codec_bridge.c" \
  -L"$OUTPUT/prefix/lib" -lavformat -lavcodec -lswresample -lavutil \
  -Wl,--out-implib,"$OUTPUT/stage/libpulsar_codec_bridge.dll.a" \
  -o "$OUTPUT/stage/pulsar_codec_bridge.dll"

gcc -O2 -static-libgcc -I"$ROOT/scripts/codec_spike/native" \
  "$ROOT/scripts/codec_spike/native/pulsar_codec_probe.c" \
  -L"$OUTPUT/stage" -lpulsar_codec_bridge \
  -o "$OUTPUT/stage/pulsar-codec-probe.exe"
rm "$OUTPUT/stage/libpulsar_codec_bridge.dll.a"
cp "$OUTPUT/build/ffbuild/config.mak" "$OUTPUT/config.mak"
printf '%s\n' "$OUTPUT/stage/pulsar-codec-probe.exe"
