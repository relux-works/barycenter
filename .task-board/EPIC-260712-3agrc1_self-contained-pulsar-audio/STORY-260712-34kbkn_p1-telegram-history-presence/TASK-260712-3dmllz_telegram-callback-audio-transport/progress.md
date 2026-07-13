## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T16:25:59Z

## Blocked By
- TASK-260712-3coble
- TASK-260712-2xkyot
- TASK-260712-2af2dp

## Blocks
- TASK-260712-21ers7
- TASK-260712-dlltnr
- TASK-260712-wt2n7m
- TASK-260712-2zdetx

## Checklist
- [ ] Add typed transport events for callback queries, clip-eligible audio updates and document updates.
- [ ] Implement safe callback-data encoding and validation without leaking raw identifiers.
- [ ] Return honest user-facing errors for unsupported, over-limit and Phase-2-only attachment paths.
- [ ] Treat all Telegram attachment metadata as hints and defer proof to common ingest
- [ ] Test forged, expired, cross-actor and replayed opaque callbacks and terminal keyboard cleanup

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3dmllz/p1-telegram-history-presence-components.puml) — Component diagram for Telegram transport boundaries
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-3dmllz/p1-telegram-history-presence-flows.puml) — Sequence diagram for Telegram transport and callback handling
