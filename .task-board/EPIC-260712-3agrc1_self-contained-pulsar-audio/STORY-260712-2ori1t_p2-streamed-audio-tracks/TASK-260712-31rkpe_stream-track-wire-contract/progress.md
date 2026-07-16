## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T05:16:21Z

## Blocked By
- TASK-260712-51y5k9
- TASK-260712-1g70av
- STORY-260712-3l1r1u
- TASK-260712-2eympi
- TASK-260712-2rlkp7

## Blocks
- TASK-260712-2h6snp
- TASK-260712-3aj8w2
- TASK-260712-17w78q

## Checklist
- [x] Define streamed-track payloads and capability flags in the shared protocol
- [x] Update Go, Swift and Windows codec mirrors plus golden fixtures
- [x] Encode the mixed-version fallback and unsupported-target reporting policy
- [x] Add contract coverage without regressing existing clip or Spotify messages
- [x] Use seek generations and reject stale load, ready, progress and ended events
- [x] Freeze buffer thresholds, deadlines and sender-selected mixed-version behavior

## Notes
2026-07-16 strict-sequence start from synchronized main merge d26cb26 after TASK-260712-1n5fks code PR #142 merge 5478006 and tracking PR #143 merge d26cb26; hosted runs 29471845396 and 29472071694 passed 4/4. Freezing and implementing only the candidate-neutral streamed-track wire contract inline outside task-board spawn workflow. The accepted codec/player production no-go remains in force: payloads may carry opaque pinned test manifests, but cannot enable production variant generation, decoder registration or playback.
2026-07-16 accepted on exact engineering head ea2d6d42eae0999ca8f311ffca8a440db78db562 through PR #144, merge 0b9fc7d6bcc6c7c8b5d064dd18b169ca70d8959b, after hosted run 29473326227 passed coordinator, node-core, pulsar-win and signed packaged-probe. Added generation-safe stream load/resume/seek/pause/cancel commands and ready/started/progress/rebuffer/failure/ended/cancelled events across Go, Swift and the Windows mirror, with 51 shared goldens, an opaque credential-free manifest contract, coordinator-clock barriers, stale/duplicate/reordered event rejection and explicit mixed-version policy. The first hosted run exposed and the final head fixed a Go 1.25/1.26 source-mirror normalization edge. Production runtimes do not advertise stream_track_v1, no decoder/player was enabled, and no real-app or hardware playback is claimed.

## Precondition Resources
- [p2-streamed-track-sequence.puml](file://TASK-260712-31rkpe/p2-streamed-track-sequence.puml) — Buffered-start and seek flow for the streamed-track wire contract

## Outcome Resources
- [P2 streamed-track wire contract](../../../../docs/analysis/p2-stream-track-wire-contract-v1.md) — Generation, timing, integrity and mixed-version boundary
- [PR #144](https://github.com/relux-works/barycenter/pull/144) — Accepted engineering implementation and hosted CI provenance
