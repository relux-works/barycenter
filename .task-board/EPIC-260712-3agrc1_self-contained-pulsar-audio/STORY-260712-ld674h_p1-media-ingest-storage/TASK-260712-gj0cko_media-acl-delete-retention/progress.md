## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T06:05:45Z

## Blocked By
- TASK-260712-z6h6wh
- TASK-260712-2af2dp
- TASK-260712-1bnos4
- TASK-260712-3mcof4
- TASK-260712-1sae4q

## Blocks
- TASK-260712-3huupe
- TASK-260712-3e4p0c
- TASK-260712-2kec2s
- TASK-260712-3lf8r0
- TASK-260712-2zoy4u

## Checklist
- [x] Implement authorized GET and early DELETE lifecycle for generic media
- [x] Sweep failed, expired and deleted bytes according to phase-one retention rules
- [x] Cover cross-orbit negative access, delete revocation and expiry behavior with tests

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge 0d6863c462111da6ed27f851a636e40d95100d73. Scope is the integration delta across already-landed generic GET ACL, DELETE cancellation, lifecycle cleanup, Telegram legacy mapping and mixed-rollout behavior; manual real-app and hardware evidence remain in the separate manual-test epic.
Strict inline implementation committed as 608aef9 and published in draft PR #17. Generic authority now revokes linked legacy reads atomically, durable cleanup owns canonical and Telegram compatibility bytes, the current serial scheduler consumes delete and expiry cancellations idempotently, and macOS and Windows reject stale voice download or pause work after cancellation. Local coordinator vet, test, race, exact pinned predecessor suite, pulsar-win vet, test, race, Windows amd64 cross-build, and node-app swift build are green. Full Swift tests await hosted macOS CI; no manual real-app or hardware result is claimed (EPIC-260714-th54l3).
Hosted CI run 29309915183 passed coordinator including exact predecessor rollback, authoritative macOS Swift tests, portable Windows and signed packaged probe. Inline root delta-review closed linked legacy-sweeper bypass, pinned canonical and Telegram cleanup roots, stale voice-download resurrection and late pause versus next-load races. Final tracking CI and merge remain pending.
Final tracking CI run 29310143986 passed all four hosted jobs. PR #17 merged into main as 9f2aea8e5b9200d1e4077a5576dde18f8051bba5 on 2026-07-14. Automated evidence is accepted; no manual real-app or real-hardware result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-component.puml](file://TASK-260712-gj0cko/p1-media-ingest-component.puml) — Integrated generic ACL, delete, lifecycle cancellation, legacy cleanup receipt and current scheduler boundary
