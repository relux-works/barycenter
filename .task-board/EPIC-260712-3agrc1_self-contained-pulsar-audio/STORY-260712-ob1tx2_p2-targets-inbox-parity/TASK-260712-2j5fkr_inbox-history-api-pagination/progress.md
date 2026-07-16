## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:51Z

## Last Update
2026-07-16T00:25:07Z

## Blocked By
- TASK-260712-2rlkp7
- TASK-260712-2bk0vy
- TASK-260712-1c34fe

## Blocks
- TASK-260712-2zoy4u
- TASK-260712-wt2n7m
- TASK-260712-2vipy3
- TASK-260712-1vklop

## Checklist
- [x] Implement inbox and history queries with stable pagination
- [x] Add replay delete and cancel mutations with policy checks
- [x] Return sender safe and audience safe receipt views only
- [x] Keep non target lookups indistinguishable from missing
- [x] Use opaque stable cursors and test tenant isolation under concurrent pagination
- [x] Keep every replay explicit, idempotent and newly targeted with no read-triggered playback

## Notes
2026-07-16 strict-sequence start from synchronized main ed3f236 after acceptance of TASK-260712-2ctf3x. Implementing inline outside task-board spawn workflow. Scope is authenticated actor-scoped inbox/history/detail/receipt pagination plus explicit idempotent replay/dismiss/sender-delete/eligible-cancel commands, with opaque stable cursors, accepted-target isolation, uniform missing behavior, no read-triggered playback and no raw internal identifiers.
2026-07-16 accepted on engineering head 3dbf474 and merged by PR #128 as dbd6baa. Added exact-binding actor-scoped inbox list/detail, digest-only ic_/rc_ cursors (24h, 128 per actor), safe receipt pages, uniform nonexistence, idempotent dismiss and explicit inbox replay to the same current Pulsar with content-policy enforcement and lineage. History read/delete/report boundaries redact raw IDs. Local targeted tests, vet, race, previous-head, Xcode Swift and Windows acceptance passed; hosted run 29461136915 passed all four jobs. The known local system ffmpeg/libvorbis limitation is not claimed as an app failure; hosted Linux coordinator was green. No real-hardware result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-sequence.puml](file://TASK-260712-2j5fkr/p2-targets-inbox-parity-sequence.puml) — API flow for explicit target miss inbox and replay
- [p2-inbox-history-api-pagination.md](file://TASK-260712-2j5fkr/p2-inbox-history-api-pagination.md) — Engineering acceptance and API behavior summary
- [targets-inbox-contract-v1.json](file://TASK-260712-2j5fkr/targets-inbox-contract-v1.json) — Validated targets and inbox contract
- [swift-acceptance-manifest.json](file://TASK-260712-2j5fkr/swift-acceptance-manifest.json) — Local Xcode Swift acceptance manifest
- [windows-acceptance-manifest.json](file://TASK-260712-2j5fkr/windows-acceptance-manifest.json) — Local Windows acceptance manifest
