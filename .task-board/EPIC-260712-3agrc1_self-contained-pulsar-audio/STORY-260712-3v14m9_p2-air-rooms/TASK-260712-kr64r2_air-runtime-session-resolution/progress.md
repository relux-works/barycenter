## Status
reviewing

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-15T12:30:48Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-3n36ny
- TASK-260712-31vvjt

## Blocks
- TASK-260712-2vhf80
- TASK-260712-2bjdlb
- TASK-260712-3nq0tq
- TASK-260712-1c34fe
- TASK-260712-2h6snp

## Checklist
- [x] Replace link keyed state ownership and startup warmup with Air keyed resolution
- [x] Build Air peer unions and Air scoped order ownership for voice and track routing
- [x] Preserve living Air catch up, parked sessions, and leave only the caller semantics
- [x] Cover restart, stale session, and no transitive chain cases in loop and FSM tests
- [x] Instantiate only active Air runtimes and keep saved or parked rooms lazy
- [x] Catch up only the current main track and never start stale overlay for a joining member
- [x] Fade or stop only leaving nodes and restore their personal orbit state

## Notes
Strict inline execution started from synchronized main ebadce7 after accepted Air schema migration. Inspecting current link-keyed runtime/session ownership before implementing stable Air resolution and generation fencing.
Implemented stable Air-ID runtime ownership, exact current-member unions, active-only warmup, generation/revision fences, main-only join catch-up, caller-only leave, lazy parked rooms, stale async rejection, Air media cancellation and rollback-hold fail-closed behavior. Full tests/vet and race suites green; preparing clean acceptance and PR.

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-kr64r2/p2-air-rooms-components.puml) — Runtime ownership and stateFor boundaries for Air session routing

## Outcome Resources
- [p2-air-runtime-session-resolution.md](file://TASK-260712-kr64r2/p2-air-runtime-session-resolution.md) — Stable Air runtime ownership, lifecycle fencing and verification handoff
