# Independent delta review: E2EE schema and epoch foundation (TASK-260712-3w1cst)

Reviewer: independent persistence/security/migration/protocol-delta reviewer (read-only; no production or test code modified).
Date: 2026-07-19. Verdict: **APPROVE**.

## Reviewed state

- Commit: `b11377ec22e85a95bc0ad17afc8c7c8d79340cda` (`feat(coordinator): add dormant e2ee epoch store`), branch `feat/task-260712-3w1cst`; working tree clean except pre-existing `.task-board` progress files.
- All 13 SHA-256 pins in `acceptance/phase3/e2ee-schema-epoch-foundation-v1.json` independently recomputed from the working tree: **all match** (schema, repository, tests, startup, protocol, vectors, coordinator/windows/macos models + tests, adr).
- Board packet copy `.task-board/.resources/TASK-260712-3w1cst/e2ee-schema-epoch-foundation-v1.json` is byte-identical to the repo packet.
- `acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json` was consistently re-pinned to the new protocol/vector/model hashes; its validator now pins the exact failure precedence, envelope inventory, dual-fault vector, and replay-state vectors, so future drift fails closed.

## Independent reproduction (fresh runs, this session)

| Check | Command | Result |
|---|---|---|
| Store focused | `go test ./internal/store -run '^TestE2EE' -count=1 -v` | 5/5 PASS (0.9s) |
| Coordinator full | `go test ./...` (coordinator) | all packages ok (store 49.6s) |
| Race | `go test -race ./internal/store ./internal/e2eecontract` | ok (store 480.450s, e2eecontract 1.441s) |
| Windows full | `go test ./...` (pulsar-win) | all packages ok (root 15.8s) |
| macOS filtered | `swift test --filter E2EEAuditContractTests` | 3/3 pass |
| macOS full | `swift test` (node-app) | 308 tests / 52 suites pass |
| Acceptance | `python3 -m unittest scripts.acceptance.test_e2ee_schema_epoch_foundation scripts.acceptance.test_e2ee_protocol_key_lifecycle` | 6/6 OK |
| Schema adversarial probe | DDL extracted from `e2ee_schema.go`, executed in scratch SQLite (outside repo) | 10/10 constraints held |

Producer numbers (store race 475.523s, macOS 308) are consistent with my reproduction.

The scratch-DB probe adversarially confirmed, independent of the Go repository layer: `enabled=1` / non-empty suite / second feature row rejected by CHECK; group insert without matching `airs` row rejected by FK; second **accepted** commit for the same (group, epoch) rejected by the partial unique index while rejected-state rows remain insertable; `ready` without `finalized_at` rejected; payload and epoch mutation rejected by the immutability trigger; legal status-transition update allowed; audit UPDATE/DELETE rejected; nonce reuse and event-id replay rejected by UNIQUE; path-traversal `ciphertext_ref` rejected by prefix/length CHECK.

## Scope inspected (source-level)

- **Schema** (`coordinator/internal/store/e2ee_schema.go`): all 11 `e2ee_*` tables from the packet, digest/length/prefix CHECKs, state-timestamp consistency CHECKs, one-accepted-commit-per-epoch partial unique index, payload-immutability and audit no-update/no-delete triggers, transactional DDL with `foreign_key_check` and the `e2ee_ddl_before_commit` fault seam. `e2ee_feature_state` is physically locked (`enabled = 0`, empty suite/container) — activation requires a future versioned migration.
- **Startup order** (`store.go:178`): E2EE init runs after every schema it references (actors, airs) and before reconciliation sweeps, on the single production open path; FK pragma ON in DSN and per-connection.
- **Repository** (`e2ee.go`): every write is a transaction with `defer Rollback`; conditional transitions use exact-revision/exact-state predicates with `RowsAffected == 1` single-winner checks; commits require exact previous epoch + previous commit digest with fork persisted only for an exact-current competing predecessor; replay/nonce enforced both in-tx and by UNIQUE constraints; sender generation/sequence monotonic with next-generation-starts-at-one; grants/transfers/reports bound to verified devices, clean fork state, current epoch/target; audit rows written in the same transaction (including audited commit rejections); `ciphertext_ref` must equal `ciphertext/v1/<digest>`.
- **No-secret storage**: no schema column for any `forbiddenStorageFields` name (validator parses SQL declarations; test scans PRAGMA table_info); all payloads are bounded opaque blobs bound to SHA-256 digests; audit stores ids/digests/reasons only; on-disk db/-wal/-shm sentinel sweep after WAL truncate.
- **Dormancy**: no non-test import of `e2eecontract`; no runtime read of `e2ee_*` tables; `phase3_observability_http.go` hardcodes `E2EEMediaEnabled: false` (pre-existing); capability string appears only in the three dormant models, protocol docs, and the spike probe; validator enforces six production sources clean and no `protocol/golden/*e2ee*` wire fixtures.
- **Legacy compatibility**: `media`/`media_items` untouched by the delta; faulted-migration test proves transactional rollback leaves zero `e2ee_*` tables; generation-skip roll-forward and rollback-era legacy writes preserved; test asserts no protected-object/legacy-media join.

## IDR delta verification

- **IDR-001**: `failurePrecedence` pinned in `protocol/e2ee-media-audit-v1.json`; dual-fault vector `invalid-signature-precedes-tampered-manifest` added and consumed by coordinator, Windows, and macOS suites; macOS/Windows now check `invalid_signature` before `tampered_manifest`. Side-by-side source read confirms the three `accept` implementations are check-for-check identical, as are the three `applyCommit` implementations.
- **IDR-002**: Windows model now retains `lastSequences` with regression and generation-reset rules identical to coordinator/macOS; three shared `replayStateVectors` (sequence-regression, generation-reset-must-start-at-one, next-generation-starts-at-one) run on all three platforms; the store enforces the same rules durably (`AcceptE2EEReplayState`) including across restart; `stateRules` amended accordingly.
- **IDR-003**: strict coordinator decoders for commit, proposal, welcome, key-package, and history-grant envelopes with forbidden-field and unknown-field rejection (`decodeCoordinatorEnvelope`), each exercised per-envelope in `TestEveryCoordinatorPublicEnvelopeRejectsSecretsAndUnknownFields`; authority now enumerates `coordinatorEnvelopeFields` for all four non-commit kinds (commit fields pre-existing).

## Findings by severity

**Critical: none. High: none. Medium: none.**

- **L1 (Low, doc/vector nuance)** — The pinned `failurePrecedence` linearization is not exhaustively realized in two multi-fault corners, though all three platforms are identical so no cross-platform divergence exists: (a) the commit path checks `replay` before `stale_epoch` and defers residual field-emptiness `malformed` checks until after fork checks (contract.go `ApplyCommit`, pulsar-win `applyCommit`, macOS `apply`); (b) the content path checks sequence-based `replay` after `expired_grant` (event-replay is in pinned position). Every branch fails closed. Owner: protocol/vectors authority alongside platform key-state tasks (e.g. TASK-260712-25dzp4). Retest: add commit-path and expired+sequence multi-fault shared vectors, or scope the linearization note to content acceptance. Non-blocking for a dormant foundation; accepted as tracked here.
- **I1 (Info)** — The backup sentinel scan is a tripwire, not a leak proof: sentinel strings are never introduced through any code path, so their absence is trivially satisfied. The load-bearing guarantees (forbidden-column scan, strict decoders, bounded digest-bound blobs, CHECKs) are all present and verified. Future hardening: route sentinel bytes through the opaque-blob params and assert they appear only in their declared columns.
- **I2 (Info)** — Fork-state freezing of protected writes is enforced in repository code (`e2ee.go:405-410, 549-553, 649, 761-762`) but no test stages an object/replay/grant/transfer against a persisted `forked` group. Verified by source inspection; suggest a small test in the next store-touching task.
- **I3 (Info)** — `AcceptE2EEReplayState` rejections roll back without an audit row, while commit rejections are audited and committed. No state changes on rejection, so nothing is lost at the dormant stage; normalize rejected-transition auditing when runtime wiring lands.
- **I4 (Info)** — `encrypted_evidence_ref` is prefix/length-bounded but not bound to `authenticated_evidence_digest` (contrast `ciphertext_ref = 'ciphertext/v1/' + digest` enforced in the repository). Bind it when the evidence storage layout is defined.

## Posture

The exact no-claim posture holds: `manualEvidence` all `not-run`/`not-run-no-selected-stack`; `openProductionGates` = {EPC-001, EPC-002, EPC-004, EPC-005, TASK-260712-1ulshp}; production remains physically disabled and unadvertised; the acceptance unittest proves the validator fails closed if `productionEnabled` or manual evidence is flipped. Nothing in this review accepts or implies acceptance of the production E2EE capability; any protocol-affecting change after `b11377ec` requires another delta review.

## Verdict

**APPROVE** — zero open Critical/High implementation or delta-design findings. Low/informational findings L1, I1–I4 are explicitly tracked above with owners and retests. Task set to `done`; reviewer checklist items checked.
