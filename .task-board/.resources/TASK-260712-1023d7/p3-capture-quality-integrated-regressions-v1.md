# P3 capture-quality integrated engineering regressions v1

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-1023d7`

Status: repository engineering regressions pass; native C3 and manual evidence not run

The checked-in sanitized result is
[`acceptance/phase3/capture-quality-integrated-regressions-v1.json`](../../acceptance/phase3/capture-quality-integrated-regressions-v1.json).
It binds the exact engineering adapter build, the frozen fixture lock and every
command receipt. Re-run it from a clean repository root with:

```sh
python3 scripts/capture_quality/run_integrated.py \
  --output .temp/capture-quality/integrated.json
python3 scripts/capture_quality/validate_integrated.py \
  --evidence .temp/capture-quality/integrated.json
```

## Matrix and assertions

Both repository-built platform safety processors consume all 14 frozen
fixtures for recorded clip, local self-test and live PTT over speaker,
headphone and unknown routes. This produces 18 independent cells and 252
fixture runs. No platform, route, workflow or case is averaged into another.

Every run checks finite output, the final `-3 dBFS` capture ceiling, bounded
`+12 dB` gain and `3 dB/s` gain slew. Separate assertions preserve the receiver
post-mix `-1 dBFS` ceiling. The adapters also require fresh generations,
fail-closed no-consent states, explicit degraded-consent lifecycle states and
zero post-callback blocking receipts. Windows additionally measures zero Go
allocations for a callback-sized block; the macOS allocation result remains a
source guard and is not represented as a physical measurement.

The orchestrator reruns the existing Windows and macOS hostile lifecycle suites
covering permission and device loss, route/reconnect changes, cancel, lock,
sleep, slow recipient, jitter, packet loss and rollback behavior. Synthetic
route and clock-drift fixtures do not stand in for Bluetooth hardware or a
native render reference.

## Honest acceptance boundary

This task proves the common product-owned safety stage and repository lifecycle
integration. It does not prove native Windows AEC/noise suppression, signed
macOS VPIO, speaker render-reference age, acoustic ERLE/SNR, canonical STOI,
blinded listening, accessibility or physical CPU/memory. Consequently every
route remains `unsupported-pending-manual-c3`, production capability advertising
remains false and C3 acceptance remains false.

Those real-app and real-hardware checks stay `not-run` in
`EPIC-260714-th54l3`. A later manual result must cite the signed build and exact
platform/route/workflow cell; it cannot reuse or reinterpret this engineering
evidence as an acoustic pass.

## Privacy and retention

The runner generates only the content-locked synthetic corpus in a temporary
directory and deletes it after the run. Adapters retain processed SHA-256
values, content-free metrics, command receipts and categorical blockers only.
They accept no user audio, device identity, path, transcript, token or secret;
the validator rejects those metadata classes in published evidence.
