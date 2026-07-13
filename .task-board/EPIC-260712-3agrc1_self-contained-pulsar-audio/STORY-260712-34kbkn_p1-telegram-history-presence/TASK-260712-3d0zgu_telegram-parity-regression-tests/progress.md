## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T16:15:44Z

## Blocked By
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-21ers7
- TASK-260712-3e4p0c

## Blocks
- TASK-260712-1f9jtm
- TASK-260712-wy05n6
- TASK-260712-176b74

## Checklist
- [ ] Cover legacy ordering, callback flows, clip attachment errors and downgrade behavior with coordinator and bot tests.
- [ ] Prove presence sanitization plus exact DND and blocked receipt reasons with deterministic fixtures.
- [ ] Verify pairwise-approach target naming and app-vs-bot label parity under mixed compatibility.
- [ ] Cover callback replacement versus playback-start races and no-action latency
- [ ] Cover history tenant isolation, action authorization and layered DND precedence

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3d0zgu/p1-telegram-history-presence-components.puml) — Component diagram referenced by parity regression coverage
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-3d0zgu/p1-telegram-history-presence-flows.puml) — Sequence diagram referenced by regression scenarios
- [p1-telegram-history-presence-states.puml](file://TASK-260712-3d0zgu/p1-telegram-history-presence-states.puml) — State diagram referenced by regression and sanitization tests
