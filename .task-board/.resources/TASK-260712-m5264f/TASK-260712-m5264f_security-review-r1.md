# Independent security/protocol/migration review — TASK-260712-m5264f

- Task: TASK-260712-m5264f — Implement self-service onboarding and capability APIs
- Review date: 2026-07-13
- Role: independent reviewer
- Verdict: BACK TO DEVELOPMENT

## Executive verdict

The task-owned implementation generally fits the accepted identity architecture: it reuses the shared actor resolver and schema, preserves feature-off and legacy pair/websocket behavior, keeps node and control credentials domain-separated, and uses immediate transactions for onboarding mutations. Independent focused, full, race, vet, build, formatting, diff, and board checks all pass.

Acceptance is blocked by three frozen-contract violations: two security-audit omissions and an app-first reconciliation shortcut that bypasses the orbit-alignment serving invariant. The green tests do not cover these required properties.

A root challenge concerning node-token attempt counting was resolved as not a defect. The endpoint-specific Rev15 rotation/link steps require node wrong-capability rejection in authentication middleware before syntax and reservation; the broader limiter language counts lifecycle/role 403s, not the wrong credential-capability class.

## Authoritative inputs reviewed

- Full task card, implementation guard, identity model diagram, onboarding sequence diagram, and independent-review instructions.
- docs/spec-self-contained-audio.md sections 3.13, 6, 11, 12, 18, and 19.
- Full accepted Rev15 contract at .task-board/.resources/TASK-260712-3v1k7q/research.md.
- Root review amendments at .task-board/.resources/TASK-260712-3v1k7q/p1-root-review-amendments.md.
- Accepted identity-foundation implementation and review evidence for TASK-260712-1bpog0.
- Current producer outcome TASK-260712-m5264f_results.md.
- Actual combined worktree rather than the producer report alone.
- Root audit directives received at cooperative checkpoints.

## Reviewed implementation inventory

Task-owned production code was reviewed line by line:

- coordinator/cmd/duet-coordinator/main.go
- coordinator/cmd/duet-coordinator/onboarding.go
- coordinator/internal/store/onboarding.go
- coordinator/internal/store/identity.go
- coordinator/internal/store/identity_schema.go

Relevant task and compatibility tests were inspected, including:

- coordinator/cmd/duet-coordinator/onboarding_test.go
- coordinator/cmd/duet-coordinator/onboarding_compat_test.go
- coordinator/internal/store/onboarding_test.go
- relevant identity migration/security tests in coordinator/internal/store

The sibling Telegram-consume implementation was treated as an external changing boundary. It does not invalidate the task-owned Telegram-link issue interface, but this review does not accept or certify that sibling surface.

## Findings

### F1 — HIGH / release blocker: recovery rotation audit omits the old and new recovery identifiers

Evidence:

- coordinator/internal/store/identity_schema.go:113-119 defines audit_events with id, orbit_id, actor_id, type, and created_at only. There is no metadata/detail field or related detail table.
- coordinator/internal/store/onboarding.go:724-726 overwrites recovery_id and recovery_secret_hash and resets recovery_consumed_at.
- coordinator/internal/store/onboarding.go:739-740 inserts only the generic event type recovery.rotated.
- The old recovery_id is not selected or retained before the overwrite.

Failure schedule:

1. An installation has recovery generation R1.
2. A valid control-authorized rotation commits generation R2.
3. The audit row records only recovery.rotated.
4. The database can no longer establish from the audit trail that the transition was R1 to R2.

Impact:

Rev15 section 7 explicitly requires the rotation audit to record both old and new recovery_id values. Those are non-secret handles, not plaintext recovery material. The current implementation therefore does not make rotation auditable to the frozen contract, and the missing data cannot be reconstructed after commit.

Required remedy:

Add an accepted durable audit representation for non-secret event details, such as an additive normalized audit detail table or metadata column, and insert old and new recovery_id values in the same transaction as rotation. Add production-path assertions for successful rotation, rollback, and absence of plaintext/digest leakage. If the product instead intends the generic event type to be sufficient, an explicit authoritative contract erratum is required; no accepted erratum currently exists.

### F2 — HIGH / release blocker: HTTP 429 events are not durably recorded in the security audit trail

Evidence:

- coordinator/cmd/duet-coordinator/onboarding.go:404-410 handles limiter rejection by calling api.store.LogEvent and returning the rate-limit envelope.
- coordinator/internal/store/store.go:302-307 describes LogEvent as a debugging event log, inserts into the generic events table, and intentionally ignores the insertion error.
- No audit_events row is written for these rejections.

Failure schedule:

1. An attacker exhausts an onboarding identifier or source attempt budget.
2. The next request correctly receives HTTP 429 and Retry-After.
3. audit_events remains unchanged.
4. If the generic debugging-event insert fails, that failure is silently discarded and the 429 still returns.

Impact:

Rev15 section 12 requires all 429 events to be audited. The current path is neither the accepted security audit trail nor durable/error-checked. Existing tests validate the HTTP response but do not assert a persisted audit event or audit-write failure behavior.

Required remedy:

Define an additive durable security-audit representation capable of recording rate-limit events that may lack an authenticated actor or orbit, persist every frozen limiter class there, handle persistence failure according to the accepted transaction/error policy, and add production-path tests for each 429 class. Resolve this together with F1. The sibling Telegram-consume limiter uses the same generic logger and requires coordination in its own ownership scope; it is not accepted by this review.

### F3 — HIGH / release blocker, task-owned shared identity delta: app-first reconciliation bypasses orbit-alignment failure

Evidence:

- coordinator/internal/store/identity_schema.go:879-885 unconditionally continues for a live paired_by=0 slot when the installation has a control hash.
- That continue bypasses ensureMembershipTx at coordinator/internal/store/identity_schema.go:887-892.
- ensureMembershipTx at coordinator/internal/store/identity_schema.go:1050-1064 contains the only reconciliation-time check that rejects an actor whose active membership orbit differs from the credential slot orbit.
- assertIdentityServingGate at coordinator/internal/store/identity_schema.go:1078-1095 verifies only that an active slot has a current credential binding; it does not join memberships.
- memberships and installation_credentials have valid independent foreign keys and one-active-membership uniqueness, but no composite constraint tying memberships.orbit_id to installation_credentials.slot_orbit_id (coordinator/internal/store/identity_schema.go:29-65).
- foreignKeyCheck at coordinator/internal/store/identity_schema.go:553-569 therefore cannot detect this logical mismatch.
- OpenWithOptions treats successful ReconcileIdentity as the startup serving gate at coordinator/internal/store/store.go:124-128.
- The existing misaligned-membership test at coordinator/internal/store/identity_security_rework_test.go:361-381 checks only resolver rejection; it never calls ReconcileIdentity or reopens the store. Happy-path app-first tests at coordinator/internal/store/onboarding_test.go:105-113 and 388-392 do not inject misalignment.

Failure schedule:

1. Create or load an app-first installation credential bound to a live slot in orbit A with paired_by=0 and a non-null control token hash.
2. Leave its orbit-A membership and give the same actor the single active membership in orbit B. Both foreign keys and the partial uniqueness index remain valid.
3. Run startup reconciliation.
4. The live binding passes generation checks; line 884 takes the app-first continue, skipping the activeOrbit comparison.
5. Domain-disjointness, the current serving-gate query, and PRAGMA foreign_key_check all pass, so ReconcileIdentity commits and OpenWithOptions returns a serving store.
6. Actor-context/mutation joins later fail closed with insufficient_capability because they require orbit A, but the contract-required fatal startup refusal and actor disable never occur.

Impact:

Rev15 section 17.5 says migration/reconciliation MUST fail closed on orbit mismatch, disable the affected actor, and refuse serving until corrected. The current code does not create an immediate cross-orbit control escalation because endpoint joins reject it, but it permits a forbidden identity state to pass the serving gate and leaves the affected actor enabled. This is a release-blocking migration/security invariant failure in the task-owned app-first preservation delta.

Required remedy:

Before preserving an app-first role, explicitly verify that the actor has exactly one active membership and that its orbit equals binding.orbitID. Add a serving-gate assertion that independently detects all credential/membership orbit mismatches. Resolve the transaction semantics needed to satisfy both durable actor disablement and startup refusal. Add an executable handcrafted fixture test that reopens with the feature enabled and proves reconciliation/startup fails closed for the mismatch, plus a corrected-state recovery test and happy-path app-first role-preservation coverage.

## Resolved root challenge — node-token limiter ordering is not a defect

The general section 9 wording does not override the more specific endpoint sequence:

- Rev15 section 7 step 1 for recovery rotation and section 10 step 1 for Telegram-link issue state that authentication middleware resolves the bearer and node tokens fail with 403 insufficient_capability.
- Their step 2 is syntax validation and step 3 is actor reservation, so wrong credential-capability rejection intentionally precedes both.
- Section 9 says 403 insufficient_capability for lifecycle/role counts. It does not add the wrong credential-capability class to that counting rule.
- The current withControl rejection at coordinator/cmd/duet-coordinator/onboarding.go:341-360 and the zero-node-attempt assertion at coordinator/cmd/duet-coordinator/onboarding_test.go:416-427 therefore implement the specific contract.
- Valid control tokens with satellite, left, or disabled lifecycle state proceed through syntax/reservation and count; coordinator/cmd/duet-coordinator/onboarding_test.go:274-323 and 389-415 exercise that distinction.
- Device-invite issuance is not assigned a per-actor limiter in the frozen rate table; its early node 403 is also consistent.

No F4 blocker is retained.

## Contract/schema inconsistency resolution

The conflict between Rev15 section 7 and the frozen audit_events schema is a release blocker, not silently waived and not an implicit erratum. Resolution requires either an accepted additive schema/detail mechanism that preserves previous-head and rollback compatibility, or an explicit authoritative contract amendment changing the data requirement.

## Architecture and protocol observations

No additional blocking defect was found in the reviewed task-owned paths for:

- feature-off service behavior, apart from F3's serving gate;
- legacy pair and websocket registration compatibility;
- route registration, bounded request decoding, and specific node-capability ordering;
- no-store responses and staged authentication;
- node/control domain separation and node-token administration rejection;
- live actor, membership, orbit, slot, binding-generation, and paired-generation checks inside mutations;
- constant-time credential comparison and dummy-hash invalid paths;
- atomic device-invite and recovery single-winner behavior;
- recovery node-token preservation;
- hash-only persistence and response redaction;
- rollback projection and previous-head coexistence.

These observations do not waive F1-F3.

## Independent verification commands

All Go commands were run from coordinator unless noted.

1. Focused repeated production-path tests:

    go test -count=10 ./internal/store -run 'Test(AuthenticatedMutationsPrepare|DeviceInviteReuses|DeviceInviteNullable|ConcurrentDeviceInvite|ConcurrentRecovery|RecoveryRotationConsume|RecoveryValidation|SatelliteRecovery)'

    Result: PASS (5.726s)

    go test -count=10 ./cmd/duet-coordinator -run 'Test(Onboarding|ConcurrentInstallation|Capability|Authenticated|ControlMutation|DeviceInviteHTTP|RecoveryHTTP|TelegramLinkHTTP|RecoveryLimiter|RequestObjects|MalformedOrMultipleAuthorization|AttemptLimiter|ForwardedHeaders|ResolverInternal)'

    Result: PASS (3.051s)

2. Previous-head compatibility:

    go test -count=1 -tags previoushead ./internal/store -run '^TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition)$' -v

    Result: PASS (5.917s; authority round trip 2.32s, two-generation projection 3.19s)

3. Focused identity/migration suite:

    go test -count=1 ./internal/store -run 'Test(R[1-8]|Identity|ActorResolver|FeatureOn|Rollback|Reconciliation|OrbitStatus|Legacy|HashOnly|TelegramMigration|TelegramResolver)'

    Result: PASS (2.363s)

4. Full uncached coordinator suite:

    go test -count=1 ./...

    Result: PASS (cmd 1.902s, store 5.911s)

5. Full race detector:

    go test -race -count=1 ./...

    Result: PASS (cmd 12.459s, store 28.279s)

6. Static/build/format/diff validation:

    go vet ./...
    go vet -tags previoushead ./internal/store
    go build ./...
    test -z "$(gofmt -l cmd/duet-coordinator/main.go cmd/duet-coordinator/onboarding.go cmd/duet-coordinator/onboarding_test.go cmd/duet-coordinator/onboarding_compat_test.go internal/store/identity.go internal/store/identity_schema.go internal/store/onboarding.go internal/store/onboarding_test.go)"
    git diff --check

    Result: PASS, exit 0, no output

7. Board validation from the project root:

    task-board validate

    Result: PASS — Board valid, including after review resource and note mutations.

8. Cooperative directives:

    task-board spawn directives "$TASK_BOARD_RUN_ID"

    Result: the app-first alignment challenge is F3; the node rate-order challenge is explicitly resolved as not a defect; the hash-evidence correction was applied below.

No task test skip was found.

Administrative corrections:

- A non-validation follow-up attempted task-board show TASK-260712-m5264f, which returned unknown command. The supported task-board q 'get(TASK-260712-m5264f) { full }' query then succeeded.
- An earlier review-resource draft transcribed incorrect SHA-256 values. The inventory below replaces them with direct output from one unabridged shasum invocation. The error was in reviewer evidence transcription, not a worktree change, and was corrected before terminal handoff.

## Recomputed SHA-256 inventory

Direct command:

    shasum -a 256 coordinator/cmd/duet-coordinator/main.go coordinator/cmd/duet-coordinator/onboarding.go coordinator/cmd/duet-coordinator/onboarding_test.go coordinator/cmd/duet-coordinator/onboarding_compat_test.go coordinator/internal/store/identity.go coordinator/internal/store/identity_schema.go coordinator/internal/store/onboarding.go coordinator/internal/store/onboarding_test.go .task-board/.resources/TASK-260712-m5264f/TASK-260712-m5264f_results.md TASK-260712-m5264f_results.md

Exact results:

- coordinator/cmd/duet-coordinator/main.go — ebac82641471039ec0dcb66e3f4fa8f49b543d38a71aa5caa9e56f030e26039d
- coordinator/cmd/duet-coordinator/onboarding.go — 6aa01fd1ee8f34526ebfba9db4807e468c46850b0e13bcb38ba6510a2a3064c3
- coordinator/cmd/duet-coordinator/onboarding_test.go — 02d18a2c64dc3d39f447d5057ca0a5cfa735f94cc7f1da82d9fdcb359213fd95
- coordinator/cmd/duet-coordinator/onboarding_compat_test.go — f37935809bd369df543b9e2a67333a81a6c8da3f73e8fa6da081f559cb08e0b4
- coordinator/internal/store/identity.go — dcd4cc3c1188569439335c1742c657cb4235aec223f1d2ed5f4cb4fcde0de5dd
- coordinator/internal/store/identity_schema.go — 892238d4d8d6aa3adbeb7c9a1009df693d84fb9803c5fa21b718521ca33472bb
- coordinator/internal/store/onboarding.go — 66948069ac47d7a8f2f21718149f129a1ff89bba8b47b20fe863a550e5c1ea43
- coordinator/internal/store/onboarding_test.go — 4e059c4684cc9fb78e19a12217ed5c090ad64f9dfb8a9fce595daef1898612a7
- board producer outcome TASK-260712-m5264f_results.md — 9066b4d0767164bd0ba20cee79ff5dcf9ba48c6fe0e691a155d21dc8fb755746
- root producer outcome copy TASK-260712-m5264f_results.md — 9066b4d0767164bd0ba20cee79ff5dcf9ba48c6fe0e691a155d21dc8fb755746

The two producer outcome copies are byte-identical. A final exact-prefix scan confirmed the discarded erroneous values are absent from this report.

LOGBOOK.md had unrelated sibling activity after the producer outcome; the task's recorded block remained present. This independent review made no code, test, checklist, logbook, commit, or push changes. Per the attached independent-review guard, shared files were not edited; findings are persisted in this task-scoped board outcome and task notes.
