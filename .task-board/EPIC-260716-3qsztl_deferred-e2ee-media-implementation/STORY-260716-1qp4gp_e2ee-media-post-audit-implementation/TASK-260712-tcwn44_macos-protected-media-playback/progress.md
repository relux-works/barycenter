## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-19T22:18:51Z

## Blocked By
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6

## Checklist
- [ ] Verify manifest envelope and each chunk before decode
- [ ] Implement authenticated ranges seeks and ciphertext-only durable cache
- [ ] Purge revoked deleted expired corrupt and wrong-target state
- [ ] Meet Phase 2 player gates and existing mixer semantics
- [ ] Scan macOS disk logs memory artifacts and crashes for leakage
- [ ] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.

## Precondition Resources
(none)

## Outcome Resources
(none)
