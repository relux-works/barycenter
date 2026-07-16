## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T08:19:07Z

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
- [x] Replace voice-only cache assumptions with bounded shared track caching
- [x] Integrate the candidate decoder seam and buffer-threshold readiness
- [x] Support pause, seek, resume and ended without full reload
- [x] Recover cleanly after cache eviction and partial range refetch
- [x] Add duration-independent, deadline and production-coexistence source regressions
- [x] Route real 5 s start, 3 s seek and 200 MiB RSS proof to TASK-260712-1fpb9q
- [x] Purge revoked cache and keep all fetch, disk, decode and locks off the render callback

## Notes
2026-07-16 strict-sequence start from synchronized main merge d427f82 after TASK-260712-2h6snp exact head 020c9e9 and hosted run 29480661409 passed 4/4. Implementing the Windows streamed-track player best-effort in deterministic code and unit/integration tests while preserving the accepted codec/player no-go: no production capability advertisement, real-device timing, one-hour playback, RSS or signed-hardware evidence will be claimed here; those remain in the manual testing epic.
2026-07-16 accepted on exact engineering head a7bfeb7f1787fec12f06be8a7d9afdba7e66e830 through PR #154, merge feabd2ee076de6bf990e6f8a30413b2f45825ad3, after hosted run 29482823224 passed coordinator, node-core, pulsar-win and signed packaged-probe. Added candidate-only same-origin bearer ranges, HMAC-keyed atomic bounded chunk caching, integrity retry, ETag invalidation, durable revoke tombstones, injected verified-chunk decoder, exact 1 MiB PCM ring and generation-safe readiness, scheduled start, pause, seek, resume, rebuffer, cancel and drained-ended behavior. Local full/race/vet, 10x shuffled focused race, acceptance 7/7 and Windows amd64/arm64 cross-builds passed. Production advertises no stream capability and registers no decoder; real p95, RSS, long playback and physical audio stay in TASK-260712-1fpb9q.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-17w78q/p2-streamed-track-components.puml) — Windows cache, decoder and player boundaries for streamed tracks
- [p2-streamed-track-sequence.puml](file://TASK-260712-17w78q/p2-streamed-track-sequence.puml) — Windows buffered-start and seek flow for streamed tracks

## Outcome Resources
- [P2 Windows streamed-track candidate player](../../../../docs/analysis/p2-windows-streamed-track-player.md) — Cache, decoder, lifecycle, realtime and no-go handoff
- [PR #154](https://github.com/relux-works/barycenter/pull/154) — Accepted engineering implementation and hosted CI provenance
