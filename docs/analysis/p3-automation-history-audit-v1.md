# Automation history, audit and quick-disable v1

Status: engineering contract implemented by `TASK-260712-11e4e3`.

This document narrows the privacy and control boundary frozen in
`p3-automation-safety-contract-v1.md`. It does not claim real-app, audible or
physical-device verification; those observations remain in the dedicated
manual-testing epic `EPIC-260714-th54l3`.

## Canonical history projection

The existing `/v1/history` list and detail routes remain the only user history
surface. An accepted automation execution enriches its ordinary transmission
item with an `automation` object. A denied terminal attempt is a sent
`automation_attempt` item with an opaque `hi_a...` handle. There is no second
history service or target/receipt authority.

The display-safe projection contains:

- trigger kind (`manual_soundboard`, `scoped_api`, or `schedule`);
- opaque principal fingerprint and current display label, never its internal
  secret hash or bearer;
- schedule/execution IDs, labels and frozen revisions;
- cue ID, label and frozen revision;
- audience kind and resolved target count, never raw selectors or hidden
  target rows;
- scheduled, accepted and terminal timestamps;
- exact outcome, denial/cancellation reason and bounded retry delay.

Accepted execution state remains reconciled through the canonical
transmission and receipt rows. Denied attempts have no fabricated transmission.
The same 30-day history window, actor-bound cursor and authorization snapshot
apply to both item kinds. An actor outside the owning orbit receives the same
not-found result for a foreign audit handle as for a nonexistent handle.

## Append-only attribution

`automation_audit_events` is additive and contains no bearer token,
idempotency key, request digest, raw selector, storage key, media URL or local
filename. A SQLite terminal-attempt trigger appends the final admission result
in the same writer transaction that changes the bounded attempt row from
`reserved` to `accepted` or `denied`. Automation control mutations append their
operation, actor, orbit, affected resource class and terminal result in the
same transaction as their existing idempotency record. Replayed control
requests do not create duplicate audit rows.

Database triggers reject direct update and delete of audit rows. Runtime
attempt retention remains independent: pruning the bounded admission ledger
does not rewrite its immutable audit evidence.

The internal denial vocabulary remains exactly:

`automation_disabled`, `invalid_automation_credential`,
`principal_disabled`, `principal_revoked`, `principal_expired`,
`idempotency_conflict`, `insufficient_scope`, `cue_not_found`,
`cue_not_ready`, `cue_not_eligible`, `quiet_hours`, `too_many_attempts`,
`execution_in_progress`, `audience_not_allowed`, `air_policy_denied`,
`automation_capability_missing`, and `delivery_capability_missing`.

After target resolution the ordinary transmission/receipt vocabulary remains
authoritative. Runtime cancellation additionally preserves
`principal_revoked`, `schedule_disabled`, and `automation_disabled`.

## Fast controls

History advertises only actions authorized for the current viewer:

- `cancel` for a still-cancellable transmission;
- `disable_schedule` when the item has schedule lineage;
- `revoke_principal` when it has scoped-principal lineage;
- `emergency_disable_automation` for a primary viewer of an owned execution.

`cancel` now has a real history action handler and uses the ordinary atomic
sender-cancel/disarm path. The other actions reuse the existing primary-only,
revision-checked, actor-idempotent automation control methods and then run the
shared cancellation reconciler. They do not introduce a bypass or accept the
opaque principal fingerprint as authority. Emergency disable preserves the
current policy fields and changes only the emergency state at the caller's
expected feature revision.

All four actions are fail-closed. Foreign or stale lineage cannot be used to
discover a cue, schedule, principal, target, actor or orbit.

## Automated evidence

Repository tests cover additive migration rollback/retry, successful lineage,
terminal denial projection, exact reason fidelity, opaque principal display,
foreign-is-missing behavior, secret redaction, immutable audit rows, bounded
attempt pruning and the previously missing history cancel handler. Full
coordinator tests, vet, focused race tests and exact-head repository acceptance
are the engineering gates for this task.
