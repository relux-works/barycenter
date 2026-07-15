## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-15T05:10:08Z

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
- [x] Bind Create, Join and authenticated media actions to the identity and ingest APIs without hard-coding transport details.
- [x] Render routing, presence, history and receipt states from the transmission and presence stories with honest degraded copy.
- [x] Handle outage, retry and delete flows so local drafts survive reconnects and UI status never overstates delivery.
- [x] Prove finalized unsent drafts survive restart and self-test media never enters upload or history

## Notes
2026-07-15 kickoff: strict inline execution started from synchronized main 04a23c8 after PR #61. Scope is deterministic Windows UI/data integration over accepted identity, ingest, transmission, presence, history and durable local-draft contracts. Real app, Windows hardware, physical audio and live network-outage observations remain manual in EPIC-260714-th54l3.
2026-07-15 accepted on exact engineering head af961a5 via PR #62. Windows now binds direct self-service Create/Join with DPAPI and explicit recovery export; authenticated media upload/transmission, canonical routing, ready/online presence, history receipts and allowed delete/replay/block actions; and a durable owner-only outbox for microphone and picked-file drafts. Route, delivery, source provenance and idempotency keys survive restart; upload confirmation is persisted before local cleanup; explicit interrupt fallback requires a memory-only confirmation; self-test never attaches. Local pulsar-win test/race/vet and Windows amd64 cross-vet/build passed, coordinator test/vet passed, and Xcode Swift test passed 211 tests in 35 suites. Hosted run 29390609436 passed node-core, pulsar-win, signed packaged probe and coordinator. No real Windows UI, DPAPI prompt, physical audio, network outage or hardware result is claimed; those remain in EPIC-260714-th54l3. Outcome analysis and two PlantUML sources are attached; PlantUML rendering was unavailable locally.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-2fe5bz/p1-main-ui-capture-components.puml) — Component diagram for task placement and dependencies
- [p1-main-ui-capture-flows.puml](file://TASK-260712-2fe5bz/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
- [p1-windows-ui-data-integration.md](file://TASK-260712-2fe5bz/p1-windows-ui-data-integration.md) — Accepted Windows Phase 1 UI/data boundary, persistence rules, and automated evidence
- [p1-windows-ui-data-restart-sequence.puml](file://TASK-260712-2fe5bz/p1-windows-ui-data-restart-sequence.puml) — Durable upload, cleanup, restart, retry, and explicit fallback sequence
- [p1-windows-ui-data-components.puml](file://TASK-260712-2fe5bz/p1-windows-ui-data-components.puml) — Windows identity, capture, outbox, client, and coordinator component boundary
