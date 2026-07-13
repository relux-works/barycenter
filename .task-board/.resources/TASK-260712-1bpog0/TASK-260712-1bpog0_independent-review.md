# TASK-260712-1bpog0 independent review

Date: 2026-07-13
Role: independent reviewer
Verdict: RETURN TO DEVELOPMENT

## Scope reviewed

Read the complete task card, producer outcome, identity model, review and implementation guards, docs/spec-self-contained-audio.md sections 3, 6, 11, 12, 13, 18, and 19 P1.2, the complete accepted Frozen Contract in TASK-260712-3v1k7q/research.md, root amendments, and every task-scoped changed file in full.

## Findings

### HIGH — node/control credential domains can alias

Files: coordinator/internal/store/identity.go:92, coordinator/internal/store/identity.go:100, coordinator/internal/store/identity.go:118, coordinator/internal/store/identity.go:316, coordinator/internal/store/identity.go:360.

Failure schedule: create and reconcile an installation; call ProvisionInstallationSecrets through an otherwise valid primary or companion Telegram/control authority while supplying the installation node token as controlToken; the method accepts the 64-lowercase-hex value and stores the same digest already present in slots.token_hash; ResolveTokenActorContext checks installation_credentials.control_token_hash first and returns CapabilityNode plus CapabilityControl. A holder of material defined as node-only now receives control authority. The same ambiguity can occur if a newly minted node token aliases an existing control digest.

Violated invariant: spec section 6.1 and the accepted capability matrix require node_token to remain playback-only. The review guard explicitly requires collision and ambiguous-match handling.

Required correction: enforce disjoint node/control hash domains transactionally on both provisioning and node minting, and make the resolver fail closed if a digest exists in both domains. Add deliberate same-value and cross-orbit ambiguity tests; do not rely on random collision probability.

### HIGH — provisioning can overwrite or revive target credential state without validating the target lifecycle

File: coordinator/internal/store/identity.go:316-387.

Failure schedule: keep a current slot binding but set the target installation actor revoked_at, or leave its membership; an active primary or companion authority can call ProvisionInstallationSecrets for that actor. The UPDATE checks only actor_id, orbit, and live slot binding. It does not join target actors.revoked_at or an active aligned target membership, and it does not require the credential to be unprovisioned. It overwrites control and recovery hashes and commits credential.provisioned for a revoked, left, or already-provisioned target.

Violated invariant: revocation is stronger than recovery/relinking; stale, left, and revoked target paths must fail closed. Existing control replacement belongs to the recovery/rotation contracts, not the initial provisioning primitive.

Required correction: revalidate the target actor kind, revocation, active orbit-aligned membership, and intended provisioning generation inside the same BEGIN IMMEDIATE transaction. Define and enforce whether only unprovisioned rows may use this primitive; route replacement through the frozen recovery/revoke/re-pair flows. Add revoked, left, stale-binding, and already-provisioned tests.

### HIGH — combined old-binary dissolution plus unconstrained status repair cannot reach reconciliation

Files: coordinator/internal/store/store.go:104-111 and coordinator/internal/store/identity_schema.go:168-181, 291-380, 542-566.

Failure schedule: a prior partial rollout leaves a valid but unconstrained orbits.status plus additive identity rows; an old coordinator with foreign keys off dissolves an orbit, leaving additive children that reference the deleted orbit; the new coordinator opens with the feature on. initIdentitySchema repairs the status constraint before ReconcileIdentity. rebuildOrbitsWithStatusConstraint runs PRAGMA foreign_key_check at line 366 and aborts on those expected stale children. The cleanup for exactly this old-binary state exists only later in reconcileIdentityTx at lines 546-566, so it is unreachable.

Violated invariant: existing rollback databases must migrate and reconciliation must clean old-binary dissolution before the serving gate. The review guard requires combined partial-migration and rollback-state handling.

Required correction: establish an ordering that can safely identify and remove only contract-authorized stale additive children before rebuild validation, or perform the repair and cleanup in one controlled migration transaction while preserving fail-closed handling for unrelated FK corruption. Add the combined fixture and prove unrelated FK violations still abort.

### MEDIUM — mandatory rollback and collision regression matrix is incomplete

File: coordinator/internal/store/identity_test.go.

The tests pass but do not execute the accepted full new-on to previous-binary mutations to new-on cycle covering AddMember, SetMemberName, leave, transfer, new slot, revoke, rebind, and dissolve together. They also omit the deliberate node/control same-digest ambiguity, revoked or already-provisioned target provisioning, the combined unconstrained-status plus dissolved-orbit fixture, emergency rollback gap and re-enable behavior, two projection generations, and projection/restoration crash barriers. Current feature-off coverage uses the new Open implementation, not the previous coordinator binary.

Required correction: add the focused cases above and the complete downgrade cycle required by sections 17.8.4 and 17.11.

## Independent verification

PASS: go test -count=1 ./...
PASS: go test -count=1 -race ./internal/store
PASS: go vet ./...
PASS: go build ./...
PASS: targeted identity, migration, downgrade-emulation, feature-off, authorization, projection, and locking tests
PASS: gofmt task-scoped file check
PASS: git diff --check for coordinator and LOGBOOK.md
PASS: task-board validate
NOT RUN: golangci-lint was not installed

The green suite does not cover the failure schedules above. No product code was modified by this review; only the required LOGBOOK review entry, board metadata, and this outcome were added. All unrelated dirty-worktree content was preserved.
