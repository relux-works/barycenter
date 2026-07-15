## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-15T04:19:10Z

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
- [x] Bind Create, Join and authenticated media actions to the identity and ingest APIs without hard-coding transport details.
- [x] Render routing, presence, history and receipt states from the transmission and presence stories with honest degraded copy.
- [x] Handle outage, retry and delete flows so local drafts survive reconnects and UI status never overstates delivery.
- [x] Prove finalized unsent drafts survive restart and self-test media never enters upload or history

## Notes
2026-07-15 kickoff: strict inline execution started from synchronized main 22bd461a822e47983f50a696c3165b1720e26f03 after PR #60. Scope is deterministic macOS UI/data integration over accepted identity, ingest, transmission, presence, history and local-draft services; real app, physical audio and network outage observations remain manual in EPIC-260714-th54l3.
2026-07-15 accepted best-effort engineering scope. Engineering head 04f4c0f integrates self-service Create/Join, authenticated Phase 1 upload/transmission, canonical routing/presence/history/receipts/policy actions, durable finalized draft restart/retry/delete semantics, and strict self-test/recovery boundaries. Local verification: 211 tests in 35 suites plus release build; focused PhaseOne 4/4 and application-boundary 1/1. Hosted CI run 29388582864 passed coordinator, node-core, pulsar-win and pulsar-win-packaged-probe on the exact engineering head. No real-app, physical-audio, real-hardware or live-outage result is claimed; those remain in EPIC-260714-th54l3. PR #61.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-3dqc3l/p1-main-ui-capture-components.puml) — Component diagram for task placement and dependencies
- [p1-main-ui-capture-flows.puml](file://TASK-260712-3dqc3l/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
- [p1-macos-ui-data-integration.md](file://TASK-260712-3dqc3l/p1-macos-ui-data-integration.md) — macOS Phase 1 data integration contract, failure semantics and verification handoff
- [p1-macos-ui-data-components.puml](file://TASK-260712-3dqc3l/p1-macos-ui-data-components.puml) — Phase 1 macOS data integration component diagram
- [p1-macos-ui-data-restart-sequence.puml](file://TASK-260712-3dqc3l/p1-macos-ui-data-restart-sequence.puml) — Durable draft restart and idempotent retry sequence
