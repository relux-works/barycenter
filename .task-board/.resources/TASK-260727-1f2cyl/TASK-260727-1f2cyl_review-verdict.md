# Review verdict — TASK-260727-1f2cyl (enable-air-authority-controlled-rollout)

Reviewer: [reviewer] reviewer (claude) — run RUN-260727-e5ef6d
Date: 2026-07-27
Verdict: **BLOCKED** — human-only production-execution gate + concrete external blocker.
Work quality: ACCEPT-READY. The delivered mechanism, tests, and rehearsal all pass review.
Nothing here is dev rework; the block is the terminal production flip only.

## What was independently verified (all PASS)

- `go build ./...` → exit 0.
- `gofmt -l` on main.go + air_authority_command_test.go → clean.
- `go vet ./cmd/duet-coordinator/` → exit 0.
- 4 new tests green (re-run here): TestAirAuthorityCutoverEnablesHealthThenCleanRollback,
  TestAirAuthorityCutoverAllowsCreatePlusInviteProbeThenRollbackHolds,
  TestAirAuthorityTransitionGuards, TestRunAirAuthorityCommandCutsOverStoreFromConfig.
- `go test ./internal/store/` → PASS.
- Pre-existing failures confirmed unrelated: TestModerationHTTPAuthPrivacyEvidenceAndDecision
  and TestModerationHTTPRevokedAndLeastPrivilegeOperatorsFailClosed fail **identically on a
  clean `git worktree` at HEAD** (no working-tree changes at all) — proven, not just claimed.
  They live in moderation_http_test.go and never touch the air-authority path.
- Rehearsal log is real: shipped binary, real exit codes, live /healthz returning
  `"air_rooms_enabled":true,"air_authority_state":"airs_authoritative"`, matching-sha256 backup,
  create+invite probe (parked air + open invite, no revision_conflict), held rollback on a
  diverged store (exit 1 + "restore from backup"), backup-restore back to airs_shadow,
  `orbits_readable=2` at every step.

## Architecture fit: GOOD

runAirAuthorityCommand / applyAirAuthorityTransition mirror the existing single-writer
--project-identity-rollback pattern (open store with self-service disabled → transition →
redacted before/after receipt → exit). All safety lives in the store state machine already
accepted under BUG-260727-1hjfxi (generation CAS, WAL checkpoint before authority flip,
divergence + rollback-unsafe gate). No secrets printed. Clean, minimal, idiomatic.

## Why BLOCKED and not DONE

The task's terminal AC — "The **deployed** health endpoint reports Air rooms enabled with
authoritative ownership" — describes REAL production state. Real production is a remote
Coolify deployment (duet-data volume at /var/lib/duet) that is **unreachable from this
environment**, and flipping live production authority is an outward-facing, hard-to-reverse
action that requires explicit human authorization. Prod is still `airs_shadow`
(air_rooms_enabled=false). Marking this DONE would falsely assert prod Air authority is
live and would mislead the downstream manual acceptance journey (TASK-260727-1msjz6).

This is not dev rework (→ not to-dev): the code is complete and correct. It is a genuine
stop-the-line boundary: concrete external blocker (unreachable prod host) + human-only,
outward-facing production-execution/approval decision.

Note: the parent STORY-260712-3v14m9 AC says migration/rollback are "**rehearsed** against
production-shaped data" — a bar the developer fully met. So option B below is likely the
intended close, but it needs an explicit human re-scope/approval, which is why this is blocked.

## Options / tradeoffs

- **A. Execute for real.** Ops runs the documented runbook against production during a change
  window (stop coordinator → point-in-time backup → --air-cutover → start → confirm /healthz),
  shares the redacted receipt; then accept. Tradeoff: needs ops + change window; risk mitigated
  by the proven backup + two rollback paths.
- **B. Re-scope + accept (recommended).** Treat this task's bar as the story's "reversible
  mechanism delivered + rehearsed against production-shaped data" (already satisfied), accept as
  DONE, and carry the actual prod flip into the human-run downstream acceptance task
  TASK-260727-1msjz6 (add the runbook as an explicit pre-step there). Tradeoff: prod stays
  airs_shadow until that human step — must be tracked, not forgotten.

## Exact human input needed (pick one)

1. Authorize + run the runbook against real production and attach the redacted /healthz receipt
   (air_rooms_enabled:true + airs_authoritative), OR
2. Confirm re-scope to "mechanism + production-shaped rehearsal" → accept as done, with the real
   prod flip tracked under TASK-260727-1msjz6.
