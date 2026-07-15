# Phase 2 Air control-plane API handoff

- Date: 2026-07-15
- Task: `TASK-260712-2vhf80`
- Contract: `pulsar.air-lifecycle-policy.v1`

The coordinator now serves all 15 frozen Air lifecycle routes from
`protocol/air-lifecycle-policy-v1.json`. They use control-token `ActorContext`,
strict JSON, opaque public IDs, stable errors and actor-scoped
`Idempotency-Key` records. There is no public Air discovery and no lifecycle
operation writes a legacy link row.

## Transaction and secret boundary

Every mutation reacquires the SQLite writer lock, re-resolves the presented
control-token digest, then checks visibility, current actor role, revisions,
policy and capacity in that order. Air, membership, invite and active-pointer
changes commit with the idempotency result and content-free Air audit event.
The stored idempotency material contains only key/request digests and a
non-secret response projection.

Invite codes have 256 bits, a fixed 15-minute TTL and are returned only by the
issue response. A persisted coordinator HMAC key deterministically recreates
the same code for an exact idempotent retry; SQLite contains only the keyed
code digest. Consume is single-use under concurrent callers, unavailable code
states collapse to `invite_unavailable`, and actor/source-IP failures are
limited after five attempts per minute. Issue is limited to ten accepted
invites per actor and Air per rolling hour.

## Runtime barrier

Confirm-with-activation, activate/switch, deactivate, leave and dissolve signal
the serialized coordinator loop only after the transaction commits. The HTTP
request waits for the loop to re-resolve all authoritative Air runtimes, park
stale controllers, apply membership changes and rescan the transmission
scheduler. A runtime failure returns `503 service_unavailable`; retrying the
same idempotency key replays the durable mutation and retries this barrier.

## Verification and downstream boundary

Unit and HTTP integration coverage exercises exact replay and conflicting-key
reuse, secret redaction, wrong confirmer, foreign-ID collapse, concurrent
consume, eight-barycenter capacity, active-pointer changes, governance role and
ownership transfer, leave/dissolve, restart persistence, stable error codes and
the synchronous runtime acknowledgment.

The next task, `TASK-260712-25862f`, owns applying the stored Air policy and
membership/pointer snapshot to transmission acceptance. Online Pulsar lease
admission remains a runtime concern outside this task's eight-barycenter
control-plane capacity gate; this API exposes the frozen capacity value but
does not claim new hardware/runtime lease evidence.
