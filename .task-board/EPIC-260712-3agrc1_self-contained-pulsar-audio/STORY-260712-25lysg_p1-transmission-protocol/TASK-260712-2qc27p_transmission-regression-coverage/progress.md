## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T12:27:04Z

## Blocked By
- TASK-260712-1aprcb
- TASK-260712-2qpp6w
- TASK-260712-1g70av
- TASK-260712-31vvjt
- TASK-260712-2bbz13
- TASK-260712-26ip33

## Blocks
- TASK-260712-2cdjq8
- TASK-260712-2hodti
- TASK-260712-wy05n6
- TASK-260712-176b74
- TASK-260712-1xkn75

## Checklist
- [ ] Cover ordering, receipts, ACL, and visible downgrade in coordinator tests
- [ ] Add migration and rollback coverage for transmission tables and ACL
- [ ] Keep Go, Windows, and Swift contract suites green with the new messages enabled
- [ ] Test whole-transmission downgrade, exact barrier timing and opposite-origin serialization
- [ ] Map automated evidence and required real-hardware timing evidence to every story criterion

## Notes
Strict inline execution started 2026-07-14 from synchronized main merge 8d2b7d3825536ed9dc732f1e86040edc227a7acf (PR #26; tracking CI 29332298395 green). Scope is deterministic engineering regression, migration, protocol-mirror and compatibility evidence only. Any real-app playback, timing, packaged-install or physical-hardware proof is explicitly mapped to EPIC-260714-th54l3 and is neither executed nor claimed by this task.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-2qc27p/p1-transmission-scheduler-sequence.puml) — Regression coverage reference for barrier flow and downgrade paths

## Outcome Resources
- [transmission-regression-evidence.md](file://TASK-260712-2qc27p/transmission-regression-evidence.md) — Story AC, adversarial regression, rollback, codec and deferred manual-evidence matrix
