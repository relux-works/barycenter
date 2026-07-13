# Root review round 11 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 3,438 lines of Rev 11. The authoritative and
outcome copies are byte-identical (3,438 lines, 26,294 words, 202,566 bytes,
SHA-256 `be69d3d7e80e489d200949b9e3fc6fa7bb3b5a68c62217f64c09a23f7fb85edf`).
Product source remains untouched. Rev 11 fixes the endpoint live-slot joins,
the unsafe rename-old-first SQLite order, the recovery response shape, origin
vectors, NULL `paired_at`, and the short fingerprint. It still is not an
executable security contract: five prior blockers survive in their algorithms,
and two new contradictions were introduced by the attempted repairs.

## Blocking corrections

1. **Freeze a complete per-user Windows DPAPI write algorithm.** The sequence
   at lines 905–927 names only `FILE_ATTRIBUTE_NORMAL |
   FILE_FLAG_WRITE_THROUGH` and a nonexistent symbolic value
   `FILE_SHARE_NONE`; it omits `dwDesiredAccess` and `dwCreationDisposition`.
   It also permits `CRYPTPROTECT_LOCAL_MACHINE or user scope as appropriate`.
   This is a control/recovery credential for the interactive user, so the scope
   is not an implementation choice. Microsoft documents that data protected
   with `CRYPTPROTECT_LOCAL_MACHINE` can be decrypted by **any user on that
   computer**:
   https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata

   Mandate current-user DPAPI scope by omitting
   `CRYPTPROTECT_LOCAL_MACHINE`; specify whether
   `CRYPTPROTECT_UI_FORBIDDEN` is required. Freeze every `CreateFileW`
   parameter: temp open with `GENERIC_WRITE`, share mode `0`, `CREATE_NEW`, and
   the exact flags; destination read-back with `GENERIC_READ`, the chosen share
   mode, `OPEN_EXISTING`, and exact flags. Specify the complete-write loop and
   byte-count checks, maximum/expected blob framing and rejection of truncated
   or trailing data, `CloseHandle` handling, and `LocalFree` for DPAPI output:
   https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew

   The short protocol at lines 686–691 must name the same flush-before-close
   barrier as the normative algorithm. Preserve `FlushFileBuffers` → close →
   `MoveFileExW(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)` → exclusive
   reopen/decrypt/exact read-back; any failure means no network send. Add tests
   that assert the exact DPAPI flags and access/share/disposition values, not
   only the call order.

2. **Detect and rebuild `orbits.status` by behavior and an exact schema, not a
   substring or `table_info` reconstruction.** Lines 2099–2103 still decide
   that the constraint exists by searching `sqlite_master.sql` for one exact
   whitespace-sensitive spelling. The only behavior probe occurs after a
   rebuild, so a formatted constraint, a comment containing that text, or a
   semantically different expression can select the wrong path.

   Before declaring the existing column constrained, execute a rolled-back
   behavior probe that proves an otherwise-valid row/update with
   `status='bogus'` fails with the expected constraint error. Freeze the empty-
   table probe too. If rebuilding, do not derive `CREATE TABLE` from
   `PRAGMA table_info`: it does not preserve CHECKs, FKs, collations, generated
   columns, or table options, and the shown `... original columns ..., status`
   can duplicate an already-present `status` column. Use the exact known live
   schema or an actual schema-preserving parser and explicitly replace the
   status definition.

   Capture dependent index/trigger/view DDL before `DROP TABLE` and recreate it
   **inside the same exclusive migration transaction before COMMIT**, then run
   the behavior probe and `foreign_key_check`. “Recreate after rebuild” leaves
   a committed crash window with missing objects. Keep the now-correct
   create-new → copy → drop-old → rename-new order and the out-of-transaction
   `foreign_keys` toggle. Add fixtures for alternate whitespace, a misleading
   comment, equivalent/non-equivalent constraints, an empty table, dependent
   objects, and each crash edge. SQLite’s normative procedure is:
   https://www.sqlite.org/lang_altertable.html#making_other_kinds_of_table_schema_changes

3. **Make the rollback projection journal genuinely idempotent.** Lines
   3039–3044 use `INSERT OR REPLACE ... SELECT max_pulsars,max_members` and then
   set both values to zero, while lines 3064–3068 claim re-running that
   projection is safe. I executed the shown SQL against an orbit with quotas
   `5/10`: the second projection replaced the journal originals with `0/0`, and
   restoration produced `0/0`.

   Freeze an explicit projection generation/state machine. A rerun while a
   pending projection exists must preserve its original values (for example,
   conditional UPSERT/DO NOTHING for an unrestored row); a new projection after
   a completed restoration must create a new generation without confusing the
   old one. Specify crash behavior before/after journal insertion, legacy
   projection, and restoration marking. Test `project → project again → restore`
   and prove the exact original quotas return; also test two complete projection
   cycles and a user-changed quota between cycles. Do not call a destructive
   `OR REPLACE` sequence idempotent.

4. **Repair the disabled-orbit emergency-gap algorithm, not only its test
   prose.** Phase B at lines 2581–2596 creates an ordinary unrevoked actor and
   credential for every unmatched unrevoked slot. It has no disabled-orbit
   branch. Yet lines 3116–3124 say an “orbit-alignment check” immediately
   revokes that actor and then claim `LookupToken` fails. Live
   `coordinator/internal/store/orbits.go:616`–`626` queries only
   `slots.token_hash` and `slots.revoked_at`; it neither reads the actor nor
   `orbits.status`. Thus a gap-minted unrevoked slot remains accepted. This is
   the same prior blocker, now hidden behind a nonexistent step.

   Choose and freeze an executable repair: either reconciliation/projected
   state revokes the legacy slot before serving, or every new-code node-token
   authorization caller uses a status-aware query and raw `LookupToken` is no
   longer security-authoritative. If Phase B creates a revoked audit actor,
   put that branch in Phase B and define whether a credential row exists. List
   every playback/heartbeat/media caller affected. The test must invoke the
   real auth path and prove a token minted during the old-binary gap cannot be
   used after new-code startup. Assertions about additive actor state do not
   make the live legacy lookup fail.

5. **Store an independent binding identity and use it in endpoint joins.** The
   collision handler at lines 2341–2351 is tautological. A uniqueness conflict
   means the new `external_ref` equals the existing one; extracting its
   fingerprint and comparing it with the same fingerprint derived for the
   current token will always match, including a real fingerprint collision.
   The old credential may already have been deleted, so no independent old
   preimage remains. The handler therefore silently reuses the wrong actor in
   the very case it claims to fail closed.

   Persist an immutable independent binding value (for example the canonical
   full `slots.token_hash`, which is already stored in the database) on the
   actor/binding record, or use that full system identity directly in
   `external_ref`. Define exact comparison and migration rules. Add an injected
   conflict with equal fingerprint but different stored binding value; it must
   abort.

   Use the same generation identity in §6/§7/§10 endpoint SQL. The new joins
   check only `(orbit_id, slot_name, revoked_at)`. I executed the §6 shape with
   a credential for the old generation and a new unrevoked slot at the same
   coordinate; it still returned the old actor. Therefore the promised
   same-coordinate-rebind zero-row test at lines 3312–3318 cannot pass. Add an
   exact binding predicate (not only reconciliation) and execute all three
   normative queries against a rebind that has not yet been reconciled.

6. **Make the serving gate and node context response mutually consistent.**
   Lines 1234–1238 allow an unbackfilled legacy slot to return `200` with only
   orbit/slot context. The endpoint’s frozen success schema everywhere else is
   `{orbit_id, actor_id, role}`, and the pending-token client requires that
   schema. No `actor_id` or `role` exists in the allowed branch. If startup
   backfill/reconciliation is a serving prerequisite, make an unbackfilled live
   slot a failed startup invariant and use an inner generation-bound join. If a
   compatibility mode is intentionally served, define a distinct exact response
   schema and prove every consumer handles it. Do not return fields that cannot
   be produced.

7. **Separate invalid credential binding from valid-token lifecycle errors in
   every authenticated endpoint.** Section 6 maps a missing/revoked/mismatched
   live-slot binding to `401 unauthorized`, but rotate at lines 1313–1315 (and
   the corresponding link-issuance path) maps the same failed join to
   `403 insufficient_capability`. A single inner join also conflates invalid
   credential binding, missing membership, and orbit mismatch. This contradicts
   the uniform error table: revoked/stale bearer credentials are `401`; a valid
   credential lacking role/lifecycle authority is `403`.

   Freeze a staged query or LEFT-join decision tree that first validates the
   current generation-bound credential and actor, then evaluates membership,
   role, and orbit status. Apply it consistently to context, rotate, and link
   issuance, both before and inside mutation transactions. Add table-driven
   tests for missing credential, revoked slot, same-coordinate rebind, revoked
   actor, left/missing membership, orbit mismatch, disabled orbit, satellite,
   and valid companion/primary.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; keep product
  source untouched and return to `to-review`, never `done`.
- Preserve the accepted consume/replay transaction, recovery response and
  canonical-origin vectors, full live-slot joins, source-first reconciliation,
  NULL-`paired_at` handling, safe SQLite table-swap order, writer inventory,
  limiter ordering, and no-node-escalation policy.
- Execute, rather than narrate, the per-user durable-file state machine,
  existing-constraint behavior probes, projection rerun/restore, disabled-gap
  auth path, independent collision verifier, same-coordinate endpoint SQL, and
  uniform 401/403 matrix. Include the commands/results in the revision.
