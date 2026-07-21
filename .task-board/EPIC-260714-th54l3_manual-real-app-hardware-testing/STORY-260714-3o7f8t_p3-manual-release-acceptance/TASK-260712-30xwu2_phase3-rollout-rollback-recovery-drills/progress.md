## Status
closed

## Assigned To
(none)

## Created
2026-07-12T16:41:36Z

## Last Update
2026-07-21T10:59:26Z

## Blocked By
- TASK-260712-2uo81g
- TASK-260712-flaiie
- TASK-260712-yj668d
- TASK-260712-1gyohk
- TASK-260712-1ulshp

## Blocks
- TASK-260712-1actom

## Checklist
- [ ] Rehearse additive migration, inert coordinator posture, and reviewed node rollout order
- [ ] Prove live_ptt-only rollout, later e2ee_media enablement, and independent disable paths
- [ ] Exercise rollback preserving phase-three rows and legacy service behavior
- [ ] Run capture-stop, revoke, lost-device, and key-loss recovery drills with exact commands
- [ ] Publish operator-ready drill results and rollback signatures

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.
2026-07-21 owner-directed consolidation: closed as superseded, not passed. Remaining highest-value real-app and hardware observations now live only in TASK-260721-ryk8c0 Ivan Oparin final real-app verification, gated by TASK-260721-2346wf Desktop UI automated acceptance and owner handoff.

## Precondition Resources
- [p3-acceptance-rollout-sequence.puml](file://TASK-260712-30xwu2/p3-acceptance-rollout-sequence.puml) — Rollout, independent gating, rollback, and drill sequence this rehearsal must preserve

## Outcome Resources
(none)
