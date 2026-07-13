# Root review round 6 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. R6 substantially improves generation and link-code safety,
but lifecycle ordering, legacy dual-write, and schema enforcement are not yet
coherent with the real SQLite schema.

## Blocking corrections

1. **Put recovery read, lifecycle check, secret verification, winning update,
   audit, and response data in one explicit writer transaction.**
   The current sequence reads actor/membership/orbit before the conditional
   credential update but never freezes a `BEGIN IMMEDIATE ... COMMIT` boundary.
   A revoke/leave/disable writer can commit after the check and before consume;
   consume then affects one credential row and returns 200 despite the rule that
   the now-disabled context must fail. After the in-memory rate-limit reservation,
   start one writer transaction, read and verify within it, perform the
   generation-bound update and audit, then commit before returning 200. This
   gives a clear linearization: whichever writer obtains/commits first wins;
   if revocation/state change commits first, consume sees it and fails. Do the
   same for rotate's bearer/lifecycle check plus overwrite. On every error use
   rollback, and reload idempotency state inside the same transaction. Add
   deterministic barrier tests for both possible orderings of consume versus
   revoke, leave, disable, and rotate.

2. **Make link issuance itself a serialized all-or-nothing operation and fix
   concurrency expectations.**
   Invalidating prior codes and inserting the replacement must share one
   `BEGIN IMMEDIATE` transaction with the issuer capability/lifecycle check;
   otherwise reissue-versus-consume has no frozen linearization. With
   `BEGIN IMMEDIATE`, two consume writers serialize: the second two-code/same-user
   transaction normally sees the first membership at step 7 and returns the
   appropriate 409 before reserving its code—it does not normally race at the
   partial index. For same-code/two-user, the winner commits the code as consumed;
   the loser rolls back only its own work and the shared code **remains consumed**,
   not “restored for the loser.” Correct the prose and test expectations while
   retaining the partial unique index as defense in depth.

3. **Replace destructive legacy `INSERT OR REPLACE` with conflict-safe
   dual-write.**
   `members` has both `(orbit_id,tg_user_id)` primary key and a global unique
   `members_user(tg_user_id)`. `INSERT OR REPLACE` can delete an existing
   foreign-orbit row and insert the target row, silently moving a user despite
   the “foreign orbit → no consume” promise. Inside the link transaction, check
   the legacy row by `tg_user_id` as well as the new membership. Reject any
   foreign-orbit mismatch. For same-orbit insert/update use an UPSERT that never
   deletes a conflicting row; any unexpected uniqueness conflict rolls back code
   reservation. Freeze equivalent leave/role-change ordering. Add a handcrafted
   divergent old/new DB test proving no legacy row is deleted or moved.

4. **Define reconciliation after an actual rollback to the old coordinator.**
   The previous binary can mutate only `members`; when the new binary is later
   redeployed, additive rows may be stale. “Log and never overwrite differences”
   permanently preserves divergence and contradicts the claimed same view.
   Freeze authority while `self_service_onboarding` is off (legacy `members` is
   authoritative) and an idempotent reconciliation/backfill policy for a
   rollback→old mutation→upgrade cycle, including join, leave/delete, display
   name, and role change. Once the feature is on, all supported new mutations
   dual-write transactionally. Test the full old-schema/open-new/open-old/
   mutate/open-new sequence.

5. **Complete the additive schema required by the contract and enable its
   constraints.**
   Live `orbits` has no `status` column, yet every lifecycle rule queries
   `orbits.status`. Freeze the additive default-active migration and allowed
   values. SQLite foreign keys are disabled by default and the current DSN does
   not enable them, so merely declaring slot/actor references is ineffective;
   require `foreign_keys=ON` on every connection and tests proving violations
   fail. Enforce one actor identity per stable external ref for both Telegram
   and app installations (for example a partial unique `(kind,external_ref)`),
   not only one credential row per slot, or duplicate app actors can drift.
   Freeze NOT NULL/CHECK/foreign-key behavior for active rows and validate the
   migration against handcrafted legacy DBs before relying on these predicates.

6. **Keep capability semantics precise for backfilled actors.**
   `satellite` is a membership role, not by itself proof that every control
   endpoint is impossible; authorization comes from the combination of token
   capability and role policy. State that orphan installations remain
   unprovisioned (`control_token_hash`/recovery NULL), node-auth paths expose only
   node capability, and no control resolver can succeed. If later separately
   authorized provisioning is allowed for an orphan, the authorizer must also
   explicitly choose/repair its role—do not automatically turn the placeholder
   satellite membership into control authority. Keep the negative node-only
   provisioning test and add ActorContext capability×role matrix tests.

## Resubmission

- Amend and reattach the single authoritative note byte-identically; product
  source remains untouched.
- Preserve accepted canonical-string hashes/vectors, unbiased secrets, pending
  credential safety, generation-bound recovery ID, single-row rotation,
  rollback-safe link consume, no node escalation, integer IDs, uniform errors/
  limits, and additive legacy tables.
- Add explicit recovery/rotate/issuance transaction boundaries, corrected race
  outcomes, non-destructive legacy UPSERT/reconciliation, `orbits.status` and
  enforced FK/identity constraints, plus the named rollback and barrier tests.
- Return to `to-review`, never `done`.
