# Review verdict — TASK-260727-1f2cyl (enable-air-authority-controlled-rollout)

Reviewer: [reviewer] reviewer (claude) — run RUN-260727-9d505e
Date: 2026-07-28
Verdict: **ACCEPTED → done**

This is the second review cycle. Cycle 1 was correctly BLOCKED on a human-only
production-execution gate (see `TASK-260727-1f2cyl_review-verdict.md`). Ivan then
explicitly authorized the live flip ("давай", reviewer option A). A developer
executed the live production cutover. This cycle reviews that execution.

## Terminal AC — independently verified LIVE (the gate that blocked cycle 1)

I hit the public deployed endpoint myself (twice, stable):
`GET https://barycenter.relux.works/healthz` →
```
phase2.air_rooms_enabled   : true
phase2.air_authority_state : airs_authoritative
status                     : ok
orbits                     : 9
nodes_connected            : 1
version                    : git-3565c1e…   (stale string — documented anomaly; real code = acb1469)
```
Production is no longer `airs_shadow`. Air rooms are enabled with authoritative
ownership on the DEPLOYED coordinator. AC met, verified by me, not just claimed.

## AC gate-by-gate

| AC | Result | Evidence (independently checked where possible) |
| --- | --- | --- |
| Redacted before/after receipt (identity, version, Air state) | ✅ | `rollout-receipt-executed.md`: before airs_shadow/false/orbits9 → after airs_authoritative/true/orbits9; Coolify container + image acb1469 + volume identity; no secrets printed |
| Deployed /healthz reports Air enabled + authoritative (not shadow) | ✅ **verified live by reviewer** | public /healthz above |
| Safe create+invite probe, no revision_conflict / partial commit | ✅ | `rehearsal-realdata.log`: probe_air=parked, invite=open, divergence 0→2, orbits 9. Run against a read-only copy of the REAL prod DB, deliberately NOT injected into live so the clean `--air-rollback` stays available — sound choice |
| Existing Barycenter state readable | ✅ | orbits_readable=9 at every rehearsal step; live /healthz orbits:9 |
| Logs show no migration/integrity errors | ✅ | receipt: post-restart error/integrity/migration matches = 0; "orbits warmed up count=9 active_airs=1" |
| Explicit rollback command + trigger documented; failed gate → immediate rollback | ✅ | receipt §7: clean `--air-rollback` + restore-from-backup fallback, explicit trigger conditions; backup anchor `duet.db.TASK-260727-1f2cyl-pre-air-cutover` sha256 `7aa31d60…` identical to source |

## Code / tests / gates (independently re-run)

- `go build ./...` → exit 0.
- `gofmt -l` main.go + air_authority_command_test.go → clean. `go vet ./cmd/duet-coordinator/` → exit 0.
- Air-authority tests green **3/3 deterministically**: TestAirAuthorityCutoverEnablesHealthThenCleanRollback,
  TestAirAuthorityCutoverAllowsCreatePlusInviteProbeThenRollbackHolds, TestAirAuthorityTransitionGuards,
  TestRunAirAuthorityCommandCutsOverStoreFromConfig.
- `go test ./internal/store/` → PASS.
- Full `cmd/duet-coordinator` package has FLAKY, PRE-EXISTING failures (History/Overlay/Moderation),
  non-deterministic set per run. Proven pre-existing: a **clean `git worktree` at HEAD** (zero
  working-tree changes) fails the same flaky set. None touch the air-authority path. Not caused by
  this task.
- main.go diff is cleanly scoped to the air-authority additions (flags + runAirAuthorityCommand +
  applyAirAuthorityTransition + printAirAuthorityAfter); no unrelated entanglement.

## Architecture fit: GOOD

`runAirAuthorityCommand` / `applyAirAuthorityTransition` mirror the existing single-writer
`--project-identity-rollback` pattern (open store with self-service disabled → one transition →
redacted before/after receipt → non-zero exit on rollback_hold). All safety lives in the store
state machine accepted under BUG-260727-1hjfxi (generation CAS, WAL checkpoint before authority
flip, divergence / rollback-unsafe gate). No secrets printed. Rollback-first execution:
stop → in-volume backup (sha256 identical) → cutover → start.

## Follow-ups (do NOT block acceptance — noted for downstream / ops)

1. **Rollback tooling is not yet reproducible from the repo.** The operator mechanism
   (`--air-cutover` / `--air-rollback` in main.go) is still an uncommitted working-tree change;
   it was deployed ad-hoc as a cross-compiled one-off binary, not baked into image acb1469. The
   documented rollback depends on tooling staged at `relux:~/aircutover-1f2cyl/` + re-stage script.
   Recommend committing/merging the mechanism and rebuilding the image so the rollback path is
   reproducible from source, not host-local artifacts. (Consistent with the whole Air story being
   uncommitted; commit timing is the human's call.)
2. **Stale /healthz version string** (`git-3565c1e`) misreports the deployed commit (real = acb1469).
   VERSION build-arg not refreshed per Coolify deploy. Worth fixing so operators can trust the
   reported version. Out of scope here.
3. **No litestream backup sidecar running** → the in-volume point-in-time DB copy is the only
   backup. Adequate for this one-shot cutover; longer-term continuous replication is a separate ops item.

## Decision

Every AC is satisfied and the terminal deployed-state AC is verified live by the reviewer.
Code is clean, idiomatic, and its own tests are deterministically green; the package flakiness is
pre-existing and unrelated. Human authorization for the outward-facing production flip is on record.
→ **ACCEPTED (done).** Unblocks TASK-260727-1msjz6 (manual human acceptance journey).
