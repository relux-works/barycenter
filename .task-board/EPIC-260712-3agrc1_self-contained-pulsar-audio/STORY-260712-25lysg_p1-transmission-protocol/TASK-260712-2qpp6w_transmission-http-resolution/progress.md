## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T09:36:09Z

## Blocked By
- TASK-260712-51y5k9
- TASK-260712-1aprcb
- TASK-260712-m5264f
- TASK-260712-2af2dp

## Blocks
- TASK-260712-31vvjt
- TASK-260712-2qc27p
- TASK-260712-21ers7
- TASK-260712-3e4p0c
- TASK-260712-2fe5bz
- TASK-260712-3dqc3l
- TASK-260712-2h6snp
- TASK-260712-1c34fe
- TASK-260712-25862f

## Checklist
- [x] Implement create, status, and cancel handlers with control versus audience auth
- [x] Resolve audiences into explicit targets and persist effective delivery data
- [x] Preserve acceptance ordering inputs and unauthorized media error behavior
- [x] Enforce origin defaults, clip-delivery matrix, overlay duration and coordinator-owned ordering
- [x] Apply one stable after_current downgrade to the entire transmission when any mandatory target lacks capability
- [x] Return and consume a stable requires_confirmation contract before any interrupt fallback

## Notes
Strict inline execution started from synchronized main merge 24730209e60cfcb24c8b41577a0648ba1d0a5327 on branch task/task-260712-2qpp6w-transmission-http-resolution. Scope is the frozen transmission-v1 create/status/cancel HTTP contract, immutable audience resolution, whole-transmission capability downgrade and explicit interrupt confirmation. Best-effort coding and automated tests only; real-app and physical-hardware checks remain in manual epic EPIC-260714-th54l3.
Implemented strict create/status/cancel HTTP resolution with transactional bearer reauthentication, actor-scoped hashed idempotency, immutable live-binding audience snapshots, media/policy/capability evaluation, whole-delivery downgrade, hashed single-use interrupt confirmation, safe visibility and generation-bound cancellation handoff. Root review added omitted-vs-empty slot validation, empty-selector and corrupt-link fail-closed behavior, actual-socket presence, capability-aware can_cancel and non-disclosing block output. Local coordinator vet/test/race, 20x idempotency+confirmation stress, Windows vet/test/race/cross-build, Swift build, diff/resource comparison and board validation are green. Outcome resource attached. No real-app or physical-hardware result claimed; those remain in EPIC-260714-th54l3.
PR self-review found and closed a stale-WebSocket binding TOCTOU: hub capability snapshots now carry a transient SHA-256 witness of the exact authenticated node/control credential, and transactional audience resolution compares it with the current installation generation before accepting online state or capabilities. The witness is never logged, returned or persisted. A negative stale-binding regression and current node/control cases pass focused, 20x stress, full vet/test and race gates.

## Precondition Resources
- [p1-transmission-protocol-components.puml](file://TASK-260712-2qpp6w/p1-transmission-protocol-components.puml) — HTTP API and audience resolution context

## Outcome Resources
- [TASK-260712-2qpp6w_transmission-http-resolution.md](file://TASK-260712-2qpp6w/TASK-260712-2qpp6w_transmission-http-resolution.md) — Accepted HTTP resolution boundary, automated evidence and scheduler/client handoff
