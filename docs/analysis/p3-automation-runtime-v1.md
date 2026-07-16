# P3 automation runtime v1

Task: `TASK-260712-1eva0y`

The coordinator now composes scoped triggers and current-minute schedules into
the ordinary clip transmission scheduler. Automation has no capture API,
microphone dependency, local playback path, outbound webhook or volume
override. It can submit only an active saved cue as `overlay`; the existing
target snapshot, block, report, DND, Air policy, presence, capability, media
ACL, prepare/play/receipt and recipient-side mixer paths remain authoritative.

## Durable admission and limits

Each scoped request is keyed by `(principal_id, digest(Idempotency-Key))` and
each scheduled occurrence by `(schedule_id, schedule_revision, canonical local
minute)`. SQLite serializes reservation, current-policy checks, immutable
target snapshot, transmission creation, execution linkage and accepted result.
Exact API retries return the committed execution/transmission; a different
request digest returns `409`. Concurrent workers and duplicate timer ticks
therefore cannot create a second transmission.

The bounded attempt ledger counts every reserved attempt before later policy
denials. A principal gets five attempts per rolling minute, an orbit twenty per
rolling hour, one nonterminal execution per principal and two per orbit. A
limit rejection does not insert another attacker-controlled key. Attempts have
a 90-day retention boundary and a bounded pruning API. HTTP `429` includes
`Retry-After`.

Admission is fail-closed in this order: feature/emergency switch and current
principal, durable idempotency, rate/concurrency reservation, immutable cue and
audience scope, global/additional quiet hours, current content-policy consent,
live source installation, cue readiness, current target/Air generation, then
the shared block → report → DND → online/capability transmission policy. A
target set with no eligible recipient creates no transmission. Missing overlay
capability is denied; automation never accepts the ordinary after-current
downgrade.

## Scheduler and system cue

The one-second evaluator considers only the current UTC minute mapped through
each schedule's IANA timezone. It never searches backwards. Spring gaps do not
run and only the first UTC mapping of a repeated fall-back minute is canonical.
The occurrence uniqueness fence survives restart and concurrent evaluators.

User clips reuse their pinned ready `audio_clip`. The hash-pinned builtin cue
is regenerated from the reviewed two-partial synthesis source, verified against
`479b1a9d...730fd`, atomically materialized in canonical media storage and
published as an orbit-owned system `builtin_cue`. Both then use the same media
download ACL and transmission scheduler.

## Revoke and disable

Principal revoke, schedule disable/delete/replacement and feature/emergency
disable are reconciled immediately after control commit and once per runtime
tick. Linked nonterminal scheduler work is cancelled with one of
`principal_revoked`, `schedule_disabled` or `automation_disabled`; repeated
reconciliation is idempotent. Cancellation uses the existing `cancel_media`
delivery and cannot bypass recipient-local mixer ceilings.

This is automated repository evidence only. It does not claim audible output,
real DST transition behavior, packaged UI behavior or hardware results; those
remain in the separate manual-test epic.
