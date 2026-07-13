# Independent security/protocol/migration review — TASK-260712-m5264f

Act only as an independent reviewer. Do not edit production code, tests, board
checklists, or shared files; do not commit or push. Root retains acceptance.
Inspect the actual combined worktree rather than trusting the producer outcome.

Read the full Rev15 identity/recovery contract and root amendments attached to
the foundation, this task's implementation guard, and the current producer
outcome. Recompute hashes and review every task-owned production hunk plus the
shared `identity.go` and `identity_schema.go` deltas line by line.

At minimum, challenge and report:

1. Feature-off/legacy compatibility, route registration, exact HTTP bodies,
   `no-store`, TLS/loopback-proxy trust, duplicate headers and bounded JSON.
2. Auth → syntax → atomic attempt reservation → generation/hash →
   `BEGIN IMMEDIATE` ordering, including exact rolling-window math and bounded
   attacker-controlled limiter state.
3. Node/control domain separation; stale bearer re-auth; revoked/left/disabled/
   satellite classification; paired-generation and live-slot binding checks.
4. Fixed-shape invite/recovery invalid-credential paths, dummy target use,
   constant-time comparisons, replay/idempotency, collision handling, and lack
   of plaintext/digest leakage through errors, logs, assertions, or artifacts.
5. Device-invite one-winner serialization, full-capacity behavior, revoked-slot
   reuse, retirement of stale owners/credentials/memberships, and old-binary
   reconciliation/rollback safety.
6. Recovery consume/rotation races, exact generation predicates, satellite
   recovery without authority escalation, node-token preservation, and
   response-loss semantics that are actually in the frozen contract.
7. Schema/migration/FK/serving-gate invariants and previous-head compatibility.
8. Explicitly resolve or flag the apparent contract/schema inconsistency:
   Rev15 section 7 requires the rotation audit to record old and new
   `recovery_id`, while the frozen `audit_events` schema has no metadata column
   and current code records only `recovery.rotated`. State whether this is a
   release blocker, a contract erratum, or requires an additive schema change;
   do not silently waive it.
9. Treat sibling Telegram-consume files as an external changing boundary.
   Confirm they do not invalidate task-owned evidence, but do not claim their
   acceptance in this report.

Run your own focused tests, full uncached coordinator tests, full race detector,
vet, build, formatting/diff checks, and board validation. Report every finding
with severity, exact file/line, exploit/failure schedule, and a concrete remedy.
If no defect is found, say so only after documenting the reviewed inventory and
commands. Attach a task-scoped outcome named
`TASK-260712-m5264f_security-review-r1.md` and leave final acceptance to root.
