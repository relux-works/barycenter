# P1 media ingest persistence contract

Date: 2026-07-14
Task: `TASK-260712-z6h6wh`
Scope: additive SQLite schema, repository state machines, migration and rollback
compatibility. HTTP upload authorization, byte transfer, probing/normalization,
download ACL, retention workers and Telegram routing remain downstream tasks.

## Durable model

| Table | Responsibility | Compatibility rule |
| --- | --- | --- |
| `media_items` | Canonical owner, actor, metadata, lifecycle and CAS revision | Does not replace or mutate the legacy `media` shape |
| `media_upload_sessions` | Hashed scoped token, hashed idempotency key, monotonic offset and finalize state | Plaintext token and idempotency key are never persisted |
| `media_storage_operations` | Recoverable publish/cleanup outbox with independent CAS revision | File work is acknowledged only after a durable SQLite transition |
| `media_legacy_wav_links` | Explicit bridge from a generic item to the existing WAV row | Legacy WAV path remains readable by the previous API |
| `media_ingest_audit_events` | Metadata-only lifecycle audit | No token, idempotency key, local path or arbitrary error text |

New media IDs, upload-session IDs, storage-operation IDs and storage keys are
generated inside the repository. A storage key has the opaque form
`media/v1/<64 lowercase hex>`; no repository request accepts a storage-key or
filesystem-path field.

## State ownership

Media lifecycle:

```text
processing --publish CAS--> ready --delete/expiry CAS--> deleted|expired
     |
     +--failure CAS--------> failed --delete/expiry CAS--> deleted|expired
```

Upload lifecycle:

```text
open --monotonic offset--> open --complete-size + CAS--> finalizing
     --expiry/failure-----------------------------------> expired|failed
finalizing --atomic publication receipt---------------> completed
```

Every worker mutation carries the last observed revision. A revision mismatch,
duplicate finalize, duplicate publication receipt or stale upload offset
returns `ErrMediaStateConflict` and leaves the committed row unchanged.

## Publication and cleanup recovery

1. `StageMediaPublication` advances the processing item revision and records a
   pending publish operation with a repository-generated destination key.
2. A storage worker may atomically rename prepared bytes to that key.
3. `CompleteMediaPublication` atomically marks the item ready, completes the
   upload session, closes the outbox row and writes an audit event.
4. If step 3 is interrupted, the item exposes no key and the pending operation
   is returned by `PendingMediaStorageOperations` for retry.
5. Delete, expiry, failure or orbit dissolution cancels any pending publish,
   removes the key from the visible item and records an idempotent cleanup row.
   A stale publisher can no longer transition the tombstoned item to ready.
6. A cleanup receipt is also CAS-protected. An interrupted receipt leaves the
   row pending so a later worker can safely repeat physical deletion.

SQLite does not pretend to make a filesystem rename transactional. The outbox
is the recovery contract across that boundary.

## Migration and rollback contract

- Schema installation is a single immediate transaction. A late DDL fault
  leaves none of the five additive tables partially installed.
- Existing `media`, `settings`, `orbits`, `members`, `slots` and identity rows
  are not rebuilt or dropped.
- Generic writes verify a live `(orbit, actor, membership)` inside the same
  writer transaction. External foreign keys are intentionally not added to
  legacy authority tables because an older coordinator must be able to mutate
  them while it ignores the new tables.
- Current orbit dissolution revokes owned generic media in the same SQLite
  transaction. If the exact predecessor dissolves an orbit during rollback,
  the next roll-forward marks its orphaned media deleted and queues canonical
  cleanup before serving callers.
- The tagged compatibility test extracts exact predecessor commit
  `06a06c099ed5b4f37f5e2dd3648772ffd041dfd9`, injects a test using that
  revision's real Store API, and proves legacy media, pairing tokens and
  session snapshots remain readable and mutable. Roll-forward then proves the
  generic upload survived and the predecessor-dissolved media was revoked.

Operational rollback is therefore additive: deploy the pinned predecessor and
leave unknown tables in place. Do not drop or rewrite the generic tables. A
later roll-forward consumes their original revisions and outbox state.

## Automated acceptance evidence

Local commands run from `coordinator/`:

```text
go vet ./...
go test ./...
go test -race ./internal/store -run '^TestMediaIngest|^TestMediaItemForLegacyWAV' -count=1
go test -tags previoushead -count=1 ./internal/store -run '^(TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition|ConfigBootstrapContract)|TestMediaIngestExactPreviousHeadRollback)$'
```

Covered cases include fresh and migrated databases, exact-old/new rollback,
legacy WAV mapping, pairing/session preservation, idempotency races, concurrent
offset races, stale/duplicate finalize, publication and cleanup fault
injection, conditional failed/deleted/expired transitions, immediate and
predecessor orbit dissolution, SQLite artifact scans for plaintext session
tokens/idempotency keys, and rejection of caller-shaped storage paths.

Hosted CI run `29298686287` passed the remediated commit `ecc034b` in all four
jobs: coordinator vet/tests plus exact-predecessor compatibility, node-core on
`macos-15`, portable Windows tests/cross-build, and the signed packaged-probe
contract. Root review R1 on those bytes added canonical MIME/codec validation,
bounded object-shaped loudness JSON, and malformed scoped-token rejection.

No result in this document claims a real-app, production, Store, beta or
physical-hardware pass. Those checks belong to `EPIC-260714-th54l3`.
