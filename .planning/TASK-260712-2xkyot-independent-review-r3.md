# Independent security/compatibility review R3 — TASK-260712-2xkyot

Act only as an independent reviewer. Do not edit production code, tests,
board state/checklists, task resources other than your own report, or shared
files; do not commit or push. Root retains acceptance. Inspect the actual
combined worktree and recompute evidence instead of trusting producer claims.

Read completely before reviewing:

- the frozen Rev15 identity/recovery contract and root amendments attached to
  `TASK-260712-3v1k7q`;
- all implementation/rework guards attached to this task, especially the R1
  security guard and R2 deterministic `BEGIN IMMEDIATE` proof guard;
- `TASK-260712-2xkyot_rework-r2-results.md`;
- every current Telegram task production and test file, plus its shared
  identity/onboarding/schema dependencies.

Review the actual current code line by line. At minimum, challenge:

1. Feature-off exact compatibility, migrated member roles/slot ownership,
   mixed legacy/self-service flows, and the rule that linking Telegram never
   transfers installation/orbit identity ownership.
2. Trusted-principal boundary: Telegram user/chat/message fields must come only
   from an authenticated Bot API Update; no public HTTP link-consume path; only
   private chats may consume; unknown/left/revoked/disabled classification must
   not reopen forbidden stranger mutations.
3. Link issuance and consume ordering, issuer re-auth/lifecycle checks,
   fixed-shape lookup, exactly one constant-time submitted-code comparison,
   dummy target behavior, uniform invalid-credential responses, expiry,
   invalidation, desired-role policy, and collision behavior.
4. Exact rolling 10/15-minute limiter semantics: every syntactically valid
   attempt including rejected attempts advances the rolling boundary; state is
   atomic and bounded; malformed input does not reserve; no attacker-controlled
   unbounded key or timestamp state.
5. Transaction linearization and rollback: `_txlock=immediate`, all reads and
   writes through the `Tx`, issuer lifecycle checked inside the writer, code
   reservation/member creation/legacy UPSERT/audit committed together, and
   no `INSERT OR REPLACE` identity mutation.
6. Independently validate the R2 concurrency proof. Confirm writer two signals
   from a production-neutral seam immediately before the real `db.Begin()`,
   then is proven unable to reach the credential preflight until writer one is
   released. Check same-code/two-user single winner and two-code/same-user loser
   code preservation. Reject scheduler-delay or tautological tests.
7. Same-orbit and foreign-orbit conflict handling across both `memberships` and
   legacy `members`; revoked actor behavior; partial-unique membership defense;
   display-name sanitation; and no unauthenticated role overwrite.
8. Bot transport and error graph end to end: redirect fail-closed behavior for
   calls and downloads; cloned injected-client semantics; raw URL, bot token,
   form body, message text, file ID, destination path, link code, and Telegram
   description must not survive rendering, wrapping, `errors.As/Is`, logs,
   assertions, or task artifacts. Best-effort source deletion occurs only after
   commit and never echoes the code.
9. Shared `identity.go`, onboarding, schema, loop, bot, and Store changes must
   not invalidate the producer's frozen boundary. Report sibling drift rather
   than silently accepting it. Do not claim acceptance of onboarding or Windows
   work in this report.
10. Previous-head compatibility must be exercised from the pinned local
    previous source, not inferred from current tests.

Recompute at least the two R2 file hashes and compare them with the producer
boundary. Run your own focused repetitions, full uncached coordinator suite,
the pinned `previoushead` suite, race detector, vet, build, coordinator gofmt,
diff checks, secret/URL/error-graph scans, and `task-board validate`.

Report every finding with severity, exact file/line, concrete failure or attack
schedule, and a minimal remedy. Distinguish release blockers from observations.
If no defect is found, state that only after listing the reviewed inventory,
hashes, and command results. Attach exactly one new task-scoped outcome named
`TASK-260712-2xkyot_security-review-r3.md`; do not alter task status or mark it
accepted.
