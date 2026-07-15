# P2 macOS native streaming decoder probe

## Decision

`macos-avfoundation-resource-loader-v1` is **rejected as the production
streaming candidate**. AVFoundation decodes every frozen MP3, AAC and Ogg/Opus
smoke fixture, but the measured `AVAssetResourceLoaderDelegate` path requests
at least the complete source before the first PCM sample. The result therefore
does not satisfy `start_before_full_download`, even though reads are chunked and
bounded. This is an engineering result, not a physical-hardware, audible,
production-signing, notarization or supported-OS-matrix claim.

The result is still useful for the comparative ADR: native macOS decoding has
broad format coverage and small memory use, but this AVFoundation adapter
cannot be paired with the Windows Media Foundation candidate as a conforming
streaming implementation without an additional incremental demux/decode layer
or a canonical fully prepared local-cache policy that changes the frozen gate.

## Frozen boundary

The signed app uses a custom `pulsar-range://` `AVURLAsset`. A retained
`AVAssetResourceLoaderDelegate` is the only component allowed to read the
sealed immutable fixture that stands in for an app-private cached object. It declares byte-range
support, clamps every underlying read to 65,536 bytes and records offsets,
operation count and cumulative bytes. Production would attach the same narrow
adapter to the already frozen authenticated private cache; this decoder probe
contains no bearer or network access. There is no `AVAudioEngine`, output
device or render callback in the probe.

The pull loop exercises:

- coordinator-style monotonic scheduled start;
- pause without calling `copyNextSampleBuffer`, including detection of
  background resource-loader reads;
- a new reader generation at a five-second VBR seek point;
- resume, PCM drain and cooperative `cancelReading()`;
- exact error domain, code and description when a container is rejected.

The application is ad-hoc signed with hardened runtime and exactly one
entitlement, `com.apple.security.app-sandbox=true`. It has no network client,
network server, audio-input or audio-output entitlement. The executable also
checks the sandbox and network-client entitlements from its live task.

## Local measured result

The reproducible local run used Intel macOS 15.7.4 (build 24G517). All six
fixtures decoded to 48 kHz stereo float PCM. The maximum individual cache read
was 65,536 bytes and peak RSS remained about 23 MiB. Start and seek timings
were generally single-digit to low-double-digit milliseconds, but repeated
runs also exposed a 110 ms scheduled-start sample and one background read
during a 20 ms pause. Every run read at least the complete source before its
first sample; cumulative counts can exceed source size because AVFoundation
re-requests probe or seek ranges. Those are measured candidate failures, not
test-harness failures.

Exact per-fixture values live in `evidence.json`; `receipt.json` binds that file
to the signed executable hash, architecture, OS string, hardened-runtime flag
and entitlement plist. CI runs the same package independently on hosted Apple
silicon and Intel macOS runners. Physical Macs, route/device behavior and the
supported release matrix remain in `EPIC-260714-th54l3`.

## Reproduction

```sh
python3 scripts/codec_spike/validate_macos_native_probe.py
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/codec_spike/test_codec_spike.py
bash scripts/codec_spike/build_macos_native_probe.sh .temp/macos-native-codec-probe
```

The final command compiles the Swift probe from the installed macOS SDK,
assembles the `.app`, copies the six frozen fixtures into sealed resources,
applies the sandbox entitlement and hardened runtime, verifies the signature,
runs the app, validates fail-closed evidence and emits the receipt. Candidate
rejection is a successful probe outcome; malformed evidence, missing signing
posture or an unexplained format error still fails the command.

## Remaining boundary

This task does not approve a native decoder for shipping. The next comparative
matrix must keep the candidate rejected unless a new adapter demonstrates
incremental first PCM and all lifecycle gates without hidden full-file
materialization. Developer ID signing, notarization, physical Apple-silicon
hardware, audible routes and OS-version coverage remain explicitly unproven.
