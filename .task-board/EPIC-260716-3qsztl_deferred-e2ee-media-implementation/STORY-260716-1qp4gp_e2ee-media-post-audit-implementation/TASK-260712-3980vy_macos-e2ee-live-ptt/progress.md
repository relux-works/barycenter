## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T02:51:09Z

## Blocked By
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-2kj9kj
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6
- TASK-260712-1bcpda

## Checklist
- [ ] Derive a unique session key and bind all live context into AAD
- [ ] Encrypt sender frames off capture callbacks and verify before jitter decode
- [ ] Reject nonce reuse replay tamper stale epoch and removed sender
- [ ] Preserve C1 C2 FEC PLC backpressure DND and teardown bounds
- [ ] Prove coordinator traffic capture cannot reproduce macOS speech
- [ ] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.

## Precondition Resources
(none)

## Outcome Resources
(none)
