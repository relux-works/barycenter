# Phase 1 transmission store and target snapshots

- Date: 2026-07-14
- Task: `TASK-260712-1aprcb`
- Contract: [`p1-transmission-contract-v1.md`](p1-transmission-contract-v1.md)
- Scope: engineering and automated verification only; real-app and physical-
  device evidence remains in manual epic `EPIC-260714-th54l3`.

## Durable model

Startup installs five additive data sets after generic media migration:

- `transmissions` stores the trusted source, immutable media/audience/domain
  decision, requested and effective delivery, downgrade reason,
  `(accepted_at, id)` FIFO key, delivery expiry and aggregate lifecycle;
- `transmission_targets` stores exact orbit, actor, slot and
  `slot_paired_at` binding generation for online and offline recipients,
  capability decisions, generation-safe receipts, reasons and timestamps;
- `blocks` stores personal actor and recipient-orbit blocks with explicit
  ownership, author, revision and revocation time;
- `node_dnd_settings` stores exact-binding local DND with monotonic client
  revision; and
- `orbit_dnd_settings` stores primary-owned orbit DND with revision and author.

Acceptance fields and target identity/capability fields are protected by
SQLite triggers. Repository updates may change only lifecycle status, receipt
generation/revision, reasons and timestamps. Internal references use foreign
keys, while legacy orbit/actor tables deliberately remain outside the new FK
graph so the pinned previous coordinator can still open and mutate the same
database during rollback.

The repository validates ready, unexpired, source-orbit-owned clip media and a
live non-satellite source before acceptance. Every target binding is resolved
again in the same immediate writer transaction. Overlay and interrupt expiry
is derived as `min(media.expires_at, accepted_at + 5 minutes)`; direct
`after_current` uses the media expiry. Aggregate state is recomputed
transactionally from target rows with the frozen precedence and reason map.

## ACL boundary

`Store.AllowsMediaDownload` is the production implementation of
`MediaTargetSnapshotReader`. It grants only when all of the following match:

- media ID on a persisted transmission;
- exact target orbit, actor and slot;
- the still-live `slot_paired_at` binding generation; and
- a target status/reason that is not blocked, with no active recipient block
  against the source actor or source orbit.

The query never consults current approach or orbit membership. A later join,
copied media ID or replacement slot cannot gain access; breaking an approach
does not rewrite an already accepted row. The existing media service still
re-resolves the bearer, exact persisted row, active block and ready/unexpired
media state before opening bytes and again inside the immediate descriptor-open
transaction under the canonical-file lock. The coordinator now installs the
store reader during production download-service construction.

## Repository seams

The store exposes atomic create/read/FIFO operations, compare-and-swap target
transitions, receipt-generation advancement, transmission-level cancellation
or expiry causes, exact block lookup/mutation and effective layered DND lookup.
Duplicate receipts and policy revisions are idempotent only when their content
is identical; stale or conflicting revisions fail closed.

The current task does not expose transmission HTTP routes, resolve caller
audiences, consume WebSocket receipts or schedule playback. Those remain with
`TASK-260712-2qpp6w`, `TASK-260712-1g70av` and `TASK-260712-31vvjt` and must use
these repository seams rather than direct SQL or a membership-derived ACL.

## Automated evidence

Unit and integration coverage proves:

- atomic fresh schema installation and foreign-key health;
- immutable online and offline snapshots plus accepted-time terminal receipts;
- FIFO ordering by trusted time and ULID, generation/revision CAS and
  concurrent terminal-receipt serialization;
- deterministic played, failed and cancelled aggregate outcomes;
- actor/orbit block ownership, revocation and node/orbit DND revision rules;
- production HTTP download through persisted rows, uniform denial for copied
  IDs and non-targets, approach-split stability, replacement-binding denial and
  block-vs-descriptor-open race closure; and
- exact rollback through merge `2aa97c2d08cb93b110200ae159fd43265410ff5a`,
  previous-binary legacy/session mutation and source-orbit dissolution,
  roll-forward transmission/target preservation, media revocation and final
  `foreign_key_check`.

The contract delta review also corrected the newly written logical `md_`
examples to the already shipped and tested `m_<ULID>` media identifier. No
production identifier migration was introduced.
