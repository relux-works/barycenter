## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T10:21:23Z

## Blocked By
- TASK-260712-1g70av

## Blocks
- TASK-260712-2qc27p
- TASK-260712-3d6cnn

## Checklist
- [x] Announce clip capabilities and implement prepare/download/hash/decode-ready flow
- [x] Emit transmission lifecycle and DND or presence messages from CoordinatorClient and PlayerCore
- [x] Keep legacy play_voice and solo_voice working while routing scheduled play through mixer hooks
- [x] Use synchronized coordinator time and reject stale, duplicate or cancelled scheduled starts
- [x] Keep prepare I/O and scheduling out of render and CoordinatorClient blocking paths

## Notes
Strict inline execution started from synchronized main merge 30f1c552c9824934922becab4637c34746d190dc on branch task/task-260712-26ip33-macos-transmission-client-hooks. Scope is best-effort macOS coding and deterministic automated verification of the frozen prepare/download/hash/decode, coordinator-clock scheduling, generation idempotency, lifecycle receipts, DND/presence and legacy compatibility contract. No real-app speaker, packaged-install or physical-hardware result will be claimed; those remain in manual epic EPIC-260714-th54l3.
Implemented the macOS base media_clip_v1 client hook behind a delivery-capability-gated MediaClipMixer seam. Added same-origin bearer fetch with redirect refusal, 34 MiB bound, exact size and SHA-256, AVAudioFile decoder readiness and exact ceil duration, generation-safe prepare/play/cancel state, coordinator-clock late-window enforcement, exactly-once terminal cleanup, serialized CoordinatorClient sends, durable local DND and privacy-bounded presence, and deterministic Swift regressions. overlay_mix_v1 and interrupt_resume_v1 intentionally remain unadvertised until their strict downstream mixer tasks. Local swift build, test-file parser, coordinator vet/tests, Windows vet/tests and Windows cross-build are green; local swift test is blocked before execution by the known host toolchain missing the Testing module, so hosted macOS CI is required. No manual audio or hardware claim.
Exact implementation head 9622e00914195a5a17e4420cc1de5d8ce7a16921 passed hosted CI run 29324579129: all four jobs green and node-core reported 145 tests passed with the new suites. Root review retained only media_clip_v1, added absolute streamed-body cutoff and async-safe test locking, and found no remaining in-scope defect. PR #24 is ready for exact tracking-head validation and merge. Manual app and hardware testing remains exclusively deferred to EPIC-260714-th54l3.
PR #24 merged exact tracking head d72147c2c785c8c94bc0061396595b559db27bc0 into main at merge commit 0b54899073e4dc4948b248f7c77666e7151f5459 after CI run 29324846258 passed all four jobs.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-26ip33/p1-transmission-scheduler-sequence.puml) — macOS client flow for prepare, ready, play, and cancel

## Outcome Resources
- [macos-transmission-client-hooks-outcome.md](file://TASK-260712-26ip33/macos-transmission-client-hooks-outcome.md) — Accepted automated implementation evidence and downstream mixer boundary
