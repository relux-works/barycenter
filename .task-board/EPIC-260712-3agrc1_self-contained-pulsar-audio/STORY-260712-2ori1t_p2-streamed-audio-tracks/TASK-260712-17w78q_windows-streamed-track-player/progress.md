## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-12T16:30:33Z

## Blocked By
- TASK-260712-285pag
- TASK-260712-3lf8r0
- TASK-260712-31rkpe
- TASK-260712-2h6snp
- TASK-260712-1hqiek
- TASK-260712-2eympi

## Blocks
- TASK-260712-1fpb9q
- TASK-260712-3lximx

## Checklist
- [ ] Replace voice-only cache assumptions with bounded shared track caching
- [ ] Integrate the selected incremental decoder and buffer-threshold readiness
- [ ] Support pause, seek, resume and ended without full reload
- [ ] Recover cleanly after cache eviction and partial range refetch
- [ ] Add Windows tests for one-hour track, seek latency and clip or Spotify coexistence
- [ ] Prove 5 s start, 3 s seek, 200 MiB RSS and duration-independent bounds
- [ ] Purge revoked cache and keep all fetch, disk, decode and locks off the render callback

## Notes

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-17w78q/p2-streamed-track-components.puml) — Windows cache, decoder and player boundaries for streamed tracks
- [p2-streamed-track-sequence.puml](file://TASK-260712-17w78q/p2-streamed-track-sequence.puml) — Windows buffered-start and seek flow for streamed tracks

## Outcome Resources
(none)
