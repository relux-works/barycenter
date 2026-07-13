## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T16:19:49Z

## Blocked By
- TASK-260712-3coble

## Blocks
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-21ers7
- TASK-260712-e1ie4x
- TASK-260712-2vipy3

## Checklist
- [ ] Extract shared sender, target, audience and delivery label resolution out of ad hoc bot helpers.
- [ ] Normalize receipt reason text and pairwise-approach naming across direct and linked deliveries.
- [ ] Cover missing actor, slot and approach metadata with stable human fallbacks instead of raw identifiers.
- [ ] Provide golden RU and EN labels for confirmation, downgrade and exact receipts
- [ ] Prove no raw database, Telegram, node or composite peer identifier appears

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-1gx6mh/p1-telegram-history-presence-components.puml) — Component diagram for shared presentation-model ownership
- [p1-telegram-history-presence-states.puml](file://TASK-260712-1gx6mh/p1-telegram-history-presence-states.puml) — State diagram for shared receipt wording and lifecycle mapping
