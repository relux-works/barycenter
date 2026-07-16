## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:10Z

## Last Update
2026-07-16T04:42:12Z

## Blocked By
- TASK-260712-z6h6wh
- TASK-260712-2eympi

## Blocks
- TASK-260712-285pag
- TASK-260712-3lf8r0
- TASK-260712-2h6snp
- TASK-260712-2ogntd

## Checklist
- [x] Add additive schema for audio_track records, variant rows and recovery fields
- [x] Persist variant metadata and repository queries by media id and profile
- [x] Keep legacy clip and session reads compatible on upgraded databases
- [x] Add migration and repository coverage for upgrade and rollback behavior
- [x] Persist exact ADR container, integrity, ETag and VBR seek metadata plus seek generations
- [x] Keep Phase 1 target, history and Spotify state canonical and migration-safe

## Notes
2026-07-16 strict-sequence start from synchronized main merge b7bc2b4 after TASK-260712-20cuna code PR #140 merge e51c937 and tracking PR #141 merge b7bc2b4; hosted runs 29470807661 and 29471003186 passed 4/4. Implementing candidate-neutral additive schema and repository foundations inline outside task-board spawn workflow. The accepted codec/player ADR remains production no-go: production variant selection, generation and playback stay disabled, while schema/test-double engineering may proceed.
2026-07-16 accepted on exact engineering head b64a671d2356dad7d021905a61f498e9cd94ac18 through PR #142, merge 54780069a2b39805882872a5a1f491fdced7a7a8, after hosted run 29471845396 passed coordinator, node-core, pulsar-win and signed packaged-probe. Added only additive audio-track metadata, immutable candidate variants, pinned canonical profile/range lookup and restart-safe playback queue/progress generations. Store vet/full tests, exact previous-binary rollback and 41 contract tests passed. Phase 1 media/transmission/inbox/history and Spotify/session authority remain unchanged; production selection remains fail-closed and no real-app or hardware playback is claimed.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-1n5fks/p2-streamed-track-components.puml) — Schema and storage context for streamed-track persistence

## Outcome Resources
- [P2 streamed-track persistence handoff](../../../../docs/analysis/p2-stream-track-schema-variants.md) — Schema, concurrency, no-go and rollback boundary
- [PR #142](https://github.com/relux-works/barycenter/pull/142) — Accepted engineering implementation and hosted CI provenance
