## Status
done

## Assigned To
[analyst] researcher (claude)

## Created
2026-07-12T15:30:16Z

## Last Update
2026-07-12T22:45:12Z

## Blocked By
- (none)

## Blocks
- TASK-260712-m5264f
- TASK-260712-2xkyot
- TASK-260712-2u1w16
- TASK-260712-47uve0

## Checklist
- [x] Write the contract note for recovery and Telegram link flows
- [x] Specify recovery rotation and control-token revocation rules
- [x] Specify Telegram role selection and conflict behavior
- [x] Freeze one-time recovery display and explicit nonpersistence semantics
- [x] Freeze uniform errors, brute-force protection and concurrent single-use Telegram link handling
- [x] Findings written to file
- [x] Key aspects highlighted
- [x] Fact-checking performed — claims verified, sources cited
- [x] Document linked as outcome resource
- [x] All questions from task description answered

## Notes
Wrote .research/260712-recovery-telegram-link-contract.md. Froze recovery consume/rotate endpoints, one-time display and nonpersistence, control-only recovery plus control-token revocation, uniform secret failure envelope and rate limits, Telegram desired_role/defaults, private-chat-only consume, same-orbit and foreign-orbit conflicts, and concurrent single-use semantics. Attached the note as an outcome resource.
agent completed: [analyst] researcher (codex) (exit=0)
agent spawned: codex (pid=3555, exit=0)
Root review round 1: changes required. Address every blocker in attached root-review-r1.md, amend the existing contract and outcome byte-identically, keep source code untouched, complete the checklist, then return to-review.
agent completed: [analyst] researcher (codex) (exit=1)
agent spawned: codex (pid=9504, exit=1)
R1 review findings addressed: (1) entropy corrected to 27 chars via rejection sampling = 132.49 bits; (2) retry-safe recovery via client-generated replacement_control_token with idempotent retry; (3) role reflects current active membership, never hard-coded primary; (4) Telegram consume restricted to trusted adapter only; (5) complete JSON error envelope with exact schema, Retry-After, bounded limiter keys; (6) authorization matrix for link issuance frozen; (7) opaque ID formats and scalar types defined; (8) all checklist items checked. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=9952, exit=0)
Root review round 2: still rejected. Address every blocker in attached root-review-r2.md, leave one byte-identical authoritative outcome, keep source code untouched, then return to-review.
R2 corrections addressed: (1) client crash gap — pending credential protocol in Keychain/DPAPI before destructive request, with promotion/retry/discard semantics; (2) orbit_id and actor_id use existing INTEGER types, no orb_/act_ strings; (3) recovery_id increased to 128-bit (rec_ + 32 hex); (4) single at-rest hash: HMAC-SHA-256 with 256-bit server key, consistent for all secrets, dummy hash for timing equalization; (5) exact alphabet regex [ABCDEFGHJKMNPQRSTVWXYZ2-9] rejects I/L/O/U; (6) limiter ordering corrected — source-IP before per-ID, LRU cap bounds state, restart resets accepted; (7) Telegram consume frozen as in-process service method, trust from authenticated TLS transport not Update object; (8) companion may issue both companion and satellite links, primary never granted; (9) HTTPS required, Cache-Control: no-store, bodies excluded from logs, 401 for bearer failures, bot deletes consumed code messages; (10) RFC 7235/FIDO2 citations removed, protocol explained from invariants; (11) single authoritative outcome resource. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=12207, exit=0)
Root review round 3: still rejected. Address the four blockers in root-review-r3.md, keep the single outcome byte-identical, keep source code untouched, then return to-review.
R3 corrections addressed: (1) at-rest hash changed from HMAC-SHA-256 to unkeyed SHA-256, consistent with existing hashToken() at orbits.go:105 — removed HMAC key file, impossible rotation claim, and key-related failure modes; (2) added GET /v1/actor/context probe endpoint — client on restart probes pending token before deciding whether to retry recovery, 403 from recovery alone never deletes pending state, Cancel requires destructive-abandon confirmation unless probe confirmed 401; (3) schema ownership separated — actors hold identity only, installation_credentials holds control/recovery hashes with FK to slots for node token hash, no duplication of slots.token_hash, single control_token_hash overwrite is the sole revocation mechanism; (4) authorization errors made consistent — disabled orbit uses 403 insufficient_capability (not credential_invalid), attempt limits made concurrency-safe by atomically counting all syntactically valid attempts before hash work. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=17494, exit=0)
Root review round 4: rejected. Address every blocker in root-review-r4.md, preserve one byte-identical outcome, keep product source untouched, then return to-review.
R5 revision addressing all R4 blocking corrections: (1) Corrected hash input bytes — SHA-256 of the 64-char ASCII hex string, NOT hex-decoded 32 bytes; added concrete test vectors (hashToken("000...0") and hashToken("ABCDEFG...")); froze TEXT storage format for all additive tables; added LookupToken compatibility test requirement. (2) Added ever_sent marker to pending state; once true, pending credential is NEVER auto-deleted; removed step 5/7d deletion branches; terminal conditions: promote, supersede, or user-confirmed destructive abandon only. (3) Complete probe response table covering 200, 401, 403 insufficient_capability, 429, 5xx, network failure; 403 proves authentication and triggers promote-with-limited-context; race note explains why 401+403 sequence cannot safely delete pending state. (4) Explicit additive/coexistence schema: legacy tables remain intact, backfill is idempotent, credential columns nullable for unprovisioned installations, role derivation from paired_by frozen, uniqueness constraints enumerated, feature-flag-off rollback is safe. (5) Explicit conditional write (UPDATE WHERE consumed_at IS NULL + RowsAffected) selects the single winner; losers fall to idempotency path; lifecycle checks apply consistently to both first consume and idempotent replay.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=21620, exit=0)
Root review round 5: rejected. Address every blocker in root-review-r5.md, keep one byte-identical outcome and product source untouched, then return to-review.
R6 revision addressing all R5 blocking corrections: (1) Recovery consume is now generation-bound — conditional write predicates on recovery_id AND consumed_at IS NULL, not merely actor_id; concurrent rotation causes old consume to match zero rows; loser reloads current state before any idempotency comparison, never uses stale pre-rotation fields; corrected false SQLite row-level locking claim to database-level writer serialization. (2) Single-row rotation model frozen — rotation atomically overwrites recovery_id, recovery_secret_hash, resets consumed_at=NULL on the same installation_credentials row; no multi-row generation history; old recovery_id permanently invalid; rotation-vs-consume serialization proven via recovery_id predicate mismatch; collision retry; idempotent replay lifetime ends at next rotation. (3) Telegram link consume is a rollback-safe transaction — BEGIN IMMEDIATE wraps actor resolution, conflict checks, conditional code reservation (with invalidated_at IS NULL AND expires_at > ? predicates), membership creation, legacy dual-write, and audit; any failure rolls back code consumption; added invalidated_at to telegram_link_codes for reissue revocation; two-code/same-user race handled via database partial unique index on memberships. (4) No node-token control escalation — removed upgrade path where node_token auth provisions control/recovery; legacy installations must obtain control through device invite from primary/companion, explicit Telegram-owner authorization, or another separately-authorized proof; node possession alone is insufficient; added negative test requirement. (5) Conservative database-enforced backfill and coexistence — orphan/inconsistent slots backfill as satellite (not companion) to prevent unauthorized link issuance; backfilled role is informational until separately authorized provisioning; memberships_one_active partial unique index replaces application-level check (race-proof); new/reactivated Telegram memberships dual-write to legacy members table transactionally; feature-flag-off and previous coordinator see consistent data. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=25192, exit=0)
Root review round 6: rejected. Address every blocker in root-review-r6.md, keep one byte-identical outcome and product source untouched, then return to-review.
R7 revision addressing all R6 blocking corrections: (1) explicit BEGIN IMMEDIATE transactions for recovery consume, rotate, link issuance, and link consume; (2) correct same-code/two-user behavior (winner commits, code stays consumed); (3) conflict-safe UPSERT replacing INSERT OR REPLACE for legacy dual-write; (4) reconciliation policy after rollback to old coordinator; (5) orbits.status additive column and PRAGMA foreign_keys=ON; (6) authorization = token capability × role policy with capability×role matrix. All test requirements updated for barrier tests, reconciliation cycle, FK violation, and ActorContext matrix.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=32528, exit=0)
Root review round 7: rejected. Address every blocker in root-review-r7.md, keep one byte-identical outcome and product source untouched, then return to-review.
R8 revision addressing all R7 blocking corrections. Outcome resource byte-identical sha256 ddcfc14e, 2458 lines. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=42991, exit=0)
Root review round 8: rejected. Address every blocker in root-review-r8.md, keep one byte-identical outcome and product source untouched, then return to-review.
R9 revision addressing all R8 blocking corrections: (1) slot rebind made generation-safe — external_ref uses binding fingerprint SHA-256(token_hash)[:8] as collision-proof generation marker; old credential row DELETED (not just nulled) to release UNIQUE(slot_orbit_id, slot_name); orbit-alignment invariant added with fail-closed mismatch handling; (2) deleted-orbit reconciliation now DELETEs memberships instead of setting left_at, satisfying FK to orbits(id); explicit child-first deletion order; (3) full legacy mutation surface covered: CreateOrbit (Telegram actor only, no slot), AddMember, SetMemberName added; TransferPrimary updates both Telegram and installation actors; LeaveOrbit handles promoted Telegram actor; new Create Barycenter service-level transaction frozen; (4) constrained fail-fast DDL: CHECK(status IN ('active','disabled')) on orbits.status, consumed-state all-or-none CHECK on telegram_link_codes, duplicate-column error distinguished from fatal errors; (5) pending-recovery double-start fixed: lone 401 NEVER permits auto-delete while ever_sent is true; pending keyed by (coordinator_origin, recovery_id); at most one unresolved ever_sent candidate per target installation; Windows persistence uses atomic file-replace (write-to-temp + NTFS rename); (6) rate-limit ordering unified: auth → bounded syntax validation → atomic reservation → generation → writer transaction for ALL endpoints; 400 never touches limiter; (7) old-binary rollback acknowledged NOT unconditionally safe; fail-closed procedure: stop ingress → project disable/revoke into legacy slots.revoked_at → deploy old binary; known gap documented. Outcome resource byte-identical sha256 78ce4d49, 2754 lines. Source code untouched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=52594, exit=0)
Root review round 9: rejected after complete 2754-line read and executable SQLite/source checks. Address every blocker in root-review-r9.md; preserve accepted invariants; keep product source untouched; reattach one byte-identical outcome and return to-review.
Rev 10 (R9 amendments). 7 corrections applied: (1) binding fingerprint expanded from truncated 8-hex (32-bit) to full 64-hex SHA-256 digest — eliminates collision risk for same-millisecond rebinds; (2) pending recovery target scoped by (canonical_coordinator_origin, actor_id) instead of coordinator_origin alone — actor_id added to recovery export, canonical origin specified with IDNA/port normalization, Windows durable write barrier requires FlushFileBuffers before ReplaceFile before network send; (3) slot_orbit_id and slot_name restored to NOT NULL in DDL — rows are deleted on rebind/revoke, never nulled; (4) slot reconciliation reordered to source-first: Phase A (revoke/delete for revoked, rebound, dissolved) before Phase B (create for unmatched unrevoked only) — prevents revoked slots from being re-created as active actors on next pass; two-pass idempotency test required; (5) orbit-alignment join added to all normative endpoint algorithms (§5.2, §6, §7, §10) with exact SQL — lifecycle checks now explicitly join installation_credentials.slot_orbit_id to memberships and orbits; (6) orbits.status migration handles three cases: absent, present with CHECK, present WITHOUT CHECK — validates existing values and rebuilds table if needed, aborts on invalid data; (7) rollback projection expanded: max_pulsars=0 blocks PairSlot, max_members=0 blocks AddMember, invites burned — prevents old PairSlot from minting new node tokens in disabled orbits; emergency rollback without projections requires keeping affected tenants offline. Authoritative/outcome copies byte-identical: b015e03dee6cd8d35e71e1700251baf47577705340d46356cfc2f7cd7d62313f. 3068 lines. No source code touched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=61230, exit=0)
Root review round 10: rejected after complete 3068-line read, byte/SHA verification, live-source inspection, official Win32/SQLite checks, and executable SQLite reproductions. Address every blocker in root-review-r10.md; preserve accepted invariants; keep product source untouched; reattach one byte-identical outcome and return to-review.
Rev 11 (R10 amendments). 3,438 lines, 26,294 words, 202,566 bytes, SHA-256 be69d3d7e80e489d200949b9e3fc6fa7bb3b5a68c62217f64c09a23f7fb85edf. Authoritative and outcome copies are byte-identical.
R10 corrections applied:
1. Windows durable write: replaced ReplaceFile (REPLACEFILE_WRITE_THROUGH unsupported) with MoveFileExW(MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH) — complete executable algorithm with CreateFileW flags, FlushFileBuffers, close, move, reopen+decrypt+read-back verification, and failure handling at each step. Network send blocked until read-back confirms ever_sent=true.
2. Endpoint SQL slot joins: §6 (actor context), §7 (rotate), §10 (link issuance) now all include JOIN slots s ON s.orbit_id=ic.slot_orbit_id AND s.slot=ic.slot_name AND s.revoked_at IS NULL. A revoked slot fails the join. Node-token context query also defined (starts from unrevoked slot, resolves credential/actor).
3. orbits.status rebuild: corrected from rename-old-first (which rewrites child FKs) to the SQLite-documented safe sequence: create orbits_new → copy → drop orbits → rename orbits_new. PRAGMA foreign_keys toggled outside transaction. SQLite docs cited.
4. Old-binary projection: max_members=0 (not current count — a member leaving would re-open the gap). Added rollback_projections journal table with exact DDL, transactional projection, idempotent restoration, and crash recovery. Phase B contradiction resolved: slot minted in disabled orbit during emergency gap gets actor created-and-immediately-revoked.
5. Recovery export response: added actor_id to §7 rotate success body. Both create and rotate now return actor_id for pending-recovery scope. Round-trip test requirement added.
6. Origin canonicalization: complete algorithm with IDNA2008/UTS46 profile, trailing-dot stripping, IPv6 RFC 5952 normalization, zone ID rejection, userinfo rejection, malformed URL rejection. 17 shared test vectors for cross-platform byte-identical keys.
7. Nullable paired_at backfill: sentinel value 0 for NULL slots.paired_at. Binding fingerprint remains authoritative. Test requirement for NULL→backfill→rebind cycle.
8. Domain-separated binding fingerprint: SHA-256("barycenter/slot-binding/v1:" + token_hash) replaces raw SHA-256(token_hash). Versioned domain tag. Conflict handling: match=reuse, mismatch=fail-closed.
Preserved all R1-R9 decisions. No source code touched.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=70154, exit=0)
Root review R11 rejected Rev11 after full 3438-line read. Seven blockers: exact per-user DPAPI persistence; behavior-based exact status migration; idempotent projection journal; executable disabled-gap auth repair; independent binding identity and generation-bound endpoint SQL; consistent node serving gate/response; uniform 401/403 decision tree. See precondition root-review-r11.md.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=76863, exit=0)
Root review R12 rejected Rev12 after full 3825-line read, byte/SHA verification, live-source checks, official SQLite review, and executable SQLite counterexamples. Six blockers: destructive disabled-orbit Phase A0 contradicts recoverable 403 lifecycle; binding_token_hash ownership/redaction contradictions; rotate/serving-gate SQL defects; dependent-view and post-commit migration validation failures; competing projection-journal algorithms with broken second cycle; unbounded/incomplete DPAPI ciphertext I/O. See precondition root-review-r12.md.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=88297, exit=0)
Root review R13 rejected Rev13 after complete 3906-line read, canonical outcome normalization and SHA verification, exact SQLite executions, live-source consistency checks, and DPAPI cleanup audit. Five blockers: recovery success SQL never selects orbit_id; orbits rebuild still omits tbl_name, uses SQL LIKE as dependency parser, and cannot recover missing-orbits shape without durable DDL; Phase B copies nullable paired_at into NOT NULL instead of sentinel 0; final rollback summary falsely claims revoked slots revive on re-enable; DPAPI read/cleanup branches and required fault tests remain incomplete. Projection two-cycle SQL independently passed with changed quotas 3/8. See root-review-r13.md. Product source untouched.
Rev 14 (4,111 lines, 32,057 words, 246,924 bytes, SHA-256 8a39cc6c0151013949e0342b546ee906d1ce82049551aed3be9361d9e3bf488e). Addresses all five R13 blockers: (1) recovery consume SELECT now includes slot_orbit_id for orbit_id response field, (2) SQLite rebuild query selects tbl_name for ownership classification instead of sql LIKE, (3) Phase B INSERT uses COALESCE(slots.paired_at, 0) for nullable paired_at, (4) slot revocations during rollback projection are truthfully stated as one-way requiring explicit re-pair, (5) complete DPAPI resource ownership/cleanup table and all R12-required fault-injection tests added. All accepted R1-R12 decisions preserved.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=98098, exit=0)
Root review R14: REJECTED after amended-section review and independent SQLite reproduction. See root-review-r14.md. Preserve accepted orbit-id, COALESCE, rollback re-pair, and DPAPI ownership repairs. Rev15 must: drop/recreate all user views and external triggers or specify a real parser; restore PRAGMA foreign_keys on every exit; choose one read-CloseHandle/send policy and one matching test oracle. Product source untouched; return to to-review.
Rev 15 addressing both R14 blocking corrections. Outcome research.md byte-identical to authoritative note (sha256 ba049711820d42cab102a51aa91316613dd8183fc1ebb109ec6eefd1f068ba0a, 4181 lines, 252149 bytes). Product source untouched.
(1) SQLite orbits.status rebuild — removed the prose sql-body token heuristic and the preserve-unchanged/recreate-all self-contradiction. Frozen the conservative strategy R14 named: classify strictly by type/tbl_name; owned indexes/triggers (tbl_name=orbits, auto-indexes excluded) auto-drop and recreate; ALL user-defined views and ALL external triggers are captured and dropped/recreated unconditionally with no sql scan, so no already-exists collision and no false-negative omission is possible. Froze a defer/finally that restores PRAGMA foreign_keys on EVERY exit (COMMIT, ROLLBACK from FK-check or behavior-probe failure, any SQL error, panic/unwind, intermediate-state fatal abort), first ensuring no txn is open; cited the reproducible OFF-after-ROLLBACK control at .research/root-checks/recovery-r14-foreign-keys.sql. Updated tests: six-object all-drop/recreate rebuild, no-sql-scanning (comment/string false positive + quoted/qualified/CTE reference), and PRAGMA foreign_keys==1 asserted after each of the six exit paths. Final-answer summary aligned.
(2) DPAPI read-handle CloseHandle vs send — resolved the split policy to the single conservative one R13/R14 required: a read-handle CloseHandle failure (step 6i) is FATAL for the attempt, records the OS error, escalates/restarts, and the network is NOT called. Fixed the resource-ownership table row, step 6i, the summary paragraph (no longer claims a handle is always cleanly closed; asserts no send unless every op including close succeeded), and the fault-injection test (asserts network-not-called + exact resource counts). Consistent with the steps 1-6-all-succeed send barrier and the every-failure-path network-not-called rule.
Preserved all accepted R1-R14 decisions. Returning to-review; only root may mark done.
agent completed: [analyst] researcher (claude) (exit=0)
agent spawned: claude (pid=3016, exit=0)
ROOT APPROVED Rev15. Authoritative and canonical outcome SHA-256 ba049711820d42cab102a51aa91316613dd8183fc1ebb109ec6eefd1f068ba0a. Independent conservative rebuild produced all 6 objects, foreign_keys=1, zero FK violations, bogus status rejected; DPAPI read-close failure consistently blocks network. See root-review-r15.md. Approval is research-only; implementation requires separate root code review.

## Precondition Resources
- [p1-root-review-amendments.md](file://TASK-260712-3v1k7q/p1-root-review-amendments.md) — Root-reviewed Phase 1 identity recovery and Telegram-link invariants
- [research-instructions.md](file://TASK-260712-3v1k7q/research-instructions.md) — Task-specific research deliverable and safety instructions
- [root-review-r1.md](file://TASK-260712-3v1k7q/root-review-r1.md) — Root review round 1 blocking corrections
- [root-review-r2.md](file://TASK-260712-3v1k7q/root-review-r2.md) — Root review round 2 blocking corrections
- [root-review-r3.md](file://TASK-260712-3v1k7q/root-review-r3.md) — Root review round 3 blocking corrections
- [root-review-r4.md](file://TASK-260712-3v1k7q/root-review-r4.md) — Root review round 4 blocking byte-level hash, pending-state, probe, coexistence, and atomicity corrections
- [root-review-r5.md](file://TASK-260712-3v1k7q/root-review-r5.md) — Root review round 5 blocking recovery-generation, rotation, Telegram transaction, capability, and coexistence corrections
- [root-review-r6.md](file://TASK-260712-3v1k7q/root-review-r6.md) — Root review round 6 blocking lifecycle transactions, issuance races, legacy reconciliation, schema enforcement, and capability corrections
- [root-review-r7.md](file://TASK-260712-3v1k7q/root-review-r7.md) — Root review round 7 blocking stale-bearer auth, issuer lifecycle, executable DDL, full rollback reconciliation, legacy conflict, limiter, and pending-state corrections
- [root-review-r8.md](file://TASK-260712-3v1k7q/root-review-r8.md) — Root review round 8 blocking rebind, FK cleanup, mutation surface, DDL, pending-state, limiter, and rollback corrections
- [root-review-r9.md](file://TASK-260712-3v1k7q/root-review-r9.md) — Root review round 9 blocking generation, pending target/durability, nullable binding, reconciliation, orbit joins, migration, and rollback corrections
- [root-review-r10.md](file://TASK-260712-3v1k7q/root-review-r10.md) — Root review round 10 blocking durability, endpoint binding, migration, rollback, target, and generation corrections
- [root-review-r11.md](file://TASK-260712-3v1k7q/root-review-r11.md) — Root round 11 rejection: executable blockers and acceptance tests
- [root-review-r12.md](file://TASK-260712-3v1k7q/root-review-r12.md) — Root round 12 rejection: disabled lifecycle, SQL, migration, journal, DPAPI blockers
- [root-review-r13.md](file://TASK-260712-3v1k7q/root-review-r13.md) — Root review round 13: rejected; five executable SQL, migration, rollback, and DPAPI blockers
- [root-review-r14.md](file://TASK-260712-3v1k7q/root-review-r14.md) — Root review round 14: rejected after independent SQLite and contract checks
- [root-review-r15.md](file://TASK-260712-3v1k7q/root-review-r15.md) — Root approval round 15 with independent SQLite evidence

## Outcome Resources
- [research.md](file://TASK-260712-3v1k7q/research.md) — Recovery and Telegram-link contract (Rev 15: conservative all-object SQLite rebuild with FK-restore-on-every-exit; single fatal read-handle-close send policy)
