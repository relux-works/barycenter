# P2 comparative streaming evidence matrix

Date: 2026-07-15

Task: `TASK-260712-ibuaxj`

Normative artifact: `acceptance/codec-spike/comparative-matrix-v1.json`

## Decision

No complete Windows plus macOS decoder combination passes every frozen hard
gate. Production selection is therefore **not allowed**. This conclusion is
not a weighted score: one failed format, missing pairing, failed hard gate or
unrun mandatory measurement rejects the whole combination.

The matrix is generated directly from the pinned hosted artifacts attached to
the four candidate tasks. Every source path and SHA-256 is embedded in the
matrix, and validation regenerates the complete JSON before comparing it with
the checked-in copy. A changed receipt cannot silently retain an old decision.

| Complete combination | All MP3/AAC/Opus smoke formats | Incremental start and random seek | Bounded lifecycle/hostile smoke | Frozen 30-sample and two-hour matrix | Release package and license closure | Result |
| --- | --- | --- | --- | --- | --- | --- |
| Bundled FFmpeg 8.1.2 on Windows and macOS | pass | not run end to end | pass | not run | fail | rejected |
| Media Foundation plus native macOS AVFoundation | fail: Windows Ogg/Opus | fail: macOS full-source preparation | incomplete | not run | fail | rejected |
| Pure-Go composite on Windows and macOS | fail: AAC absent | fail: MP3 full-scan seek and Ogg no random seek | pass for supported formats | not run | fail | rejected |

Each row is expanded independently for `windows_windows`, `windows_macos` and
`macos_macos`. All nine pairing rows are rejected with the exact non-pass gate
identifiers; a cross-platform row cannot inherit a pass from another pairing.

## Comparable facts retained

The bundled prototype decodes and seeks all six 12-second fixtures on hosted
Windows amd64 and macOS ARM64. It retains bounded process/RSS, CPU, lifecycle,
hostile-input and package receipts. It does not decode directly from the range
substrate: the coordinator gives it a prepared local file. Consequently its
smoke success cannot prove start before full download, reset/stall recovery,
cache reuse or two-hour duration independence. Windows ARM64 decode, production
signing, notarization, release SBOM/advisory/counsel closure and accepted native
decoder isolation are also absent.

The native combination preserves a real AppContainer Media Foundation package
on Windows and a sandboxed AVFoundation app on macOS. Windows returns
`0xC00D36C4` for both real Ogg/Opus fixtures. The macOS probe decodes all six
formats but requests at least the complete source before first PCM; cold MP3
scheduled skew is 213 ms on hosted ARM64 against the 100 ms gate. These are raw
format-specific failures, not prose deductions hidden behind a combined value.

The CGo-free pure-Go composite produces incremental first PCM for MP3 and Ogg,
uses a fixed 1 MiB PCM ring and passes hostile/race coverage. MP3 seek-enabled
construction nevertheless consumes 289,818 bytes for the 289,197-byte CBR
fixture and 52,674 bytes for the 52,437-byte VBR fixture. The Ogg reader exposes
no random-seek contract. AAC is rejected before reads because the only audited
module is GPL-2.0-only and intentionally absent. Heap-system telemetry is kept
distinct from RSS.

## Transport and manual boundary

The shared range/cache substrate has deterministic repository coverage for
normal, no-range, slow, reset, truncate, corruption, ETag-flip and revocation
profiles. The matrix labels this `repository-pass-only`; it does not transform
substrate tests into candidate-specific packaged evidence.

Physical packaged Windows/macOS pairing timings, 30 measured samples per group,
one- and two-hour process-tree RSS, real route behavior and production-shaped
hardware evidence remain unclaimed in `EPIC-260714-th54l3`. The missing rows are
explicit `not-run` gates. They cannot be converted to pass by this engineering
task or averaged with the short hosted probes.

## Reproduction

```sh
python3 scripts/codec_spike/generate_comparative_matrix.py
python3 scripts/codec_spike/validate_comparative_matrix.py
python3 -m unittest scripts/codec_spike/test_codec_spike.py
```

The next ADR task may publish the result and a remediation handoff, but it may
not identify a production-selected decoder until one complete combination has
every format row, hard gate and required pairing at `pass`.
