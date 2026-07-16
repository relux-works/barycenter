## Status
development

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:13:27Z

## Last Update
2026-07-16T06:22:23Z

## Blocked By
- TASK-260712-1n5fks
- TASK-260712-285pag
- TASK-260712-3mcof4
- TASK-260712-gj0cko
- TASK-260712-1aprcb
- STORY-260712-ob1tx2
- TASK-260712-2eympi
- TASK-260712-2bk0vy
- TASK-260712-2zoy4u
- TASK-260712-2ogntd

## Blocks
- TASK-260712-2h6snp
- TASK-260712-3aj8w2
- TASK-260712-17w78q
- TASK-260712-1fpb9q
- TASK-260712-n11rg6

## Checklist
- [ ] Expose variant selection and standards-compliant range responses
- [ ] Authorize fetches from target snapshot ACL rather than live membership
- [ ] Revoke new fetches on delete, report or actor disable without media id churn
- [ ] Emit size, hash and egress metrics needed for quotas and support
- [ ] Cover 200, 206, 416 and revocation paths in handler tests
- [ ] Implement conditional authenticated range semantics and cap range amplification
- [ ] Separate report-local protection from moderator delete or disable revocation

## Notes
2026-07-16 strict-sequence start from synchronized main merge 4749a76 after TASK-260712-285pag code PR #148 and hosted run 29476335634 passed 4/4. Implementing authenticated conditional range serving, immutable target-snapshot authorization, revocation and actual-egress/amplification accounting inline outside task-board spawn workflow. Production codec/player selection remains no-go, so deterministic candidate-neutral fixtures may exercise the route but no production stream capability or real-app playback will be claimed.

## Precondition Resources
- [p2-streamed-track-components.puml](file://TASK-260712-3lf8r0/p2-streamed-track-components.puml) — Range-serving and revocation context for streamed-track fetch

## Outcome Resources
(none)
