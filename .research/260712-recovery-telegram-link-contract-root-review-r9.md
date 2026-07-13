# Root review round 9 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 2,754 lines of Rev 9 and verified that the
authoritative and outcome copies are byte-identical
(`78ce4d49837d5cd28728a65567266b6bf554af0e431dac64fc6d0721c856137d`).
Product source remains untouched. Rev 9 fixes deleted-orbit FK cleanup,
enumerates the legacy writers, adds constrained fresh-install DDL, removes the
unsafe lone-401 deletion, and acknowledges rollback semantics. It introduces a
32-bit security generation marker, contradictory pending-target rules, and a
nullable credential binding that the proposed DDL accepts with a clean
`foreign_key_check`.

## Blocking corrections

1. **Do not truncate the slot binding generation to 32 bits.** Section 17.6
   defines `SHA-256(token_hash)[:8]` as the first eight **hex characters** —
   only 32 bits — then calls it collision-proof/overwhelming. A collision makes
   a same-millisecond rebind indistinguishable, exactly the stale-control-token
   failure this marker exists to prevent. There is no storage pressure that
   justifies weakening it: use the full 64-hex domain-separated digest (or at
   least a frozen >=128-bit generation with an explicit collision response),
   and validate its exact shape in app actor `external_ref`. Add an injected
   test where old/new bindings share the first eight hex characters; the new
   design must still detect the rebind and revoke the old authority. Remove the
   contradictory takeaway saying the old actor's external ref is “updated” —
   the detailed transition says it retains its generation-scoped identity.

2. **Define a real pending-recovery target key and durable `ever_sent`
   barrier.** Section 5.1.2 simultaneously says:
   - different `(coordinator_origin, recovery_id)` keys are fully independent;
   - different recovery IDs on one origin do not block each other; and
   - at most one sent candidate across all recovery IDs on the same origin is
     allowed because origin identifies the installation.

   A coordinator origin is not an installation; one coordinator legitimately
   hosts many app actors. The current rule either permits two generations for
   one actor or globally blocks independent recoveries for different actors.
   Freeze a stable non-secret installation target (for example `actor_id` or a
   dedicated installation ID returned at creation and included in explicit
   recovery export) and key ambiguity by canonical origin + target, with
   recovery ID as the generation. If the protocol intentionally knows only
   `recovery_id`, scope the guarantee to that key and stop claiming
   cross-generation protection. Canonicalize origin (scheme, IDNA/lowercase
   host, effective port, no path/query ambiguity) so equivalent URLs cannot
   create separate namespaces.

   Windows durability also needs a send barrier, not just atomic-shaped bytes.
   An atomic replace can recover the old complete blob (`ever_sent=false`)
   after power loss unless the new encrypted blob and replacement metadata are
   flushed. Freeze the exact `ReplaceFile`/`MoveFileEx` flags, temp-file
   `FlushFileBuffers`, replacement/write-through behavior, cleanup, and
   read-back; the network request MUST NOT begin until durable storage confirms
   `ever_sent=true`. Test power loss at every write/flush/replace/send edge.

3. **Restore non-null current slot ownership in executable DDL.** Rev 9 changed
   `installation_credentials.slot_orbit_id` and `slot_name` from `NOT NULL` to
   nullable, saying they are cleared before deletion, while every rebind/revoke
   algorithm simply deletes the row. The exact DDL now accepts a live unique
   control token with both slot columns NULL and `PRAGMA foreign_key_check`
   reports no violation; I reproduced this. That credential has no enforceable
   orbit and contradicts schema ownership/orbit alignment. Keep both columns
   `NOT NULL` if old current credentials are deleted, or add a complete
   current-vs-historical state model with an all-or-none slot CHECK and a rule
   that historical rows cannot hold control/recovery material. Test NULL/half-
   NULL/control-without-slot rows as rejected by the database.

4. **Make revoked-slot reconciliation idempotent.** Case 2 deletes the
   credential for a revoked slot. On the next startup, case 1 sees “slot not in
   installation_credentials” and creates a new active actor/credential unless
   its query explicitly excludes revoked slots; the current ordered prose does
   not. Freeze a source-first algorithm: reconcile/delete missing, revoked, and
   rebound current credentials first; then create actors/credentials only for
   unmatched legacy rows with `slots.revoked_at IS NULL`. Initial backfill must
   likewise either skip revoked slots or create only a revoked historical actor
   with no current credential. Run reconciliation twice after revoke and prove
   the second run is a true no-op with no extra actor or membership.

5. **Apply the orbit-alignment invariant in the normative endpoint
   algorithms, not only a later summary.** Sections 5.2, 6, 7, and 10 still
   read “the actor's membership” without selecting/joining the credential's
   exact `slot_orbit_id`; their shown SQL does not even fetch the slot orbit.
   Replace each detailed query with the exact join and require the current
   referenced slot to exist and be unrevoked. Recovery response, rotation, link
   issuance, and actor context must all derive orbit/role from that same row.
   A mismatch/current-binding failure must fail closed before mutation and be
   repaired only by startup reconciliation. Update the stale-bearer and
   cross-orbit tests to exercise the actual endpoint SQL, not a standalone
   invariant helper.

6. **Make an already-present unconstrained `orbits.status` fail closed or get
   repaired.** The fresh `ALTER TABLE ... CHECK` is correct, but the idempotent
   path merely ignores “duplicate column name.” A database left by a partial
   rollout can already contain `status TEXT` without the CHECK; ignoring the
   duplicate leaves `status='bogus'` writable. I reproduced that exact sequence.
   Inspect the existing schema and values. If the constraint is absent, rebuild
   the table safely or install equivalent validated triggers; if compatibility
   cannot be proven, abort startup. Add a fixture with an unconstrained status
   column and a bogus existing value, plus interrupted-migration restart tests.

7. **The proposed old-binary projection does not keep a disabled orbit
   disabled.** Section 17.11 revokes its current legacy slots, re-enables
   ingress, and calls the interval fail-closed. But live old `PairSlot` ignores
   `orbits.status`, counts only unrevoked slots, and reuses revoked letters via
   `INSERT OR REPLACE`; a legacy member can immediately mint a new node token
   in the disabled orbit. “Cannot be un-revoked without explicit PairSlot” is
   not a security boundary because PairSlot is an exposed legacy operation.
   Choose a procedure the old binary actually enforces: keep the service/that
   tenant offline, deploy a compatibility-patched old binary that rejects the
   projected disabled state, or project into a legacy state that also prevents
   pair/invite mutations. Do not re-enable ingress and call the result
   fail-closed. Add disabled-orbit → project → old `/pair`/`PairSlot` attempts
   to the rollback test. Any emergency rollback without this enforcement is an
   explicitly unsafe maintenance mode, not an accepted security procedure.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; keep product
  source untouched and return to `to-review`, never `done`.
- Preserve the accepted recovery generation transaction, stale-bearer and
  issuer checks, full writer inventory, deleted-orbit cleanup, same-orbit
  conflict, constrained fresh DDL, limiter ordering, and no-node-escalation
  policy.
- Execute the exact schema and two-pass reconciliation. Prove full-strength
  binding identity, target-scoped/durable pending state, non-null credential
  ownership, endpoint orbit joins, partial-migration repair, and disabled-orbit
  rollback against the live old `PairSlot` behavior.
