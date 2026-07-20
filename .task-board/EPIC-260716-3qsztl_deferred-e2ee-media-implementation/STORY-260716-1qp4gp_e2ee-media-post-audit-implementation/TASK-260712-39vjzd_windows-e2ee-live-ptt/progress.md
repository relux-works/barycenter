## Status
reviewing

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T06:58:17Z

## Blocked By
- TASK-260712-25dzp4
- TASK-260712-1yz5ca
- TASK-260712-2jbo5i
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2q4jbu
- TASK-260712-1bcpda

## Checklist
- [x] Derive a unique session key and bind all live context into AAD
- [x] Encrypt sender frames off capture callbacks and verify before jitter decode
- [x] Reject nonce reuse replay tamper stale epoch and removed sender
- [x] Preserve C1 C2 FEC PLC backpressure DND and teardown bounds
- [ ] Prove coordinator traffic capture cannot reproduce Windows speech

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
2026-07-20 strict sequential execution started on branch feat/task-260712-39vjzd from accepted main merge e47eb6b583fa0319beee460b87397bdb75dbcf39. Scope is production-dark best-effort Windows E2EE live-PTT engineering using accepted opaque BE wire, Windows witnessed key-state repository, existing live sender/receiver hooks and an injected reviewed provider seam; no provider, library, suite, nonce algorithm, runtime, UI or capability selection. Real coordinator traffic capture, native DPAPI/MSIX/NTFS, real provider/crypto/codec, microphone/speaker, latency/quality, memory/crash/packet forensics and macOS-Windows physical interop remain manual/deferred in EPIC-260714-th54l3. Independent Claude Fable 5 max exact-SHA review is required before acceptance.
2026-07-20 producer engineering complete at exact commit aee07339bcfe014b39edac10734f713d11333792 pending independent review. Added production-dark exact BE wire mirror, cross-process witnessed live_ptt generation reservation, shared epoch plus commitDigest AAD, retry-safe seal on the existing transport-worker seam, authentication-before-jitter open, bounded reorder/replay/nonce/membership teardown and defensive provider/caller ownership. Final evidence: 11 focused scenarios plus race; live capture/receiver/node regressions plus race; full Go plus full race; vet; acceptance discovery 215/215; Windows amd64/arm64 blind compile; automated harness 16/16 with manifest .temp/acceptance/20260720T065018Z/manifest.json. Checklist item 5 remains intentionally open: real coordinator traffic capture, signed MSIX, native DPAPI/NTFS, real provider/crypto/codec, microphone/speaker, latency/quality, memory/crash/swap/backup and macOS-Windows hardware interop are not-run manual scope in EPIC-260714-th54l3. Claude Fable 5 max exact-SHA review required before acceptance.

## Precondition Resources
- [independent-review-brief-aee0733.md](file://TASK-260712-39vjzd/independent-review-brief-aee0733.md) — Exact aee0733 independent security protocol lifecycle concurrency and realtime review brief

## Outcome Resources
(none)
