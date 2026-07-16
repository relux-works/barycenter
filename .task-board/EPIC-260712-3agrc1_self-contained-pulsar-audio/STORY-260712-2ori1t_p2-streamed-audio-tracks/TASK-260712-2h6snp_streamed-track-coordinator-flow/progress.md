## Status
development

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T07:04:00Z

## Blocked By
- TASK-260712-1n5fks
- TASK-260712-285pag
- TASK-260712-3lf8r0
- TASK-260712-31rkpe
- TASK-260712-1aprcb
- TASK-260712-2qpp6w
- TASK-260712-31vvjt
- STORY-260712-3v14m9
- STORY-260712-ob1tx2
- TASK-260712-2eympi
- TASK-260712-kr64r2
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-1c34fe
- TASK-260712-2bk0vy

## Blocks
- TASK-260712-3aj8w2
- TASK-260712-17w78q
- TASK-260712-1fpb9q
- TASK-260712-1q2kwa
- TASK-260712-wt2n7m

## Checklist
- [ ] Extend main-program element and session state for uploaded track sources
- [ ] Integrate queue and replace acceptance, progress and ended bookkeeping
- [ ] Drive buffer-ready scheduling and seek-reload flows from the coordinator
- [ ] Persist and restore track position and progress across pause and restart
- [ ] Preserve clip, overlay, interrupt and Spotify compatibility in FSM and loop tests
- [ ] Use a provider-neutral main-program adapter and persist audible rather than decoded-ahead position
- [ ] Define rebuffer, join catch-up, leave and ring-drained ended behavior

## Notes
2026-07-16 strict-sequence start from synchronized main merge cf3a33a after TASK-260712-3lf8r0 exact head 52bf876 and hosted run 29478459982 passed 4/4. Implementing provider-neutral streamed-track coordinator queue/replace, buffer-ready scheduling, audible progress, seek/rebuffer generations and restart restoration inline outside task-board spawn workflow. Production codec/player selection remains no-go, so deterministic candidate-neutral state machines may land but no production capability or hands-on playback will be claimed.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-2h6snp/p2-streamed-track-components.puml) — Coordinator component boundaries for streamed-track orchestration
- [p2-streamed-track-sequence.puml](file://TASK-260712-2h6snp/p2-streamed-track-sequence.puml) — Buffered-start and seek flow for coordinator orchestration

## Outcome Resources
(none)
