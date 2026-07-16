## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:40:34Z

## Last Update
2026-07-16T00:15:02Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-20j5tm
- TASK-260712-25dzp4
- TASK-260712-1x9ruo
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6
- TASK-260712-2q4jbu
- TASK-260712-1bcpda

## Checklist
- [ ] Implement current-epoch bootstrap and same-user device-transfer semantics.
- [ ] Enforce explicit history grants with one-time or time-bound approvals and audit.
- [ ] Rotate away lost or revoked devices without resurrecting old node credentials.
- [ ] Integrate actor or role checks and Air membership lifecycle triggers.
- [ ] Cover expiry, replay, revoke, and rollback edge cases.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.

## Precondition Resources
- [p3-e2ee-media-sequence.puml](file://TASK-260712-1rziyo/p3-e2ee-media-sequence.puml) — History-grant and device-transfer sequence for key bootstrap and recovery

## Outcome Resources
(none)
