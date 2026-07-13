## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-12T16:30:34Z

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
- [ ] Accept audio_track requests with long-file validation and phase-two limits
- [ ] Generate the canonical compressed variants chosen by the codec spike
- [ ] Persist duration, size, hash and variant metadata without full WAV retention
- [ ] Keep retries idempotent and leave no partially ready track records
- [ ] Protect clip and legacy Telegram voice behavior with regression coverage
- [ ] Require current content-policy consent and never apply the clip speech chain accidentally to tracks
- [ ] Bound hostile long-file worker CPU, memory, time, network, output and temporary disk

## Notes

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-285pag/p2-streamed-track-components.puml) — Ingest and variant pipeline context for audio_track processing

## Outcome Resources
(none)
