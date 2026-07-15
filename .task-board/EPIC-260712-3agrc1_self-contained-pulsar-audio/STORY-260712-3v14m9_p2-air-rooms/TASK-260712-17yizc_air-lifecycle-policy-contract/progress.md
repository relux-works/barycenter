## Status
reviewing

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-15T11:08:29Z

## Blocked By
- TASK-260712-1xik11

## Blocks
- TASK-260712-3n36ny
- TASK-260712-kr64r2
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-2i3u7v
- TASK-260712-31zja2

## Checklist
- [x] Freeze Air and membership statuses plus invite expiry and confirmation rules
- [x] Freeze alias mapping for approach and accept and decline and apart plus the current Air read model
- [x] Freeze default invite and overlay and queue and replace policies and who may mutate them
- [x] Record the exact error vocabulary and the boundaries with the explicit targets and inbox story
- [x] Separate saved membership from one active runtime pointer and freeze switch semantics
- [x] Freeze secure single-use invites, joining-primary confirmation and join or leave during media
- [x] Freeze feature-flag authority cutover so rollback never runs link and Air runtimes together

## Notes
Strict inline P2 execution started from synchronized main c0e0d4a after P1 engineering readiness. Contract work will remain additive and reversible; Phase 1 product/release holds remain external. Reading the complete Air decomposition, P2 root amendments, source spec and current P1 identity/transmission/history implementations before freezing v1 fields and state machines.
Normative contract and executable summary frozen. Self-review corrected the alias activation gap and added explicit deactivate, role and ownership-transfer operations. Verified source-of-truth, current approach runtime and P1 immutable target boundary. Local evidence: coordinator go test ./... passed; macOS Swift 218 tests in 35 suites passed; JSON parse and git diff --check passed. Awaiting clean-tree acceptance, hosted CI and merge before done.

## Precondition Resources
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-17yizc/p2-air-rooms-lifecycle-sequence.puml) — Lifecycle and alias sequence to freeze the Air contract

## Outcome Resources
- [p2-air-lifecycle-policy-contract-v1.md](file://TASK-260712-17yizc/p2-air-lifecycle-policy-contract-v1.md) — Normative Air lifecycle, policy, alias and single-authority cutover contract
