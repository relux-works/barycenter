# TASK-260712-1bpog0 root review round 1

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — no identity implementation is accepted**

This contract is cumulative with `TASK-260712-1bpog0_implementation-guard.md`,
`TASK-260712-1bpog0_review-guard.md`, the accepted frozen recovery/Telegram-link
contract, and `TASK-260712-1bpog0_independent-review.md`. Passing the current
suite is not acceptance: root reproduced authorization and migration failures
through normal Store APIs and exact SQLite state transitions.

## R1 — node and control capability domains alias (critical)

Locations: `identity.go:92-153`, `identity.go:316-387`.

`ProvisionInstallationSecrets` accepts caller-provided 64-hex material without
checking either credential domain. `resolveTokenActorContext` checks
`installation_credentials.control_token_hash` before `slots.token_hash`, so one
digest in both tables resolves as control instead of failing closed.

Root normal-API proof created orbit A and its paired node token, then provisioned
an installation in orbit B with that same value as its control token. The A node
token resolved as orbit B actor 4 with control capability. Starting the current
coordinator over the fixture then allowed that node token to fetch orbit B media
with HTTP 200. This violates the playback-only node capability and tenant
boundary.

Required correction:

- make credential issuance safe and server-generated, or otherwise reject
  cross-domain values transactionally;
- enforce disjoint node/control digest classes on provisioning, pairing/minting,
  reconciliation, and migration paths;
- make resolution query both domains and fail closed on zero, multiple, or
  ambiguous matches rather than relying on query order or collision probability;
- preserve the legacy node-token hash convention and existing valid pair tokens;
- add normal-public-API same-value, cross-orbit, old-database, and concurrent
  issuance tests plus an HTTP tenant-isolation regression.

## R2 — initial provisioning overwrites/revives credentials (high)

Location: `identity.go:316-387`.

The target update checks only actor/orbit and a live slot binding. It neither
requires an unprovisioned generation nor validates the target actor kind,
revocation, active aligned membership, role, or lifecycle. Root provisioned one
target twice through normal APIs: the old credential became unauthorized and the
second immediately resolved. The same primitive can write credentials for
revoked, left, stale, or satellite targets.

Required correction:

- treat this method as single-winner initial provisioning only;
- revalidate authority and target actor kind, non-revocation, active same-orbit
  membership, permitted role, live binding, and intended generation inside the
  same `BEGIN IMMEDIATE` transaction;
- condition the update on every secret field still being unprovisioned; an
  already-provisioned row must not be overwritten even under two-store races;
- keep rotation, recovery, revoke/re-pair, and role repair as explicit separate
  flows; never silently revive a target;
- prefer minting secrets inside the trusted boundary instead of accepting
  arbitrary low-entropy syntactically valid values;
- add revoked, left, stale binding, satellite, already-provisioned, and concurrent
  double-provision regressions.

## R3 — status-table rebuild resets SQLite AUTOINCREMENT high water (critical)

Location: `identity_schema.go:291-380`.

The rebuild copies live rows into `orbits_new`, drops `orbits`, and renames the
replacement without preserving `sqlite_sequence`. Root fixture: create orbit ID
100, delete it, rebuild an unconstrained-status database. Sequence changed from
100 to 1 and the next orbit received ID 2. A new tenant can therefore inherit an
old orbit identifier still present in retained media/settings/audit data.

Required correction:

- capture the pre-rebuild `sqlite_sequence` high-water value on the same pinned
  connection before dropping the table;
- after rename, restore `max(old high water, current max(id))` atomically;
- test an actual migration with a deleted high ID and assert the next ID is 101,
  including rollback/error paths.

## R4 — old-binary dissolution cannot reach the cleanup intended for it (high)

Locations: `store.go:104-111`, `identity_schema.go:168-181`,
`identity_schema.go:291-380`, `identity_schema.go:542-566`.

On open, unconstrained `orbits.status` is rebuilt before `ReconcileIdentity`.
The rebuild calls `foreign_key_check`; expected stale additive children from an
old binary's FK-off dissolution make it abort. The cleanup for exactly those rows
exists only in the later, now-unreachable reconciliation.

Required correction:

- order or combine repair and narrowly authorized stale-child cleanup so this
  downgrade state can migrate;
- continue failing closed for unrelated FK corruption;
- add a combined partial-schema + old-binary dissolution fixture and a distinct
  unrelated-corruption fixture.

## R5 — flag-on `LeaveOrbit` is transactionally racy (critical)

Location: `orbits.go:491-544`.

Membership, role, and member count are read through `s.db` before the write
transaction. Two stores can both observe count 2, then delete sequentially and
leave an active orbit with zero members. A concurrent `TransferPrimary` can make
the leaver primary after the stale role read, allowing deletion without
promotion. A last-member decision can also race `AddMember` and dissolve the
newly joined member's orbit.

Required correction:

- on the feature-on dual-write path, acquire the immediate transaction first;
- re-read membership, role, count, orbit state, promotion candidate, and every
  mutation through that transaction;
- serialize dissolution, transfer, add, and leave decisions across two Store
  handles;
- add deterministic barrier-driven schedules for leave/leave,
  leave/transfer-primary, and last-leave/add-member; assert no active zero-member
  orbit, exactly one primary when active, and coherent additive projection.

`BootstrapLegacyOrbit` (`orbits.go:850+`) has the same pre-transaction count
pattern. Recheck/claim seed eligibility inside the transaction and add a
two-store test preventing duplicate legacy orbits.

## R6 — migration cleanup/restoration errors are suppressed (high)

Location: `identity_schema.go:291-320`.

The deferred rollback error is ignored, and foreign-key restore/read/assertion
errors are recorded only when `retErr == nil`. A preceding migration failure can
therefore hide failure to restore `foreign_keys=ON`; panic paths cannot report a
restore failure at all. This contradicts the accepted restore/assert-on-every-exit
contract.

Required correction:

- use a robust cleanup/finally path that joins the primary, rollback, restore,
  and postcondition errors without discarding any;
- restore and assert the pragma on success, arbitrary SQL failure, behavior-probe
  failure, commit failure, and panic (re-panic only after cleanup);
- add fault-injection tests for each reachable cleanup edge.

## R7 — identity DDL is not demonstrably atomic (high)

Location: `identity_schema.go:168-181`.

`s.db.Exec(identitySchema)` executes a multi-statement script without an explicit
transaction. A late statement failure can leave a partially installed identity
schema and indexes. The task requires transaction-safe additive migration.

Required correction:

- execute identity DDL with explicit immediate transactional ownership or prove
  the driver gives equivalent all-or-nothing semantics;
- add a deterministic late-DDL failure/crash fixture, reopen it, and prove either
  complete rollback or safe idempotent recovery while feature-off remains
  tolerant.

## R8 — mandatory migration/rollback matrix is absent (high)

Current tests do not discharge the implementation/review guards. Missing or
insufficient cases include:

- handcrafted old DBs with multiple orbits, all member roles, live and revoked
  slots, and existing node tokens;
- the complete resolver negative matrix: revoked/left/misaligned bindings,
  control satellite, multiple and cross-domain matches;
- concurrent reconciliation/backfill and real uniqueness conflicts;
- feature-flag env invalid/non-1/off/unset precedence and YAML preservation;
- two full new-on -> projection -> previous-binary mutation -> re-enable cycles,
  including add/name/leave/transfer/pair/revoke/rebind/dissolve and quota change;
- projection and restoration crash barriers, emergency rollback gap, and a real
  previous coordinator binary rather than only current-code emulation;
- sequence preservation and every migration cleanup failure from R3/R6.

Map each new test by name to the applicable guard. Do not label subset coverage
as a full rollback proof.

## Required rework evidence

The next producer outcome must include:

1. exact changed files and invariants addressed R1-R8;
2. deterministic regression-test map, with every failure schedule above covered;
3. full `go test -count=1 ./...`, focused repeated tests, `go test -race`,
   `go vet`, `go build`, formatting/diff/board validation results;
4. explicit remaining gaps, especially any real-previous-binary or crash test not
   actually executed;
5. no claim of acceptance: a new independent review and a new root line-by-line
   review are mandatory.

