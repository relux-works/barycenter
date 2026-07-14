## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:45:03Z

## Last Update
2026-07-14T14:47:37Z

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
- [x] Add additive schema and state transitions for reports, blocks and moderation audit records.
- [x] Define app control and support auth boundaries and exact semantics for report, block, delete, disable and status lookup.
- [x] Wire moderation actions into media fetch and future delivery enforcement without bypassing ACL or retention guarantees.
- [x] Cover create, action and rollback behavior with unit and integration tests, including reporter privacy and repeated actions.
- [x] Use separate least-privilege operator authentication and audit every evidence read
- [x] Delegate block, delete, disable and cancellation to canonical services and revoke live credentials
- [x] Rate-limit reporting and enforce reported-content retention without ordinary-log leakage
- [x] Apply the same frozen active-media delete policy for operator removal

## Notes
Strict inline execution started from synchronized main e588fc9b727d6264c289f69cc97ea77e4987f939 after PR #29. Implement the least-privilege report/operator control plane by extending additive store migrations and reusing canonical media lifecycle, credential revocation, live disconnect, scheduler cancellation and block services. No task-board spawn workflow and no manual hardware claim.
Accepted engineering code head 2a0b1352bd79ef8b51863ba5f2ab77188d66ff22: additive report/operator/audit schema; exact target-scoped foreign reporting; privacy-safe status; hashed scoped mod_ operator tokens and CLI; audited digest-verified 30-day evidence; crash-resumable no_action/delete_media/disable_actor/disable_orbit; canonical block/lifecycle/credential/scheduler/hub enforcement; retention and exact previous-head rollback. Local vet, full tests, focused race, full pinned rollback, Windows vet/test/cross-build, and Swift release build passed. Local Swift tests were unavailable under standalone CommandLineTools; hosted node-core Swift tests passed. Hosted run 29342009648 passed coordinator, node-core, pulsar-win and signed packaged probe on the exact head.

## Precondition Resources
(none)

## Outcome Resources
(none)
