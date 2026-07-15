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
and entitlement plist.

## Hosted architecture evidence

Dedicated run `29449314111` passed both jobs on exact engineering head
`c1ec5c0`. The macOS 15.7.7 ARM64 and x86_64 jobs independently compiled,
sandbox-signed, executed and validated the app. Both decoded all six exact
fixtures and both rejected the candidate for complete-source preparation plus
the cold MP3 CBR lifecycle gate:

| Measurement | ARM64 | x86_64 |
|---|---:|---:|
| Peak RSS | 28,065,792 bytes | 20,623,360 bytes |
| MP3 CBR start | 210 ms | 465 ms |
| MP3 CBR scheduled skew | 213 ms | 466 ms |
| Worst other scheduled skew | 39 ms | 29 ms |
| Worst seek-to-PCM | 14 ms | 21 ms |
| Maximum underlying read | 65,536 bytes | 65,536 bytes |

Every fixture had `bytesBeforeFirstSample >= sourceBytes`. The ARM64 executable
was 222,944 bytes with SHA-256
`e529ddb7fc3360ee2e39e5c990740457012c439faca6d997a4cb3180d880ffb4`;
the 198,416-byte x86_64 executable SHA-256 was
`7aa9123f86ed6c7427aebb4d076a618311eb86fb38c1d8e88600702f61472179`.
Both receipts record `0x10002(adhoc,runtime)`, the exact sandbox entitlement,
and identical hashes for every sealed fixture. They explicitly set
`realHardwareClaim=false`, production signature and notarization to
`not-proven`.

Physical Macs, route/device behavior and the supported release matrix remain
in `EPIC-260714-th54l3`.

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
