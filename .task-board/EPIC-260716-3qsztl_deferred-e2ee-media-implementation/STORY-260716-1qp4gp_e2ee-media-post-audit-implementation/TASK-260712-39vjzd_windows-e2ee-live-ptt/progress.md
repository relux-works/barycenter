## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T06:28:34Z

## Blocked By
- TASK-260712-25dzp4
- TASK-260712-1yz5ca
- TASK-260712-2jbo5i
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2q4jbu
- TASK-260712-1bcpda

## Checklist
- [ ] Derive a unique session key and bind all live context into AAD
- [ ] Encrypt sender frames off capture callbacks and verify before jitter decode
- [ ] Reject nonce reuse replay tamper stale epoch and removed sender
- [ ] Preserve C1 C2 FEC PLC backpressure DND and teardown bounds
- [ ] Prove coordinator traffic capture cannot reproduce Windows speech

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
2026-07-20 strict sequential execution started on branch feat/task-260712-39vjzd from accepted main merge e47eb6b583fa0319beee460b87397bdb75dbcf39. Scope is production-dark best-effort Windows E2EE live-PTT engineering using accepted opaque BE wire, Windows witnessed key-state repository, existing live sender/receiver hooks and an injected reviewed provider seam; no provider, library, suite, nonce algorithm, runtime, UI or capability selection. Real coordinator traffic capture, native DPAPI/MSIX/NTFS, real provider/crypto/codec, microphone/speaker, latency/quality, memory/crash/packet forensics and macOS-Windows physical interop remain manual/deferred in EPIC-260714-th54l3. Independent Claude Fable 5 max exact-SHA review is required before acceptance.

## Precondition Resources
(none)

## Outcome Resources
(none)
