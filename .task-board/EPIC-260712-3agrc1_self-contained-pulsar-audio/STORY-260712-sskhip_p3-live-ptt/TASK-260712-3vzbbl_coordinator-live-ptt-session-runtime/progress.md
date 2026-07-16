## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:25:04Z

## Last Update
2026-07-16T17:20:00Z

## Blocked By
- TASK-260712-3qviqc

## Blocks
- TASK-260712-1rzqh9
- TASK-260712-2jbo5i
- TASK-260712-2kj9kj

## Checklist
- [x] Add a live session model, generation ids and sealed-target resolution over current Air
- [x] Implement chunk ingress and fanout, backpressure, cancel or end handling and stale-session rejection
- [x] Drive synchronized duck start and release plus main-program recovery through existing scheduler or mixer seams
- [x] Surface receipts, telemetry and feature-flag behavior without leaking audio content
- [x] Add unit and integration coverage for reconnect, loss, leave-Air, partial delivery and rollback compatibility

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge cc4361d9ee34a7e2b736b8bd1af81de48524fe54. Execute inline outside task-board spawn. Treat TASK-260712-3qviqc outcome as authoritative: live_ptt_v1 remains unadvertised, no persistence or restart resume, local-user-input-only capture, sealed targets, bounded binary profile and explicit unsupported receipts.
2026-07-16 accepted via PR #189, merge 81fdb940574d13221909f31226380c8e1a9034ed, hosted CI 29518925339 4/4 and clean automated acceptance 12/12. Env-dark runtime seals current Air/barycenter targets, allocates coordinator identity, bounds sessions/rate/queues, relays without persistence, continuously removes revoked targets, tears down on faults/restart, and serializes live duck/release with durable overlay/interrupt. Physical latency/audio/hardware evidence remains manual in TASK-260712-1rzqh9.

## Precondition Resources
(none)

## Outcome Resources
- [p3-live-ptt-components.puml](file://TASK-260712-3vzbbl/p3-live-ptt-components.puml) — Task-boundary diagram for coordinator live runtime
- [p3-live-ptt-sequence.puml](file://TASK-260712-3vzbbl/p3-live-ptt-sequence.puml) — Runtime sequence for live session start, chunks and teardown
- [TASK-260712-3vzbbl_live-ptt-coordinator-runtime.md](file://TASK-260712-3vzbbl/TASK-260712-3vzbbl_live-ptt-coordinator-runtime.md) — Accepted coordinator runtime design, bounds, rollback and verification evidence
