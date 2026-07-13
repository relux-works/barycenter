# Root review round 5 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. R5 fixes hashing and pending-client safety, but the database
state transitions still permit rotation/link races and one proposed legacy
upgrade path violates the node/control capability split.

## Blocking corrections

1. **Bind recovery consume atomically to the exact recovery generation.**
   The conditional update currently uses only `actor_id` and
   `consumed_at IS NULL`. A concurrent `/recovery/rotate` can replace the row's
   `recovery_id`/secret and reset it to unconsumed after the old request read its
   row; the old request can then overwrite `control_token_hash` on the new
   generation. The winning write must be in a transaction and predicate on the
   exact current `recovery_id`, expected recovery-secret hash/generation,
   `invalidated_at IS NULL`, `consumed_at IS NULL`, and usable actor/membership/
   orbit state (or otherwise serialize those state checks). On zero affected
   rows, reload current state before any idempotency comparison; never compare
   stale fields. Replace the false “SQLite row-level locking” statement—SQLite
   serializes writers with database locking, while the conditional statement
   and transaction provide the invariant. Add consume-vs-rotate,
   consume-vs-revoke/leave/disable, and two-token concurrency tests.

2. **Choose a coherent recovery-rotation storage model.**
   One `installation_credentials` row cannot both retain an old generation's
   `invalidated_at` record and overwrite that same row with a new active
   `recovery_id`/hash. The contract also omits the mandatory reset of
   `consumed_at` for the new secret. Freeze one model: either a separate
   generation table with one active row per installation, or a single current
   row whose rotation atomically overwrites ID/hash and sets
   `consumed_at=NULL`, `invalidated_at=NULL` (with no claim that the old row is
   retained/marked). Define rotation-vs-consume serialization, collision retry,
   audit behavior, and exact idempotency lifetime after rotation.

3. **Make Telegram link consume one rollback-safe transaction.**
   The SQL writes `consuming_actor_id` before the prose creates/reuses that
   actor, and it marks a code consumed before membership creation. Two different
   link codes consumed concurrently for the same Telegram user can both pass an
   application pre-check, consume both codes, then race on membership. Resolve
   or create the actor, validate revoked/same-orbit/foreign-orbit state, reserve
   the still-current code, create/reactivate membership, update the legacy
   compatibility row, and audit in one transaction; any conflict must roll back
   code consumption so the promised “code NOT consumed” behavior holds. The
   conditional code write must include expiry and invalidation/current-issuance
   predicates, not merely `consumed_at IS NULL`; the schema currently has no
   field implementing “new issuance revokes older unconsumed codes.” Add
   same-code/two-user, two-code/same-user, expiry-boundary, reissue-vs-consume,
   and membership-insert-failure tests.

4. **Never bootstrap control authority from a playback-only node token.**
   The proposed legacy upgrade says `node_token` authentication causes the
   server to issue a control token and recovery material. That directly violates
   the core invariant that a node token grants playback/heartbeat/media only
   and cannot administer or upload. An unprovisioned legacy installation must
   obtain control through a separately authorized primary/companion flow
   (device invite, explicit existing Telegram-owner authorization, or another
   frozen proof), possibly while also proving slot possession; node possession
   alone is insufficient. Remove the escalation from all downstream guidance
   and add a negative test that node-only auth cannot provision control/recovery.

5. **Use conservative, database-enforced backfill authority and coexistence.**
   Assigning `companion` to an orphan/inconsistent slot is not a safe default:
   companion can issue links and exercise control once provisioned. Give an
   orphan no active control membership (or a frozen playback-only/satellite
   state) and require explicit authorized repair. For consistent `paired_by`
   rows, state that copied role does not become usable control authority until
   the separately authorized provisioning in item 4. Replace “partial unique
   index or application check” with the database partial unique index; an
   application check alone is race-prone. Finally freeze Phase-1 coexistence:
   new/reactivated Telegram membership mutations must transactionally keep the
   legacy `members` view/table consistent until legacy readers are removed;
   feature-flag-off and previous-coordinator behavior must still see the same
   role and membership.

## Resubmission

- Amend the one authoritative note and outcome byte-identically; product source
  remains untouched.
- Preserve accepted R1–R4 decisions: exact canonical-string SHA-256 vectors,
  hash-only storage, integer IDs, unbiased secrets, never-auto-delete pending
  state, complete probe table, role-preserving recovery, in-process Telegram
  principal, uniform errors/limits, and additive legacy tables.
- Add exact recovery-generation SQL/state transitions, rotation model,
  rollback-safe Telegram transaction, non-escalating legacy provisioning,
  database uniqueness, dual-write/coexistence rules, and the named race tests.
- Return to `to-review`, never `done`.
