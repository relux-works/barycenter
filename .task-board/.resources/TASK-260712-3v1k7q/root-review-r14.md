# Root review round 14 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I reviewed the Rev 14 amendments against the previously
full-read 3,906-line Rev 13, reread every corrected normative/test section,
and verified the authoritative and canonical outcome copies are byte-identical
(4,111 lines, 32,057 words, 246,924 bytes, SHA-256
`8a39cc6c0151013949e0342b546ee906d1ce82049551aed3be9361d9e3bf488e`).
Product source remains untouched. Rev 14 fixes the missing recovery orbit ID,
nullable Phase-B timestamp, rollback re-enable claim, and most DPAPI ownership
branches. Two R13 blockers remain in the candidate itself.

## Blocking corrections

1. **Make the SQLite rebuild dependency inventory deterministic and restore
   foreign-key enforcement on every exit.** Rev 14 now selects `tbl_name` and
   correctly classifies indexes/triggers owned by `orbits` (lines 2402–2422).
   It still decides whether a view or external trigger depends on `orbits` by
   checking whether free-form stored SQL “contains the rebuilt table name as a
   token,” explicitly calling this a heuristic (lines 2423–2443). No lexer,
   quoting/schema-qualification rules, or false-negative policy is defined.
   The tests cover a comment/string false positive (lines 2555–2559), but not
   quoted identifiers, schema-qualified identifiers, nested CTEs, or any other
   false-negative. This is the dependency parser R13 required the revision to
   remove or specify fully.

   The rebuild body is contradictory about the resulting set: it preserves
   nondependent views/triggers unchanged (lines 2430–2436), then step 3 says to
   recreate **all** dependent objects and labels every captured view and trigger
   for `CREATE` (lines 2475–2480). Recreating an object that was preserved fails
   with “already exists”; omitting a true dependency can make `ALTER TABLE ...
   RENAME` fail while the schema contains an invalid view/trigger.

   Choose one executable strategy. The conservative strategy is simplest:
   capture exact DDL for every user-defined view and every external trigger,
   drop all of them inside the same transaction, rebuild, then recreate all
   exact captured DDL; owned indexes/triggers remain classified by `tbl_name`
   and auto-indexes excluded. If selective preservation is required, specify
   and test a real SQLite SQL lexer/parser rather than a prose token heuristic.

   Foreign-key restoration is still missing on rollback/abort. The shown path
   executes `PRAGMA foreign_keys=OFF` before `BEGIN`, but executes
   `PRAGMA foreign_keys=ON` only after `COMMIT` (lines 2452–2488). Validation
   failures explicitly `ROLLBACK` and abort (lines 2481–2485, 2501–2503) with
   no restoration branch. I executed the exact control shape; after
   OFF → BEGIN → ROLLBACK, `PRAGMA foreign_keys` returns `0`. The reproducible
   check is `.research/root-checks/recovery-r14-foreign-keys.sql`. This violates
   the global requirement at line 129 that every connection has FK enforcement
   enabled. Freeze one defer/finally cleanup that restores the captured prior
   setting (or required ON setting) after COMMIT, ROLLBACK, every SQL error,
   behavior-probe failure, panic/exception boundary, and intermediate-state
   abort; test each exit with `PRAGMA foreign_keys == 1`.

2. **Resolve the read-handle close failure and send-barrier contradiction.**
   Rev 14 adds the requested resource table and detailed read/decrypt/framing
   branches. But step 6i says a failing `CloseHandle` after successful read-back
   does **not** prevent sending (lines 1086–1090), while the send barrier says
   all of steps 1–6 must succeed before network begins (lines 1096–1098), and
   the fault-test rule requires “network not called” for **every** failure path
   (lines 1146–1152). The summary simultaneously claims no exit can leave an
   open handle leaked (lines 1092–1094), which cannot be proven after
   `CloseHandle` reports failure.

   Freeze one policy. The conservative policy already requested by R13 is:
   any read-handle `CloseHandle` failure is fatal for this attempt, blocks the
   network call, records the OS error, and escalates/restarts rather than
   pretending exact cleanup was proven. If the product intentionally sends
   despite that failure, then remove the all-steps-success/no-network-on-every-
   failure claims, explicitly accept the unproven handle state, and make the
   fault oracle expect a send. Do not keep both policies. Execute the read-
   close fault and assert one exact result plus exact resource counts.

## Independently verified accepted repairs

- The recovery lookup now selects `slot_orbit_id`, and the lifecycle query
  selects `ic.slot_orbit_id AS orbit_id` inside the same transaction. Both
  success paths have a transaction-consistent orbit value.
- Phase B now mandates `COALESCE(slots.paired_at, 0)` and the nullable fixture
  executes the runtime Phase-B INSERT contract.
- Rollback projection is now described truthfully as one-way for revoked
  slots: re-enable requires explicit trusted re-pair/re-provision; Phase B does
  not revive revoked rows. Live `PairSlot` source confirms revoked letters can
  be reused only after limits are restored.
- The DPAPI contract now includes the 1 MiB cap, zero/short/error reads,
  decrypt allocation branches, framing/field cleanup, zero-progress writes,
  and exact ownership rows. Only the read-close/send decision remains split.

## Resubmission

- Amend the one authoritative note, replace the single canonical `research.md`
  outcome byte-identically, keep product source untouched, and return to
  `to-review`, never `done`.
- Preserve all accepted Rev 14 repairs. Do not rewrite unrelated accepted
  sections.
- Execute the all-dependent-object rebuild matrix, every FK restore exit, and
  the read-handle close/send fault oracle. A prose “heuristic” or summary claim
  is not an executable migration/resource contract.
