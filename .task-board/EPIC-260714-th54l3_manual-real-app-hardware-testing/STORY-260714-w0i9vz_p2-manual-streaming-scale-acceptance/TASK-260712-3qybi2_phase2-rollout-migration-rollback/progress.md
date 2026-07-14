## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:23:01Z

## Last Update
2026-07-14T00:49:06Z

## Blocked By
- TASK-260712-14rxuk
- TASK-260712-qi81vf
- TASK-260712-3nq0tq
- TASK-260712-1fpb9q
- TASK-260712-2ubzyf
- TASK-260712-1vklop
- TASK-260712-20cuna
- TASK-260712-1kfnpu
- TASK-260712-2bdi4a

## Blocks
- TASK-260712-2pnc5a
- TASK-260712-3u5cdn

## Checklist
- [ ] Stage additive DB migration, accept-only coordinator rollout, and capability-aware node rollout
- [ ] Rehearse rollback while preserving phase-two rows and restoring legacy behavior
- [ ] Verify feature-flag-off semantics and pairwise compatibility throughout the mixed fleet window
- [ ] Publish the exact rollback commands, fixtures, and artifact set for operators
- [ ] Quiesce Phase 2 before previous-binary rollback and prove no dual link or Air runtime
- [ ] Preserve Phase 2 rows for a later roll-forward and capture exact commands and hashes

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.

## Precondition Resources
- [p2-acceptance-rollout-sequence.puml](file://TASK-260712-3qybi2/p2-acceptance-rollout-sequence.puml) — Rollout and rollback sequence this rehearsal must preserve

## Outcome Resources
(none)
