## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T15:54:27Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1gx6mh
- TASK-260712-3dmllz
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-3e4p0c

## Checklist
- [ ] Define the Phase 1 history list and receipt-detail contract for app and bot consumers.
- [ ] Define DND and block mutation semantics, visibility rules and exact reason vocabulary.
- [ ] Define Telegram callback payload, callback lifetime and idempotency rules.
- [ ] Freeze the Phase 1 clip-only attachment matrix and the honest errors for Phase-2-only paths.
- [ ] Freeze immediate legacy default enqueue plus atomic pre-start callback replacement with a new trusted acceptance time
- [ ] Freeze callback actor binding, integrity, expiry, replay defense and interrupt confirmation
- [ ] Freeze history authorization, pagination, action visibility and layered local versus orbit DND

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3coble/p1-telegram-history-presence-components.puml) — Component diagram for contract and dependency boundaries
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-3coble/p1-telegram-history-presence-flows.puml) — Sequence diagram for the agreed Telegram media and inline-action contract
- [p1-telegram-history-presence-states.puml](file://TASK-260712-3coble/p1-telegram-history-presence-states.puml) — State diagram for contract status and receipt mapping
- [p1-telegram-history-presence-decomposition.md](file://TASK-260712-3coble/p1-telegram-history-presence-decomposition.md) — Contract-gap context and dependency map for the blocker task
