## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:45:03Z

## Last Update
2026-07-14T14:05:57Z

## Blocked By
- TASK-260712-1bpog0
- TASK-260712-gj0cko
- TASK-260712-31vvjt

## Blocks
- TASK-260712-pbfz37
- TASK-260712-34stvx
- TASK-260712-dlltnr
- TASK-260712-3t9nr8
- TASK-260712-1xik11
- TASK-260712-3e4p0c
- TASK-260712-wy05n6
- TASK-260712-1xkn75
- TASK-260712-2zoy4u

## Checklist
- [ ] Add additive schema and state transitions for reports, blocks and moderation audit records.
- [ ] Define app control and support auth boundaries and exact semantics for report, block, delete, disable and status lookup.
- [ ] Wire moderation actions into media fetch and future delivery enforcement without bypassing ACL or retention guarantees.
- [ ] Cover create, action and rollback behavior with unit and integration tests, including reporter privacy and repeated actions.
- [ ] Use separate least-privilege operator authentication and audit every evidence read
- [ ] Delegate block, delete, disable and cancellation to canonical services and revoke live credentials
- [ ] Rate-limit reporting and enforce reported-content retention without ordinary-log leakage
- [ ] Apply the same frozen active-media delete policy for operator removal

## Notes
Strict inline execution started from synchronized main e588fc9b727d6264c289f69cc97ea77e4987f939 after PR #29. Implement the least-privilege report/operator control plane by extending additive store migrations and reusing canonical media lifecycle, credential revocation, live disconnect, scheduler cancellation and block services. No task-board spawn workflow and no manual hardware claim.

## Precondition Resources
(none)

## Outcome Resources
(none)
