## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-12T16:43:47Z

## Blocked By
- TASK-260712-9i5se7
- TASK-260712-1p8ykc
- TASK-260712-47uve0
- TASK-260712-1bnos4
- TASK-260712-2qpp6w
- TASK-260712-2hcq1g
- TASK-260712-1c1ska

## Blocks
- TASK-260712-e5mfqj
- TASK-260712-pbfz37
- TASK-260712-cuplon
- TASK-260712-31zja2
- TASK-260712-39zh8g

## Checklist
- [ ] Bind Create, Join and authenticated media actions to the identity and ingest APIs without hard-coding transport details.
- [ ] Render routing, presence, history and receipt states from the transmission and presence stories with honest degraded copy.
- [ ] Handle outage, retry and delete flows so local drafts survive reconnects and UI status never overstates delivery.
- [ ] Prove finalized unsent drafts survive restart and self-test media never enters upload or history

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-2fe5bz/p1-main-ui-capture-components.puml) — Component diagram for task placement and dependencies
- [p1-main-ui-capture-flows.puml](file://TASK-260712-2fe5bz/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
