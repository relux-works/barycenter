## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T09:44:45Z

## Blocked By
- TASK-260712-1g70av

## Blocks
- TASK-260712-2qc27p
- TASK-260712-3d6cnn

## Checklist
- [ ] Announce clip capabilities and implement prepare/download/hash/decode-ready flow
- [ ] Emit transmission lifecycle and DND or presence messages from CoordinatorClient and PlayerCore
- [ ] Keep legacy play_voice and solo_voice working while routing scheduled play through mixer hooks
- [ ] Use synchronized coordinator time and reject stale, duplicate or cancelled scheduled starts
- [ ] Keep prepare I/O and scheduling out of render and CoordinatorClient blocking paths

## Notes
Strict inline execution started from synchronized main merge 30f1c552c9824934922becab4637c34746d190dc on branch task/task-260712-26ip33-macos-transmission-client-hooks. Scope is best-effort macOS coding and deterministic automated verification of the frozen prepare/download/hash/decode, coordinator-clock scheduling, generation idempotency, lifecycle receipts, DND/presence and legacy compatibility contract. No real-app speaker, packaged-install or physical-hardware result will be claimed; those remain in manual epic EPIC-260714-th54l3.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-26ip33/p1-transmission-scheduler-sequence.puml) — macOS client flow for prepare, ready, play, and cancel

## Outcome Resources
(none)
