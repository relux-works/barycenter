# Root review round 12 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 3,825 lines of Rev 12. The authoritative and
outcome copies are byte-identical (3,825 lines, 29,290 words, 226,335 bytes,
SHA-256 `403ffaffc6294e3820fc9081262187c2df614fdefda35760bdaf1ea2d26a509f`).
Product source remains untouched. Rev 12 improves DPAPI scoping, introduces a
real independent binding snapshot, and separates most 401/403 checks. However,
the disabled-orbit repair destroys valid credentials, the serving/migration SQL
still fails executable fixtures, and the rollback journal still publishes two
different algorithms.

## Blocking corrections

1. **Do not revoke every slot merely because an orbit is disabled.** Phase A0
   at lines 2761–2785 mutates every live legacy slot in every disabled orbit to
   `revoked_at != NULL`. The same transaction then lets A2 revoke each actor,
   delete its `installation_credentials`, and leave its membership. This is not
   a temporary authorization denial; it permanently destroys node and control
   bindings.

   It directly contradicts the frozen lifecycle: §5.3 says a previously valid
   control token in a disabled orbit remains authenticated, the context probe
   returns `403 insufficient_capability`, and the client retains it so access
   can be restored. §6 stage 1 requires an unrevoked slot; after A0 it returns
   zero rows and maps the same token to `401 unauthorized`. §15 promises the
   node token is preserved. §17.8.3 says reconciliation never modifies legacy
   tables, while A0 does exactly that.

   I executed the exact stage-1 join around A0: it returned one row before the
   UPDATE and zero afterward. Freeze a non-destructive new-code authorization
   repair instead: for playback/heartbeat/media, make the new-code lookup
   status-aware (or introduce a separate active-use lookup), while actor-context
   validation still recognizes the credential and maps disabled lifecycle to
   403. Keep the legacy slot and control binding intact so re-enable is
   possible. The one-way slot projection belongs only in the explicit
   pre-old-binary rollback runbook, not ordinary startup reconciliation.
   Reconcile any gap-minted slot before a later re-enable and require every
   flag-on mutator (`PairSlot`, `AddMember`, invite consume, etc.) to reject a
   disabled orbit. Execute disable → restart → context 403 → re-enable → same
   node/control credentials work, plus emergency-gap playback denial.

2. **Make `binding_token_hash` ownership truthful everywhere and never log the
   credential hash.** The executable DDL correctly adds
   `installation_credentials.binding_token_hash`, an immutable copy of
   `slots.token_hash`. But lines 30–34, 590–593, 2136, and 2151–2177 still say
   there is no duplication; the §16 column inventory omits the column entirely.
   These passages directly conflict with the downstream DDL and handoff.

   State one precise rule everywhere: `slots.token_hash` remains authoritative
   for node authentication; `binding_token_hash` is a duplicated immutable
   generation snapshot used only to prove current binding/collision identity;
   it is never updated in place and rebind deletes/recreates the credential.
   Update the key takeaways, hash contract, post-recovery table, schema
   ownership, backfill, and implementation handoff accordingly.

   The collision path at lines 2560–2570 also instructs logging both full
   `binding_token_hash` values. Those are credential hashes and conflict with
   the contract’s own secret-redaction rule. Fail closed while logging only
   safe actor/orbit/slot IDs and a non-credential diagnostic correlation ID;
   never emit the stored token hashes. Add a log-capture test.

3. **Fix the exact staged SQL and make the serving gate generation-aware.**
   Rotate stage 1 at lines 1438–1445 selects only
   `control_token_hash, revoked_at`, but stage 2 at line 1464 requires
   `slot_orbit_id` “from stage 1.” The shown implementation cannot bind the
   second query. Select the value explicitly, as the link-issuance query does.

   The serving-gate query at lines 1362–1371 joins credentials only by
   `(orbit_id, slot_name)`. A stale credential after same-coordinate rebind
   therefore satisfies the gate even though every endpoint subsequently
   rejects it. I executed the exact gate with a current slot hash and a
   different stored `binding_token_hash`; it returned zero violations. Join on
   `ic.binding_token_hash = s.token_hash` (and freeze any actor/membership
   invariants required before serving), so stale, missing, or duplicate binding
   state blocks startup. Execute the exact stage-1 and gate SQL, not a helper
   approximation, for fresh, revoked, missing, and same-coordinate-rebound
   rows.

4. **Use an executable SQLite rebuild with correct dependent-object and
   validation ordering.** Lines 2278–2318 collect every index, trigger, and
   view whose SQL merely contains `orbits`, drop the table, then execute every
   captured CREATE statement before commit. A dependent view is not dropped by
   `DROP TABLE`; recreating it fails with “view already exists.” I reproduced
   that failure using the shown create-new → copy → drop → rename sequence.
   SQLite documents that `DROP TABLE` automatically removes only indexes and
   triggers associated with the table, and its generalized ALTER procedure
   separately requires affected views to be dropped before recreation:
   https://www.sqlite.org/lang_droptable.html
   https://www.sqlite.org/lang_altertable.html#making_other_kinds_of_table_schema_changes

   Freeze exact object classification using schema ownership (`tbl_name`) and
   an explicit policy for external triggers/views: preserve+validate unchanged
   objects, or `DROP VIEW`/recreate affected views in the transaction. Do not
   use `sql LIKE '%orbits%'` as a dependency parser.

   Run the rebuilt status behavior probe and `PRAGMA foreign_key_check` before
   COMMIT so a failure can roll back; Rev 12 commits first and only then checks,
   leaving a committed broken migration. The interrupted branch “`orbits`
   missing, `orbits_new` exists” cannot recreate dependent DDL captured only in
   memory before a prior crash. Since transactional SQLite DDL should roll back
   on an actual process crash, treat unexplained non-transactional shapes as
   fatal unless a durable migration journal proves how to recover. Test a view,
   an index owned by `orbits`, a trigger owned by `orbits`, an external trigger,
   and injected behavior/FK failure before commit.

5. **Publish one projection-journal transaction; the first “normative” one
   still breaks cycle two.** Lines 3301–3320 call the projection procedure one
   transaction and use unconditional `ON CONFLICT(orbit_id) DO NOTHING`. Lines
   3344–3350 then acknowledge that this cannot create a new pending cycle after
   restoration, and lines 3352–3375 offer separate replacement fragments.
   There are still two competing implementations, not an exact state machine.

   I executed the first shown procedure for project → restore → change quotas
   to 3/8 → project → restore. The second projection set the orbit to 0/0,
   retained the old restored 5/10 journal row, and the second restoration found
   no pending row; final quotas remained 0/0. Replace all variants with one
   complete `BEGIN IMMEDIATE` transaction that safely retires a restored row,
   creates a pending generation only when none exists, preserves an existing
   pending original on rerun, applies every projection, and commits. Then show
   the matching restoration transaction. Run the two-cycle test against those
   exact statements.

6. **Bound and complete the DPAPI ciphertext I/O, not only the decrypted
   payload.** Rev 12 caps plaintext payload at 16 KiB but step 6 says “read the
   entire file” before decrypting and provides no ciphertext file-size cap. A
   corrupt or locally replaced multi-gigabyte file can force unbounded
   allocation/read before framing is checked. Call `GetFileSizeEx` first,
   reject zero/oversized encrypted files with a frozen conservative maximum,
   then use an exact complete-read loop and reject premature EOF or extra data.

   The `WriteFile` loop must also treat a successful zero-byte write as an error
   rather than looping forever. Freeze `CloseHandle`/delete/`LocalFree` cleanup
   for every read, decrypt, copy, framing, and field-validation failure; a temp
   close failure means abort before `MoveFileExW`. Add oversized ciphertext,
   short-read, zero-progress write, read-error, and handle-close fault tests.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; keep product
  source untouched and return to `to-review`, never `done`.
- Preserve the accepted recovery generation transaction, current-user DPAPI
  flags, independent binding snapshot, status behavior probe, source-first
  rebind cleanup, staged 401/403 policy, and safe table-swap order.
- Execute the disabled/re-enable lifecycle, exact endpoint/gate SQL, dependent
  view rebuild, second projection cycle, and bounded ciphertext I/O. Summary
  claims and later corrective paragraphs do not override earlier normative SQL.
