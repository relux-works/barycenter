# Root review round 13 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 3,906 lines of Rev 13. The agent attached the new
copy under a second filename while leaving the canonical `research.md` at Rev
12; root normalized the board to one canonical outcome without changing the
candidate text. The authoritative and canonical outcome are now
byte-identical (3,906 lines, 30,076 words, 232,548 bytes, SHA-256
`5ec94a287ed4d45873cb45682097c48a5ac38334ad2d8d453bccc1a1259bf2de`).
Product source remains untouched. Rev 13 correctly repairs ordinary disabled-
orbit handling, binding ownership, rotate/serving-gate SQL, and the second
projection cycle. It still contains non-executable migration/recovery branches
and two exact SQL/data-path defects.

## Blocking corrections

1. **Return the orbit ID from the exact recovery-consume transaction.** The
   success response requires `{orbit_id, actor_id, role}`, but step 5 selects
   only `actor_id`, hashes, and consumed state (lines 1106–1111), while the
   lifecycle query selects only revocation/left/role/status (lines 1122–1132).
   Neither query returns `ic.slot_orbit_id`, `m.orbit_id`, or `o.id`. Therefore
   both first-consume success at line 1165 and idempotent success at line 1185
   lack the `orbit_id` they promise to serialize.

   Add `ic.slot_orbit_id` (or `o.id AS orbit_id`) to one transaction-consistent
   result and use that exact value for the response and audit. Generate the
   first-consume and consumed-replay tests directly from the shown SELECTs and
   assert all three response fields, including a non-default orbit ID. Do not
   paper over the missing value with an out-of-transaction lookup.

2. **Finish the SQLite table-rebuild repair instead of retaining the rejected
   dependency parser and impossible crash recovery.** Step 1 says it classifies
   schema objects by `tbl_name`, but its exact query returns only
   `type, name, sql` (lines 2304–2314). An implementation following the query
   has no ownership column to classify. It also still filters dependencies
   with `sql LIKE '%orbits%'` (line 2311), which R12 explicitly rejected as a
   dependency parser. This admits comment/string false positives and is not a
   schema ownership model.

   Use a deterministic conservative inventory: select
   `type, name, tbl_name, sql`; classify owned indexes/triggers strictly by
   `tbl_name`; capture all user-defined views and all external triggers (or use
   another actual parser with tests), drop the selected objects inside the
   transaction, and recreate the exact captured DDL. Do not infer dependency
   from a substring. Auto-indexes remain excluded.

   Lines 2389–2394 again claim that startup with `orbits` missing and only
   `orbits_new` present can rename the table and “recreate dependent objects.”
   Owned indexes/triggers disappeared with `DROP TABLE orbits`, and their DDL
   was captured only in the memory of a prior process. No durable journal in
   this contract can recreate it. Moreover, a crash inside the stated SQLite
   transaction should roll back the DDL, making this shape evidence of an
   unexplained non-transactional/manual partial migration. Abort startup on
   this shape unless a durable migration journal contains and validates the
   exact captured schema objects. Test owned index, owned trigger, external
   trigger, view, false-positive SQL text/comment, and both intermediate table
   shapes. Verify `PRAGMA foreign_keys` is restored on every commit, rollback,
   and abort exit.

3. **Apply the nullable `paired_at` sentinel in Phase B, not only in prose and
   the handoff.** The live `slots.paired_at` column is nullable and §17.6
   correctly freezes NULL → `0`. The exact Phase B creation rule nevertheless
   says `slot_paired_at = slots.paired_at` (lines 2917–2925), while the target
   DDL declares `slot_paired_at INTEGER NOT NULL`.

   I executed that data shape: inserting a Phase-B row selected from a slot
   with `paired_at=NULL` fails with
   `NOT NULL constraint failed: installation_credentials.slot_paired_at`.
   Freeze `slot_paired_at = COALESCE(slots.paired_at, 0)` in initial backfill,
   Phase B, flag-on `PairSlot` migration/repair, and every exact INSERT. Use the
   same `COALESCE` in comparisons. The existing nullable fixture must execute
   the actual Phase-B INSERT rather than a helper approximation.

4. **Remove the false claim that rollback slot revocations revive on
   re-enable, or add a real reversible slot projection.** Section 17.11
   truthfully calls slot/invite projections one-way: after rollback projection,
   legacy slots have `revoked_at != NULL` and require explicit re-pairing.
   The final answer at line 3904 states the opposite: it says slot revocations
   “need not be undone,” that re-enabling makes status-aware lookup accept
   them, and that Phase B recreates credentials.

   I executed the stated sequence: project a disabled slot (`revoked_at=123`),
   set the orbit back to `active`, and run the exact status-aware lookup. It
   returns zero rows because the query requires `s.revoked_at IS NULL`; Phase B
   also explicitly skips revoked slots. Re-enable therefore does not restore
   either node auth or credentials.

   Choose one truthful contract. The conservative option is to keep slot
   projection one-way and state everywhere that post-rollback re-enable
   requires explicit trusted re-pair/re-provision. If credential preservation
   across rollback is required, journal each slot's prior revocation state and
   define generation-safe restoration before reconciliation; do not simply
   clear revocation on rows that the old binary may have rebound. Add project →
   old-binary run → new deploy → re-enable tests for both unchanged and rebound
   slots. Remove the stale “Phase A0 rollback-only” summary terminology.

5. **Complete the DPAPI ciphertext cleanup/fault contract and execute the
   missing R12 tests.** Rev 13 now adds the required 1 MiB ciphertext cap,
   premature-EOF rejection, and zero-progress `WriteFile` failure. However,
   step 6 still has no exact branch-by-branch cleanup for `GetFileSizeEx`
   failure, `ReadFile` error/zero progress before the expected length,
   `CryptUnprotectData` failure, plaintext-copy/framing/field failure, or
   `CloseHandle` failure on the read handle (lines 1011–1051). “Read-back
   fails” is not enough to prove every allocated handle and DPAPI output is
   released once. The test inventory never adds the R12-required oversized
   ciphertext, short-read, read-error, or handle-close fault tests; it only
   repeats framing and write-loop checks.

   Publish one ownership/cleanup table for temp handle, read handle, encrypted
   DPAPI output, decrypted DPAPI output, temp path, and destination path at
   every failure point. A successful zero-byte `ReadFile` before the captured
   size is premature EOF; a failing `CloseHandle` is reported and blocks send
   (even though the kernel handle may already be closed). Freeze whether the
   corrupt destination is retained, quarantined, or replaced on retry. Execute
   oversized/zero ciphertext, `GetFileSizeEx` error, short/zero-progress/error
   reads, decrypt error with/without output allocation, copy/framing/field
   error, and both temp/read `CloseHandle` faults, asserting exact close,
   delete, and `LocalFree` counts plus “network not called.”

## Independently verified accepted repairs

- Ordinary disable/restart is now non-destructive: A0 uses status-aware new-
  code authorization and does not mutate legacy slots.
- `binding_token_hash` is consistently an immutable generation snapshot and
  the collision path no longer logs credential hashes.
- Rotate stage 1 selects `slot_orbit_id`; the serving gate includes
  `ic.binding_token_hash = s.token_hash`.
- The projection journal now has one executable transaction. I ran the exact
  project → restore → quota change to `3/8` → project → restore sequence; the
  final quotas are correctly `3/8`.
- Behavior and FK validation are now placed before migration COMMIT.

## Resubmission

- Amend one authoritative note and replace the single canonical `research.md`
  outcome byte-identically; do not create a second outcome filename. Keep
  product source untouched and return to `to-review`, never `done`.
- Preserve the accepted non-destructive disable path, binding predicates,
  staged 401/403 policy, projection transaction, and pre-COMMIT validation.
- Execute the exact response SELECT, dependent-object rebuild, nullable Phase-B
  INSERT, rollback/re-enable, and DPAPI fault models. A downstream handoff or
  final table cannot override earlier normative SQL.
