# TASK-260715-unbb7c — Independent Phase 1 migration review verdict

- Date: 2026-07-19
- Decision: **CHANGES REQUESTED** — approval of `TASK-260712-1xkn75` is
  **withheld at the reviewed head** because this review found one new HIGH
  regression (P1-MIG-003, post-audit, detailed below) against the audited
  rollback contract. The frozen audit packet itself and both corrective HIGH
  fixes are **validated**; P1-MIG-001 and P1-MIG-002 **remain closed**.

## Reviewer identity and independence

- Reviewer: Claude Fable 5 (`claude-fable-5`), task-board tracked reviewer
  spawn run `RUN-260719-d82ed0` on branch `review/task-260715-unbb7c-fable5`.
- Owner authorization: task notes record Ivan Oparin's default (select a
  technically qualified non-implementing migration reviewer for merge
  `d7e0065` and PR #72) and the explicit 2026-07-19 authorization of a
  task-board independent review using Claude Fable 5 at maximum effort against
  exact revision `06ce330f…`, with "do not implement reviewed migrations; do
  not alter production data".
- Independence: this session implemented none of the reviewed migrations. The
  audited implementation, the corrective commit `7736b75` (2026-07-15) and all
  post-audit store commits (2026-07-12..17) were produced by earlier inline
  execution sessions; this review session began 2026-07-19 and made zero
  modifications outside `.task-board` tracking (verified via `git status`
  before verdict: only board progress files touched by the task-board CLI).
- The frozen inline audit
  (`docs/analysis/p1-independent-migration-technical-audit.md`) explicitly
  disclaims reviewer independence; it was consumed as a technical packet only.
  The board resource copy is byte-identical to the repo copy (diff-verified).

## Reviewed revision

- Reviewed head: `06ce330fe8eae9036821c3632afdb432e024fadc` = `origin/main`
  head at review time and the exact revision pinned by the owner note
  (a later exact main head containing `d7e0065`, as the AC permits).
- Contains audit merge `d7e0065` (PR #72 = exactly one corrective commit
  `7736b7546b7ef86347d86dfefb095c9f795ad9ff` "fix(store): make legacy
  migrations atomic" over frozen review base `635a8d3e`).
- Working tree matched the reviewed head bit-for-bit for all code paths.

## Method

1. Re-derived the rollback contract from the audit's source inventory and
   inspected every layer at the reviewed head: legacy (`store.go`), orbit
   (`orbits.go`, `links.go`), identity (`identity_schema.go`, `identity.go`),
   generic media (`media_ingest_schema.go`, `media_ingest.go`), transmission
   (`transmission_schema.go`), moderation (`moderation_schema.go`).
2. Inspected the full PR #72 corrective diff and each closed HIGH finding.
3. Diffed every audited file `d7e0065..06ce330` and classified each hunk
   (additive Phase 2/3 vs change to Phase-1-audited behavior).
4. Reran failure, partial, concurrent, exact-predecessor and full-module race
   suites at the reviewed head (all local, darwin/amd64, go1.26.0), and
   consumed hosted CI at the exact head.
5. Inspected backup/restore prerequisites in-repo only; no production system,
   host, or data was accessed or altered.

## Audit packet validation (layer inspection)

- **Legacy** — `initLegacySchema` (store.go:232): legacy DDL plus the one
  additive `media.orbit_id` column in a single immediate transaction;
  existence-checked ALTER; every error reaches startup; checkpoint seam at the
  pre-commit boundary. Conforms.
- **Orbit/links** — `initOrbits` (orbits.go:132): orbit + links schema and
  both column additions (`slots.provider`, `members.display_name`) in one
  immediate transaction with existence checks; deliberately defers the global
  FK check to identity (documented, correct — an earlier check would reject
  the supported predecessor-dissolution state). Unchanged since `d7e0065`.
- **Identity** — additive DDL in one immediate transaction; bounded
  `cleanupMissingOrbitIdentityTx` (only children of orbits that no longer
  exist) runs before `ensureOrbitStatusConstraint` and the global
  `assertForeignKeys` + `foreign_key_check`, exactly as the audit asserts;
  the orbits status rebuild is transactional with FK-off/FK-restore
  verification, sequence preservation, in-tx FK check and behavior probe, and
  panic-safe cleanup. Conforms.
- **Generic media** — `initMediaIngestSchema` DDL group in one immediate
  transaction with guarded ALTERs and in-tx FK check; reconciliation split
  into restart-safe idempotent transactions; legacy writes bounded through
  `media_legacy_wav_links`; cleanup receipts reopened before destructive work.
  Conforms at DDL level; see P1-MIG-003 for a post-audit reconciliation
  ordering regression.
- **Transmission** — single immediate DDL transaction; guarded ALTERs;
  immutability triggers preserved/widened; scheduler companion backfill sets
  only `updated_at = accepted_at` and invents no barrier or start time; new
  `capability_set_hash`/`resolved_at_ms` backfill mutates only the two new
  columns, guarded `resolved_at_ms = 0`, using immutable `accepted_at`.
  Conforms.
- **Moderation** — additive DDL transaction with FK check; the one legacy
  report target constraint (`CHECK(target_actor_id = reporter_actor_id)`)
  rebuild is guard-probed, fully transactional (scratch table, copy of every
  report/evidence column, drop, rename, aux DDL, in-tx FK check), with
  verified FK restore and rollback-restores-original on failure; terminal
  global `assertForeignKeys` + `foreign_key_check` close the open path.
  Conforms.
- No silently discarded errors on any startup path (the only discarded Exec
  result in the store is the pre-existing diagnostic `LogEvent`, which is not
  a startup or migration path).

## P1-MIG-001 — remains closed, verified

The corrective commit installs legacy schema + `media.orbit_id` in one
immediate transaction and orbit/links/columns in a second; duplicate-column
errors are no longer control flow; every real error fails startup
(store.go:232–255, orbits.go:132–176). Fixtures rerun at the reviewed head
under the race detector — PASS:
`TestLegacyBootstrapDDLFailureRollsBackEveryStatementAndReruns` (proves fault
at the exact pre-commit boundary leaves no table, column or row change and
restart converges), `TestOrbitBootstrapDDLFailureRollsBackColumnsAndLinksAndReruns`,
`TestPartiallyAppliedLegacyColumnsResumeWithoutRewritingRows` (mixed partial
shape with non-default `provider='apple_music'` preserved byte-for-byte while
only the missing column is added). The fixtures drive the production open path
(`openWithOptionsAndCheckpoint` is `OpenWithOptions` with a nil seam) against
real SQLite/WAL — no mocked platform behavior.

## P1-MIG-002 — remains closed, verified

The sole connection installs `busy_timeout` before WAL and foreign-key pragmas
(`execStartupPragma`, store.go:213–226); retry is bounded (5 s), BUSY-only
(`code&0xff == 5`, covering extended BUSY codes); every other error fails
immediately; immediate DDL transactions serialize concurrent openers.
`TestConcurrentBootstrapMigrationSerializesAndPreservesLegacyAuthority` rerun
10 consecutive times under the race detector at the reviewed head — PASS
(32.8 s), both openers complete, columns exist once, legacy authority rows
singular, integrity/FK checks green.

## P1-MIG-003 — NEW — HIGH — open (post-audit regression at reviewed head)

**Finding.** The Phase-1 predecessor-dissolution reconciler now depends on
tables created later in the open path. `initMediaIngestSchema` (store.go:146)
unconditionally runs `reconcileOrphanedMediaItems`
(media_ingest_schema.go:243 → media_ingest.go:1898), which transitions every
active `media_items` row whose owner orbit was dissolved by a predecessor
binary to `deleted` via `transitionMediaTerminalTx`. Since the audit, that
terminal path unconditionally calls `revokeSavedCuesForMediaTx`
(media_ingest.go:1727 → `SELECT … FROM saved_cues`, saved_cue.go:589) and
`revokeTransmissionInboxByMediaTx` (media_ingest.go:1733 →
`UPDATE transmission_inbox_items`, transmission_inbox.go:494). Neither helper
guards for table existence, and both tables are created only later in the open
path (`initTransmissionSchema`, store.go:158, transmission_schema.go:189;
`initSavedCueSchema`, store.go:170, saved_cue_schema.go:9).

**Failure shape.** A database whose newest-generation opener predates the
transmission-inbox/saved-cue schemas (e.g. the exact audited `d7e0065`-era
artifact: `saved_cue_schema.go` absent, zero `transmission_inbox_items`
references), holding ≥1 active orphaned media item from a contract-authorized
predecessor-dissolution interval, deterministically fails current-head startup
with `store: init media ingest schema: … no such table: saved_cues` (SQLite
prepare error; row count irrelevant). Sequence: Phase-1-era binary serves →
operator rolls back to its pre-media-ingest predecessor → predecessor
dissolves an orbit owning active media (the exact state the audit declares
supported) → operator upgrades directly to the current head → startup is
blocked. The transaction rolls back cleanly (no corruption, fail-closed), but
roll-forward is impossible without an intermediate-generation binary — a
violation of the audited "restart-safe idempotent dissolution reconciliation"
property at the reviewed head.

**Provenance.** Entirely post-audit: at `d7e0065`, `transitionMediaTerminalTx`
contained none of these calls (verified via `git show`); the coupling entered
with `80e892b` (Phase 2 inbox persistence) and `8ccd770` (Phase 3 saved-cue
lifecycle). Existing predecessor fixtures miss it because they open the
current store first (creating all tables) before running predecessor code, so
the revokes no-op harmlessly; the generation-skip shape has no fixture.

**Why it matters for this signoff.** The production coordinator predates
`d7e0065` (release line beta.21, 2026-07-08), so the upgrade this Phase 1
signoff gates is exactly a generation-skipping roll-forward; combined with any
rollback interval it walks into this failure.

**Required fix (for the next producer; reviewer proposes, does not
implement).** Preferred: move the three media-ingest reconcilers
(`reconcileTelegramLegacyLinks`, `reconcileOrphanedMediaItems`,
`reconcileMediaLifecycleOutboxes`) out of `initMediaIngestSchema` into the
open path's existing post-DDL reconciliation section (after
`initSavedCueSchema`), preserving the audited DDL-then-reconcile discipline —
orphaned `media_items` carry no FK to orbits by design, so the identity-stage
and moderation-stage global FK checks are unaffected. Alternative: guard both
revocations with `tableExists` inside the reconciler transaction. Either way,
add a generation-skip fixture: a raw pre-inbox/pre-cue database (or extracted
`d7e0065`-era store) containing an active orphaned media item must open
successfully under the current head, revoke the orphan, and converge on
restart. Re-review by a non-implementing reviewer is required before the
approve decision on `TASK-260712-1xkn75` can be recorded.

## Medium findings — explicit disposition

The frozen audit records no open medium findings; this review adds one:

- **MED-1 (new, non-blocking): connection-recycling pragma loss.** The
  P1-MIG-002 correction moved `busy_timeout`/`foreign_keys` off the DSN onto
  the pooled connection. modernc/sqlite discards a file-backed connection when
  `sqlite3_is_interrupted` is observed at reset/put time (driver conn.go
  `ResetSession`/`IsValid`), which a cancelled request context can trigger
  (request-scoped contexts reach the store via e.g. `AllowsMediaDownload`,
  media/download.go:118). A replacement connection would silently run with
  `busy_timeout=0` and `foreign_keys=OFF` (WAL persists in-file) until the
  next restart, whose global `foreign_key_check` fails closed on any resulting
  violation — detection, not prevention. Startup correctness (P1-MIG-002's
  scope) is unaffected; the closure stands. Disposition: follow-up hardening —
  additionally carry `_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)` in
  the DSN as defense-in-depth for replacement connections (keeping
  `execStartupPragma` for first-connection ordering), or assert pragma state
  periodically. Route as ordinary engineering work; not part of this
  signoff's blocking set.

## Low observations (no action required)

1. `reconcileTelegramLegacyLinks`/`reconcileMediaLifecycleOutboxes` run no
   local `foreign_key_check`; their writes are FK-safe and the terminal
   moderation-stage global check covers the database.
2. `CreateAuthorizedMediaUpload` commits its transaction when rejecting on
   stream quota (deliberate rejection persistence; new-code path only).
3. `LogEvent` discards its Exec error — pre-existing, diagnostic-only, not a
   startup path.

## Post-audit deltas to audited files (`d7e0065..06ce330`)

- `store.go` (+77): only additive Phase 2/3 init/reconcile calls inserted;
  Phase 1 ordering (legacy → orbits → identity → media ingest → transmission
  → moderation) preserved; both HIGH fixes intact at head.
- `identity_schema.go` (+13): concurrent duplicate-column ALTER race handled
  by schema re-read — strengthens the audited concurrency contract.
- `links.go` (+28): Phase 2 `air_authority` write gate on legacy link
  mutations; fails open only when the Phase 2 table is absent (exact Phase 1
  shape). Additive.
- `transmission_schema.go` (+350): additive Phase 2/3 tables/columns/indexes;
  immutability triggers preserved or widened; backfills idempotent and
  invent no scheduling times. Additive.
- `media_ingest.go` (+40): saved-cue/inbox revocation coupling (P1-MIG-003
  above), expiry pin-guards, quota gate. One regression, flagged.
- `orbits.go`, `identity.go`, `media_ingest_schema.go`, `moderation_schema.go`,
  `bootstrap_migration_review_test.go`, and the audit document itself:
  unchanged since `d7e0065`.

## Rerun evidence at the reviewed head (local, darwin/amd64, go1.26.0)

- Failure + partial fixtures, race detector: 3/3 PASS (6.7 s).
- Concurrent bootstrap fixture, race detector, `-count=10`: PASS (32.8 s).
- Exact-predecessor suite (`-tags previoushead`, real extracted source at all
  pinned revisions from the audit matrix plus the newer air/automation
  generations): 14/14 PASS (67.8 s), including both transmission generations
  (`pre_scheduler_companion`, `pre_transmission_schema`), all five media
  surfaces, moderation, identity R8 trio, telegram link.
- Full coordinator module under the race detector (`go test -race ./...`):
  every package ok; `internal/store` 447.5 s, `internal/media` 92.0 s.
- Hosted CI at the exact reviewed head `06ce330`: run `29690180035` (push) —
  all four jobs success (`coordinator`, `node-core`, `pulsar-win`,
  `pulsar-win-packaged-probe`).

## Backup/restore prerequisites — inspected (repo-only, nothing altered)

- Coolify/Docker path: Litestream sidecar (pinned 0.3.13) streams SQLite WAL
  frames to S3-compatible storage, 6 h snapshots, 168 h retention
  (`deploy/litestream.yml`, `docker-compose.yml`, `docs/backup-restore.md`).
  DB-only; media-byte exclusion documented. WAL-mode prerequisite satisfied by
  the store itself. Documented silent-no-op when `LITESTREAM_BUCKET` is unset,
  with a verify procedure.
- Bare-VPS path: daily cron `sqlite3 .backup` with weekly rotation
  (`docs/runbook.md:49`), backup dir installed by
  `deploy/install-coordinator.sh`.
- Restore: manual by design; stop coordinator (single writer), `litestream
  restore -if-db-not-exists` (never clobbers), post-restore media
  reconciliation before traffic; binary+config rollback keeps additive tables
  in place and `coordinator.yml` predecessor-neutral.
- Honest gaps, explicitly disclaimed in-repo: no live restore drill or
  operator rehearsal is claimed anywhere; those remain owned by open manual
  tasks (`TASK-260712-3qybi2`, `TASK-260712-30xwu2`). A pre-migration
  auto-snapshot and stale-backup alerting are documented plans only. This
  matches the audit's stated boundary; no production data was opened or
  altered by this review.

## Boundary preserved

This review covers repository-verifiable engineering evidence only. No live
deployment, backup restore, or operator drill is claimed. No production
system was accessed. The strict Phase 1 root acceptance and Store submission
holds remain in place.

## Verdict routing

- `TASK-260712-1xkn75` → **not accepted this cycle**; checklist item 1
  (non-implementing reviewer confirmation) remains unchecked; status remains
  `to-review`. P1-MIG-001/002 stay closed; the audit packet needs no rework.
- `TASK-260715-unbb7c` → `to-dev` with this verdict: fix P1-MIG-003 (+ its
  generation-skip fixture) at a new main head, then route back through
  `reviewing` for a non-implementing re-review, which can then record the
  approve decision and check item 1 on the original task.
- MED-1 may be routed as ordinary follow-up engineering; it does not block.
