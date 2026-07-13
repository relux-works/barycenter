## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-12T16:30:31Z

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
- [ ] Replace link keyed state ownership and startup warmup with Air keyed resolution
- [ ] Build Air peer unions and Air scoped order ownership for voice and track routing
- [ ] Preserve living Air catch up, parked sessions, and leave only the caller semantics
- [ ] Cover restart, stale session, and no transitive chain cases in loop and FSM tests
- [ ] Instantiate only active Air runtimes and keep saved or parked rooms lazy
- [ ] Catch up only the current main track and never start stale overlay for a joining member
- [ ] Fade or stop only leaving nodes and restore their personal orbit state

## Notes

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-kr64r2/p2-air-rooms-components.puml) — Runtime ownership and stateFor boundaries for Air session routing

## Outcome Resources
(none)
