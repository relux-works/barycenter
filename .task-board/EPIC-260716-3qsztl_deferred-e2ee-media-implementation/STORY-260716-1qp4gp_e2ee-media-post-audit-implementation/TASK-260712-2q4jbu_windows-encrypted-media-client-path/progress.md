## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:40:35Z

## Last Update
2026-07-20T08:40:46Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-20j5tm
- TASK-260712-1rziyo
- TASK-260712-2i0w6x
- TASK-260712-25dzp4
- TASK-260712-28zhpl
- TASK-260712-1u57qz
- TASK-260712-39vjzd
- TASK-260712-aniuyy

## Blocks
- TASK-260712-1bcpda

## Checklist
- [x] Store E2EE keys and grants in DPAPI or Credential Locker and scrub plaintext temp or state.
- [x] Implement local normalize, encode, and encrypt packaging for protected media.
- [x] Verify manifests, unwrap keys, decrypt, cache, and play protected media locally.
- [x] Handle rotation, history grants, device transfer or recovery, and explicit report consent.
- [x] Cover log or crash redaction and negative grant or revoke cases.

## Notes
Root-reviewed integration-only scope: TASK-260712-25dzp4 owns key state, TASK-260712-28zhpl send, TASK-260712-1u57qz playback and TASK-260712-39vjzd live crypto. Original implementation checklist items mean UX integration and validation only.
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
2026-07-20 strict sequential execution started on branch feat/task-260712-2q4jbu from accepted main merge d265228f67858111276a8b466d6c0eb50ab66e54. Scope is integration-only over accepted Windows key-state/send/playback/live services: honest encrypted/plaintext status, verification/revoke, current access transfer or selected user-held recovery, bounded history grants, irrecoverable-history warning, explicit metadata-only versus decrypted-evidence consent, accessibility and redaction. Do not reimplement crypto, select production provider/suite/container, wire or advertise runtime capability, or claim signed MSIX/native DPAPI/codec/physical hardware evidence. Manual real-app evidence stays in EPIC-260714-th54l3. Independent Claude Fable 5 max exact-SHA review with zero open Critical/High/Medium is required before acceptance.
2026-07-20 producer implementation complete pending exact-SHA independent review. Added a production-dark Windows presentation/command model and dormant one-repository composition over the accepted key-state, protected-send, protected-playback and live-PTT services. No main.go wiring, capability, provider, suite or container selection was added. Focused/full Go tests, vet, race and Windows amd64/arm64 cross-builds pass; validator plus 5 focused acceptance tests pass; full acceptance contract discovery passes. One unrelated capture-workflow timing test flaked once inside the aggregate race stage and passed immediately on exact standalone race rerun. Manual signed-MSIX/native-DPAPI/real-device/audio/accessibility/forensics evidence remains deferred to EPIC-260714-th54l3.

## Precondition Resources
- [p3-e2ee-media-components.puml](file://TASK-260712-2q4jbu/p3-e2ee-media-components.puml) — Windows client reference diagram for local prep, key storage, and decrypt playback

## Outcome Resources
- [windows-encrypted-media-client-path-acceptance-v1.json](file://TASK-260712-2q4jbu/windows-encrypted-media-client-path-acceptance-v1.json) — Automated acceptance boundary and deferred manual evidence
- [p3-windows-encrypted-media-client-path-v1.md](file://TASK-260712-2q4jbu/p3-windows-encrypted-media-client-path-v1.md) — Windows integration analysis and handoff
- [windows-encrypted-media-client-path-v1.json](file://TASK-260712-2q4jbu/windows-encrypted-media-client-path-v1.json) — Portable production-dark Windows encrypted-media client contract
