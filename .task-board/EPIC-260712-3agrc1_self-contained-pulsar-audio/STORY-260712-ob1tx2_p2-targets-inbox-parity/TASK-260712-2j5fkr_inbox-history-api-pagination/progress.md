## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:12:51Z

## Last Update
2026-07-12T16:19:23Z

## Blocked By
- TASK-260712-2rlkp7
- TASK-260712-2bk0vy
- TASK-260712-1c34fe

## Blocks
- TASK-260712-2zoy4u
- TASK-260712-wt2n7m
- TASK-260712-2vipy3
- TASK-260712-1vklop

## Checklist
- [ ] Implement inbox and history queries with stable pagination
- [ ] Add replay delete and cancel mutations with policy checks
- [ ] Return sender safe and audience safe receipt views only
- [ ] Keep non target lookups indistinguishable from missing
- [ ] Use opaque stable cursors and test tenant isolation under concurrent pagination
- [ ] Keep every replay explicit, idempotent and newly targeted with no read-triggered playback

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-sequence.puml](file://TASK-260712-2j5fkr/p2-targets-inbox-parity-sequence.puml) — API flow for explicit target miss inbox and replay
