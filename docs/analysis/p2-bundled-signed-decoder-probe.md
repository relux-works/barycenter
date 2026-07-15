# P2 bundled signed decoder probe

Date: 2026-07-15

Task: `TASK-260712-1canzv`

Candidate: `bundled-ffmpeg-8.1.2-v1`

## Decision

The bundled candidate is an **engineering prototype only**. It is technically
viable for bounded offline MP3, AAC and Opus decode, but shipping remains
fail-closed until all required platform and release-signing evidence exists.
In particular, Windows ARM64, production Partner Center signing, Developer ID
signing/notarization and release-time counsel/security review are not proven by
this task. Missing evidence is rejection, not an implicit future assumption.

The exact candidate is pristine FFmpeg 8.1.2, source SHA-256
`464beb5e7bf0c311e68b45ae2f04e9cc2af88851abb4082231742a74d97b524c`,
built as LGPL-only shared libraries. The package contains only `avformat`,
`avcodec`, `avutil`, `swresample` and the narrow Pulsar bridge. It contains no
`ffmpeg`, `ffprobe` or `ffplay` executable, no network protocol and no dynamic
code download. The exact source signature, license, patent, source-offer,
notices, vulnerability and update obligations are frozen by the preceding
[license and distribution audit](p2-codec-license-distribution-audit.md).

## Implemented boundary

The decoder opens only a coordinator-prepared local file from the existing
private bounded chunk cache. The cache has a 1 MiB chunk ceiling and a 64 MiB
prepared-input ceiling. Network authorization, ranges, ETag changes and
revocation remain outside FFmpeg. Decode is a control/worker operation; the
render callback consumes prepared PCM and never calls the decoder.

The bridge exposes one frozen C ABI operation for the probe. It uses one decoder
thread, drains at EOF, supports timestamp seek plus decoder flush, and supports
cooperative frame-boundary cancellation. The harness maps these operations to
prepare, arm, scheduled start, pause, seek/new generation, resume, cancel and
drain. It exercises all six frozen CBR/VBR/container fixtures and five malformed
mutations under process time and output bounds.

Hostile-input containment is proven only at the disposable probe-process
boundary. The in-process bridge is not accepted as release isolation: a release
must either establish an OS-supported decoder containment boundary or explicitly
accept and review native decoder crash impact. This unresolved hardening item is
another reason the shipping decision remains rejected.

## Reproducible package evidence

The repository-only macOS x86_64 run produced an ad-hoc signed `.app` of
2,009,839 bytes. Its staged inventory was exactly four FFmpeg dylibs, one bridge
dylib, the probe driver and bundle metadata/signature. All nested dylibs passed
strict code-signature validation and their Mach-O imports were recorded. All six
fixtures decoded at 48 kHz stereo; decoded samples were 576,000–577,536. Seek
changed the PCM checksum, cancellation stopped without drain, normal completion
drained, and the five hostile cases terminated with at most 651 output bytes.
Each decode result also records process CPU milliseconds and peak resident bytes;
the harness rejects a 15-second CPU overrun or peak RSS above 256 MiB. Package
disk bytes come from the signed file inventory rather than an estimate.

The dedicated `codec-bundle-probe.yml` workflow repeats the exact recipe on
GitHub-hosted macOS ARM64, macOS Intel and Windows x64 runners. Windows compiles
the same allowlist with UCRT MinGW, runs the same lifecycle/hostile harness,
Authenticode-signs every staged PE binary with an ephemeral CI-only certificate,
then builds and verifies a test-signed MSIX. Its receipt records every payload
hash and signature state. These are engineering signatures, never release
credentials.

## Reproduction

The source archive must already exist and match the frozen digest; the build
scripts never download executable code:

```sh
bash scripts/codec_spike/build_bundled_ffmpeg.sh \
  --source /path/to/ffmpeg-8.1.2.tar.xz \
  --output .temp/bundled-probe --platform darwin --arch "$(uname -m)"
python3 scripts/codec_spike/run_bundled_probe.py \
  --driver .temp/bundled-probe/stage/PulsarCodecProbe.app/Contents/MacOS/pulsar-codec-probe \
  --output .temp/bundled-probe/decode-evidence.json
```

The Windows build is invoked by the dedicated workflow under MSYS2 UCRT64, then
packaged by `package_bundled_probe_msix.ps1`. CI artifacts contain the build
configuration, file/import/signature receipt, decode evidence and package.

## Release gates and update path

A release must rebuild rather than reuse CI artifacts, run a current advisory
and binary SBOM scan, include full LGPL notices and exact corresponding source,
retain relinking rights, and re-run the codec patent/counsel gate. Nested binaries
must be production-signed before the outer package; macOS must then pass hardened
runtime/notarization and Windows must pass Partner Center signing/validation.
Windows ARM64 needs its own native build, import inventory, signed MSIX and decode
evidence. Any version/configure/library/architecture change invalidates these
receipts and requires a new contract version.

Primary references: [FFmpeg download and signatures](https://ffmpeg.org/download.html),
[FFmpeg legal](https://ffmpeg.org/legal.html),
[Apple nested code signing](https://developer.apple.com/library/archive/technotes/tn2206/_index.html),
and [Microsoft MSIX package signing](https://learn.microsoft.com/windows/msix/package/sign-app-package-using-signtool).
