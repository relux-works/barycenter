## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:37:28Z

## Last Update
2026-07-16T22:10:49Z

## Blocked By
- TASK-260712-3sj8ox
- STORY-260712-2ve1c8
- TASK-260712-hb5xz2

## Blocks
- TASK-260712-1kk8bd
- TASK-260712-1eva0y

## Checklist
- [x] Add additive tables and indexes for cues, schedules, principals, execution lineage, and disable state.
- [x] Persist principals with hashed or similarly non-plaintext secret material and explicit revoked timestamps.
- [x] Capture enough execution lineage for attribution, pending-cancel lookup, and quick-disable actions.
- [x] Verify rollback and migration behavior against the current single-file SQLite store.

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge 2823e6288d823e466bb24c3f5deb26e7f4a8a332 after TASK-260712-hb5xz2 tracking PR #206 and hosted run 29536508428 passed 4/4. Execute inline outside task-board spawn. Scope is additive schedule/principal/disable/execution lineage and deterministic claims only; no public control API, runtime worker, client composition or production capability.
2026-07-16 engineering complete. Code head 6f772ba21000915980275520a6c5a24c388909a2; clean exact-head acceptance 12/12 passed with manualEvidence not-run; PR #207 hosted run 29538458103 passed 4/4 and merged as 4cd9a30ca32faabbd1e975d5b6275ae3065affda. Production-dark additive storage only: no public API, scheduler loop, client composition, real-app or hardware claim.

## Precondition Resources
(none)

## Outcome Resources
- [acceptance-6f772ba-manifest.json](file://TASK-260712-3sv87k/acceptance-6f772ba-manifest.json) — Exact code-head automated acceptance: 12/12 pass, clean start/end, manualEvidence not-run.
