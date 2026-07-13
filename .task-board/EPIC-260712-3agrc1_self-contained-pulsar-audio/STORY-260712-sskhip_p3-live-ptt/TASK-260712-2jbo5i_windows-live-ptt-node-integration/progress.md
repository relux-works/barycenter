## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:25:04Z

## Last Update
2026-07-12T16:53:39Z

## Blocked By
- TASK-260712-ezdhpf
- TASK-260712-1ckdr7
- TASK-260712-3vzbbl

## Blocks
- TASK-260712-1rzqh9
- TASK-260712-39vjzd

## Checklist
- [ ] Hook the validated AppContainer-safe key-down or key-up path into the tray and capture lifecycle
- [ ] Stream encoded live chunks with session generations, cancel semantics and backpressure handling
- [ ] Add a bounded receiver jitter buffer and live duck or un-duck integration in the Windows audio graph
- [ ] Stop capture and playback safely on release, lock, suspend, quit, permission revoke, reconnect or stale session
- [ ] Add packaged tests or probes and publish remaining hardware-only evidence needs

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p3-live-ptt-components.puml](file://TASK-260712-2jbo5i/p3-live-ptt-components.puml) — Task-boundary diagram for Windows live PTT integration
- [p3-live-ptt-sequence.puml](file://TASK-260712-2jbo5i/p3-live-ptt-sequence.puml) — Live session sequence that the Windows node must implement
