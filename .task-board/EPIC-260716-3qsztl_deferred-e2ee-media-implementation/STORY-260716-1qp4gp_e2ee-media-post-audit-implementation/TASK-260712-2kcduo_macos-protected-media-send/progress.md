## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-19T22:18:51Z

## Blocked By
- TASK-260712-16xmy2
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6

## Checklist
- [ ] Prepare clip track and saved-cue content locally with the selected toolchain
- [ ] Generate unique keys nonces authenticated manifests and target envelopes
- [ ] Resume ciphertext upload idempotently without reuse
- [ ] Clean or retain plaintext drafts only under the reviewed explicit policy
- [ ] Prove no server plaintext and no silent downgrade on macOS
- [ ] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.

## Precondition Resources
(none)

## Outcome Resources
(none)
