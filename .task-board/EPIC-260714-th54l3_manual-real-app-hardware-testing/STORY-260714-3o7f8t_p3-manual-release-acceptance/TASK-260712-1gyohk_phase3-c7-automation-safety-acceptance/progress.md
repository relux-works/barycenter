## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:41:36Z

## Last Update
2026-07-17T04:20:17Z

## Blocked By
- TASK-260712-3da0vz
- TASK-260712-2uo81g
- TASK-260712-2f0gpu
- TASK-260712-3g0axs
- TASK-260712-1x5jfo
- TASK-260712-yj668d

## Blocks
- TASK-260712-30xwu2

## Checklist
- [ ] Run timezone, quiet-hour, and DND precedence cases on the final automation build
- [ ] Verify target policy, local volume ceiling, and no-capture invariants
- [ ] Exercise audit trail, revoke immediacy, rate limits, and runaway-stop behavior
- [ ] Archive app, automation-surface, and operator evidence for C7
- [ ] Publish the final C7 safety matrix and exact blockers

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.
2026-07-17 prerequisite engineering handoff TASK-260712-2f0gpu is complete at code head f7f52a5 / merge 793f8ae with clean automated acceptance manifest attached. This does not execute or accept this manual C7 task: signed Windows/macOS apps, physical clocks/hardware, audible DND/block/Air/ceiling, accessibility, live Telegram, real kill-switch latency, recovery and soak evidence remain not-run.

## Precondition Resources
- [p3-acceptance-evidence-map.puml](file://TASK-260712-1gyohk/p3-acceptance-evidence-map.puml) — Upstream evidence and downstream packet context for C7 acceptance

## Outcome Resources
(none)
