# Phase 2 Air migration and concurrency technical review

Date: 2026-07-16

Task: `TASK-260712-2sicfs`

Reviewed base: `090be8c68b74319c6ca50e063c78d61d2ae16064`

Engineering reviewer: `codex-inline-reviewer`

Independent approval task: `TASK-260716-19g4gd`

## Decision

The repository engineering review is complete and production remains
blocked. One High invite-admission defect and one Medium store error
classification defect were fixed and technically re-reviewed. The additive
migration, authority state machine, concurrent lifecycle store, runtime
generation fences, Telegram aliases and exact previous-binary seam pass the
targeted deterministic checks.

This permits the next reversible strict-sequence engineering task to start
under the owner's continuation instruction. It does not permit production Air
activation or Phase 2 promotion. Signed real-app Air and rollout evidence
remains in manual epic `EPIC-260714-th54l3`, and an implementation-independent
signature remains in `TASK-260716-19g4gd` with Ivan Oparin as approver.

The same root execution session changed the reviewed code, so this is a
technical review rather than independent acceptance. The fail-closed
machine-readable record is
`acceptance/phase2/air-migration-review-v1.json`.

## Authority and migration review

The schema remains additive. Legacy links are deterministically backfilled to
stable Air IDs, memberships and mappings. The persisted authority state
advances through `links_authoritative`, `airs_shadow`,
`airs_authoritative`, and fail-closed `rollback_hold`; links and Airs are never
both eligible to own playback. Cutover and rollback use immediate writer
transactions and generation compare-and-swap. Divergence after Air-authority
cutover prevents a legacy rollback and persists `rollback_hold`.

The exact pinned predecessor test archives and runs predecessor
`d4964098765ef9e53aa4de2be54d69a25c953cd1` against the migrated database. Its
legacy link mutations preserve additive Phase 2 rows. The current binary can
resume mappings, cut over, roll back while safe and roll forward again. This
is strong repository evidence, but it is not a production backup, deployment,
quiesce or operator rollback rehearsal.

## Transactional lifecycle review

SQLite writers use immediate transactions and a single connection per Store;
separate Store instances still serialize on the database writer lock.
Revision and expected-active-pointer predicates give concurrent activate,
switch, leave, role, policy, ownership and dissolve operations one winner.
Invite codes are HMAC-hashed at rest, single-use, expiring and actor-bound by
the authenticated barycenter context. Join consumption creates only a pending
membership; only the joining barycenter's current primary can confirm it.

Saved joined membership and the active pointer remain distinct. An orbit can
retain multiple joined Airs but owns zero or one active pointer. An Air runs
only for current joined pointers, parks below two active members, retains its
durable state and is reconstructed lazily.

## Runtime ownership review

The serialized coordinator loop resolves the authoritative runtime set after
committed lifecycle changes. It parks stale controllers, rebuilds the exact
current-member peer union and maps each orbit to one Air. Stable Air ID,
authority generation and Air revision fence timers and asynchronous media
completion. Switching detaches the previous owner before joining the new Air.

The peer union derives only from active pointers and active installation slots;
saved memberships cannot introduce transitive peers. A joiner receives only
the current main program after readiness and never an old voice, overlay or
interrupt. Leave stops the caller's peers; other members continue unless the
Air falls below two and parks. Telegram lifecycle callbacks execute on the
same serialized loop and reuse the authoritative Air mutations, so the legacy
approach aliases do not create a second link runtime.

## Findings

### P2-AIR-001 — High — fixed and technically re-reviewed

Invite consume rate limiting previously ran after
`ConsumeAuthorizedAirInvite`. Five invalid guesses could exhaust the nominal
budget, but a sixth request containing a valid code was already committed
before the handler considered returning `429`. With 256-bit codes this was not
a practical random-guess exploit, but it violated the frozen security boundary
and made the limiter bypassable by definition.

Actor and source-IP attempts now reserve before any store mutation. An
unavailable-code result retains both reservations. Success and failures that
are not code-guess failures release their exact reservation IDs, preserving
the frozen failure-only budget even under concurrency. The regression sends
five invalid codes, then a valid sixth code, requires `429`, resets the limiter
and proves the same idempotency key can consume the still-open invite. The
targeted suite passes under the race detector and for 100 repetitions.

### P2-AIR-002 — Medium — fixed and technically re-reviewed

The invite query evaluated zero-value status and expiry before propagating a
non-`ErrNoRows` scan error. An operational database error could therefore be
masked as `invite_unavailable`. The store now handles `ErrNoRows`, propagates
all other scan errors, and only then evaluates status and expiry. Uniform
unknown/expired/consumed behavior is unchanged.

### P2-AIR-003 — High — open, manual blocking

Repository tests cannot prove audible signed-app Air behavior, mixed-fleet
deploy, physical join/leave, real network faults, resource gates, production
quiesce, backup or previous-binary rollback. `TASK-260712-21kz3b` owns the
physical Air scale matrix and `TASK-260712-3qybi2` owns production-shaped
rollout/rollback in manual epic `EPIC-260714-th54l3`.

### P2-AIR-004 — High — open, external review

The inline reviewer is not independent. `TASK-260716-19g4gd` requires an
implementation-independent reviewer to inspect the exact candidate commit,
rerun representative checks, review the manual artifacts and sign only after
all Critical and High findings are fixed and re-reviewed.

## Representative reruns

The following checks passed on 2026-07-16:

```text
PYTHONDONTWRITEBYTECODE=1 python3 scripts/acceptance/run_air_regression.py --output .temp/acceptance/task-260712-2sicfs-air-regression.json
(cd coordinator && go test -race -count=1 ./internal/store ./cmd/duet-coordinator -run '^(TestAir|TestAuthorizedAir|TestMigratedApproachAlias|TestActiveLinkBackfill|TestConcurrentAir|TestUnsafeAirRollback|TestConflictingLegacyLinks|TestTelegramAir|TestApproach)')
(cd coordinator && go test -tags previoushead -count=1 ./internal/store -run '^TestAirExactPreviousCoordinatorLegacyServicePreservesPhase2Rows$')
(cd coordinator && go test -count=100 ./internal/store -run '^(TestConcurrentAirLifecycleChangesHaveOneTransactionalWinner|TestAuthorizedAirConcurrentConsumeAndCapacity)$')
(cd coordinator && go test -race -count=1 ./cmd/duet-coordinator -run '^(TestAirHTTPRejectsLooseShapesAndRateLimitsUnavailableInvites|TestAttemptLimiterCountsRejectedAttemptsAndConcurrentBoundary|TestAttemptLimiterReleasesExactSuccessfulReservation)$')
(cd coordinator && go test -count=100 ./cmd/duet-coordinator -run '^(TestAirHTTPRejectsLooseShapesAndRateLimitsUnavailableInvites|TestAttemptLimiterReleasesExactSuccessfulReservation)$')
```

The synthetic scale artifact reports 8 barycenters, 20 Pulsars, 20 unique
loads, zero duplicates, one runtime and zero legacy groups. These values prove
deterministic topology and ownership only; they are not physical resource,
latency or audible evidence.

## Reopen and production rule

Production Air remains blocked until the immutable signed build passes both
manual tasks and the independent reviewer closes P2-AIR-004 against the exact
candidate commit. Any later change to the pinned contract, schema, store,
authority, alias, HTTP admission, runtime, Telegram or gate-matrix sources
invalidates the affected SHA-256 anchor and requires delta review.
