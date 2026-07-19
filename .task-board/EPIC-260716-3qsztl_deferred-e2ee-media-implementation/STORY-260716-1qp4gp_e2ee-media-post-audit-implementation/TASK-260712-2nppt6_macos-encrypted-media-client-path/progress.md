## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:40:35Z

## Last Update
2026-07-19T22:18:51Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-20j5tm
- TASK-260712-1rziyo
- TASK-260712-2i0w6x
- TASK-260712-1x9ruo
- TASK-260712-2kcduo
- TASK-260712-tcwn44
- TASK-260712-3980vy
- TASK-260712-aniuyy

## Blocks
- TASK-260712-1bcpda

## Checklist
- [ ] Store E2EE keys and grants in OS-secure storage and scrub plaintext temp or state.
- [ ] Implement local normalize, encode, and encrypt packaging for protected media.
- [ ] Verify manifests, unwrap keys, decrypt, cache, and play protected media locally.
- [ ] Handle rotation, history grants, device transfer or recovery, and explicit report consent.
- [ ] Cover log or crash redaction and negative grant or revoke cases.
- [ ] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.

## Notes
Root-reviewed integration-only scope: TASK-260712-1x9ruo owns key state, TASK-260712-2kcduo send, TASK-260712-tcwn44 playback and TASK-260712-3980vy live crypto. Original implementation checklist items mean UX integration and validation only.
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.

## Precondition Resources
- [p3-e2ee-media-components.puml](file://TASK-260712-2nppt6/p3-e2ee-media-components.puml) — macOS client reference diagram for local prep, key storage, and decrypt playback

## Outcome Resources
(none)
