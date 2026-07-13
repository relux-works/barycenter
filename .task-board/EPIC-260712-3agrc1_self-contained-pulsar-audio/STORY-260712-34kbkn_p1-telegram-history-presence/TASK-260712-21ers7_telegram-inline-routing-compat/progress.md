## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T15:54:27Z

## Blocked By
- TASK-260712-1gx6mh
- TASK-260712-3dmllz
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-12ojcb
- TASK-260712-2qpp6w

## Blocks
- TASK-260712-3d0zgu

## Checklist
- [ ] Show inline delivery actions and audience choices for clip-eligible Telegram media after processing completes.
- [ ] Preserve the acceptance-time FIFO and default after_current path when the user takes no action.
- [ ] Surface visible capability downgrades and reuse the shared presentation model for all bot wording.
- [ ] Create the legacy default immediately at media readiness without waiting for a callback window
- [ ] Atomically replace only a not-started default and require explicit interrupt fallback confirmation

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-21ers7/p1-telegram-history-presence-components.puml) — Component diagram for inline routing and legacy compatibility
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-21ers7/p1-telegram-history-presence-flows.puml) — Sequence diagram for inline routing and legacy fallback behavior
