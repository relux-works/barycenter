# Phase 3 realtime audio technical pre-review

- Date: 2026-07-17
- Original task: `TASK-260712-3j4a06`
- Root-reviewed source: `d94f51644a3acf37601b4a869b4247380372f9ec`
- Root-reviewed tree: `4e4cca878db806650eda6f1e1642051b87a18b93`
- Engineering reviewer: `codex-inline-pre-reviewer`
- Independent approval: `TASK-260717-3dbi2v`, owner Ivan Oparin

## Decision

The repository technical pre-review is complete. No new Critical or High code
finding was found in the frozen non-E2EE realtime path, and the two High
realtime/privacy findings fixed by the root review remain closed under the
representative reruns below. Reversible strict-sequence engineering may move to
`TASK-260712-1x5jfo`.

This is deliberately **not** an independent acceptance. The same inline
execution chain implemented parts of the reviewed paths, no physical hardware
identity was supplied, and C1-C3 have not run. `live_ptt`, C1-C3 acceptance and
Phase 3 promotion therefore remain blocked. The machine-readable fail-closed
record is `acceptance/phase3/realtime-technical-pre-review-v1.json`.

## Reviewed seams

The audit traced the sender and target lifecycle through coordinator admission,
accept/reject, capture ownership, binary relay, per-target backpressure, jitter
decode, audible consumption, terminal cleanup and capture-quality reporting.

| Seam | Repository conclusion |
| --- | --- |
| Capture authority | A fresh generation and accepted local hold own one microphone route. Stale release, watchdog, lock, sleep, disconnect, rollback and quit paths close it idempotently. |
| Coordinator bounds | Sessions, targets, events, frame rate and frame payloads have explicit ceilings. Audio bytes are relayed ephemerally and are not retained or persisted. |
| Backpressure | A slow target is removed independently; it does not block a fast target or create an unbounded coordinator queue. |
| Jitter/loss | The receive window is bounded, authenticates and generation-checks frames before decode, reorders within the frozen window and selects FEC before bounded PLC. |
| Audio callback | Source guards and tests keep transport, storage, waits, locks and allocation outside capture/render callbacks. Fixed rings and atomics cross the callback boundary. |
| Audible receipt | macOS now waits for the render-consumed frame counter; Windows uses the corresponding render seam. A route activation alone is not audible evidence. |
| Mixer/limiter | Live audio shares the post-mix limiter. Capture safety applies bounded gain slew and the distinct input ceiling last. |
| AEC/route claims | Unknown routes, unavailable processing and speaker reference ambiguity degrade explicitly. Synthetic fixtures do not claim audible C3 success. |

## Reproduced evidence

All commands ran against source-identical root-reviewed paths on 2026-07-17:

```text
(cd coordinator && go test -race -count=10 ./internal/session ./internal/protocol ./internal/hub && go test -race -count=10 ./internal/store -run 'LivePTT|Phase3' && go test -race -count=10 ./cmd/duet-coordinator -run 'LivePTT|Phase3')
(cd pulsar-win && go test -race -count=10 ./... -run 'Live|Capture|Audio|Mixer|PTT|Jitter')
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter 'Live|Capture|Audio|Mixer|PTT|Jitter'
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v scripts/live_ptt/test_transport_model.py scripts/live_ptt/test_codec_transport.py scripts/capture_quality/test_harness.py scripts/capture_quality/test_integrated.py
python3 scripts/capture_quality/validate_integrated.py
```

The Windows run passed four packages under race and ten repetitions. The Swift
run passed 75 tests in 14 suites on the available x86_64 macOS test host. The
Python group passed 23 tests; the integrated harness recorded 18 cells and 252
synthetic fixture runs with `manualEvidence=not-run`. The coordinator group
passed under race and ten repetitions.

An earlier deliberately broad attempt ran the entire 302-test store package ten
times under race. It exceeded Go's default ten-minute package timeout while an
unrelated identity rollback test was executing, so it is recorded as
`timeout-not-counted`, not as passing evidence and not as a realtime finding.
The source-scoped store `LivePTT|Phase3` group then passed ten repetitions under
race in 39.918 seconds; the coordinator command group passed in 59.509 seconds.

## Evidence boundary and external closure

Repository evidence covers deterministic transitions, exercised race safety,
bounded synthetic jitter/backpressure, callback source guards, limiter and
capture-quality math, and fixed-code diagnostics. It cannot prove foreground
application hold/release, two-home mouth-to-ear latency, real two-percent-loss
intelligibility, audible speaker/headphone echo and double-talk, or packaged
device-route behavior.

Those physical checks remain in manual task `TASK-260712-flaiie` under
`EPIC-260714-th54l3`. Ivan Oparin must select a reviewer who implemented none of
the paths and close `TASK-260717-3dbi2v` against the exact root-reviewed commit
and signed C1-C3 artifacts. Every Critical or High finding must be fixed and
independently re-reviewed. Any affected source, fixture, build or runtime-config
delta reopens the root and realtime reviews.
