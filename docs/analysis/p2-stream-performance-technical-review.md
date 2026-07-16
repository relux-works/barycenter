# Phase 2 streamed-track performance and realtime technical review

Date: 2026-07-16  
Task: `TASK-260712-28mn7w`  
Reviewed base: `f1041ae6a340e7368cf87a1fd8f90fa2cd4203b3`  
Engineering reviewer: `codex-inline-reviewer`  
Independent approval task: `TASK-260716-3voo6j`

## Decision

The repository engineering review is complete and production remains
blocked. One High lifecycle defect was fixed and technically re-reviewed.
Three High gates remain open: a bounded whole-object integrity design for long
tracks, the signed packaged physical performance matrix, and an
implementation-independent signature.

This outcome permits the next strict-sequence reversible engineering task to
start under the owner's continuation instruction. It does **not** permit
`stream_track_v1` capability advertisement, a production decoder, streamed
playback activation, Phase 2 promotion, or a performance claim.

The same root session implemented prior streamed-track work, so this document
is deliberately a technical review rather than an independent acceptance.
Ivan Oparin owns the independent approval in `TASK-260716-3voo6j`. Physical
application and hardware work remains in manual epic `EPIC-260714-th54l3`.
The fail-closed machine-readable record is
`acceptance/phase2/stream-performance-review-v1.json`.

## Frozen gates

The review preserves the exact accepted rubric:

| Gate | Required result | Repository result |
|---|---:|---|
| Track start | nearest-rank p95 <= 5,000 ms, 3 warmups + 30 samples | Manual required |
| Seek to audible audio | nearest-rank p95 <= 3,000 ms, 3 warmups + 30 samples | Manual required |
| Scheduled skew | nearest-rank p95 <= 100 ms, 3 warmups + 30 samples | Manual required |
| Process-tree RSS | maximum <= 200 MiB | Manual required |
| Duration growth | <= 16 MiB and absolute slope <= 1 MiB/hour through two hours | Manual required |
| Pairings | Windows-Windows, Windows-macOS, macOS-macOS | Manual required |

Repository automation can verify structural bounds and deterministic state
transitions. Mock clocks, test decoders and synthetic PCM cannot produce any
of the measurements in this table.

## Source review

The review pins SHA-256 for the codec rubric, range/cache contract, rollout
handoff, Phase 2 matrix, coordinator flow and accounting, and both platform
candidate cache/player implementations.

The coordinator flow retains all-target readiness, fresh post-seek readiness,
generation-bound progress and end events, Air catch-up without leader restart,
leave cancellation for leavers only, and completion only after all counted
participants report drained end. Its race tests cover stale generations,
rebuffer, join/leave, failure and timeout paths. Accounting remains in the one
canonical store path and its reconciliation tests run under the race detector.

Both render seams access only their fixed SPSC ring, atomics and bounded
arithmetic. Network, cache I/O, decode, allocation, waits, queue sync and
blocking locks remain on control or worker paths. The production Windows and
macOS compositions still advertise no streamed-track capability and register
no production decoder.

## Findings

### P2-PERF-001 — High — fixed and technically re-reviewed

`WindowsStreamCandidatePlayer.Close` cancelled the worker but returned before
decoder or cache cleanup completed. The defect surfaced twice as a hosted
Windows `TempDir RemoveAll` failure, including packaged-probe run
`29497999354`.

`Close` now joins the serialized decoder/cache worker after cancellation and
after closing the event channel. The deterministic
`TestWindowsStreamCandidateCloseJoinsDecoderCleanup` fixture delays cleanup
after observing cancellation and requires it to have completed when `Close`
returns. That test and the formerly flaky clock-failure test passed 100
repetitions; the full Windows suite passed under the race detector. The hosted
packaged-probe rerun job `87620208950` also passed.

### P2-PERF-002 — High — open, production blocking

The candidate integrity story is not valid for production long tracks.
Windows `VerifyWhole` requires every chunk to coexist at terminal verification,
but the immutable per-variant ceiling is 64 MiB. Any larger permitted variant
must evict an earlier chunk and therefore cannot pass terminal verification.
The macOS candidate verifies individual chunks but has no equivalent bounded
whole-object completion proof.

Raising the cache ceiling or dropping whole-object integrity would weaken the
frozen contract. The exact selected codec/player design must instead provide a
reviewed bounded proof that validates each chunk before decode and the entire
object without full download or duration-proportional app-private storage.
Closure belongs to the exact codec, legal and supply-chain task
`TASK-260716-tlxe3s`. Until then, the empty production decoder registry is the
correct behavior.

### P2-PERF-003 — High — open, manual blocking

There is no accepted signed packaged physical evidence for one-hour/two-hour
playback, all three pairings, p95 start/seek/skew, process-tree RSS, real
network faults, audible drain, Air catch-up/leave, or profiler callback safety.
Repository tests are not substitutes. `TASK-260712-1fpb9q` and
`TASK-260712-2bdi4a` own those runs in manual epic `EPIC-260714-th54l3`.

### P2-PERF-004 — High — open, external review

The current reviewer is not independent. `TASK-260716-3voo6j` requires an
implementation-independent reviewer to inspect the exact candidate commit,
rerun representative checks, review the physical matrix, and sign only after
all Critical and High findings are fixed and re-reviewed.

## Representative reruns

The following checks passed on 2026-07-16:

```text
(cd pulsar-win && go test ./... -run 'TestWindowsStreamCandidate(CloseJoinsDecoderCleanup|ClockFailureDoesNotConsumeGenerationCommand)$' -count=100)
(cd pulsar-win && go test -race ./...)
(cd coordinator && go test -race ./internal/session ./internal/store ./internal/protocol ./internal/presentation)
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacStreamTrackPlayerTests
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v scripts/codec_spike/test_codec_spike.py scripts/acceptance/test_streamed_track_rollout.py scripts/acceptance/test_phase2_gate_matrix.py
```

The stress group completed 100 repetitions, the macOS group completed six
candidate-player tests, and the Python group completed 38 contract tests.
The local macOS run targeted the available x86_64 test host; it is not the
required signed Apple-silicon application evidence.

## Reopen and production rule

Production streamed playback remains blocked until one exact decoder/player
combination is accepted, P2-PERF-002 has a bounded implementation and tests,
the physical matrix closes P2-PERF-003, and an independent reviewer closes
P2-PERF-004 against the exact commit. Any later change to the reviewed cache,
player, coordinator, accounting, rubric, gate matrix or rollout sources
invalidates the affected hashes and requires delta review.
