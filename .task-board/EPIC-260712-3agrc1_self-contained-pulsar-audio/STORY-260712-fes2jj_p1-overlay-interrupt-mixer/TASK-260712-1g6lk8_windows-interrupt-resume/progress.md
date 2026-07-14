## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:44:15Z

## Last Update
2026-07-14T18:08:04Z

## Blocked By
- TASK-260712-1hqiek
- TASK-260712-1viwvi

## Blocks
- TASK-260712-3d6cnn

## Checklist
- [x] Capture and restore interrupt resume anchors from audible position and ring state
- [x] Coordinate daemon pause, clip playback, resume seek and fade-in on the new mixer branch
- [x] Clear pending timers or prepared clips on cancel, stop and reconnect so no ghost resume survives
- [x] Derive the resume anchor from audible position including buffered ring frames
- [x] Return capability error without fallback when exact interrupt is unavailable

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge f77b1d5 after TASK-260712-2zbmq4 acceptance. Scope is exact Windows interrupt semantics with audible-position anchoring, deterministic buffered-tail handling, provider pause/resume or seek, 250 ms fade-out, 120 ms fade-in, generation-safe cancellation/reconnect cleanup and explicit capability failure without overlay or after_current fallback. Physical A4 timing and audible evidence remain in EPIC-260714-th54l3.
2026-07-14 implementation checkpoint: Windows now has a single-owner render-safe interrupt branch with 250 ms wire-controlled pre-fade, exact main-ring stop at T, limited replacement rendering, audible anchor derived from provider/extrapolated position minus queued ring frames, off-render pause/seek/resume and 120 ms default fade-in. Load/stop/voice/wait/reconnect invalidate generations and prepared clips; unavailable exact ownership returns interrupt_capability_lost with no overlay or after_current fallback. Deterministic unit, render-boundary, reconnect and stale-token tests pass; go test -race, go vet, Windows amd64 cross-build, coordinator tests and macOS release build are green. Physical A4 remains deferred to EPIC-260714-th54l3.
2026-07-14 acceptance: exact engineering head a29db301e139e46f00154a29c2411e8578268eab passed all four hosted CI jobs in run 29356446731 (coordinator, node-core, pulsar-win and signed packaged Windows probe). Local go test, go test -race, go vet, Windows amd64 cross-build, coordinator tests and macOS release build also passed. Accepted as best-effort engineering only; physical Windows A4 remains unclaimed in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
(none)
