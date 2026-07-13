# TASK-260712-1bpog0 — implementation and R1–R8 rework evidence

Date: 2026-07-13  
Role: developer  
Intended handoff status: `to-review` (independent review and root line-by-line review remain mandatory)

## Outcome

Implemented the additive transport-neutral identity foundation and corrected the complete root round-1 R1–R8 rework contract. Legacy `members` and `slots` remain authoritative while `self_service_onboarding` is off. Feature-on startup reconciles those surfaces into actors, memberships, and generation-bound installation credentials before actor-aware serving.

No commit or push was created. No plaintext server-side credential is present in production persistence, committed fixtures, logs, screenshots, or this outcome. The exact-previous-HEAD integration gate transfers two ephemeral old-minted node tokens only through a mode-0600 test-temporary file, deletes it with the test temporary directory, never prints their values, and verifies only their behavior.

## Exact changed files

Task-scoped files at handoff:

- `.github/workflows/ci.yml` — coordinator checkout now fetches history and CI explicitly executes the pinned previous-HEAD gate. This dirty file also contains unrelated pre-existing `pulsar-win-packaged-probe` edits, which were preserved.
- `LOGBOOK.md` — added R1–R8 identity decisions/evidence. Existing unrelated logbook entries were preserved.
- `coordinator/cmd/duet-coordinator/main.go` — passes the feature option into Store and uses status/capability-aware playback lookup.
- `coordinator/cmd/duet-coordinator/identity_auth_test.go` — HTTP cross-orbit node/control escalation regression.
- `coordinator/internal/config/config.go` — YAML/env `self_service_onboarding` flag.
- `coordinator/internal/config/config_env_test.go` — flag default, YAML preservation, and env precedence matrix.
- `coordinator/internal/store/store.go` — Store option, immediate SQLite transactions, additive migration startup, and test-only deterministic checkpoints.
- `coordinator/internal/store/orbits.go` — feature-on transactional dual writes, lifecycle serialization, collision-safe node minting, rebind/revoke cleanup, and legacy bootstrap serialization.
- `coordinator/internal/store/identity.go` — shared `ActorContext`, node/control/Telegram resolution, canonical hashed lookup, and guarded initial provisioning.
- `coordinator/internal/store/identity_schema.go` — additive DDL, legacy backfill/reconciliation, rollback projection/restoration, constrained-status rebuild, sequence preservation, and cleanup/error guarantees.
- `coordinator/internal/store/identity_test.go` — baseline migration, persistence, resolver, rollback, and schema coverage.
- `coordinator/internal/store/identity_security_rework_test.go` — R1/R2 and resolver negative/security matrix.
- `coordinator/internal/store/identity_migration_rework_test.go` — R3/R4/R6/R7 migration/fault matrix.
- `coordinator/internal/store/orbits_concurrency_rework_test.go` — deterministic two-Store R5 schedules.
- `coordinator/internal/store/identity_rollback_rework_test.go` — R8 old-database, rollback, projection, interruption, re-enable, and reconciliation matrix.
- `coordinator/internal/store/identity_previous_head_integration_test.go` — timeout-bounded tagged integration gate against exact revision `e8bd240664a40b9cc78b974f3c34ad30712e2aa5`.
- `coordinator/internal/store/testdata/previous_head_authority_test.go` — runtime-injected driver that calls the exact previous Store API rather than emulating it with SQL.

SHA-256 at handoff:

```text
d94f33c2fdad98113f01119fbe4d6488762a5d08115315c9248ed903e4c93922  .github/workflows/ci.yml
45b137cde16331d200871928fb6947b02879f24e44cfc4af1b162080eb4b3ef5  LOGBOOK.md
d3918d93ca775d6f894c019f1f1c4ac65ee528fc24ceb1e913cb64660a33848a  coordinator/cmd/duet-coordinator/main.go
95c105ab5f5fde24bda7de04468959ea3e957fa86c6b0a1330d7631f0291d58a  coordinator/cmd/duet-coordinator/identity_auth_test.go
8f31e69d863d79dccb4a44f9dcf045f01f03cfe4341f32e082189184162da824  coordinator/internal/config/config.go
bc92967bcba0ab122fe81b4c82b370dced27e0a35add11bb15e35bbea5c395e4  coordinator/internal/config/config_env_test.go
ffefbf2510ce5d8d18c4aa39dbe30baaf75c5d3acbeac62359e944621ca6b34a  coordinator/internal/store/store.go
63bd1e1717fd4c964aff470ab3af05932aa66e0b8543372bc9c1a2aa25cc8450  coordinator/internal/store/orbits.go
128d3f4379a184733b620fcb062291817e456efa9210ce80f428869ec230ae2c  coordinator/internal/store/identity.go
b6bf8fd25d273473522424ed507f50d488ca6ba5856d29ef331f794e0a4269e6  coordinator/internal/store/identity_schema.go
74e7a16c543eb118ceabe52af5ccbd556326785fdfad1ed233d12efd2e9d7547  coordinator/internal/store/identity_test.go
e4d5e0c790a20981d63d2fd9dff890e34832db4387c9b6aa998b84b7c28b3d27  coordinator/internal/store/identity_security_rework_test.go
ec910b369798981a6906c8ba1385cdcbb03769f2da2964ca9891c7bafe33544f  coordinator/internal/store/identity_migration_rework_test.go
5c7cdadfa6bb8d91f654badb838567857912de45f5b2a16095abe8030fbaaf27  coordinator/internal/store/identity_rollback_rework_test.go
7410edc114317306fb1ce14a22218699cd09bbc35cb5cd70e66e306c326e47e8  coordinator/internal/store/identity_previous_head_integration_test.go
5098a15a1aa00e0ed4dd34cd3b6964bb07bb41eb22062b515c187b37930e1ce2  coordinator/internal/store/orbits_concurrency_rework_test.go
5fc9fcbdc85f963e126c9627e1604a98885ad01559f6acc65e2c89f22ed34009  coordinator/internal/store/testdata/previous_head_authority_test.go
```

## Acceptance criteria and checklist mapping

### Existing databases migrate without changing roles, ownership, or live node-token validity

- Identity DDL is additive and installed atomically under the Store DSN's immediate writer ownership.
- Telegram `members` backfill to `telegram_user` actors and memberships with their exact legacy role. Slot installation actors are generation-bound to orbit, slot, paired-at, and the existing token hash; neither `slots.token_hash` nor `slots.paired_by` is rewritten by reconciliation.
- Representative handcrafted pre-feature databases cover multiple orbits, primary/companion/satellite roles, live and revoked slots, and real token lookup.
- `orbits.status` rebuild preserves schema objects and the `sqlite_sequence` high-water value, including deleted high IDs and rollback/retry.
- Old-binary missing-orbit cleanup is narrowly limited to additive children whose orbit no longer exists; unrelated FK corruption still fails closed.

Evidence: `TestIdentityMigrationBackfillsRepresentativeLegacyDatabase`, `TestR8MultiOrbitLegacyBackfillPreservesRolesSlotsAndTokens`, `TestR3OrbitStatusRebuildPreservesAutoincrementHighWater`, `TestR3OrbitStatusRebuildRollsBackSequenceOnFailure`, `TestR4PartialSchemaAndOldBinaryDissolutionMigrates`, `TestR4UnrelatedForeignKeyCorruptionStillFailsClosed`.

### Newly introduced server-side secrets are hash-only at rest

- Control tokens, recovery secrets, device invite codes, and Telegram link codes persist only SHA-256 hex digests. Recovery IDs are selectors, not authenticators. Node tokens retain the existing `slots.token_hash` convention.
- Hash comparison canonicalizes recovery material and uses `subtle.ConstantTimeCompare` over decoded fixed-length hashes.
- Audit rows record only event type and actor/orbit coordinates.

Evidence: `TestHashOnlyCredentialPersistenceAndActorLookup`, identity DDL hash constraints, and repository secret-literal scan. No token value is emitted by the pinned rollback test.

### Shared resolver distinguishes node and control and returns orbit, role, actor

- `ActorContext` returns `OrbitID`, `ActorID`, `Role`, `Slot`, and capability bits.
- Resolution counts node and control digest matches before lifecycle lookup and requires exactly one total match. Zero, duplicate, and cross-domain matches fail closed.
- Node is playback-only; control includes control plus node; Telegram is a verified transport principal. Revoked actors, left/misaligned memberships, disabled orbits, stale bindings, and satellite-control attempts are rejected with the intended fail-closed distinction.

Evidence: `TestR1CredentialDomainsRejectSameValueAndAmbiguousResolution`, `TestR1OldDatabaseCrossDomainCollisionFailsFeatureOn`, `TestR8ActorResolverAmbiguityAndAlignmentNegativeMatrix`, `TestActorResolverDistinguishesLifecycleFromCredentialFailure`, `TestNodeTokenCannotEscalateAcrossOrbitMediaAuthorization`.

### Resolver and dual-write behavior are gated by `self_service_onboarding`

- The default/zero-value Store path remains feature-off. Additive DDL is tolerated, but the actor resolver refuses service and legacy lookups/mutations remain available.
- YAML value is preserved unless the env variable is set; only exact env value `1` enables. Unset, empty, `0`, `off`, invalid, and non-`1` values are covered.

Evidence: `TestActorResolverFeatureGateAndLegacyCompatibility`, `TestSelfServiceOnboardingDefaultsOffAndLoadsFromYAML`, `TestSelfServiceOnboardingEnvironmentPrecedenceAndYAMLPreservation`, `TestContainerConfigWithoutEnv`.

### Rollback to the previous coordinator tolerates additive rows while the flag is off

- The exact pinned pre-identity source is archived from commit `e8bd240664a40b9cc78b974f3c34ad30712e2aa5`, receives a test driver, and calls its actual `CreateOrbit`, `AddMember`, `SetMemberName`, `TransferPrimary`, `RevokeSlot`, `PairSlot`, `LookupToken`, `LeaveOrbit`, and `DeleteOrbit` methods.
- After current reconciliation, both node tokens minted by that exact source authenticate in their retained tenant as node-only and have new generation actors; revoked/left/dissolved prior authority remains retired.
- CI uses `fetch-depth: 0` and explicitly runs the tagged pinned gate, so missing shallow history cannot be silently skipped.
- Two projection generations, quota changes, emergency rollback gap containment, projection/restoration interruption barriers, and unchanged/rebound projected-slot re-enable branches are covered.

Evidence: `TestR8ExactPreviousHEADAuthorityRoundTrip`, `TestR8FullPreviousAuthorityMutationEmulationReconciles` (supplemental current-code emulation, explicitly named), `TestR8TwoRollbackProjectionGenerationsPreserveQuotaChanges`, `TestR8ProjectionInterruptionAfterJournalIsAtomicAndRetryable`, `TestR8RestorationInterruptionAfterQuotaUpdateIsAtomicAndRetryable`, `TestR8ProjectedSlotReenableBranches`, `TestR8EmergencyRollbackGapIsContainedOnReenable`.

## Root R1–R8 correction and deterministic test map

### R1 — node/control capability domains

- Provisioning rejects any control digest already present in either domain while holding the immediate transaction.
- Pairing mints random node material inside the Store transaction and rejects any digest found in slots or control credentials.
- Startup reconciliation asserts global cross-domain disjointness.
- Resolver queries both domains and requires one total match; query order cannot escalate capability.
- Public-API same-value/cross-orbit, old-database, duplicate/ambiguous, and HTTP tenant isolation are covered.

Tests: `TestR1CredentialDomainsRejectSameValueAndAmbiguousResolution`, `TestR1OldDatabaseCrossDomainCollisionFailsFeatureOn`, `TestR2ConcurrentInitialProvisioningHasSingleWinner`, `TestNodeTokenCannotEscalateAcrossOrbitMediaAuthorization`.

### R2 — initial provisioning lifecycle and generation

- Authority and target lifecycle are re-read inside one immediate transaction.
- Target must be a non-revoked app installation, active aligned same-orbit member, permitted non-satellite role, active orbit, and exact live slot generation.
- Every provisioning field must still be NULL; the conditional update is single-winner and never acts as rotation/recovery/revival.

Tests: `TestR2ConcurrentInitialProvisioningHasSingleWinner`, `TestR2InitialProvisioningTargetLifecycleMatrix` subtests `revoked_target`, `left_target`, `stale_binding`, `satellite_target`, `already_provisioned`.

### R3 — AUTOINCREMENT high-water preservation

- Captures `sqlite_sequence` before dropping `orbits` on the pinned connection and restores `max(old sequence, max(id))` atomically after rename.

Tests: `TestR3OrbitStatusRebuildPreservesAutoincrementHighWater`, `TestR3OrbitStatusRebuildRollsBackSequenceOnFailure`.

### R4 — old-binary dissolution before status rebuild

- Installs additive schema, then cleans only additive children with a missing orbit before the status rebuild/global FK check.
- A distinct unrelated-FK fixture remains rejected.

Tests: `TestR4PartialSchemaAndOldBinaryDissolutionMigrates`, `TestR4UnrelatedForeignKeyCorruptionStillFailsClosed`.

### R5 — serialized lifecycle/legacy bootstrap

- `_txlock=immediate` acquires the writer lock at `Begin`; leave reads role/count/status/primary after acquisition and performs promote/delete/reconcile in that transaction.
- Add, transfer, leave/dissolve, and legacy seed eligibility serialize across two Store handles.

Tests: `TestR5ConcurrentLeaveLeaveMaintainsOrbitInvariant`; `TestR5ConcurrentLeaveTransferPrimaryMaintainsOrbitInvariant` (`leave_first`, `transfer_first`); `TestR5ConcurrentLastLeaveAddMemberMaintainsOrbitInvariant` (`leave_first`, `add_first`); `TestR5ConcurrentLegacyBootstrapClaimsSeedOnce`.

### R6 — cleanup/restoration error preservation

- Status rebuild joins primary, rollback, pragma restore, pragma read, and postcondition failures.
- Cleanup runs for success, SQL/probe/commit errors and panic; panic is re-raised only after cleanup.

Test: `TestR6OrbitStatusRebuildCleanupFaultMatrix` covers arbitrary SQL failure, FK check, behavior probe, pre-commit, rollback failure, restore failure, read failure, combined failures, and panic restoration.

### R7 — atomic identity DDL

- Entire multi-statement identity DDL runs inside an explicit immediate transaction.
- A deterministic late-object conflict proves no partial table/index installation; obstacle removal and feature-off reopen recover idempotently.

Test: `TestR7IdentityDDLIsAtomicAndFeatureOffRecoveryIsIdempotent`.

### R8 — mandatory migration/rollback matrix

- Multi-orbit/all-role/live+revoked handcrafted DB: `TestR8MultiOrbitLegacyBackfillPreservesRolesSlotsAndTokens`.
- Full previous-authority surface: `TestR8ExactPreviousHEADAuthorityRoundTrip`; supplemental emulation: `TestR8FullPreviousAuthorityMutationEmulationReconciles`.
- Resolver revoked/left/misaligned/satellite/multiple/cross-domain matrix: `TestR8ActorResolverAmbiguityAndAlignmentNegativeMatrix` plus R1/R2 tests.
- Concurrent reconciliation and uniqueness convergence: `TestR8ConcurrentReconciliationAndBackfillIsIdempotent` plus `TestR2ConcurrentInitialProvisioningHasSingleWinner`.
- Env invalid/non-1/off/unset/YAML precedence: `TestSelfServiceOnboardingEnvironmentPrecedenceAndYAMLPreservation`.
- Two projection cycles and quota change: `TestR8TwoRollbackProjectionGenerationsPreserveQuotaChanges`.
- Projection journal interruption and retry: `TestR8ProjectionInterruptionAfterJournalIsAtomicAndRetryable`.
- Restoration quota interruption and retry: `TestR8RestorationInterruptionAfterQuotaUpdateIsAtomicAndRetryable`.
- Unchanged/rebound projected-slot re-enable branches: `TestR8ProjectedSlotReenableBranches`.
- Emergency no-projection gap containment: `TestR8EmergencyRollbackGapIsContainedOnReenable`.
- Real previous code, not a moving reference: exact full commit hash constant and CI history guarantee in `TestR8ExactPreviousHEADAuthorityRoundTrip` / `.github/workflows/ci.yml`.

## Exact verification commands and unabridged summaries

Working directory for Go commands: `coordinator`.

### Focused repeated R1–R8 store regressions

```text
$ go test -count=10 ./internal/store -run '^TestR[1-8]'
ok  relux.works/duet/coordinator/internal/store  12.287s
```

### Exact previous-HEAD gate, repeated

```text
$ go test -tags previoushead -count=3 ./internal/store -run '^TestR8ExactPreviousHEADAuthorityRoundTrip$'
ok  relux.works/duet/coordinator/internal/store  6.958s
```

This executed the exact revision locally; it was not an external CI claim.

### Full uncached coordinator suite

```text
$ go test -count=1 ./...
ok   relux.works/duet/coordinator/cmd/duet-coordinator  1.047s
ok   relux.works/duet/coordinator/internal/bot  1.142s
ok   relux.works/duet/coordinator/internal/config  0.753s
ok   relux.works/duet/coordinator/internal/hub  1.511s
ok   relux.works/duet/coordinator/internal/links  2.133s
ok   relux.works/duet/coordinator/internal/media  1.827s
ok   relux.works/duet/coordinator/internal/protocol  1.706s
ok   relux.works/duet/coordinator/internal/resolver  1.786s
ok   relux.works/duet/coordinator/internal/session  2.027s
?    relux.works/duet/coordinator/internal/spotify  [no test files]
ok   relux.works/duet/coordinator/internal/store  3.985s
?    relux.works/duet/coordinator/internal/ulid  [no test files]
```

### Store race suite

```text
$ go test -race -count=1 ./internal/store
ok  relux.works/duet/coordinator/internal/store  13.619s
```

### HTTP tenant isolation and flag parsing, repeated

```text
$ go test -count=10 ./cmd/duet-coordinator -run '^TestNodeTokenCannotEscalateAcrossOrbitMediaAuthorization$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  0.788s

$ go test -count=10 ./internal/config -run '^TestSelfServiceOnboarding'
ok  relux.works/duet/coordinator/internal/config  0.475s
```

### Vet, tagged vet, build, format, diff, board

```text
$ go vet ./...
(exit 0; no output)

$ go vet -tags previoushead ./internal/store
(exit 0; no output)

$ go build ./...
(exit 0; no output)

$ gofmt -l cmd/duet-coordinator/main.go cmd/duet-coordinator/identity_auth_test.go internal/config/config.go internal/config/config_env_test.go internal/store/store.go internal/store/orbits.go internal/store/identity.go internal/store/identity_schema.go internal/store/identity_test.go internal/store/identity_security_rework_test.go internal/store/identity_migration_rework_test.go internal/store/orbits_concurrency_rework_test.go internal/store/identity_rollback_rework_test.go internal/store/identity_previous_head_integration_test.go internal/store/testdata/previous_head_authority_test.go
(exit 0; no output)

$ git diff --check -- .github/workflows/ci.yml LOGBOOK.md coordinator
(exit 0; no output)

$ task-board validate
Board is valid. No issues found.
```

### Implementation-stage anomaly (corrected before the repeated gates)

The first pinned-HEAD driver run failed because the driver called old `LeaveOrbit(202)` before old `RevokeSlot(b)`; the old method correctly revoked member 202's slot, so the later explicit revoke returned `found=false`. The driver was reordered to revoke/rebind `b` before member 202 leaves, ensuring both old methods are truthfully exercised. The corrected gate then passed once and passed the recorded three-run command above. No production workaround was added.

## Worktree scope

Task paths are enumerated above. The repository was already dirty and remains dirty. Unrelated existing paths were not modified for this task, including `docs/idea-air-rooms.md`, `docs/spec.md`, `pulsar-win/.gitignore`, `.planning/`, `.research/`, `.spec/`, most `.task-board/` state, `diagrams/`, `docs/analysis/`, `docs/diagrams/`, `docs/goal-self-contained-audio.md`, `docs/plans/`, `docs/spec-self-contained-audio.md`, `pulsar-win/cmd/`, `pulsar-win/internal/`, `pulsar-win/native/`, `pulsar-win/probe-msix/`, `pulsar-win/pulsar-win-probe.exe`, and `task-board.config.json`.

`git status --short` at evidence capture:

```text
 M .github/workflows/ci.yml
 M LOGBOOK.md
 M coordinator/cmd/duet-coordinator/main.go
 M coordinator/internal/config/config.go
 M coordinator/internal/config/config_env_test.go
 M coordinator/internal/store/orbits.go
 M coordinator/internal/store/store.go
 M docs/idea-air-rooms.md
 M docs/spec.md
 M pulsar-win/.gitignore
?? .planning/
?? .research/
?? .spec/
?? .task-board/
?? coordinator/cmd/duet-coordinator/identity_auth_test.go
?? coordinator/internal/store/identity.go
?? coordinator/internal/store/identity_migration_rework_test.go
?? coordinator/internal/store/identity_previous_head_integration_test.go
?? coordinator/internal/store/identity_rollback_rework_test.go
?? coordinator/internal/store/identity_schema.go
?? coordinator/internal/store/identity_security_rework_test.go
?? coordinator/internal/store/identity_test.go
?? coordinator/internal/store/orbits_concurrency_rework_test.go
?? coordinator/internal/store/testdata/
?? diagrams/
?? docs/analysis/
?? docs/diagrams/
?? docs/goal-self-contained-audio.md
?? docs/plans/
?? docs/spec-self-contained-audio.md
?? pulsar-win/cmd/
?? pulsar-win/internal/
?? pulsar-win/native/
?? pulsar-win/probe-msix/
?? pulsar-win/pulsar-win-probe.exe
?? task-board.config.json
```

## Residual risks and evidence boundaries

- Deterministic SQLite checkpoints inject interruption errors at the exact projection/restoration boundaries and prove transaction rollback plus retry. A real OS process kill or power-loss test was not executed.
- The previous-coordinator test runs the exact pinned source in a nested Go test and its real Store implementation; it does not execute a separately archived production binary artifact.
- The workflow change was validated locally and the tagged test executed locally. No GitHub Actions run was claimed or observed in this run.
- `go test -race -count=1 ./internal/store` was run because Store concurrency is the changed risk boundary. A repository-wide `go test -race ./...` was not run.
- Independent security/migration review and root line-by-line review are still required; this producer outcome does not claim acceptance.
