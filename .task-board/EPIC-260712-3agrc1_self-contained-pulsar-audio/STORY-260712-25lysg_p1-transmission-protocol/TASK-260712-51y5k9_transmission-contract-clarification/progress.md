## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T06:52:45Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1aprcb
- TASK-260712-2qpp6w
- TASK-260712-1g70av
- TASK-260712-1hqiek
- TASK-260712-1c1ska
- TASK-260712-31rkpe

## Checklist
- [ ] Write the transmission HTTP and WebSocket contract note
- [ ] Define visible downgrade and cancel semantics
- [ ] Define DND and block ownership boundaries for downstream stories
- [ ] Freeze origin defaults, delivery-kind rules, overlay limit, trusted accepted_at and stale-play behavior
- [ ] Freeze the exact barrier formula and whole-transmission mixed-fleet downgrade rule
- [ ] Separate visible overlay downgrade from interrupt fallback that requires explicit user confirmation
- [ ] Freeze sender-delete behavior and receipts for queued, prepared, scheduled and already-playing media

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge c4cb324bb4e783e97bb1fbf1bb61efef9dfbf10f after TASK-260712-jolzhh and the full P1 media ingest story were accepted. Scope is contract clarification and durable documentation; implementation remains in the ordered downstream tasks. Manual real-app and physical-hardware evidence stays in EPIC-260714-th54l3.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-51y5k9/p1-transmission-scheduler-sequence.puml) — Clarification diagram for transmission flow, receipts, and legacy downgrade

## Outcome Resources
(none)
