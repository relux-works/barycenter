## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:44:15Z

## Last Update
2026-07-14T18:45:27Z

## Blocked By
- TASK-260712-2zbmq4
- TASK-260712-1viwvi
- TASK-260712-8mwyiv
- TASK-260712-1g6lk8
- TASK-260712-2bbz13
- TASK-260712-26ip33
- TASK-260712-31vvjt

## Blocks
- TASK-260712-2hodti
- TASK-260712-176b74
- TASK-260712-1uz0za

## Checklist
- [x] Cover duck ramps, limiter behavior, cancellation and interrupt resume drift with deterministic assertions
- [x] Add a 100 overlay soak or regression case on both node implementations
- [x] Map each deterministic part of A3 or A4 to automated evidence and note what remains hardware-only
- [x] Instrument and fail on render callback allocation, I/O, waits or forbidden locks
- [x] Verify maximum-clip memory bound and exact gain-order behavior on both platforms
- [x] Test active sender-delete during overlay and interrupt including main-program recovery

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge a21c79b after TASK-260712-8mwyiv acceptance. Scope is the common automated A3/A4 regression gate: exact ramps/gain order/limiter/ring continuity, cancel and sender-delete recovery, generation races, drift bounds, callback safety, maximum-clip memory bounds and 100 sequential overlays on Windows and macOS. Physical/audible route evidence remains exclusively in EPIC-260714-th54l3.
2026-07-14 engineering gate: deterministic Windows and macOS coverage now asserts ramps, gain order, limiter behavior, 200/500 ms report bounds, exact interrupt anchor, stale-generation protection, sender-delete recovery, realtime callback safety, a bounded single 63,504,000-byte maximum PCM buffer, and 100 sequential overlays. Local gate green: Go, Go race, Windows cross-build, coordinator, 165 Swift tests, focused repeats, and Swift release. Physical audible and route evidence remains unclaimed in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [automated-evidence](file://TASK-260712-3d6cnn/automated-evidence) — Deterministic A3/A4 evidence and hardware-only boundary
