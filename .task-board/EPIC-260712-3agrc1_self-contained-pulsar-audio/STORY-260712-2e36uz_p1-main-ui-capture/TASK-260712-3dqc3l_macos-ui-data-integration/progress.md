## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-12T16:43:47Z

## Blocked By
- TASK-260712-1c04pk
- TASK-260712-1s6h6t
- TASK-260712-2u1w16
- TASK-260712-1bnos4
- TASK-260712-2qpp6w
- TASK-260712-2hcq1g
- TASK-260712-1c1ska

## Blocks
- TASK-260712-e5mfqj
- TASK-260712-34stvx
- TASK-260712-2nto40
- TASK-260712-2i3u7v
- TASK-260712-1getbv

## Checklist
- [ ] Bind Create, Join and authenticated media actions to the identity and ingest APIs without hard-coding transport details.
- [ ] Render routing, presence, history and receipt states from the transmission and presence stories with honest degraded copy.
- [ ] Handle outage, retry and delete flows so local drafts survive reconnects and UI status never overstates delivery.
- [ ] Prove finalized unsent drafts survive restart and self-test media never enters upload or history

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-3dqc3l/p1-main-ui-capture-components.puml) — Component diagram for task placement and dependencies
- [p1-main-ui-capture-flows.puml](file://TASK-260712-3dqc3l/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
