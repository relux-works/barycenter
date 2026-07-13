## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-12T17:03:16Z

## Blocked By
- TASK-260712-1aprcb
- TASK-260712-2qpp6w
- TASK-260712-1g70av

## Blocks
- TASK-260712-2qc27p
- TASK-260712-3d6cnn
- TASK-260712-2kec2s
- TASK-260712-2h6snp
- TASK-260712-kr64r2

## Checklist
- [ ] Add separate overlay controller state and FIFO ordering by accepted_at plus ULID
- [ ] Drive prepare barrier, scheduled start, and missed receipt transitions
- [ ] Bridge unsupported or legacy targets to after_current and cancel pending transmissions on leave or apart or delete
- [ ] Cover partial readiness, offline, DND, blocked, and cancel flows in coordinator tests
- [ ] Key one controller by the effective playback domain so opposite approach origins share one FIFO
- [ ] Implement the three-second deadline and exact RTT-based coordinator-clock schedule
- [ ] Reject unconfirmed interrupt fallback at the scheduler boundary

## Notes

## Precondition Resources
- [p1-transmission-protocol-components.puml](file://TASK-260712-31vvjt/p1-transmission-protocol-components.puml) — Coordinator scheduler component context
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-31vvjt/p1-transmission-scheduler-sequence.puml) — Prepare barrier and receipt sequencing for overlay scheduling

## Outcome Resources
(none)
