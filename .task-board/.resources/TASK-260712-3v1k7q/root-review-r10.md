# Root review round 10 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. I read all 3,068 lines of Rev 10. The authoritative and
outcome copies are byte-identical (3,068 lines, 23,548 words, 180,443 bytes,
SHA-256 `b015e03dee6cd8d35e71e1700251baf47577705340d46356cfc2f7cd7d62313f`).
Product source remains untouched. Rev 10 fixes the 32-bit binding marker,
restores non-null slot ownership, and makes revoked-slot reconciliation
source-first. Four R9 safety blockers remain executable failures, and the new
target/rollback text contains additional contradictions.

## Blocking corrections

1. **Finish the Windows durable-before-send barrier; `ReplaceFile` is not a
   write-through alternative.** Sections 5.1 and 5.1.2 still permit
   `ReplaceFile` after flushing only the temp file, begin the request as soon as
   that call returns, omit the required destination read-back, and claim that a
   cleanup file is automatically recovered on the next call. Microsoft
   documents `REPLACEFILE_WRITE_THROUGH` as unsupported and documents several
   partial failure layouts for `ReplaceFileW`; the optional backup file is not
   an automatic recovery protocol:
   https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilew

   Freeze one supported, executable persistence algorithm, including exact
   `CreateFile` flags/share modes, same-volume temp name, complete write/close,
   `FlushFileBuffers`, supported replacement flags, handling of every partial
   failure layout, reopen + decrypt + schema/field read-back of the destination,
   and cleanup. `MoveFileExW(MOVEFILE_REPLACE_EXISTING |
   MOVEFILE_WRITE_THROUGH)` is documented, unlike the unsupported ReplaceFile
   flag, but the contract must still prove the resulting destination and exact
   `ever_sent=true` record before allowing network I/O:
   https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw
   Any write, flush, replace, reopen, decrypt, or read-back failure means **no
   send**. Test process crash and power loss at every edge; do not infer
   durability merely from atomic namespace replacement.

2. **The normative context/rotate/issuance SQL still accepts a revoked slot.**
   Contrary to the takeaway and downstream summary, the exact queries at
   lines 1058–1065 (§6), 1122–1130 (§7), and 1429–1437 (§10) do not join
   `slots` at all. I executed those three queries with an active actor,
   membership, orbit, and credential whose referenced slot has
   `revoked_at=1234`; all three returned one row. Only recovery consume §5.2
   contains the required live-slot join.

   Put the same exact current-binding predicate in every normative control
   path:
   `JOIN slots s ON s.orbit_id=ic.slot_orbit_id AND s.slot=ic.slot_name AND
   s.revoked_at IS NULL`. Freeze the node-token context query too: it must start
   from the matching unrevoked slot and resolve the current credential/actor
   generation, rather than vaguely “fall through” to `LookupToken`. Add direct
   tests of the shown endpoint SQL for slot revoke, same-coordinate rebind,
   missing slot, and membership mismatch. A standalone reconciliation helper is
   not a substitute for endpoint fail-closed behavior.

3. **Replace the invalid `orbits.status` rebuild sequence.** Section 17.1 uses
   the unsafe order `RENAME orbits TO orbits_backup` → create a new `orbits` →
   copy → drop backup. On modern SQLite, renaming a parent table rewrites child
   FK declarations to reference the new name. I executed the proposed sequence
   under SQLite 3.43.2 with `foreign_keys=ON`: `memberships` was rewritten to
   `REFERENCES "orbits_backup"(id)`, and `DROP TABLE orbits_backup` failed with
   `FOREIGN KEY constraint failed`. SQLite explicitly labels the
   rename-old-first sequence incorrect and specifies create-new → copy → drop
   old → rename-new as the safe pattern:
   https://www.sqlite.org/lang_altertable.html#making_other_kinds_of_table_schema_changes

   Freeze an actually executable, connection-exclusive migration. If using the
   generalized rebuild, account for the rule that `PRAGMA foreign_keys` cannot
   be toggled inside a transaction, preserve/recreate dependent indexes,
   triggers, and views, run `foreign_key_check`, and define restart behavior.
   A validated trigger alternative is acceptable. Do not identify effective
   enforcement by a whitespace-sensitive substring alone; test behavior and
   schema shape. Add the exact child-FK fixture above.

4. **The old-binary projection is still not fail-closed or recoverable as
   written.** Section 17.11 permits `max_members = current member count`. After
   one existing member leaves, live `AddMember` observes `count < max_members`;
   an existing member can mint a fresh invite after the projection and a new
   member can join the disabled orbit. Require `max_members=0`, without the
   current-count alternative, and test project → old member leaves → new invite
   → consume/AddMember remains blocked.

   The procedure also destroys `max_pulsars`/`max_members` but never defines the
   promised durable store for their original values. “Stored in additive state
   or configuration” is not an algorithm or schema. Define an idempotent
   projection journal with exact DDL, transaction boundary, generation/state,
   crash recovery, and guarded restoration; otherwise a restart cannot know
   whether zero was a product setting or a projection. Finally, the emergency
   test says reconciliation creates a revoked actor for a slot minted in a
   disabled orbit, while Phase B creates an ordinary unrevoked actor for every
   unmatched unrevoked slot and never checks orbit status. Choose one behavior
   and make the algorithm and test identical.

5. **Freeze one recovery-export response and one cross-platform origin
   canonicalizer.** Section 4 requires both create and rotate to return
   `actor_id`, but the normative §7 success body omits it; the downstream note
   later adds it back. Since `(canonical_origin, actor_id)` is the ambiguity
   key, this cannot remain derivative prose. Show the exact create and rotate
   JSON schemas, require export to include the same target, and add round-trip
   tests.

   `{scheme}://{idna_lowercase_host}:{effective_port}` is also incomplete for
   two independent client implementations: no IDNA profile/transitional mode,
   trailing-root-dot rule, IPv6 bracket/zone handling, userinfo rejection, or
   malformed/opaque URL policy is frozen. Select one standard URL-origin
   serialization and exact IDNA operation, then publish shared vectors for
   default ports, Unicode/punycode, case, trailing dots, IPv4, bracketed IPv6,
   loopback, userinfo, paths, queries, and fragments. Equivalent coordinator
   URLs must produce byte-identical protected-store keys on macOS and Windows.

6. **Make generation backfill executable for the live nullable legacy column,
   and finish the fingerprint contract.** Live `slots.paired_at` is nullable,
   while Rev 10 copies it into `installation_credentials.slot_paired_at INTEGER
   NOT NULL`. A schema-valid legacy row with `paired_at=NULL` therefore cannot
   pass the promised backfill; the handcrafted-legacy test does not cover it.
   Freeze a deterministic policy (for example an explicit sentinel plus the
   full fingerprint as the authoritative generation, or a nullable field with
   null-safe comparison), and test NULL → first reconciliation → second-pass
   no-op → later rebind.

   R9 requested a domain-separated full-strength binding identity (or a shorter
   marker with explicit collision handling). Rev 10 uses raw
   `SHA-256(token_hash)` and calls different inputs cryptographically unique,
   but gives no domain tag or conflict response. Freeze a versioned domain
   string in the exact hash input and, on `(kind, external_ref)` conflict,
   verify whether the current binding is truly the same; fail closed on a
   mismatch. Keep the full 64-hex output and the first-eight-hex collision test.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; keep product
  source untouched and return to `to-review`, never `done`.
- Preserve the accepted generation-bound recovery transaction, stale-bearer and
  consume-time issuer checks, non-null slot ownership, source-first two-pass
  reconciliation, full writer inventory, same-orbit conflict behavior, limiter
  ordering, and no-node-escalation policy.
- Execute the durable file state machine, every endpoint SQL statement, the
  child-FK status migration, leave-after-projection rollback attack, canonical
  origin vectors, and NULL-`paired_at` backfill. Assertions in summaries do not
  replace these proofs.
