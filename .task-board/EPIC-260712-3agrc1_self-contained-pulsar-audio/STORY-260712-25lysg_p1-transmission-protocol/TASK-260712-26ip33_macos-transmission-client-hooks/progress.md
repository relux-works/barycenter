## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-12T17:03:16Z

## Blocked By
- TASK-260712-1g70av

## Blocks
- TASK-260712-2qc27p
- TASK-260712-3d6cnn

## Checklist
- [ ] Announce clip capabilities and implement prepare/download/hash/decode-ready flow
- [ ] Emit transmission lifecycle and DND or presence messages from CoordinatorClient and PlayerCore
- [ ] Keep legacy play_voice and solo_voice working while routing scheduled play through mixer hooks
- [ ] Use synchronized coordinator time and reject stale, duplicate or cancelled scheduled starts
- [ ] Keep prepare I/O and scheduling out of render and CoordinatorClient blocking paths

## Notes

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-26ip33/p1-transmission-scheduler-sequence.puml) — macOS client flow for prepare, ready, play, and cancel

## Outcome Resources
(none)
