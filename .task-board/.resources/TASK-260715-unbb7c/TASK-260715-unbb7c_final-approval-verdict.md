# TASK-260715-unbb7c — Final independent P1 migration review verdict

- Date: 2026-07-19
- Decision: **APPROVE** — the Phase 1 migration audit packet, both original
  corrective HIGH fixes, and the P1-MIG-003 corrective ordering are accepted
  at the reviewed head. **P1-MIG-003 is closed.** No Critical or High finding
  remains open in this signoff's scope.

## Reviewer identity, independence, reviewed revision

- Reviewer: Claude Fable 5 (`claude-fable-5`), task-board tracked finalization
  reviewer run `RUN-260719-c83d59` on branch `review/task-260715-unbb7c-fable5`,
  local environment darwin/amd64, go1.26.0.
- Reviewed revision: exact PR #278 head
  `aafcfc222518e7a32e2acaf365a1af4d247cc03c` (= local HEAD bit-for-bit;
  `gh pr view 278` confirms the same headRefOid; PR title "fix(store): close
  generation-skip media migration gap"). Contains the producer correction
  `831d6d7671f9e8964cf70d1856cbd501dd3e5e0e` over the previously reviewed
  main head `06ce330f…`.
- Independence: this session implemented none of the reviewed migrations, none
  of the audited store code, and none of the corrective commits. It made zero
  modifications outside `.task-board` tracking (verified via `git status`
  immediately before this verdict). The audited implementation and the
  P1-MIG-003 correction were produced by earlier producer runs; the prior
  review cycles were runs `RUN-260719-d82ed0` (CHANGES REQUESTED at `06ce330`)
  and `RUN-260719-260288` (non-final re-review; ended with the full-race gate
  explicitly unresolved and deliberately published no approval).
- Mandate honored: the missing full-race result was established by this run's
  own execution, not inferred from any prior runner's exit code.

## What this finalization verified

### 1. P1-MIG-003 correction — inspected and CLOSED

Code inspection at the reviewed head:

- `initMediaIngestSchema` is DDL-only again (media_ingest_schema.go:237-241:
  commit, return nil — the three reconciler calls are gone).
- The three media reconcilers run from the production open path
  (store.go:178-189) strictly after `initTransmissionSchema` (store.go:158,
  creates `transmission_inbox_items`) and `initSavedCueSchema` (store.go:170,
  creates `saved_cues`), and before `ReconcileSavedCues` (store.go:190) — the
  exact remediation the prior verdict proposed, and the correct side of the
  cue reconciliation (cue validity must be derived from post-revocation media
  state).
- Full dependency closure re-derived from source: every table reached by
  `reconcileTelegramLegacyLinks`, `reconcileOrphanedMediaItems`,
  `reconcileMediaLifecycleOutboxes` and the shared
  `transitionMediaTerminalTx` (media_items, media_upload_sessions,
  media_storage_operations, media_delivery_cancellations,
  media_legacy_wav_links, media_ingest_audit_events, media, orbits,
  saved_cues, saved_cue_revocations, saved_cue_audit_events,
  transmission_inbox_items) is created at or before `initSavedCueSchema`.
  No table from the later automation schemas is reached — the bug class does
  not recur for any later generation skip at this head.
- Each reconciler has exactly one production call site (the open path); no
  bypass exists. Each remains its own restart-safe idempotent immediate
  transaction with checkpoint seams; `reconcileOrphanedMediaItems` retains its
  in-transaction `foreign_key_check` (media_ingest.go:1935), so destructive
  dissolution cleanup is still FK-validated even though the moderation-stage
  global check now precedes it; all reconciler writes run on the sole
  connection with `foreign_keys=ON` inline enforcement. DDL-then-reconcile
  discipline preserved; contract-authorized cleanup was not moved ahead of
  rollback reconciliation.

Independent reproduction and fixture validation:

- Pre-fix head `55d2105` (commit immediately before the fix), with the new
  fixture applied in a scratch worktree: `TestGenerationSkippingMediaReconcileWaitsForLaterSchemas`
  FAILS with exactly the audited signature — `store: init media ingest
  schema: SQL logic error: no such table: saved_cues (1)` — confirming both
  the finding and that the fixture actually exercises the gap.
- Reviewed head: the same fixture PASSES under the race detector. The fixture
  drives the true production `Open` path end-to-end: seeds an active media
  item whose owner orbit was dissolved by a predecessor, drops both
  late-generation tables to model the pre-inbox/pre-cue generation, then
  asserts current-head startup succeeds, recreates both tables, revokes the
  orphan (status deleted, storage key cleared, revision 2), records exactly
  one durable cleanup receipt, and converges idempotently on a second open
  with a healthy database (integrity + FK checks).

### 2. Full-race gate — established independently, GREEN

`go test -race -count=1 ./...` for the entire coordinator module, executed by
this run at the reviewed head: **every package ok, zero failures** —
`internal/store` 455.8s, `internal/media` 80.6s, `cmd/duet-coordinator`
220.1s, all remaining packages ok (full log retained in the run transcript).

### 3. Regression fixtures rerun at the reviewed head (race detector)

- P1-MIG-001 — **remains closed**:
  `TestLegacyBootstrapDDLFailureRollsBackEveryStatementAndReruns`,
  `TestOrbitBootstrapDDLFailureRollsBackColumnsAndLinksAndReruns`,
  `TestPartiallyAppliedLegacyColumnsResumeWithoutRewritingRows` — 3/3 PASS.
  The fix commit touches neither the legacy/orbit DDL transactions nor
  startup pragma handling (diff-verified: its store.go hunk only inserts the
  reconciler calls after `initSavedCueSchema`).
- P1-MIG-002 — **remains closed**:
  `TestConcurrentBootstrapMigrationSerializesAndPreservesLegacyAuthority` —
  PASS; `busy_timeout`-before-WAL ordering and bounded BUSY-only retry
  (store.go:110-119, 229-242) unchanged.
- Exact-predecessor suite (`-tags previoushead`, real extracted predecessor
  source at the pinned revisions): **13/13 PASS** — all five media surfaces,
  transmission (both generations), moderation, automation, air,
  identity R8 trio, telegram link.

### 4. Hosted CI at the exact reviewed head

Run `29691922727` (branch `review/task-260715-unbb7c-fable5`, head
`aafcfc2…`): **4/4 jobs success** (`coordinator`, `node-core`, `pulsar-win`,
`pulsar-win-packaged-probe`).

### 5. Scope discipline of the delta under review

Product-code delta `06ce330..aafcfc2` is exactly the correction:
`media_ingest_schema.go` (reconciler calls removed), `store.go` (+16, the
relocated reconcilers), `bootstrap_migration_review_test.go` (+81, the
generation-skip fixture). Everything else is documentation, board tracking,
and the acceptance delta record. The acceptance record
(`acceptance/phase3/migration-recovery-technical-pre-review-v1.json` +
validator) honestly records `independentApproval: "pending"` as of that
commit and anchors `store.go` by sha256 `be666ea0…`, which matches the
reviewed working tree (digest re-verified). The updated audit packet's
P1-MIG-003 addendum claims match the verified code exactly; the board
resource copy is byte-identical to
`docs/analysis/p1-independent-migration-technical-audit.md`.

### 6. Backup/restore prerequisites

No file under `deploy/`, `docs/backup-restore.md`, `docs/runbook.md` or
`docker-compose.yml` changed between the previously reviewed head and this
head (diff-verified), so the prior cycle's detailed repo-only inspection
stands: Litestream sidecar WAL replication (Coolify path), daily
`sqlite3 .backup` cron (bare-VPS path), manual restore-by-design with
`-if-db-not-exists`, and the explicitly disclaimed live-drill gap owned by
open manual tasks `TASK-260712-3qybi2`/`TASK-260712-30xwu2`. Nothing was
altered in this cycle.

## Medium findings — disposition preserved

- **MED-1 (connection-recycling pragma loss) — non-blocking, disposition
  unchanged**: follow-up hardening (DSN defense-in-depth for replacement
  connections, or periodic pragma assertion), routed as ordinary engineering
  work. The correction under review does not touch connection or pragma
  handling, so the finding is neither worsened nor silently absorbed; startup
  correctness (P1-MIG-002 scope) remains unaffected. It does not block this
  signoff.

## Production boundary

No production system, database, backup, or host was accessed or altered. No
manual restore or operator drill was performed or is claimed by this review;
those remain owned by the open manual tasks. This verdict covers
repository-verifiable engineering evidence plus hosted CI reads only.

## Verdict routing

- `TASK-260715-unbb7c` → **done** (APPROVE recorded; this external review
  action is complete).
- P1-MIG-001, P1-MIG-002, P1-MIG-003: all **closed** at
  `aafcfc222518e7a32e2acaf365a1af4d247cc03c`.
- `TASK-260712-1xkn75`: the non-implementing-reviewer approval its checklist
  item 1 requires now exists; per the finalization instructions the
  orchestrator reconciles and accepts that original task separately.
- MED-1: route as ordinary follow-up engineering work (non-blocking).
