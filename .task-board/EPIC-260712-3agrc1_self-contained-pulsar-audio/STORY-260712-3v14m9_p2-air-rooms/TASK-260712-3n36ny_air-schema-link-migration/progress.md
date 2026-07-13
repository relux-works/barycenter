## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-12T16:25:58Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-1bpog0

## Blocks
- TASK-260712-kr64r2
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-3nq0tq

## Checklist
- [ ] Add additive schema and repository methods for Airs, members, invites, policies, and active Air lookup
- [ ] Backfill active links into stable two member Air rows without duplicate membership state
- [ ] Preserve rollback compatibility and feature flag safety for older coordinators
- [ ] Rehearse upgrade and rollback on production shaped fixtures
- [ ] Backfill deterministic link-to-Air mappings exactly once with failure injection
- [ ] Prove previous-binary legacy service preserves Phase 2 rows without dual delivery

## Notes

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-3n36ny/p2-air-rooms-components.puml) — Persistence and migration boundaries for Air schema work
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-3n36ny/p2-air-rooms-lifecycle-sequence.puml) — Join, leave, and park flow that the migration must preserve

## Outcome Resources
(none)
