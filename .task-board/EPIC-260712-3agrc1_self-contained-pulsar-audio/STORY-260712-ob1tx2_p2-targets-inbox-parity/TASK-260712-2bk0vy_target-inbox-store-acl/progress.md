## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:12:51Z

## Last Update
2026-07-12T16:30:31Z

## Blocked By
- TASK-260712-2rlkp7
- TASK-260712-1aprcb

## Blocks
- TASK-260712-2j5fkr
- TASK-260712-2zoy4u
- TASK-260712-1vklop
- TASK-260712-3lf8r0
- TASK-260712-2h6snp

## Checklist
- [ ] Add additive schema for target snapshots inbox rows and receipt pagination
- [ ] Persist replay lineage expiry and revocation state
- [ ] Replace membership based media auth with snapshot based authorization
- [ ] Cover migration and rollback safety
- [ ] Extend existing target rows and indexes rather than replacing the Phase 1 snapshot schema
- [ ] Guarantee exactly one eligible inbox item per missed target and no old-item inheritance by new members

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-components.puml](file://TASK-260712-2bk0vy/p2-targets-inbox-parity-components.puml) — Store and ACL context for explicit targets and inbox state
