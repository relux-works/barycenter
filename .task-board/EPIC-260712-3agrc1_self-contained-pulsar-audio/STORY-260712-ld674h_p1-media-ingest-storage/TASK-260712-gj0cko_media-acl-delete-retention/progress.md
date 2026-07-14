## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T05:09:33Z

## Blocked By
- TASK-260712-z6h6wh
- TASK-260712-2af2dp
- TASK-260712-1bnos4
- TASK-260712-3mcof4
- TASK-260712-1sae4q

## Blocks
- TASK-260712-3huupe
- TASK-260712-3e4p0c
- TASK-260712-2kec2s
- TASK-260712-3lf8r0
- TASK-260712-2zoy4u

## Checklist
- [ ] Implement authorized GET and early DELETE lifecycle for generic media
- [ ] Sweep failed, expired and deleted bytes according to phase-one retention rules
- [ ] Cover cross-orbit negative access, delete revocation and expiry behavior with tests

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge 0d6863c462111da6ed27f851a636e40d95100d73. Scope is the integration delta across already-landed generic GET ACL, DELETE cancellation, lifecycle cleanup, Telegram legacy mapping and mixed-rollout behavior; manual real-app and hardware evidence remain in the separate manual-test epic.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-component.puml](file://TASK-260712-gj0cko/p1-media-ingest-component.puml) — ACL, delete and retention boundaries for generic media
