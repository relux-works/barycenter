# Phase 1 migration and rollback technical audit

- Date: 2026-07-15; P1-MIG-003 corrective addendum 2026-07-19
- Task: `TASK-260712-1xkn75`
- Frozen review base: `635a8d3e3e9d7929a474ae6a5278187071c520c9`
- Review mode: rigorous inline self-audit with corrective patch
- Acceptance state: P1-MIG-003 producer correction validated locally;
  independent re-review open

## Independence and production boundary

The strict inline execution chain implemented some migrations reviewed here.
This report therefore does **not** claim the required non-implementing reviewer
independence. It is a source-linked, reproducible packet for a separate
migration reviewer. It neither opens nor alters production data.

The repository fixtures use production SQLite, WAL, immediate writer locks,
real schema DDL and real extracted predecessor source trees. They cover the
database and config rollback contract without claiming a live deployment,
backup restore or operator drill.

## Source inventory and ownership

| Layer | Installation/backfill source | Rollback rule |
| --- | --- | --- |
| Legacy session/media | `coordinator/internal/store/store.go` | Preserve `elements`, `media`, `settings`, `events`; add only `media.orbit_id` with default zero. |
| Orbits/slots/links | `coordinator/internal/store/orbits.go`, `links.go` | Preserve legacy actor/role/slot/token authority; add only `slots.provider`, `members.display_name` and links. |
| Identity/onboarding | `identity_schema.go` and reconciliation in `identity.go` | Add actor/membership/credential state; project disabled-orbit authority for old code; clean only contract-authorized stale children after roll-forward. |
| Generic media/upload | `media_ingest_schema.go` and lifecycle reconciliation in `media_ingest.go` | Keep legacy media authoritative to old code; retain additive rows/outboxes; reconcile predecessor dissolution and cleanup receipts idempotently. |
| Transmission/policy/history | `transmission_schema.go` | Old code ignores new tables; immutable acceptance/target rows survive; scheduler companion backfill invents no barrier or start time. |
| Moderation | `moderation_schema.go` | Rebuild the one legacy report target constraint transactionally while preserving evidence, reports and old-code operation. |

Every current DDL group now commits in its own immediate transaction. Identity,
media and moderation post-DDL reconciliation is deliberately split into
restart-safe idempotent transactions: a crash may leave completed additive
work, never an unjournaled destructive cleanup. The final open path performs
foreign-key validation only after the bounded old-binary dissolution cleanup;
moving that check earlier would reject a supported rollback state.

## Findings

### P1-MIG-001 — HIGH — closed

**Finding.** The oldest legacy and orbit bootstrap ran outside explicit DDL
transactions. More seriously, startup discarded errors from all three column
migrations (`media.orbit_id`, `slots.provider`, `members.display_name`). A disk,
lock, incompatible partial table or DDL failure could therefore leave some
tables/columns committed and still let the coordinator continue toward serving
requests with a schema it could not safely use.

**Correction.** Legacy schema plus media ownership now installs in one
immediate transaction. Orbit, link and both column additions install in a
second immediate transaction. Each column is inspected before alteration;
duplicate-column errors are no longer used as control flow and every real
error reaches startup. A checkpoint can fault either exact pre-commit boundary
through the production open path.

**Re-review evidence.** `TestLegacyBootstrapDDLFailureRollsBackEveryStatementAndReruns`
and `TestOrbitBootstrapDDLFailureRollsBackColumnsAndLinksAndReruns` prove no
table, column or legacy row is partially changed after injected failure and
that restart converges. `TestPartiallyAppliedLegacyColumnsResumeWithoutRewritingRows`
starts from a mixed partial shape and proves existing provider, media ownership,
link and authority rows remain byte-semantically unchanged while the missing
column is added.

### P1-MIG-002 — HIGH — closed

**Finding.** Two coordinators opening the same legacy file concurrently could
return `SQLITE_BUSY` during WAL negotiation before the DSN-installed
`busy_timeout` became effective. This contradicted the stated rollout writer
serialization contract and was reproduced by the new parallel startup fixture.

**Correction.** The sole database connection now installs `busy_timeout`
before WAL and foreign-key pragmas. Startup pragma negotiation retries only
SQLite BUSY within the same bounded five-second policy; every other error
fails immediately. Immediate DDL transactions then serialize the two openers.

**Re-review evidence.** `TestConcurrentBootstrapMigrationSerializesAndPreservesLegacyAuthority`
starts two production open paths simultaneously against one pre-column legacy
database. Both complete, all three columns exist once, legacy media/member/slot
rows remain singular and the final database passes the global integrity/FK
checks. The test passed ten consecutive runs after the correction.

### P1-MIG-003 — HIGH — producer correction complete, re-review open

**Independent finding.** A post-audit regression made the Phase 1 media
dissolution reconciler call current saved-cue and transmission-inbox revocation
helpers while `initMediaIngestSchema` was still running. A direct roll-forward
from a generation that had media ingest but predated `saved_cues` and
`transmission_inbox_items` therefore failed startup with `no such table` when
an older coordinator had dissolved an orbit owning active media. The
transaction failed closed, but the supported roll-forward was blocked.

**Correction.** `initMediaIngestSchema` is DDL-only again. The three media
reconcilers now run from the store open path after both transmission and
saved-cue schema installation, before saved-cue reconciliation. This preserves
the DDL-then-reconcile discipline and ensures every table reached by terminal
media cleanup exists before the cleanup transaction starts.

**Producer evidence.** `TestGenerationSkippingMediaReconcileWaitsForLaterSchemas`
models a database with an active orphaned media item and neither late-generation
table. Current startup recreates both tables, revokes the orphan, clears its
canonical storage key, records exactly one cleanup receipt, and a second open
proves the transition is idempotent. The focused migration race set, full
coordinator suite, full coordinator race suite, and the complete
`previoushead`-tagged store race suite pass after the correction. A
non-implementing reviewer must still verify the correction before this HIGH is
accepted as closed.

No other open critical or high producer finding remains in this audit packet.

## Transaction, worker and rollback evidence

- Identity atomic-DDL, partial schema, interrupted orbit rebuild, constraint
  validation rollback, two projection generations, emergency rollback gap and
  concurrent reconciliation tests pass.
- Generic media atomic-DDL, legacy migration, upload idempotency, concurrent
  offsets, publication/delete fault injection, stale CAS workers and cleanup
  reconciliation tests pass.
- Transmission atomic-DDL, immutable snapshots, concurrent terminal receipts,
  scheduler generation/tombstone races and policy/history migrations pass.
- Moderation schema rebuild failure restores the original table and evidence;
  successful migration preserves immutable evidence and passes FK validation.
- Full coordinator tests and the entire store package under Go's race detector
  are recorded after the corrective revision is frozen.

The build-tagged predecessor matrix extracts and executes real source at these
pinned revisions rather than mocking old behavior:

| Surface | Pinned predecessor revision |
| --- | --- |
| Identity and rollback-neutral config | `e8bd240664a40b9cc78b974f3c34ad30712e2aa5` |
| Generic media scaffold | `06a06c099ed5b4f37f5e2dd3648772ffd041dfd9` |
| Upload sessions | `31bbeb9257b2555c86858c4087521466b58d673a` |
| Processing | `050c9792e328730e33bb65cf03fcda8e3d690061` |
| Lifecycle | `451e50bb1375b7db85b6e909c0ae4ef256efd2cc` |
| Media integration | `0d6863c462111da6ed27f851a636e40d95100d73` |
| Transmission pre-scheduler / pre-schema | `0c1e1946ff692aa553c19ca6bf7328150d1a24b8`, `2aa97c2d08cb93b110200ae159fd43265410ff5a` |
| Moderation | `45cb0fbbd954fac12818915abbb52647b6f045c5` |

All ten exact-old scenarios pass: predecessor code opens the upgraded file,
uses its real Store/config APIs, mutates legacy authority/session/media state,
then current code rolls forward without losing additive data or reviving stale
workers. No rollout task drops, rewrites or cleans unknown predecessor data.

## Required independent signoff

A non-implementing migration reviewer must still:

1. inspect the frozen base and corrective patch across every source in the
   inventory;
2. reproduce the failure, partial, concurrent and exact-predecessor commands;
3. verify `P1-MIG-001`, `P1-MIG-002` and the P1-MIG-003 corrective ordering
   close the startup gaps without moving contract-authorized cleanup ahead of
   rollback reconciliation;
4. inspect backup/restore prerequisites and record reviewer identity, exact
   revision, findings and approve/reject decision.

Until then, the original task remains open even though reversible engineering
continues in strict plan order.
