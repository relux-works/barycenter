## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:25:04Z

## Last Update
2026-07-12T16:53:39Z

## Blocked By
- TASK-260712-26mnp1
- TASK-260712-19w1qn
- TASK-260712-3vzbbl

## Blocks
- TASK-260712-1rzqh9
- TASK-260712-3980vy

## Checklist
- [ ] Hook the supported global key-down or key-up path into the existing macOS capture and menu lifecycle
- [ ] Stream encoded live chunks with session generations, cancel semantics and backpressure handling
- [ ] Add a bounded receiver jitter buffer and live duck or un-duck integration in the audio graph
- [ ] Stop capture and playback safely on release, lock or sleep where observable, quit, reconnect or stale session
- [ ] Add platform tests or probes and publish remaining hardware-only evidence needs

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p3-live-ptt-components.puml](file://TASK-260712-2kj9kj/p3-live-ptt-components.puml) — Task-boundary diagram for macOS live PTT integration
- [p3-live-ptt-sequence.puml](file://TASK-260712-2kj9kj/p3-live-ptt-sequence.puml) — Live session sequence that the macOS node must implement
