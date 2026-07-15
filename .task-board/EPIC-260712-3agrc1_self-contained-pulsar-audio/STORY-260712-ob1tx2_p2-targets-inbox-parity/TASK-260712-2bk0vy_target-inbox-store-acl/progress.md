## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:51Z

## Last Update
2026-07-15T22:56:06Z

## Blocked By
- TASK-260712-2rlkp7
- TASK-260712-1aprcb

## Blocks
- TASK-260712-2j5fkr
- TASK-260712-2zoy4u
- TASK-260712-1vklop
- TASK-260712-3lf8r0
- TASK-260712-2h6snp

## Checklist
- [x] Add additive schema for target snapshots inbox rows and receipt pagination
- [x] Persist replay lineage expiry and revocation state
- [x] Replace membership based media auth with snapshot based authorization
- [x] Cover migration and rollback safety
- [x] Extend existing target rows and indexes rather than replacing the Phase 1 snapshot schema
- [x] Guarantee exactly one eligible inbox item per missed target and no old-item inheritance by new members

## Notes
2026-07-15 strict-sequence start from synchronized main 1b25abb. Implementing inline outside task-board spawn workflow. Extending existing Phase 1 transmission_targets and history/media ACL with additive inbox, lineage, expiry and revocation persistence under p2-targets-inbox-parity.v1. No parallel membership ACL/history/moderation model and no real-hardware claim.
2026-07-16 accepted from merged PR #124 (main 80e892b; engineering commit a2814c8). Implemented additive capability/resolution snapshots, exactly-once eligible inbox projection, exact-binding keyset reads, replay lineage, expiry and canonical media/moderation revocation without a membership ACL. Local: store and coordinator HTTP suites green; targeted race green; exact previous-head rollback green; go vet green; p2 contract validator and 12 acceptance tests green; Windows Go suite green. Full local coordinator matrix had only environment-only ffmpeg libvorbis fixture failures; local Swift toolchain lacked the Testing module. Hosted run 29456807669 resolved both environments and passed coordinator 2m17s, Swift 1m13s, Windows 1m45s and packaged MSIX 2m31s. No real-hardware claim.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-components.puml](file://TASK-260712-2bk0vy/p2-targets-inbox-parity-components.puml) — Exact target snapshot, inbox projection, replay lineage and canonical revocation architecture
- [p2-target-inbox-store-acl.md](file://TASK-260712-2bk0vy/p2-target-inbox-store-acl.md) — Implementation, migration, ACL and verification handoff
- [hosted-coordinator-manifest.json](file://TASK-260712-2bk0vy/hosted-coordinator-manifest.json) — Hosted run 29456807669 coordinator and rollback evidence
- [hosted-swift-manifest.json](file://TASK-260712-2bk0vy/hosted-swift-manifest.json) — Hosted run 29456807669 full-Xcode Swift evidence
- [hosted-windows-manifest.json](file://TASK-260712-2bk0vy/hosted-windows-manifest.json) — Hosted run 29456807669 Windows portable, race and cross-build evidence
