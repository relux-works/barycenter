## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:40:35Z

## Last Update
2026-07-12T16:54:24Z

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

## Blocks
- TASK-260712-1bcpda

## Checklist
- [ ] Store E2EE keys and grants in DPAPI or Credential Locker and scrub plaintext temp or state.
- [ ] Implement local normalize, encode, and encrypt packaging for protected media.
- [ ] Verify manifests, unwrap keys, decrypt, cache, and play protected media locally.
- [ ] Handle rotation, history grants, device transfer or recovery, and explicit report consent.
- [ ] Cover log or crash redaction and negative grant or revoke cases.

## Notes
Root-reviewed integration-only scope: TASK-260712-25dzp4 owns key state, TASK-260712-28zhpl send, TASK-260712-1u57qz playback and TASK-260712-39vjzd live crypto. Original implementation checklist items mean UX integration and validation only.

## Precondition Resources
- [p3-e2ee-media-components.puml](file://TASK-260712-2q4jbu/p3-e2ee-media-components.puml) — Windows client reference diagram for local prep, key storage, and decrypt playback

## Outcome Resources
(none)
