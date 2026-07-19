## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:40:33Z

## Last Update
2026-07-19T19:56:50Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2i0w6x
- TASK-260712-2nppt6
- TASK-260712-2q4jbu
- TASK-260712-1bcpda
- TASK-260712-1yz5ca
- TASK-260712-25dzp4
- TASK-260712-1x9ruo
- TASK-260712-1rziyo

## Checklist
- [ ] Replace plaintext protected-media routing with manifest and ciphertext handling behind the feature flag.
- [ ] Seal recipient snapshots to epochs and rotate on join, leave, revoke, and Air changes.
- [ ] Reuse ACL, delete, retention, and inbox or history services without plaintext regressions.
- [ ] Reject unsupported or stale-epoch fetches explicitly in mixed-version tests.
- [ ] Keep feature-off legacy behavior intact.

## Notes
Root-reviewed correction: rotate means serialize and route client-produced signed group commits. Coordinator must never create unwrap escrow or log group or content secrets. Original checklist wording is subordinate to this invariant.
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
Execution started 2026-07-19 on branch feat/task-260712-20j5tm from merged schema foundation 2ab8a135. Scope remains feature-off and production-dark; independent delta review required before acceptance.
Producer implementation in progress: additive protocol-actor bindings, exact group-member snapshots, rotation requirements, durable event deliveries/acks, strict injected-verifier proposal/commit routing, and protected-write rotation gate are implemented production-dark. Focused and focused-race suites pass; full coordinator + go vet pass. Evidence packet and independent delta review remain pending.
Producer verification complete before commit: focused tests, full coordinator go test ./..., go vet ./..., 207/207 acceptance-contract checks, targeted evidence validators, and full race go test -race ./internal/store ./internal/e2eecontract -count=1 passed (store 502.193s; contract 1.427s). Evidence hashes reproduce; independent review and exact producer SHA still pending.

## Precondition Resources
- [p3-e2ee-media-components.puml](file://TASK-260712-20j5tm/p3-e2ee-media-components.puml) — Coordinator ciphertext-routing and rotation boundary diagram
- [p3-e2ee-media-sequence.puml](file://TASK-260712-20j5tm/p3-e2ee-media-sequence.puml) — Protected-send and revoke-rotation sequence for coordinator runtime

## Outcome Resources
(none)
