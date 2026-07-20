## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T03:21:21Z

## Blocked By
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-2kj9kj
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6
- TASK-260712-1bcpda

## Checklist
- [x] Derive a unique session key and bind all live context into AAD
- [x] Encrypt sender frames off capture callbacks and verify before jitter decode
- [x] Reject nonce reuse replay tamper stale epoch and removed sender
- [x] Preserve C1 C2 FEC PLC backpressure DND and teardown bounds
- [x] Prove coordinator traffic capture cannot reproduce macOS speech
- [x] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
2026-07-20 producer implementation is production-dark: exact BE opaque wire mirror, witnessed live_ptt generation reservation and provider derivation seam, AAD binding, retry-safe sealing, auth-before-jitter, replay/nonce/membership teardown, 8 focused Swift tests, 348 full Swift tests, 200 acceptance discovery tests and 16/16 harness. Audit transform only; no production provider/runtime/capability/UI claim. Real C1-C2, packet capture, signed package, memory/crash, cross-process contention and macOS-Windows interop remain manual under TASK-260712-flaiie and TASK-260712-yj668d in EPIC-260714-th54l3. Production constructor remains gated on reviewed provider plus explicit cross-process generation serialization approval.

## Precondition Resources
(none)

## Outcome Resources
(none)
