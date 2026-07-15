## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:56Z

## Last Update
2026-07-15T16:30:43Z

## Blocked By
- TASK-260712-3n36ny
- TASK-260712-kr64r2
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-2i3u7v
- TASK-260712-31zja2
- TASK-260712-2zdetx

## Blocks
- TASK-260712-21kz3b
- TASK-260712-3qybi2
- TASK-260712-1fpb9q
- TASK-260712-qi81vf
- TASK-260712-2sicfs

## Checklist
- [x] Extend store, loop, and FSM coverage for B2 to B4 lifecycle and no duplicate delivery
- [x] Add production shaped migration and rollback rehearsal fixtures
- [x] Run or script the eight barycenter and twenty Pulsar synthetic load proof and record exact remaining gaps
- [x] Feed evidence and cross story gaps into the phase two acceptance handoff
- [x] Fault-inject migration cutover and prove no dual link or Air delivery
- [x] Test join and leave during track, overlay and prepare plus lazy parked resource use

## Notes
Strict inline execution started from synchronized main 8d20fa9 after accepted Telegram Air tracking merge. Auditing existing Air store/runtime/alias/HTTP/Windows/macOS/Telegram coverage before adding adversarial B2-B4, migration rollback, synthetic 8-Barycenter/20-Pulsar and lifecycle-boundary evidence. Stream catch-up and explicit-target ACL remain named downstream dependencies, not claimed here.
Repository engineering evidence complete locally. Closed Air overlay leave boundary so only the departing Barycenter is cancelled during prepare or cancelling during playback. Deterministic rehearsal passed 8 Barycenters, 20 Pulsars, 20 unique load commands, zero duplicates, one Air runtime and zero legacy groups. Full coordinator tests, vet and race passed; full repository acceptance passed 12/12 including exact previous-head rollback, Windows race/cross-build and Swift fixtures. Artifacts remain repository-automated-only with manualEvidence not-run. Streamed-track catch-up/performance, explicit-target ACL/inbox and all real app/hardware/audio checks remain explicitly downstream.
Accepted after hosted CI run 29432415158 passed all four jobs (node-core 1m10s, pulsar-win 1m43s, coordinator 2m13s, packaged probe 2m54s). Engineering commit b984230 landed through PR #100 at merge e4aa266913ed7daed1ae07c50d7b33c1e7d1288f.

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-3nq0tq/p2-air-rooms-components.puml) — System boundaries for regression, migration, and load proof
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-3nq0tq/p2-air-rooms-lifecycle-sequence.puml) — Lifecycle sequence that B2 to B4 regression proof must preserve

## Outcome Resources
- [p2-air-lifecycle-regression-rehearsal.md](file://TASK-260712-3nq0tq/p2-air-lifecycle-regression-rehearsal.md) — B2-B4, lifecycle, migration, capacity and downstream-gap evidence handoff
- [air-regression-rehearsal.json](file://TASK-260712-3nq0tq/air-regression-rehearsal.json) — Machine-readable 8-Barycenter/20-Pulsar repository-only rehearsal result
- [repository-acceptance-manifest.json](file://TASK-260712-3nq0tq/repository-acceptance-manifest.json) — Passing 12-command repository acceptance manifest including exact predecessor rollback
