# Root review round 7 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 1,917 lines of Rev 7, verified the authoritative
and outcome copies are byte-identical
(`f852df7295598bf74bb45015742fb67592aebfea53b5bcfe7d44f770a53dadb0`),
and checked the contract against the live Store DSN, legacy mutations, slot
replacement, and deletion paths. Rev 7 fixes the R6 transaction and destructive
UPSERT findings, but leaves stale-bearer authorization, issuer lifecycle,
executable DDL, and old-binary reconciliation gaps that can preserve authority
after revocation/rebind.

## Blocking corrections

1. **Re-authenticate the exact presented control token inside every destructive
   writer transaction.**
   `/recovery/rotate` and `/telegram-links` resolve the bearer outside the
   transaction, then re-read actor/membership/orbit state but never verify that
   the submitted token still equals the current `control_token_hash`. A request
   can authenticate with token A, pause, let recovery consume commit token B
   (revoking A), then resume and rotate recovery material or issue a link using
   the already-revoked A. Pass the presented-token digest into the store method
   and, after `BEGIN IMMEDIATE`, require the same actor credential row to contain
   that exact current digest before any mutation. Apply this invariant to every
   control-authenticated mutation, not just these two endpoints. Rotation must
   also enforce the normative capability matrix inside the transaction:
   control+satellite is `403`, but the current rotation steps never read/check
   role. Add deterministic stale-token barriers for rotate and link issuance:
   authenticate A → pause → recovery commits B → resume A → no mutation and
   `401`; also test role/lifecycle changes after middleware and before the lock.

2. **Validate link issuer authority and the stored grant at consume time.**
   Telegram consume checks code age/state and the consuming Telegram actor, but
   never re-checks the code issuer, issuer membership/role, or target orbit.
   Consequently a code remains usable after its issuer is revoked, leaves, is
   downgraded to satellite, or the orbit is disabled, contradicting §5/§18's
   stated generic failure. Inside the same consume transaction, after code
   lookup and before actor creation/reservation, require: active orbit, non-
   revoked issuer, active issuer membership in that exact orbit, issuer role
   primary/companion, and stored `desired_role` companion/satellite. Failure is
   generic `credential_invalid`, code unconsumed. Add issue→revoke/leave/
   satellite/disable→consume tests plus both writer orderings for concurrent
   lifecycle change vs consume.

3. **Freeze executable DDL and the actual Go mechanism that provides
   `BEGIN IMMEDIATE`.**
   The note still lists columns and “allowed values,” not a complete schema.
   `ALTER TABLE ... status TEXT NOT NULL DEFAULT 'active'` has no `CHECK`; the
   only identity index shown covers Telegram despite R6 requiring stable app
   identity too; FK lists omit the composite slot reference and consuming
   actor; control-token hashes are not unique; null/hash/role/kind invariants are
   unenforced. Provide the exact idempotent DDL, including at least:
   `CHECK(status IN ('active','disabled'))`, actor-kind and role checks, one
   database-enforced `(kind, external_ref)` identity for both actor kinds,
   partial unique non-null control/recovery/code lookups, lowercase-64-hex
   checks, recovery-field all-or-none consistency, consumed timestamp/actor
   consistency, complete actor/orbit/slot FKs with explicit delete behavior,
   and timestamp units (the live store uses Unix milliseconds).

   The live modernc driver defaults `sql.Tx` to deferred transactions; merely
   writing “BEGIN IMMEDIATE” in prose is insufficient. Freeze the supported
   implementation: for example add `_txlock=immediate` to the DSN and use
   `BeginTx`/`sql.Tx` consistently (modernc v1.53.0 explicitly supports this),
   or use one pinned `sql.Conn` helper without issuing statements back through
   `sql.DB`. With `SetMaxOpenConns(1)`, a manual `BEGIN IMMEDIATE` followed by
   `db.Query` can self-block. Tests must assert `PRAGMA foreign_keys`, exercise
   two independent DB connections/stores, and run `PRAGMA foreign_key_check`
   after migration/reconciliation.

4. **Reconcile the full old-binary authority surface, not only Telegram
   `members`.**
   The previous coordinator can also:
   - transfer primary roles (`TransferPrimary`);
   - leave, revoke an owner's slots, and promote another member (`LeaveOrbit`);
   - rebind/re-pair a slot (`PairSlot` uses `INSERT OR REPLACE` and changes
     token hash/`paired_at`);
   - revoke a slot; and
   - dissolve an orbit by deleting `members`, `slots`, then `orbits`.

   During rollback its DSN has foreign keys off. On re-deploy, the current
   Telegram-only reconciliation can therefore leave app-installation roles
   stale, retain a control token for a revoked/rebound old installation, and
   leave additive rows pointing at deleted parents. Enabling FK enforcement does
   not retroactively repair those rows (`PRAGMA foreign_key_check` reports them),
   while the live `DeleteOrbit` fails under enforced child FKs unless new-code
   deletion ordering/cascades are defined.

   Freeze authority for both `members` **and `slots`** while the flag is off,
   plus reconciliation for slot creation, revoke, rebind/generation change,
   ownership/role change, and orbit dissolution. A rebind must conservatively
   revoke/unprovision the old app control credential; because the node hash must
   not be duplicated, store/compare a non-secret binding generation (for
   example the bound `paired_at`/generation) or choose another explicit proof.
   While the flag is on, update `CreateOrbit`/`TransferPrimary`/`LeaveOrbit`/
   `RevokeSlot`/`PairSlot`/`DeleteOrbit` with atomic dual-write or a canonical
   mutation service. Do not enable new auth/endpoints until reconciliation and
   `foreign_key_check` succeed. Extend the rollback test with rebind, slot
   revoke, owner leave/promotion, and orbit dissolution.

5. **Treat a same-orbit legacy member as already linked; never overwrite its
   role from a new code.**
   §11 step 7b currently sees a same-orbit `members` row and proceeds; step 9
   creates/reactivates additive membership with the code's `desired_role`, and
   step 10 overwrites the authoritative legacy role. This directly contradicts
   “desired_role never changes a migrated member's role.” If the legacy row is
   active in the target orbit, return `already_linked_same_orbit` and leave the
   code unconsumed. If additive state is missing/divergent, reconcile it first
   or fail closed; do not repair authority from an unauthenticated link code.
   State that the feature becomes serving-ready only after startup reconciliation
   succeeds. Add handcrafted cases for legacy-only same-orbit primary/companion,
   additive-only, foreign-orbit, and role divergence, proving no role or row is
   moved and no code is consumed on conflict.

6. **Make rate-limit policy internally consistent and concurrency-safe.**
   The rate table and endpoint steps say rotations/link issuances count only
   successes and merely “check” a counter outside the transaction, while the
   final answer says all syntactically valid attempts are atomically counted.
   Concurrent valid callers can all pass a pre-check and exceed 3/5 successes.
   Choose one rule. The simplest frozen rule is atomic reservation of every
   authenticated, syntactically valid attempt before expensive generation/DB
   work; otherwise define reservation plus exact rollback/refund without a race.
   Specify collision retries, lifecycle failures, stale-token failures, and
   response-loss retries. Also define source-IP extraction: use the direct peer
   unless the request came through an explicitly trusted proxy; never accept a
   spoofable forwarding header from arbitrary peers. Add barrier tests at the
   last available slot for every limiter.

7. **Prevent one ambiguous pending recovery from being overwritten by another.**
   The protected client record is safe for one attempt, but the contract does
   not say what happens if the user starts recovery again while an `ever_sent`
   candidate is unresolved. Replacing that record can discard the only token a
   prior server commit accepted. Namespace the record by coordinator origin and
   recovery handle, make its writes/state transitions atomic, and permit at most
   one unresolved sent candidate per target. Starting another attempt must first
   promote/supersede via a separately authenticated credential or require the
   same explicit destructive-abandon warning; it may never silently overwrite
   the candidate. Add crash barriers before/after pending write, `ever_sent`,
   send, active promotion, and pending deletion, plus a double-start test.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; product source
  remains untouched.
- Preserve the accepted canonical-string hash vectors, pending-token retention,
  generation-bound single-row recovery, explicit writer transactions, rollback-
  safe code reservation, non-destructive legacy UPSERT, no node escalation, and
  capability×role model.
- Add in-transaction bearer proof, consume-time issuer proof, exact constrained
  DDL/transaction wiring, full member+slot+orbit rollback reconciliation,
  same-orbit legacy conflict preservation, atomic limiter semantics, and the
  named deterministic tests.
- Return to `to-review`, never `done`.
