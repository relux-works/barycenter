## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-14T00:49:05Z

## Blocked By
- TASK-260712-3lf8r0
- TASK-260712-2h6snp
- TASK-260712-3aj8w2
- TASK-260712-17w78q
- STORY-260712-3v14m9
- STORY-260712-ob1tx2
- TASK-260712-2ogntd
- TASK-260712-3lximx
- TASK-260712-2psvhu
- TASK-260712-3nq0tq
- TASK-260712-1vklop
- TASK-260712-e5mfqj

## Blocks
- TASK-260712-2bdi4a
- TASK-260712-21kz3b
- TASK-260712-3u5cdn
- TASK-260712-3qybi2

## Checklist
- [ ] Add one-hour track coverage for start-before-full-download and bounded RSS
- [ ] Measure seek-to-audio latency and buffer-ready skew against section 20 gates
- [ ] Cover cache eviction, pause or resume and range-refetch recovery
- [ ] Prove mixed-version fallback plus delete and ACL regression behavior
- [ ] Keep clip, overlay, interrupt and Spotify regression suites green
- [ ] Run all three platform pairings, network faults, max duration, quota reconciliation and cache revocation
- [ ] Map exact hard-gate evidence and leave only scale and seven-day beta to acceptance

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.

## Precondition Resources
- [p2-streamed-track-sequence.puml](file://TASK-260712-1fpb9q/p2-streamed-track-sequence.puml) — Verification flow for buffered start, seek and eviction recovery

## Outcome Resources
(none)
