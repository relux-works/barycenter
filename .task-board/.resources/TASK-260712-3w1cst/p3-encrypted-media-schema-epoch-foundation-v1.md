# P3 encrypted-media schema and epoch foundation v1

Task: `TASK-260712-3w1cst`

Status: additive dormant foundation; production E2EE remains disabled

## Outcome

The coordinator now has an additive SQLite persistence boundary for public
device state, public group events and epochs, encrypted protected-object
metadata, replay state, history grants, recovery/device-transfer ciphertext,
consented report-evidence metadata and immutable audit. No route, worker,
capability advertisement, production suite, media container or cryptographic
implementation consumes these repositories.

`e2ee_feature_state` is physically locked by SQLite checks to `enabled = 0`,
an empty suite and an empty container. Table existence therefore cannot enable
the capability. Any future activation requires a versioned migration and a new
independent review.

## Storage boundary

The schema has no column for a device private key, key-package private
material, epoch/session/content key, sender key, history/recovery secret,
protected plaintext, user filename/title/caption, decoded sample, waveform or
unconsented decrypted evidence. Public packages and public group events are
bounded blobs with SHA-256 pins. Protected manifests, recipient envelopes,
grants and transfer packages are explicitly client-produced opaque ciphertext
with bounded sizes and immutable digests/references.

Legacy `media` and `media_items` remain unchanged and may continue to contain
coordinator-readable data only for explicitly non-E2EE paths. Protected
objects live in a separate table and never inherit a legacy plaintext row.
Old coordinators ignore the additive tables, while current coordinators retain
the E2EE rows after a rollback-era legacy write.

Report rows contain only consent/version digests, an authenticated evidence
digest, a bounded encrypted evidence reference and retention metadata. There
is no server decrypt or plaintext evidence column.

## Conditional state machines

- A verified public device is required before a group or signed public event
  can be persisted.
- A verified commit advances exactly one epoch from the exact previous commit
  digest. SQLite's immediate writer transaction and revision predicate give a
  single winner. A stale concurrent loser is rejected; an exact-current
  competing predecessor records `forked` and freezes protected-object writes.
- Protected objects transition `staged -> ready -> revoked` with exact revision
  predicates. Payload columns are immutable by trigger; duplicate finalize and
  revoke attempts fail closed.
- Replay state stores event and nonce digests plus the last accepted
  device/object generation and sequence. Regression, nonce reuse, generation
  gaps and a known next generation not beginning at sequence one are rejected,
  including after restart.
- History grants and transfer packages are bound to current group/target/epoch
  state. Grant revocation is exact-revision single-winner.
- Every accepted/rejected/revoked transition writes an append-only audit row;
  update and delete triggers protect the audit ledger.

## Independent-review delta

The implementation also addresses IDR-001 through IDR-003 from
`TASK-260712-aniuyy`:

- the authority now pins canonical multi-fault failure precedence and all
  three platform models execute the invalid-signature-plus-tampered-manifest
  vector;
- shared sequence-regression, generation-reset and valid next-generation
  vectors execute on coordinator, Windows and macOS, and Windows now retains
  monotonic sequence state;
- coordinator-visible commit, proposal, welcome, key-package and history-grant
  envelopes each use strict unknown-field and forbidden-secret rejection.

These are protocol-affecting changes, so the prior independent verdict is not
reused. The task cannot be accepted until a new Claude Fable 5 max delta review
reproduces the exact packet hashes and returns a terminal verdict.

## Automated evidence and non-claims

Go tests cover fresh schema, migration failure rollback, generation-skipping
roll-forward, legacy rollback writes, concurrent commit/revoke, restart replay
state, immutable payload/audit triggers, foreign keys and on-disk sentinel
scans. Coordinator, Windows and macOS consume the extended shared vectors.
The machine-readable evidence packet is
`acceptance/phase3/e2ee-schema-epoch-foundation-v1.json`.

No signed app, physical device, real cryptography, cross-platform KAT,
hardware, acoustic, recovery drill or production security claim is made.
EPC-001, EPC-002, EPC-004 and EPC-005 remain open. Manual evidence stays in
`EPIC-260714-th54l3`; external implementation review stays in
`TASK-260712-1ulshp`.
