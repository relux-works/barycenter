# TASK-260712-m5264f — developer review handoff

Date: 2026-07-13

## Outcome

Implemented the Phase 1 coordinator self-service onboarding and shared capability surface behind `self_service_onboarding`:

| Method and path | Contract implemented | Principal |
|---|---|---|
| `POST /v1/onboarding/orbits` | Atomic orbit/app actor/primary membership/slot creation; separate node and control credentials; one-time recovery material; `201` | Bootstrap with source-IP and installation-attempt limits; ambient bearer authority is rejected |
| `POST /v1/device-invites` | Atomic 15-minute invite issue; hash-only storage; `201` | Active primary/companion control token |
| `POST /v1/device-invites/consume` | Uniform fixed-shape validation, live-slot capacity, safe revoked-slot generation reuse, atomic one-winner consume, separate node/control credentials, no recovery material; `200` | One-time invite code with source-IP limit |
| `POST /v1/recovery/consume` | Uniform anchored validation, dummy digest for unusable lifecycle, first consume and exact-tuple replay, control-only replacement, node preservation; `200` | Recovery handle/secret plus client-generated replacement control token; source-IP and handle limits |
| `POST /v1/recovery/rotate` | Atomic explicit generation replacement and one-time secret return; `200` | Active primary/companion control token with actor limit |
| `GET /v1/actor/context` | Shared node/control probe with exact `401` credential versus `403` lifecycle classification; `200` | Node or control token |
| `POST /v1/telegram-links` | Atomic prior-code invalidation plus 15-minute hash-only code issue, without a code-bearing URL; `201` | Active primary/companion control token with actor limit |

No public Telegram-consume HTTP route, second identity resolver, alternate schema, client secure-storage behavior, or Phase 2 surface was added. The trusted in-process Telegram consume/adapter remains owned by sibling task `TASK-260712-2xkyot`.

## Security and lifecycle invariants

- Node and control credentials are independent 256-bit lowercase-hex values. Recovery/invite/link secrets use the frozen 27-character rejection-sampled alphabet and exceed 128 bits of entropy.
- SQLite receives only credential digests; recovery plaintext is returned only by initial create or deliberate rotation. Invite joins return node/control credentials only.
- Authenticated mutation entrypoints hash the bearer and generated material before `BEGIN IMMEDIATE`. Transaction helpers receive fixed digests and compare digest bytes with `crypto/subtle`; no bearer or generated plaintext is logged or persisted.
- Invite and recovery failures use anchored fixed-shape reads. Device invite performs one submitted-code SHA and one digest comparison. Recovery selects the real digest only for a usable actor/membership/orbit/slot generation and otherwise selects the precomputed dummy digest before its one submitted-secret comparison.
- Invite validation rechecks issuer actor, membership, orbit, control lifecycle, unrevoked slot, binding token, and paired generation in the consume transaction. Full live capacity is uniformly `credential_invalid`; revoked coordinates are safely reused by retiring stale additive ownership before the authoritative slot UPSERT.
- Recovery permits a current satellite actor to replace only its control credential. Role remains satellite, node authority is unchanged, and the replacement control token still cannot rotate, invite, link, or reach control-admin handlers.
- Every onboarding-owned concurrent-writer proof uses independent Store connections. Writer two signals from a production-neutral checkpoint immediately before its actual `db.Begin`; it cannot return until writer one releases its post-acquisition hold.
- The in-memory sliding limiters are concurrency-safe, count every syntactically valid request including rejected attempts, retain bounded per-key state, and compute the exact next admissible boundary. This is the accepted single-process Phase 1 durability boundary, not a distributed guarantee.
- Forwarded scheme/source fields are accepted only from a configured loopback proxy boundary, with exactly one canonical `X-Forwarded-Proto: https` and a valid rightmost `X-Forwarded-For` hop. Direct TLS uses the direct peer and ignores spoofed forwarding identity.
- Error responses are stable JSON envelopes with `Cache-Control: no-store`; only `429` includes positive `Retry-After`. Unexpected resolver/store errors are generic `500` responses and logs contain no bearer or request secret.

## Accepted identity foundation delta requiring fresh review

The prior accepted `coordinator/internal/store/identity.go` SHA-256 was `128d3f4379a184733b620fcb062291817e456efa9210ce80f428869ec230ae2c`. It is intentionally stale; the current file hash is recorded below.

Exact semantic delta:

1. Control and node resolver joins now require `COALESCE(sl.paired_at, 0) = ic.slot_paired_at`, so unchanged coordinates/token hashes cannot authenticate across a new slot generation.
2. Valid node/control credentials with a left membership or disabled orbit return only minimal non-secret actor/orbit/slot/capability context with `ErrInsufficientCapability`; role is withheld. Unknown, revoked, stale-binding, ambiguous, and cross-domain credentials still return zero context with `ErrUnauthorized`.
3. Authenticated mutation rechecks accept a precomputed bearer digest, compare it to stored control/node digests inside the writer transaction, preserve node-as-insufficient classification, and never carry plaintext bearer into that helper.
4. Telegram resolution now distinguishes unknown, known-revoked, deliberately-left/no-active-membership, and active-disabled identities without granting capabilities. Non-positive Telegram principals are rejected before any storage query.
5. A fixed-digest comparison helper was factored from the existing hash-and-compare helper; both decode 32-byte digests and use `subtle.ConstantTimeCompare`.

Audited callers: `ResolveActorContext`, `ResolveTokenActorContext`, `ResolveTelegramActorContext`, `LookupPlaybackToken`, onboarding control middleware, transactional onboarding mutations, hub websocket lookup, and tenant-scoped media lookup. The full identity/migration/rollback/previous-head Store matrix was rerun.

The prior accepted `identity_schema.go` hash `b6bf8fd25d273473522424ed507f50d488ca6ba5856d29ef331f794e0a4269e6` is also stale. Its task-owned delta preserves an app-first primary/companion/satellite membership for `paired_by=0` only after a control credential exists; unprovisioned legacy orphan slots retain the conservative satellite projection.

During the shared-tree audit, root briefly observed a duplicated `ProvisionInstallationSecrets` condition while concurrent edits were active. Immediate reread showed one valid condition; no checkout/reset/overwrite was used. Current formatting, vet, build, and test evidence compile that file.

## Acceptance criteria and test mapping

| Criterion | Production-path evidence |
|---|---|
| Separate credentials, recovery shown once, hash-only persistence | `TestCreateSelfServiceOrbitMintsSeparatedHashOnlyCredentials`, `TestOnboardingHTTPCreateContextAndSecretRedaction`, `TestDeviceInviteHTTPJoinAndUniformReplay`, `TestRecoveryHTTPExactContractAndRotation` |
| Capability × role and node negative admin/upload behavior | `TestCapabilityMiddlewareRejectsNodeAcrossAdministrationSurfaces`, `TestCapabilityRoleMatrixForOnboardingMutations`, `TestControlMutationOrderingForSatelliteAndNode`, `TestDeviceInviteCapabilityLifecycleAndOneTimeConsume` |
| Exact request/error ordering and malformed auth rejection | `TestRequestObjectsAndPreNormalizationBoundsRejectBeforeWork`, `TestMalformedOrMultipleAuthorizationStopsBeforeAuthDispatch`, `TestOnboardingErrorEnvelopeAndTransportAndFlagOff`, `TestResolverInternalFailureIsGeneric500WithoutCredentialLeak` |
| Uniform invite/recovery failures and constant-time/dummy target shape | `TestDeviceInviteInvalidIssuerMatrixHasFixedValidationAndNoSideEffects`, `TestDeviceInviteUniformFailuresUseOneValidationReadAndHash`, `TestRecoveryValidationUsesOneReadAndOneSubmittedSecretHash` |
| Expiry, replay, role/lifecycle/generation/capacity negatives | `TestDeviceInviteExpiredAndIssuerInvalidatedAreUniform`, `TestDeviceInviteNullableIssuerGenerationAndFullOrbit`, `TestRecoveryAndAuthenticatedMutationsRejectInvalidLifecycle`, `TestActorResolverRejectsPairedGenerationMismatchForBothDomains` |
| Revoked-slot reuse and stale feature-off coexistence | `TestDeviceInviteReusesRevokedSlotAndRetiresStaleCredential` (uses the real feature-off `Open` + `RevokeSlot` path, not SQL emulation) |
| Recovery node preservation, satellite no-escalation, rotation | `TestRecoveryConsumeReplayRotateAndNodePreservation`, `TestSatelliteRecoveryRestoresControlWithoutAuthorityEscalation` |
| Single-winner/idempotent and rotation serialization | `TestConcurrentDeviceInviteConsumeHasOneWinner`, `TestConcurrentRecoverySameTupleIsIdempotentWithOneAudit`, `TestRecoveryRotationConsumeSerialization` |
| Prehash ordering and transaction re-auth | `TestAuthenticatedMutationsPrepareDigestsBeforeWriterTransaction`, `TestAuthenticatedMutationRechecksBearerAndRoleInsideTransaction` |
| Audit and state rollback atomicity | `TestCreateSelfServiceOrbitRollbackIncludesAuditAndSecrets`, `TestOnboardingMutationsRollbackWhenAuditBoundaryFails` |
| Link issue hash-only, invalidation, no secret URL | `TestTelegramLinkIssueInvalidatesPriorAndPersistsHashOnly`, `TestTelegramLinkHTTPContractHasNoSecretURL` |
| Rate limits, transport boundary | `TestRecoveryLimiterOrderingBoundaryAndExactEnvelope`, `TestAttemptLimiterCountsRejectedAttemptsAndConcurrentBoundary`, `TestForwardedHeadersRequireLoopbackProxyPeer` |
| Legacy pair/websocket and flag compatibility | `TestSelfServiceFlagPreservesLegacyPairAndWebSocketRegistration`, `TestOnboardingStoreFeatureOff`, `TestActorResolverFeatureGateAndLegacyCompatibility` |
| Identity lifecycle classification and migration coexistence | `TestActorResolverPartialContextIsMinimalAndInvalidCredentialsAreZero`, `TestTelegramResolverClassifiesKnownLifecycleFailuresWithoutCapabilities`, full `R1`–`R8`/identity/rollback matrix command below |

No skips or mock-only SQL substitutes are counted as evidence.

## Verification commands and results

Commands were run from `coordinator/` unless an absolute/root-scoped path is shown.

```text
go test -count=10 ./internal/store -run 'Test(AuthenticatedMutationsPrepare|DeviceInviteReuses|DeviceInviteNullable|ConcurrentDeviceInvite|ConcurrentRecovery|RecoveryRotationConsume|RecoveryValidation|SatelliteRecovery)' && go test -count=10 ./cmd/duet-coordinator -run 'Test(Onboarding|ConcurrentInstallation|Capability|Authenticated|ControlMutation|DeviceInviteHTTP|RecoveryHTTP|TelegramLinkHTTP|RecoveryLimiter|RequestObjects|MalformedOrMultipleAuthorization|AttemptLimiter|ForwardedHeaders|ResolverInternal)'
PASS — internal/store 5.898s; cmd/duet-coordinator 3.382s

go test -count=1 ./...
PASS — every coordinator package; internal/store 5.761s; cmd/duet-coordinator 2.305s

go test -race -count=1 ./...
PASS — exit 0 for the full coordinator package set; the captured package stream includes cmd/duet-coordinator and all non-Store packages, and the Store critical race command below provides explicit Store output

go test -race -count=1 ./internal/store -run 'Test(ConcurrentDeviceInvite|ConcurrentRecovery|RecoveryRotationConsume|DeviceInviteReuses|AuthenticatedMutationsPrepare|RecoveryValidation)'
PASS — internal/store 4.107s

go test -count=1 ./internal/store -run 'Test(R[1-8]|Identity|ActorResolver|FeatureOn|Rollback|Reconciliation|OrbitStatus|Legacy|HashOnly|TelegramMigration|TelegramResolver)'
PASS — internal/store 2.329s

go vet ./... && go build ./... && test -z "$(gofmt -l cmd/duet-coordinator/main.go cmd/duet-coordinator/onboarding.go cmd/duet-coordinator/onboarding_test.go cmd/duet-coordinator/onboarding_compat_test.go internal/store/identity.go internal/store/identity_schema.go internal/store/onboarding.go internal/store/onboarding_test.go)" && git diff --check
PASS — exit 0, no output
```

Corrected failures retained in the work record:

- Two early `gofmt` invocations used a `coordinator/` prefix while already inside `coordinator/`; both failed with `lstat ... no such file or directory`. The corrected relative-path formatting command passed.
- The first focused Store run after satellite recovery was enabled failed because an older test still expected satellite recovery denial. The test was split into lifecycle denial versus recovery-without-escalation; focused and repeated runs pass.
- An early HTTP test draft attempted a nonexistent exported `Store.DB` accessor; it was replaced with an independent SQLite inspection connection. A subsequent format/type mismatch in that new assertion was corrected before the passing focused run.
- The first two combined race attempts intersected live sibling edits: one temporarily lacked an `errors` import; the next exposed an in-progress Telegram lifecycle fixture ordering problem. The sibling preserved ownership and corrected both; subsequent uncached and race runs pass.
- One focused command was launched from the repository root and correctly failed with `go: cannot find main module`; rerunning the exact test from `coordinator/` passed.

## Current file SHA-256

```text
0941be0d0df477884aa90f967b7eca65daacd2f928d56b8555e725199a4e9afc  LOGBOOK.md
ebac82641471039ec0dcb66e3f4fa8f49b543d38a71aa5caa9e56f030e26039d  coordinator/cmd/duet-coordinator/main.go
6aa01fd1ee8f34526ebfba9db4807e468c46850b0e13bcb38ba6510a2a3064c3  coordinator/cmd/duet-coordinator/onboarding.go
02d18a2c64dc3d39f447d5057ca0a5cfa735f94cc7f1da82d9fdcb359213fd95  coordinator/cmd/duet-coordinator/onboarding_test.go
f37935809bd369df543b9e2a67333a81a6c8da3f73e8fa6da081f559cb08e0b4  coordinator/cmd/duet-coordinator/onboarding_compat_test.go
dcd4cc3c1188569439335c1742c657cb4235aec223f1d2ed5f4cb4fcde0de5dd  coordinator/internal/store/identity.go
892238d4d8d6aa3adbeb7c9a1009df693d84fb9803c5fa21b718521ca33472bb  coordinator/internal/store/identity_schema.go
66948069ac47d7a8f2f21718149f129a1ff89bba8b47b20fe863a550e5c1ea43  coordinator/internal/store/onboarding.go
4e059c4684cc9fb78e19a12217ed5c090ad64f9dfb8a9fce595daef1898612a7  coordinator/internal/store/onboarding_test.go
```

## Dirty-worktree ownership

Task-owned files/hunks are the nine hashed files above. `main.go`, `identity.go`, `identity_schema.go`, and `LOGBOOK.md` are shared foundation files; changes were limited to the disclosed onboarding/identity hunks and the task logbook block. No commit, push, reset, checkout, or unrelated cleanup was performed.

The full status at evidence freeze was:

```text
 M .github/workflows/ci.yml
 M LOGBOOK.md
 M coordinator/cmd/duet-coordinator/loop.go
 M coordinator/cmd/duet-coordinator/main.go
 M coordinator/internal/bot/bot.go
 M coordinator/internal/bot/bot_test.go
 M coordinator/internal/bot/commands.go
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
?? coordinator/cmd/duet-coordinator/onboarding.go
?? coordinator/cmd/duet-coordinator/onboarding_compat_test.go
?? coordinator/cmd/duet-coordinator/onboarding_test.go
?? coordinator/cmd/duet-coordinator/telegram_identity_test.go
?? coordinator/internal/store/identity.go
?? coordinator/internal/store/identity_migration_rework_test.go
?? coordinator/internal/store/identity_previous_head_integration_test.go
?? coordinator/internal/store/identity_rollback_rework_test.go
?? coordinator/internal/store/identity_schema.go
?? coordinator/internal/store/identity_security_rework_test.go
?? coordinator/internal/store/identity_telegram.go
?? coordinator/internal/store/identity_telegram_previous_head_test.go
?? coordinator/internal/store/identity_telegram_test.go
?? coordinator/internal/store/identity_test.go
?? coordinator/internal/store/onboarding.go
?? coordinator/internal/store/onboarding_test.go
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
?? task-board.config.json
```

## External and review boundaries

- Rate-limit state is intentionally process-local and resets on restart. No distributed or restart-durable claim is made.
- The trusted proxy boundary implemented here is the actual configured loopback terminator boundary, not a general proxy allow-list system.
- Client Keychain/DPAPI handling, UI one-time display, Windows execution, external CI, and Phase 2 behavior are outside this coordinator task and were not claimed as local evidence.
- Telegram link consume and bot adapter code belong to `TASK-260712-2xkyot`. Root directive T10 requesting an exact pre-`db.Begin` checkpoint for its two consume races arrived after that sibling run became terminal and was rejected by the runtime. Per the explicit shared-worktree boundary, this task did not edit those sibling-owned files. That deterministic Telegram-consume race follow-up must be resolved in sibling review/rework before downstream acceptance treats the combined identity/onboarding story as accepted.
- The task requires a fresh independent security/protocol/migration review and a new root line-by-line/hash/test audit. Prior `TASK-260712-1bpog0` identity acceptance cannot cover the disclosed hash delta.
