## Status
to-review

## Assigned To
codex-root-inline

## Created
2026-07-12T16:40:34Z

## Last Update
2026-07-20T00:55:14Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-20j5tm
- TASK-260712-25dzp4
- TASK-260712-1x9ruo
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6
- TASK-260712-2q4jbu
- TASK-260712-1bcpda

## Checklist
- [x] Implement current-epoch bootstrap and same-user device-transfer semantics.
- [x] Enforce explicit history grants with one-time or time-bound approvals and audit.
- [x] Rotate away lost or revoked devices without resurrecting old node credentials.
- [x] Integrate actor or role checks and Air membership lifecycle triggers.
- [x] Cover expiry, replay, revoke, and rollback edge cases.
- [x] Define fail-closed re-enrollment or reset for partial or lost macOS identity slots and bounded cleanup for expired history grants.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
Strict sequential execution started 2026-07-20 on branch feat/task-260712-1rziyo from merged report-evidence main f9fd2ec965e9b8b3396a10339541ae1327dd6a90. Scope is production-dark best-effort coding and automated evidence only. No real-device recovery, Keychain/DPAPI interop, production crypto, signed package, hardware or irreversible-history recovery claim may be self-certified; manual evidence remains in EPIC-260714-th54l3 and production EPC gates remain open.
Production-dark implementation complete on feat/task-260712-1rziyo: exact current-epoch one-time opaque transfer, explicit bounded history grants, atomic lost-device revoke plus rotation, macOS/Windows fail-closed identity reset and bounded expired-grant cleanup. Full local acceptance harness passed 16/16 at .temp/acceptance/task-260712-1rziyo-local/manifest.json. Real-device, signed-package and production-crypto evidence remains explicitly not run in EPIC-260714-th54l3.

## Precondition Resources
- [p3-e2ee-media-sequence.puml](file://TASK-260712-1rziyo/p3-e2ee-media-sequence.puml) — History-grant and device-transfer sequence for key bootstrap and recovery

## Outcome Resources
- [p3-e2ee-recovery-device-transfer-history-grants-v1.md](file://TASK-260712-1rziyo/p3-e2ee-recovery-device-transfer-history-grants-v1.md) — Production-dark recovery, transfer, history-grant and manual-evidence boundary
- [e2ee-recovery-device-transfer-v1.json](file://TASK-260712-1rziyo/e2ee-recovery-device-transfer-v1.json) — Fail-closed acceptance packet with pinned artifacts and deferred manual scope
- [e2ee-recovery-v1-vectors.json](file://TASK-260712-1rziyo/e2ee-recovery-v1-vectors.json) — Shared coordinator/macOS/Windows recovery policy vectors
