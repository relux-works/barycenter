## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:44:15Z

## Last Update
2026-07-14T18:29:29Z

## Blocked By
- TASK-260712-1hqiek
- TASK-260712-2zbmq4

## Blocks
- TASK-260712-3d6cnn

## Checklist
- [x] Capture interrupt resume anchors from audible position rather than raw provider position
- [x] Coordinate provider pause, clip playback, resume seek and fade-in without stale timers
- [x] Report capability or resume failures distinctly and keep the node ready for subsequent overlays
- [x] Derive the resume anchor from audible position including buffered ring frames
- [x] Return capability error without fallback when exact interrupt is unavailable

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge adaac8c after TASK-260712-1g6lk8 acceptance. Scope is exact macOS interrupt semantics on the existing prepared/render-safe branch: audible anchor including ring tail, deterministic 250 ms pre-fade, provider pause/seek/resume, 120 ms fade-in, generation-safe cancel/stop/provider/reconnect cleanup and explicit capability/resume failure without overlay or after_current fallback. Physical A4 timing and audible evidence remain in EPIC-260714-th54l3.
2026-07-14 implementation checkpoint: macOS now binds interrupt_resume_v1 only with the exact PlayerCore controller, drives the wire 250 ms pre-fade on the existing prepared AVAudioPlayerNode branch, captures audible provider position minus queued ring tail, serializes pause/seek/resume behind a provider barrier and applies the 120 ms default fade-in. Reconnect, stop and provider restart reset prepared generations and invalidate tokens; cancel during resume has one terminal outcome; unsupported ownership is interrupt_capability_lost with no fallback; resume failure maps to media_failed(play/audio_graph_failed) and leaves the mixer reusable. Full 162-test Swift suite, 20 repeated focused suites, release build, coordinator tests, Windows race tests and Windows cross-build are green. Physical A4 remains deferred to EPIC-260714-th54l3.
2026-07-14 acceptance: exact engineering head 2a06f2f55379a5aeeb5e1f27fb9733adc7e01e4f passed all four hosted CI jobs in run 29357878003 (coordinator, node-core, pulsar-win and signed packaged Windows probe). Full 162-test local Swift suite, 20 repeated focused runs, release build, coordinator tests, Windows race tests and Windows cross-build passed. Accepted as best-effort engineering only; physical macOS A4 remains unclaimed in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
(none)
