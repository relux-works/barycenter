## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-19T21:23:42Z

## Blocked By
- TASK-260712-aniuyy
- TASK-260712-3w1cst
- TASK-260712-20j5tm

## Blocks
- TASK-260712-2i0w6x
- TASK-260712-28zhpl
- TASK-260712-2kcduo
- TASK-260712-1u57qz
- TASK-260712-tcwn44
- TASK-260712-39vjzd
- TASK-260712-3980vy
- TASK-260712-1bcpda

## Checklist
- [ ] Add bounded ciphertext manifest chunk envelope and live-frame routes
- [ ] Enforce actor target epoch range quota and rate authorization
- [ ] Preserve canonical upload cache delete report DND and receipt services
- [ ] Prove slow recipient malformed input and restart remain bounded
- [ ] Prove coordinator artifacts cannot decode protected fixtures

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
Upstream TASK-260712-20j5tm independent review follow-up L1: if opaque-router work exposes or consumes rotation audit reasons, make multi-cause reason_code selection deterministic or preserve the full cause set. This is audit-fidelity only and did not block the production-dark routing/rotation foundation.
Upstream schema/routing review carry-over I2: add an explicit opaque-object staging/fetch test against a group already persisted in forked state. Commit coverage exists, but downstream object/replay/grant/transfer paths must prove the persisted fork blocks them fail closed.
Execution started 2026-07-20 on branch feat/task-260712-1yz5ca from merged routing/rotation foundation main 32fee4ac. Scope remains production-dark and keyless; no runtime capability activation or production crypto selection is authorized.
Producer implementation in progress: production-dark object router now freezes exact recipient lineage, enforces encrypted-manifest and per-chunk/whole-object hashes, contiguous idempotent chunks, aligned bounded ranges + If-Range, upload/egress quotas, author-only delete and server chunk removal. Separate BE opaque-live envelope binds epoch/generation/target and cannot downgrade to legacy BP; persisted generation/replay/rate state, restart termination, slow/DND/block recipient isolation and monotonic receipts are implemented without frame persistence. Focused/full tests, focused race and vet pass; evidence packet and independent delta review remain pending.
Producer verification complete before commit: focused E2EE opaque store and contract tests pass; full coordinator go test ./... and go vet ./... pass; acceptance-contract 212/212 pass; focused race passes (store 10.285s, e2eecontract 1.388s); full race passes with explicit 15m timeout (store 594.955s, e2eecontract 1.460s). The initial full-race attempt used the default 10m timeout and timed out in unrelated TestTransmissionSchedulerRechecksDNDWithoutSuppressingUserMessagesOnly during transmission schema initialization; it is retained as a non-accepted attempt and no race diagnostic was emitted. Production remains disabled and runtime HTTP/WS wiring is intentionally absent pending platform key-state and selected crypto/container stack. Exact producer SHA and independent review are pending.

## Precondition Resources
(none)

## Outcome Resources
(none)
