## Status
development

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T09:07:16Z

## Blocked By
- TASK-260712-285pag
- TASK-260712-3lf8r0
- TASK-260712-31rkpe
- TASK-260712-2h6snp
- TASK-260712-1hqiek
- TASK-260712-2eympi

## Blocks
- TASK-260712-1fpb9q
- TASK-260712-2psvhu

## Checklist
- [ ] Replace voice-only cache assumptions with bounded shared track caching
- [ ] Integrate the selected incremental decoder and buffer-threshold readiness
- [ ] Support pause, seek, resume and ended without full reload
- [ ] Recover cleanly after cache eviction and partial range refetch
- [ ] Add macOS tests for one-hour track, seek latency and clip or Spotify coexistence
- [ ] Prove 5 s start, 3 s seek, 200 MiB RSS and duration-independent bounds
- [ ] Purge revoked cache and keep all fetch, disk, decode and locks off the render callback

## Notes
2026-07-16 strict-sequence start from synchronized main merge 0cb18b9 after TASK-260712-1q2kwa exact head c6e9a68 and hosted run 29485664677 passed 4/4. Implementing the macOS streamed-track candidate player inline outside task-board spawn workflow; production capability advertisement and real-device timing, RSS, long-playback and audible evidence remain deferred to the manual testing epic.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-3aj8w2/p2-streamed-track-components.puml) — macOS cache, decoder and player boundaries for streamed tracks
- [p2-streamed-track-sequence.puml](file://TASK-260712-3aj8w2/p2-streamed-track-sequence.puml) — macOS buffered-start and seek flow for streamed tracks

## Outcome Resources
(none)
