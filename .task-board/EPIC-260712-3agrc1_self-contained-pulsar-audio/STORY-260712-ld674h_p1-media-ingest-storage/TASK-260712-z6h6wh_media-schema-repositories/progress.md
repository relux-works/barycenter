## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T01:33:45Z

## Blocked By
- (none)

## Blocks
- TASK-260712-2af2dp
- TASK-260712-1bnos4
- TASK-260712-gj0cko
- TASK-260712-3mcof4
- TASK-260712-1sae4q
- TASK-260712-1aprcb
- TASK-260712-1n5fks

## Checklist
- [x] Add additive schema and repository methods for media items, upload sessions and audit metadata
- [x] Preserve legacy media reads and compatibility WAV mapping during rollout
- [x] Cover fresh DB, migrated DB and rollback behavior with migration tests
- [x] Use conditional state transitions and server-generated storage keys to defeat stale workers and user paths

## Notes
Strict inline engineering execution started 2026-07-14 from merged main 06a06c099ed5b4f37f5e2dd3648772ffd041dfd9 on branch task/task-260712-z6h6wh-media-schema-repositories. Manual real-app acceptance is tracked separately in EPIC-260714-th54l3; this task retains coding, migrations, repositories and automatable tests only.
Local implementation gate: additive five-table schema; media/upload/storage CAS repositories; legacy WAV bridge; atomic orbit revocation; exact predecessor 06a06c rollback and roll-forward cleanup. Green: coordinator go vet ./..., go test ./..., focused race, full tagged previous-head matrix, coordinator build, pulsar-win vet/tests, task-board validate and git diff check. Local node-app swift test is environment-limited because CommandLineTools-only Swift 6.2.3 cannot import the existing Testing module; hosted macos-15 CI remains authoritative. No physical/manual claim.
Commit 23ef4f1 pushed; draft PR #11 opened: https://github.com/relux-works/barycenter/pull/11. Awaiting hosted CI and final root review before acceptance/merge.
Root review R1 closed before acceptance: publication metadata now rejects header-shaped/uppercase MIME, unsafe codec names, non-object or oversized loudness JSON; malformed scoped tokens are rejected before database lookup. Focused store tests and vet are green on the remediated bytes.
Acceptance candidate on commit ecc034b4809f2e7857d30d958539c8be50c99b94: hosted CI run 29298686287 passed coordinator (including exact predecessor), node-core, pulsar-win and signed packaged-probe jobs. Full local coordinator race matrix also passed. Root review found only R1, fixed in ecc034b; no unresolved findings. Marked done on the merge-bound branch; acceptance becomes durable when PR #11 lands on main.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-component.puml](file://TASK-260712-z6h6wh/p1-media-ingest-component.puml) — Schema and repository context for generic ingest persistence
- [p1-media-ingest-persistence-contract.md](file://TASK-260712-z6h6wh/p1-media-ingest-persistence-contract.md) — Additive schema, CAS lifecycle, outbox recovery, migration and exact-predecessor rollback contract
