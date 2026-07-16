# P3 automation schema and lineage foundation

Task: `TASK-260712-3sv87k`

This increment installs the additive, production-dark persistence required by
`automation-safety-v1`. It exposes repository primitives only. No HTTP route,
scheduler loop, target resolver, transmission creator, client composition or
`automation_cue_v1` capability is registered.

## Durable model

The coordinator now owns these additive SQLite domains:

- `automation_feature_state`: independent soundboard/automation flags, explicit
  emergency disable, IANA timezone, canonical quiet-hours JSON/hash and a
  compare-and-swap policy revision;
- `automation_schedules`: owner/actor/cue attribution, IANA timezone, weekday
  mask, local minute, immutable audience selector, overlay-only delivery,
  policy revision, enabled state and mutation revision;
- `automation_principals` plus scope tables: owner/issuer attribution, one
  `automation:trigger` permission, cue/audience/target allow-lists, exact bound
  Air, maximum target count, required expiry, reversible disable and terminal
  revoke timestamps;
- `automation_executions`: immutable trigger/cue/source/audience/schedule or
  principal snapshots, feature/policy revisions, occurrence or idempotency
  identity, future target/transmission linkage, outcome fields, retry/lease
  generation and retention boundary.

Principal issuance returns 32 random bytes as lowercase hex once. SQLite stores
only a domain-separated SHA-256 digest and its version; public principal and
lineage structs expose neither the secret nor its digest. Principal expiry is
required and capped at 90 days. Scope child rows are immutable; changing scope
requires replacement and revocation.

All usage is derived from live rows. There is no mutable schedule, principal or
execution quota counter to strand after rollback or crash.

## At-most-once identities

A scheduled occurrence is uniquely keyed by:

```text
schedule_id / schedule_revision / local_date / local_HH:MM
```

The repository validates that the proposed UTC instant maps to the configured
IANA local minute and weekday and that the tick occurs inside that exact UTC
minute. A spring-forward gap has no valid mapping. For a fall-back fold, a
bounded offset search accepts only the earliest UTC mapping; the repeated wall
minute is rejected. A unique partial index makes restarts, backward jumps and
concurrent workers return the same execution.

Scoped API idempotency is unique per resolved principal. Only the
domain-separated key digest and canonical request digest are stored. Exact
replay returns the existing execution; another request under the same key is a
conflict. Principal and feature state are re-read in the same serialized writer
transaction before insert, so revoke or quick-disable that commits first wins.

## Crash and cancellation lineage

An execution is committed in `claimed` state before runtime work. A worker may
acquire a bounded, hashed-owner lease with compare-and-swap retry generation.
Only one worker can own it. Explicit release or startup reconciliation of an
expired lease clears ownership and increments `retry_generation`; the immutable
occurrence/idempotency row remains, so recovery retries work rather than
creating a second event.

Pending-cancellation lookup derives exact nonterminal execution IDs for:

- principal revoke/disable or issuer authority loss;
- schedule disable or creator authority loss;
- orbit automation/emergency disable; and
- saved-cue source revocation.

This task records the lookup and lineage only. The later runtime task consumes
it to perform generation-safe cancel/fade/resume actions through canonical
transmission services.

## Migration and evidence boundary

Fresh schema, transaction rollback, eight-worker occurrence contention,
API replay/conflict, principal revoke, feature/schedule quick-disable, DST
gap/fold, clock-jump, lease recovery and derived accounting are covered by Go
tests. The previous exact coordinator at code head `8ccd770` opens the upgraded
single-file database, mutates legacy settings and closes it; the current binary
then recovers every media, cue, principal and execution row with a clean foreign
key check.

These are synthetic repository results. No real scheduled playback, audible
output, packaged app, physical clock transition or hardware quick-disable is
claimed; those remain in the manual-test epic.
