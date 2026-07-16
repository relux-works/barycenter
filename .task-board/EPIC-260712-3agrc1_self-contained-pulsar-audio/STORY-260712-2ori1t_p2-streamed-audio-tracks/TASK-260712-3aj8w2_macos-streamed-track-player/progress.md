## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T09:40:00Z

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
- [x] Replace voice-only cache assumptions with bounded shared track caching
- [x] Integrate the selected incremental decoder and buffer-threshold readiness
- [x] Support pause, seek, resume and ended without full reload
- [x] Recover cleanly after cache eviction and partial range refetch
- [x] Add macOS tests for one-hour track, seek latency and clip or Spotify coexistence
- [x] Prove 5 s start, 3 s seek, 200 MiB RSS and duration-independent bounds
- [x] Purge revoked cache and keep all fetch, disk, decode and locks off the render callback

## Notes
2026-07-16 strict-sequence start from synchronized main merge 0cb18b9 after TASK-260712-1q2kwa exact head c6e9a68 and hosted run 29485664677 passed 4/4. Implementing the macOS streamed-track candidate player inline outside task-board spawn workflow; production capability advertisement and real-device timing, RSS, long-playback and audible evidence remain deferred to the manual testing epic.
2026-07-16 accepted on exact engineering head e6f0685f29e6be0dec95b0ea89b7c5463ee1206b through PR #158, merge 606994898a6e6873c3cc8ed330c82a236bbd3f01, after hosted run 29487762262 passed coordinator, node-core, pulsar-win and signed packaged-probe. Added exact authenticated bounded ranges, HMAC-keyed atomic chunk cache, LRU/refill and durable revoke, an injected candidate decoder, fixed 1 MiB SPSC PCM ring and generation-safe ready/start/pause/seek/resume/progress/rebuffer/cancel/drain lifecycle. Focused coverage passed 6/6, full macOS coverage passed 248 tests in 40 suites, release build and frozen validators passed. The accepted codec ADR still selects no production decoder; therefore checklist decoder/timing/RSS/long-track items are satisfied only as deterministic seams and fixed code bounds. Real p95, RSS, two-hour/audible/packaged/hardware evidence remains unclaimed in manual TASK-260712-1fpb9q.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-3aj8w2/p2-streamed-track-components.puml) — macOS cache, decoder and player boundaries for streamed tracks
- [p2-streamed-track-sequence.puml](file://TASK-260712-3aj8w2/p2-streamed-track-sequence.puml) — macOS buffered-start and seek flow for streamed tracks

## Outcome Resources
- [p2-macos-streamed-track-player.md](file://TASK-260712-3aj8w2/p2-macos-streamed-track-player.md) — bounded candidate cache, decoder, render lifecycle and explicit manual-evidence handoff
