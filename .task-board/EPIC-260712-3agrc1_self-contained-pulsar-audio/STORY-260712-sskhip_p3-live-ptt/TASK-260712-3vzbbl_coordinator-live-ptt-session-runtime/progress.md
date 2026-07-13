## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:25:04Z

## Last Update
2026-07-12T16:41:46Z

## Blocked By
- TASK-260712-3qviqc

## Blocks
- TASK-260712-1rzqh9
- TASK-260712-2jbo5i
- TASK-260712-2kj9kj

## Checklist
- [ ] Add a live session model, generation ids and sealed-target resolution over current Air
- [ ] Implement chunk ingress and fanout, backpressure, cancel or end handling and stale-session rejection
- [ ] Drive synchronized duck start and release plus main-program recovery through existing scheduler or mixer seams
- [ ] Surface receipts, telemetry and feature-flag behavior without leaking audio content
- [ ] Add unit and integration coverage for reconnect, loss, leave-Air, partial delivery and rollback compatibility

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p3-live-ptt-components.puml](file://TASK-260712-3vzbbl/p3-live-ptt-components.puml) — Task-boundary diagram for coordinator live runtime
- [p3-live-ptt-sequence.puml](file://TASK-260712-3vzbbl/p3-live-ptt-sequence.puml) — Runtime sequence for live session start, chunks and teardown
