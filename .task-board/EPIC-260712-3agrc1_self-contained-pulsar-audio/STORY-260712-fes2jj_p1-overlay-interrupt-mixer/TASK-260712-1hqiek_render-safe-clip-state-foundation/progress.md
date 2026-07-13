## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:42:17Z

## Last Update
2026-07-12T17:03:16Z

## Blocked By
- TASK-260712-51y5k9
- TASK-260712-1g70av

## Blocks
- TASK-260712-2zbmq4
- TASK-260712-1viwvi
- TASK-260712-8mwyiv
- TASK-260712-1g6lk8
- TASK-260712-3aj8w2
- TASK-260712-17w78q
- TASK-260712-1ckdr7
- TASK-260712-19w1qn

## Checklist
- [ ] Replace render-time mutable ownership with preallocated clip or mixer state handoff on both nodes
- [ ] Define shared duck, interrupt, limiter and telemetry parameter carriers used by both platforms
- [ ] Preserve and document the legacy play_voice compatibility path while the new media lifecycle lands
- [ ] Use generation-safe prepared, armed, playing, cancelling and terminal state on both platforms
- [ ] Prove render handoff is preallocated and free of I/O, waits, allocation and blocking locks
- [ ] Make active sender-delete generation-safe and expose the frozen terminal reason

## Notes

## Precondition Resources
(none)

## Outcome Resources
(none)
