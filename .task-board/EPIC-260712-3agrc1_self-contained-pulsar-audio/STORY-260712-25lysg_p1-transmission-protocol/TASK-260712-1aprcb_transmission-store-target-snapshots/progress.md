## Status
to-review

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T07:58:20Z

## Blocked By
- TASK-260712-51y5k9
- TASK-260712-3mcof4
- TASK-260712-z6h6wh

## Blocks
- TASK-260712-2qpp6w
- TASK-260712-31vvjt
- TASK-260712-2qc27p
- TASK-260712-2hcq1g
- TASK-260712-3lf8r0
- TASK-260712-2h6snp
- TASK-260712-2bk0vy

## Checklist
- [x] Add additive schema for transmissions, target snapshots, receipts, and block data
- [x] Persist per-target state transitions and ordering fields
- [x] Replace media GET auth with transmission target ACL and cover migration plus rollback tests
- [x] Persist expiry, effective delivery and immutable offline plus online target snapshots
- [x] Integrate the generic ACL service without authorizing from current membership

## Notes
Strict sequential handoff after accepted TASK-260712-51y5k9. Start only from merged PR #20/main; implement the frozen p1-transmission-v1 persistence and immutable ACL contract. Manual real-app and physical-hardware evidence remains in EPIC-260714-th54l3.
Execution branch task/task-260712-1aprcb-transmission-store-target-snapshots started from clean main merge 2aa97c2d08cb93b110200ae159fd43265410ff5a after PR #20 and both hosted CI runs passed. Frozen input is docs/analysis/p1-transmission-contract-v1.md. Work remains inline and strict-sequential outside task-board spawn.
Initial code-to-contract audit found and corrected one upstream documentation mismatch: existing generic media IDs are m_<ULID>, not the logical md_ example. The contract resource and its guard now pin the shipped identity; no production ID migration is introduced.
Implemented additive transmission/target/block/DND schema; immutable accepted snapshots; trusted FIFO/expiry/effective-delivery persistence; generation-safe CAS receipts and deterministic aggregate states; active-block-aware Store target ACL; and production media download wiring. Added atomic schema, concurrency, policy, HTTP ACL, binding replacement, approach split and exact previous-head rollback coverage. Local go vet, all coordinator tests, race detector, full pinned previous-head CI selection, diff check and board validation pass. Outcome: docs/analysis/p1-transmission-store-target-snapshots.md. No real-app or physical-hardware evidence is claimed; that remains in EPIC-260714-th54l3.
Self-review delta closed a block/terminal-receipt vs descriptor-open authorization race: the production Store reader now rechecks the exact persisted target and active block inside both immediate media authorization transactions, including the transaction that acquires the canonical descriptor. Added a 20x repeated race regression. Strengthened exact-previous-head rollback to dissolve a source orbit and prove transmission/target history survives while media is revoked.

## Precondition Resources
- [p1-transmission-protocol-components.puml](file://TASK-260712-1aprcb/p1-transmission-protocol-components.puml) — Store and ACL context for transmission persistence

## Outcome Resources
- [TASK-260712-1aprcb_transmission-store-target-snapshots.md](file://TASK-260712-1aprcb/TASK-260712-1aprcb_transmission-store-target-snapshots.md) — Implemented schema, repository, immutable ACL, policy hooks, automated evidence, and rollback handoff
