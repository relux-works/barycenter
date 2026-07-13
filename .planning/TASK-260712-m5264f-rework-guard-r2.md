# TASK-260712-m5264f — root R1 audit / mandatory R2 rework guard

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — producer result and initial review are not accepted**

This guard is cumulative with the original implementation guard, frozen Rev15,
the root amendments, and the corrected independent R1 review. Preserve the
dirty worktree; do not commit or push. Do not edit the sibling Telegram consume
implementation in `identity_telegram.go` or its tests during this run. This task
must provide the shared durable audit repository that the Telegram task can
adopt in a later, separately reviewed rework.

The independent reviewer initially reported a fourth node-token limiter issue.
Root rejected that finding after checking the controlling endpoint contracts:
Rev15 section 7 step 1 and section 10 step 1 explicitly require node tokens to
fail with `403 insufficient_capability` in auth middleware before syntax and
reservation. Preserve that behavior and the existing zero-counter assertion.
Only lifecycle/role `403` results reached with a valid control credential count.

## R2-F1 — rotation audit must retain the exact recovery transition (HIGH)

`RotateRecovery` currently overwrites `recovery_id` and inserts only the generic
`recovery.rotated` row. Rev15 section 7 step 10 requires the old and new
non-secret recovery handles in the durable audit record.

Required correction:

1. Add an additive, rollback-compatible, typed audit-detail representation tied
   to the existing `audit_events` rotation row. Prefer a constrained normalized
   table dedicated to recovery-rotation details. Do not rebuild
   `audit_events`, use a fake `orbit_id = 0`, or introduce arbitrary JSON.
2. Select and retain the prior `recovery_id` before the overwrite. An absent
   prior generation must be represented explicitly as NULL; every new handle
   must satisfy the frozen `rec_` format.
3. Insert the base audit row and exact old/new detail inside the same
   `BEGIN IMMEDIATE` transaction as the credential update. A detail-write,
   base-audit, update, or commit failure must roll back the new generation.
4. Never persist the recovery secret, its digest, a bearer/control token, or any
   other credential material in the audit tables, errors, logs, or fixtures.

Required executable evidence includes exact old/new assertions, first-generation
NULL handling where reachable, trigger-injected base/detail audit failures with
full credential rollback, collision retry, response redaction, and a scan proving
that no plaintext secret or credential digest entered either audit table.

## R2-F2 — every emitted rate-limit rejection needs durable audit (HIGH)

The HTTP limiter path uses `Store.LogEvent`, which is explicitly a best-effort
debug sink and discards SQLite errors. It cannot satisfy Rev15 section 12.

Required correction:

1. Add an additive typed rate-limit audit table/repository that supports both
   scoped events (known orbit/actor) and pre-identity events (neither known).
   Nullable scope is legitimate; sentinel/fabricated orbit or actor IDs are not.
   At minimum record event type, limiter class, a domain-separated SHA-256 of
   the limiter subject, optional real orbit/actor IDs, and timestamp. Constrain
   the digest shape. Do not store raw IPs, installation-attempt IDs,
   `recovery_id`, Telegram user IDs, codes, tokens, or generic JSON payloads.
2. Give every frozen limiter class a distinct stable class identifier:
   create/source-IP, create/installation-attempt, invite-consume/source-IP,
   recovery-consume/source-IP, recovery-consume/recovery-ID,
   recovery-rotate/actor, and Telegram-link-issue/actor.
3. Keep reservation order unchanged: the attempt is atomically reserved first.
   If the durable audit insert succeeds, emit the exact `429` plus Retry-After.
   If it fails, return the normal internal-error envelope and do **not** emit a
   `429` that lacks its audit row. The reserved attempt remains consumed.
4. Expose a small shared store method suitable for the sibling in-process
   Telegram consume limiter, but do not change that sibling in this run. The
   method must return persistence errors and must not use `LogEvent`.

Required executable evidence must independently cross the N+1 boundary for all
seven HTTP limiter classes, assert one durable row with the correct class and
scope/digest behavior, and inject an audit failure for scoped and unscoped paths.
Prove failure produces neither `429` nor Retry-After and leaks no subject value.
Keep exact rolling-window, atomic boundary, bounded-key, trusted-proxy, and
reservation-before-lookup/generation behavior unchanged.

## R2-F3 — app-first reconciliation can bypass orbit alignment (HIGH)

The `paired_by=0 && controlHash.Valid` shortcut preserves the app-first role but
skips the only active-membership orbit check. A credential bound to orbit A and
an active membership in orbit B can therefore pass startup reconciliation even
though request-time joins later deny it. Rev15 section 17.5 requires startup to
fail closed, disable the actor, and refuse serving.

Required correction:

1. Preserve the app-first role, but before taking the shortcut verify that the
   actor has exactly one active membership and its orbit equals the credential's
   `slot_orbit_id`. Missing or cross-orbit membership is a typed fatal alignment
   violation, not an automatic role rewrite.
2. Extend the final serving gate with an independent credential-to-active-
   membership alignment assertion so a future shortcut cannot bypass it.
3. Resolve rollback versus disablement explicitly. On a typed alignment
   violation, roll back ordinary reconciliation changes, then use a separate
   immediate quarantine transaction to re-verify the violation, durably mark
   the affected actor revoked/disabled, and record a scoped non-secret audit
   event. Commit quarantine, then return the fatal error so
   `OpenWithOptions(...SelfServiceOnboarding:true)` closes the store and refuses
   service. Do not silently repair authority, delete evidence, or return a
   serving store. If quarantine itself fails, still fail startup and surface
   both errors without credential material.
4. Preserve all request-time orbit/binding joins as defense in depth and keep
   feature-off, legacy backfill, stale-generation retirement, and app-first role
   preservation behavior intact.

Required executable evidence must handcraft a foreign-key-valid fixture with a
live app-first credential in orbit A and the actor's sole active membership in
orbit B, close and reopen through the real production startup path, and prove:
startup returns a fatal alignment error; no serving store escapes; the actor is
durably disabled; secrets are untouched/not logged; and a second reopen still
fails until explicit repair. Then explicitly repair membership and actor state
and prove reopen succeeds with the intended app-first role unchanged. Also test
missing active membership and the independent final serving-gate query.

## Verification and outcome

After the final edit, run focused R2 tests repeatedly, all onboarding/identity/
migration/Telegram compatibility tests, exact previous-head round trips, the
full uncached coordinator suite, the full race suite, vet (including
`previoushead`), build, gofmt, secret scans, `git diff --check`, and
`task-board validate`. Test the real production repository and HTTP paths; SQL
emulation or source-string checks alone do not count.

Attach a new canonical outcome named
`TASK-260712-m5264f_rework-r2-results.md` with exact post-edit SHA-256 values,
the full command results, failure-injection evidence, schema/rollback notes,
dirty-worktree boundaries, and the explicit untouched Telegram-consume hash.
Set the task to `to-review`. Fresh independent security/migration review and a
new root line-by-line, hash, and test audit remain mandatory before acceptance.
