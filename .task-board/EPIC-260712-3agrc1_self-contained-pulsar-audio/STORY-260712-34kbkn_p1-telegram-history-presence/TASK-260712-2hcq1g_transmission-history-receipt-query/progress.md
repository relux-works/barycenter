## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T16:03:43Z

## Blocked By
- TASK-260712-3coble
- TASK-260712-1gx6mh
- TASK-260712-1aprcb
- TASK-260712-1bpog0

## Blocks
- TASK-260712-21ers7
- TASK-260712-3d0zgu
- TASK-260712-3e4p0c
- TASK-260712-2fe5bz
- TASK-260712-3dqc3l

## Checklist
- [ ] Build the recent-transmission query shape over media, transmissions, targets and memberships.
- [ ] Map processing, ready, playing, played, partial, expired and error into stable Phase 1 client states.
- [ ] Expose exact per-target reason codes and aggregate counts without inventing Phase 2 inbox behavior.
- [ ] Enforce ActorContext tenant isolation, deterministic pagination and 30-day visibility
- [ ] Expose requested versus effective delivery and authorization-derived action capabilities

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-2hcq1g/p1-telegram-history-presence-components.puml) — Component diagram for history and receipt query ownership
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-2hcq1g/p1-telegram-history-presence-flows.puml) — Sequence diagram for history and receipt updates across the Telegram flow
- [p1-telegram-history-presence-states.puml](file://TASK-260712-2hcq1g/p1-telegram-history-presence-states.puml) — State diagram for history and per-target receipt lifecycle
