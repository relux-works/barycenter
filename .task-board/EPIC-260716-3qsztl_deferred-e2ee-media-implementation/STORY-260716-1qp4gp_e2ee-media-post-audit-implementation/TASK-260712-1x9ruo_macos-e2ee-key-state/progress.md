## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-19T22:06:11Z

## Blocked By
- TASK-260712-aniuyy
- TASK-260712-2u1w16
- TASK-260712-20j5tm

## Blocks
- TASK-260712-1rziyo
- TASK-260712-2kcduo
- TASK-260712-tcwn44
- TASK-260712-3980vy
- TASK-260712-2nppt6

## Checklist
- [x] Store distinct device group grant and content-key state in Keychain
- [x] Implement transactional persist-before-ack and clone or rollback detection
- [x] Pass known-answer epoch replay fork and crash vectors
- [x] Redact preferences logs telemetry crashes and diagnostics
- [x] Publish narrow send playback live and UX interfaces

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
Upstream TASK-260712-20j5tm independent review follow-up I1 / EPC-005: explicitly pin client semantics for an active Air member whose registered device rows are all revoked. Current coordinator treats those devices as removed endpoints rather than an unsupported target; macOS key-state review must confirm or reject that interpretation.
Execution started 2026-07-20 on branch feat/task-260712-1x9ruo from merged opaque-router main 3b08b745. Scope remains production-dark best-effort coding with unit/state-machine evidence; real app and physical Keychain behavior stay in the manual epic, and production crypto/library activation remains externally gated.
Producer evidence 2026-07-20: production-dark macOS E2EE Keychain repository implemented with separate device metadata, signing, agreement, group, grant and bounded content-cache slots; exact record/witness readback before ack; predecessor epoch and fork checks; crash, replay, clone, expiry, deletion and redaction vectors. Focused Swift 10/10, full NodeCore 318/318, acceptance 217/217, swift-format clean. ADR docs/analysis/p3-macos-e2ee-key-state-v1.md and packet acceptance/phase3/macos-e2ee-key-state-v1.json. Physical Keychain, signed package, backup/restore and real crypto stay not-run in EPIC-260714-th54l3. Awaiting exact-SHA independent Fable 5 max review.

## Precondition Resources
(none)

## Outcome Resources
(none)
