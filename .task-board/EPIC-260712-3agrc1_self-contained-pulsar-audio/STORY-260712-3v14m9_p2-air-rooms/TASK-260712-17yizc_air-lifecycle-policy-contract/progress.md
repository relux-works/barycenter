## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-15T11:14:51Z

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
Accepted on exact engineering head 77fb68231e0c18a1ecb9bdeae5725386d5e64a1a. Clean local acceptance passed 12/12. Hosted run 29410722718 passed coordinator, node-core, pulsar-win and pulsar-win-packaged-probe. PR #82 merged to main at b5d10b26b22fc4cae88fef590191f8015f401fb9. No physical-device or release claim is made.

## Precondition Resources
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-17yizc/p2-air-rooms-lifecycle-sequence.puml) — Lifecycle and alias sequence to freeze the Air contract

## Outcome Resources
- [p2-air-lifecycle-policy-contract-v1.md](file://TASK-260712-17yizc/p2-air-lifecycle-policy-contract-v1.md) — Normative Air lifecycle, policy, alias and single-authority cutover contract
