# Phase 2 target, range and rights security technical review

Date: 2026-07-16

Task: `TASK-260712-n11rg6`

Reviewed base: `fb50d39754f343e4eb89f527af4aa434b587c6bd`

Engineering reviewer: `codex-inline-reviewer`

Independent approval: `TASK-260716-2l5j1a`

## Decision

Repository engineering review is complete and production remains blocked. One
High consent-integrity defect and one Medium cursor error-classification defect
were fixed and re-reviewed. Automated evidence confirms opaque selector and
cursor binding, immutable target snapshots, explicit replay lineage,
exact-target range/cache authorization, reporter-local report effects,
canonical revocation and bound Telegram callbacks.

This is not independent acceptance because the same inline session changed the
reviewed code. It permits the next reversible strict-sequence engineering task,
but not production target/range activation or Phase 2 promotion. Manual B5-B7
and rollout evidence remains in `TASK-260712-3u5cdn` and
`TASK-260712-3qybi2`; independent approval remains
`TASK-260716-2l5j1a` with Ivan Oparin.

## Findings

### P2-TGT-001 — High — fixed and re-reviewed

`PUT /v1/content-policy/acceptance` used the bounded decoder that rejected
unknown fields but not duplicate object keys. A body containing contradictory
`terms_accepted` values could therefore create one ambiguous durable consent
record. The endpoint now uses the frozen strict decoder. A regression requires
`false,true` duplicate consent to return `400`, verifies no grant exists, then
accepts one unambiguous request. Race and 100 repetitions pass.

### P2-TGT-002 — Medium — fixed and re-reviewed

Inbox and receipt cursor loaders treated every scan error as
`cursor_expired`. They now map only missing, foreign, stale or expired
capabilities to the uniform client surface and propagate real SQLite failures
to the operational error boundary. Actor, credential generation, binding,
view, limit, page-key and expiry isolation is unchanged; pagination tests pass
under race and 100 repetitions.

### P2-TGT-003 — High — open manual gate

Repository automation cannot prove packaged mixed-fleet non-enumeration, real
network cache/refill denial, audible no-autoplay, accessibility or
production-shaped backup/rollback. Those remain manual in
`TASK-260712-3u5cdn` and `TASK-260712-3qybi2`.

### P2-TGT-004 — High — open independent review

`TASK-260716-2l5j1a` requires a non-implementing security reviewer to inspect
the exact commit, rerun adversarial checks, inspect manual artifacts and sign
only after every Critical and High finding closes.

## Reviewed boundaries

- Target references are random digest-only capabilities bound to actor,
  credential authorization hash, current domain and installation generation.
- Explicit N-target creation deduplicates and fails atomically; it never
  broadcasts or expands an accepted snapshot to later Air members.
- Inbox and receipt cursors freeze page chains and reveal no tenant IDs. Reads
  and reconnect never schedule, queue or play. Replay is a new explicit
  idempotent transmission with bounded immutable lineage.
- Stream opens recheck exact persisted target, current binding, block, report,
  media and variant state in the descriptor-acquisition transaction. Quota is
  reserved before bytes; tiny ranges consume a request floor. An already-open
  bounded descriptor may finish, but delete, disable, report or variant revoke
  denies every new open/refill.
- A report protects its reporter only. Global quarantine/delete/disable needs
  audited operator authority. Telegram callbacks are opaque and bound to
  actor, orbit, role, chat, message, nonce and expiry with replay-safe
  finalization.

## Evidence boundary

Contract validators and ten negative contract tests pass. Twenty-one anchored
adversarial tests pass under race. Six selector/cursor/replay/report/callback
scenarios and the consent/pagination/range descriptor groups pass 100
repetitions. The exact previous coordinator preserves additive rows. These are
repository claims only; physical and rollout claims remain blocked.

Any change to a pinned contract, selector, cursor, target snapshot, replay,
consent, range/cache, moderation, callback or gate-matrix source invalidates
its SHA-256 anchor and requires delta review.
