# Root review round 8 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 2,458 lines of Rev 8 and verified that the
authoritative and outcome copies are byte-identical
(`ddcfc14e3e63f09ab113eece405700340dd55c9799269e69f51783ba57cbe92e`).
Product source remains untouched. Rev 8 correctly adds in-transaction bearer
re-authentication, consume-time issuer validation, `_txlock=immediate`, full
secret lookup indexes, same-orbit legacy rejection, and an explicit limiter
policy. It still contains executable schema/state-machine contradictions that
would either reject a legitimate slot rebind, leave foreign-key corruption, or
lose the only accepted recovery token.

## Blocking corrections

1. **Make slot rebind executable and generation-safe under the frozen unique
   constraints.** Section 17.8.2 says to revoke the old actor, merely null its
   secret fields, then create a new actor and credential for the same slot.
   This cannot execute against §17.9:
   - the old actor keeps `external_ref = "{orbit_id}:{slot}"`, so the new actor
     violates `actors_identity(kind, external_ref)`;
   - the old `installation_credentials` row keeps its non-null slot reference,
     so a new row violates `UNIQUE(slot_orbit_id, slot_name)` even after its
     secret fields are nulled.

   I reproduced both failures with the proposed DDL. Freeze one coherent
   transition: a binding-generation-scoped app identity plus removal/detachment
   of the old current-credential binding, or a rigorously safe reuse model that
   cannot resurrect the old actor or any old control/recovery authority. If
   historical actors remain, the current slot reference must be released in
   the same transaction before the new current credential is inserted.
   `paired_at` alone is not a collision-proof generation: live `PairSlot` uses
   `time.Now().UnixMilli()` and `INSERT OR REPLACE`, so two replacements can
   change `token_hash` within the same millisecond. Freeze a binding proof that
   detects every token replacement without making a second authoritative node
   credential (for example a domain-separated fingerprint of the current slot
   binding), and define the app actor `external_ref` from that generation.
   Add a real same-slot rebind test that executes the exact DDL and proves the
   old node/control/recovery credentials fail and the new actor succeeds.

   Also enforce the missing alignment invariant: an app actor's active
   membership must be in exactly `installation_credentials.slot_orbit_id`.
   The proposed FKs permit a credential bound to orbit 1 and an active
   membership in orbit 2; `PRAGMA foreign_key_check` still reports clean. All
   auth/recovery/rotation/issuance queries must join the exact slot orbit, and
   migration/reconciliation must fail closed on a mismatch.

2. **Repair deleted-orbit reconciliation instead of retaining an FK orphan.**
   When the old binary deletes an orbit with foreign keys off, §17.8.2 case 5
   deletes credentials and link codes but only sets
   `memberships.left_at`. The membership row still references the missing
   `orbits.id`; setting a timestamp cannot satisfy the FK. I reproduced the
   sequence and `PRAGMA foreign_key_check` returns
   `memberships|...|orbits|...`. Delete every membership for the missing orbit
   (after preserving any required audit facts), then clean current credentials
   and codes in an explicit child-first order. Add a handcrafted old-binary
   dissolution fixture and assert both `foreign_key_check = zero rows` and the
   absence of all orbit-scoped additive rows.

3. **Cover the actual legacy mutation surface while the feature is on.** The
   §17.8.4 table is incomplete relative to live source:
   - `CreateOrbit` inserts the legacy primary Telegram member but the proposed
     additive write mentions only an app installation; live `CreateOrbit` does
     not create a slot at all, so “initial slot” cannot be dual-written there
     without freezing a new service-level transaction;
   - `AddMember` and `SetMemberName` mutate `members` and are omitted;
   - `TransferPrimary` updates two Telegram member roles, but the table only
     updates installation actors;
   - `LeaveOrbit` promotion must update both the promoted Telegram actor and
     every affected installation actor, not only the installation actor.

   Inventory every writer of `members`, `slots`, and `orbits` and route it
   through one canonical transaction/dual-write layer. Freeze the transaction
   boundary for the spec-required Create Barycenter operation (orbit + app
   actor + exact membership + first slot + both credentials + recovery + audit
   in one transaction), rather than attributing a slot to the current
   non-atomic `CreateOrbit` method. Add flag-on tests for join, display-name
   refresh, transfer, leave/promotion, pair/rebind/revoke, create, and delete,
   checking both legacy and additive views immediately after each mutation.

4. **Provide the constrained, fail-fast DDL requested in R7.** Section 17.9
   still omits `CHECK(status IN ('active','disabled'))` and incorrectly claims
   SQLite cannot add that CHECK with `ALTER TABLE ... ADD COLUMN`. The local
   SQLite executable accepts
   `ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK(status IN (...))`
   and rejects a later `status = 'bogus'` write. Use the constrained form (or
   an equally enforceable migration for an already-present unconstrained
   column). Do not “ignore the error” from `ALTER TABLE`: distinguish an
   already-present compatible column from lock, corruption, syntax, or schema
   failures and fail startup on the latter.

   The Telegram code table also lacks the R7-required consumed-state
   consistency: it permits `consumed_at` without `consuming_actor_id` and vice
   versa. Add an all-or-none CHECK for those fields (and freeze how an
   invalidated code may coexist with consumption). Keep the complete FKs and
   lowercase-hex constraints already added. Test every CHECK with one accepted
   and one rejected row; do not validate only through application code.

5. **Fix the still-destructive pending-recovery double-start rule.** Section
   5.1.2 says that after a probe returns `401`, the client may delete the
   pending candidate and generate a new token. This directly contradicts the
   race note in §5.1.1: a previously sent recovery request can commit after
   that point-in-time probe. Deleting then can discard the only token the
   server accepts after consuming the recovery secret. A lone `401` is never a
   safe automatic-delete proof while any earlier send has ambiguous completion.
   Reuse/retry the same pending token, resolve it through a linearizable
   server-side proof, supersede it with a separately confirmed active
   credential, or require the explicit destructive-abandon path. Add the exact
   barrier: request A sent and paused before commit -> probe A returns 401 ->
   user starts again -> request A commits; the client must still retain and
   promote A.

   R7 also required the secure-store namespace to include coordinator origin
   and recovery handle. Rev 8 keys only by `recovery_id`; equal handles on two
   coordinators collide. Conversely it declares different recovery IDs always
   independent, which does not enforce “one unresolved sent candidate per
   target installation” across recovery generations. Freeze a canonical
   origin + installation/recovery-generation key and exact atomic serialized
   record. DPAPI encrypts bytes; it is not itself an atomic persistence layer,
   so name the Windows store/write primitive and its crash-safe replace
   behavior instead of asserting that “DPAPI item operations” are inherently
   atomic.

6. **Remove the remaining rate-limit ordering contradiction.** The global §9
   rule says authenticated input is validated before reservation and that a
   `400 invalid_request` does not count. Section 10 instead reserves the
   per-actor attempt at step 2 and validates `desired_role` at step 3, so an
   invalid request is counted. Rotation likewise needs an explicit bounded
   body parse/validation step before reservation. Use one exact order everywhere:
   bearer auth -> bounded syntax validation -> atomic reservation -> generation
   -> writer transaction. Update the endpoint algorithms and boundary tests,
   not only the summary table.

7. **Stop claiming unconditional old-binary rollback safety.** The old
   coordinator ignores both `orbits.status` and `actors.revoked_at`; its live
   `LookupToken` authorizes directly from an unrevoked `slots` row. Rolling
   back can therefore make a disabled orbit or an additively revoked actor's
   still-live node token usable again. Startup reconciliation on the next new
   deploy does not protect the interval running the old binary. Either project
   every security-relevant disable/revoke into legacy state before rollback,
   provide a reversible compatibility marker the old binary actually enforces,
   or document a fail-closed operational rollback (remove ingress/stop serving
   until upgraded). Keep schema readability compatibility separate from
   authorization-semantic safety, and test the chosen rollback procedure.

## Resubmission

- Amend the single authoritative note and reattach one byte-identical outcome;
  keep product source untouched and return to `to-review`, never `done`.
- Preserve the accepted stale-bearer barrier, issuer lifecycle checks,
  generation-bound recovery update, rollback-safe Telegram consume,
  `_txlock=immediate`, exact secret hashes, capability split, same-orbit
  conflict, and atomic limiter reservation.
- Execute the proposed DDL in tests. A document-level assertion is not evidence:
  prove rebind, deleted-orbit cleanup, app membership/slot alignment, all legacy
  writers, pending 401/late-commit, constrained status/consume state, limiter
  ordering, and old-binary rollback behavior.
