## Status
closed

## Assigned To
(none)

## Created
2026-07-12T16:23:01Z

## Last Update
2026-07-21T10:59:21Z

## Blocked By
- TASK-260712-14rxuk
- TASK-260712-qi81vf
- TASK-260712-3nq0tq
- TASK-260712-1fpb9q
- TASK-260712-2ubzyf
- TASK-260712-1vklop
- TASK-260712-1kfnpu

## Blocks
- TASK-260712-2pnc5a
- TASK-260712-2bdi4a

## Checklist
- [ ] Run the three-barycenter and five-pulsar Air matrix for exact-once clip and track delivery
- [ ] Capture offline catch-up, no-late-autoplay, and leave or continue behavior
- [ ] Execute the 8-barycenter and 20-pulsar synthetic load gate and record thresholds
- [ ] Archive duplicate-detection, catch-up, and scale artifacts for operator review
- [ ] Measure CPU, RSS, SQLite latency, hub queues, duplicate commands and recovery at 8 Barycenters or 20 Pulsars

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.
2026-07-21 owner-directed consolidation: closed as superseded, not passed. Remaining highest-value real-app and hardware observations now live only in TASK-260721-ryk8c0 Ivan Oparin final real-app verification, gated by TASK-260721-2346wf Desktop UI automated acceptance and owner handoff.

## Precondition Resources
- [p2-acceptance-evidence-map.puml](file://TASK-260712-21kz3b/p2-acceptance-evidence-map.puml) — Upstream evidence and downstream packet context for B2-B4 acceptance

## Outcome Resources
(none)
