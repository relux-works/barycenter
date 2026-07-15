## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-15T11:54:34Z

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
- [x] Add additive schema and repository methods for Airs, members, invites, policies, and active Air lookup
- [x] Backfill active links into stable two member Air rows without duplicate membership state
- [x] Preserve rollback compatibility and feature flag safety for older coordinators
- [x] Rehearse upgrade and rollback on production shaped fixtures
- [x] Backfill deterministic link-to-Air mappings exactly once with failure injection
- [x] Prove previous-binary legacy service preserves Phase 2 rows without dual delivery

## Notes
Strict inline execution started from synchronized main d496409 after accepted Air contract. Implementing additive schema, deterministic link backfill, persisted authority generations and fail-closed rollback without dual runtime.
Implemented additive Air persistence, deterministic exactly-once link migration, generation-fenced cutover/rollback, immutable legacy snapshots, concurrent one-active repositories, failure injection, and exact predecessor-binary legacy service. Focused/full/race suites green; preparing clean acceptance and PR.
Accepted: exact engineering head b5a633932e7d616bbdee252e1f255c2dfbf49054 passed local acceptance 12/12 and hosted CI run 29413065743 (4/4); PR #84 merged as 68059d9c03d6af3dcdd84468805309d4be559901.

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-3n36ny/p2-air-rooms-components.puml) — Persistence and migration boundaries for Air schema work
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-3n36ny/p2-air-rooms-lifecycle-sequence.puml) — Join, leave, and park flow that the migration must preserve

## Outcome Resources
- [p2-air-schema-link-migration.md](file://TASK-260712-3n36ny/p2-air-schema-link-migration.md) — Schema, deterministic backfill, single-authority cutover and rollback handoff
