## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T06:22:23Z

## Blocked By
- TASK-260712-1n5fks
- TASK-260712-2af2dp
- TASK-260712-1bnos4
- STORY-260712-3l1r1u
- TASK-260712-2eympi
- TASK-260712-2ctf3x
- TASK-260712-2ogntd

## Blocks
- TASK-260712-3lf8r0
- TASK-260712-2h6snp
- TASK-260712-3aj8w2
- TASK-260712-17w78q
- TASK-260712-1q2kwa
- TASK-260712-wt2n7m

## Checklist
- [x] Accept audio_track requests with long-file validation and phase-two limits
- [x] Generate the canonical compressed variants chosen by the codec spike
- [x] Persist duration, size, hash and variant metadata without full WAV retention
- [x] Keep retries idempotent and leave no partially ready track records
- [x] Protect clip and legacy Telegram voice behavior with regression coverage
- [x] Require current content-policy consent and never apply the clip speech chain accidentally to tracks
- [x] Bound hostile long-file worker CPU, memory, time, network, output and temporary disk

## Notes
2026-07-16 strict-sequence start from synchronized main merge 6e53606 after TASK-260712-2ogntd code PR #146 merge 15ebd3d and tracking PR #147 merge 6e53606; hosted runs 29475162175 and 29475408660 passed 4/4. Implementing secure long-file intake, constrained metadata/probe/cleanup and candidate-only deterministic pipeline evidence inline outside task-board spawn workflow. The accepted codec ADR is production no-go: production must return a stable unavailable failure and cannot publish/register a canonical encoder or enable stream_track_v1 until a reviewed replacement ADR exists.
2026-07-16 implementation review on exact head 7b30755 in PR #148. Consent-gated audio_track upload, 500 MiB and two-hour validation, fixed-prefix signature routing, constrained file-only probe, original stream metadata, temp/concurrency accounting, stable no-go replay and zero-variant/zero-WAV cleanup are covered by focused, race, delete-race and previous-head rollback tests. The checked canonical-variant item means the spike selected no production variant: the pipeline proves and returns codec_profile_unavailable instead of inventing one. Hosted run 29476335634 is in progress; task remains review until all four jobs pass.
2026-07-16 accepted on exact engineering head 7b307559b2b455f89b8ab206bc96802d79cf92b2 through PR #148, merge 4749a76ae0576cab628171528a01912ba025e0ea, after hosted run 29476335634 passed coordinator, node-core, pulsar-win and signed packaged-probe. The stable no-go is the exact codec-spike result, not an omitted encoder: no production variant, generated WAV, decoder capability or real-app claim was introduced.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-285pag/p2-streamed-track-components.puml) — Ingest and variant pipeline context for audio_track processing

## Outcome Resources
- [P2 audio-track intake and codec no-go pipeline](../../../../docs/analysis/p2-audio-track-variant-pipeline.md) — Secure long-audio intake, metadata/accounting and accepted codec no-go behavior
- [PR #148](https://github.com/relux-works/barycenter/pull/148) — Accepted engineering implementation and hosted CI provenance
