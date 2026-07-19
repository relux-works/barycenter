# Independent delta review: coordinator ciphertext-routing & rotation (TASK-260712-20j5tm)

Reviewer: independent protocol/persistence/security delta reviewer (board role `[reviewer] reviewer (claude)`; read-only, no production or test code modified).
Date: 2026-07-20.
Reviewed commit: **`e97717bfad6348279430012ecf0ce3de404eae0d`** (`feat(e2ee): add keyless epoch routing foundation`), branch `feat/task-260712-20j5tm`.
Working tree: clean apart from `.task-board/` bookkeeping (progress files + this reviewer brief).

## VERDICT: APPROVE

Zero open Critical/High/Medium findings against the reviewed delta. The change faithfully and additively extends the design approved at `TASK-260712-aniuyy` without granting the coordinator any group or content secret. All producer evidence reproduces. Low/Info observations below are non-blocking under this task's production-dark scope and carry downstream owners.

The delta-review gate the ADR requires (`deltaReviewRequired: true`; "any protocol-affecting change after the accepted design review requires an independent delta review") is satisfied by this review. Production E2EE remains blocked: `EPC-001, EPC-002, EPC-004, EPC-005, TASK-260712-1ulshp` stay open by design.

## 1. Reviewed hashes (all reproduce)

All 12 SHA-256 pins in `acceptance/phase3/e2ee-coordinator-routing-rotation-v1.json` recomputed from the working tree — **all match**: schema, foundation-repository, routing-repository, routing-tests, coordinator-contract, coordinator-contract-tests, protocol-authority, protocol-vectors, threat-model, key-lifecycle-packet, schema-foundation-packet, adr.

The schema-foundation and key-lifecycle packets were consistently re-pinned by this commit to the new `contract.go`/`e2ee.go`/`e2ee_schema.go`/`e2ee_test.go` hashes and the four new additive tables; the re-pins are internally consistent and byte-verified. (The stale copy under `.task-board/.resources/TASK-260712-3w1cst/` predates this re-pin and legitimately differs; the in-repo packet + validator are the authority.)

## 2. Reproduced automated evidence (fresh runs, this session)

| Check | Command | Result |
|---|---|---|
| Coordinator full | `go test ./...` (coordinator) | all packages ok (store 42.2s) |
| Vet | `go vet ./...` (coordinator) | clean |
| Race | `go test -race ./internal/store ./internal/e2eecontract -count=1` | ok (store **525.144s**, e2eecontract **1.446s**) |
| Focused routing | `go test ./internal/store -run 'TestE2EE(Routed\|Rotation\|Routing\|ProtocolActor\|Delivery)' -count=1 -v` | 5 top-level / all subtests PASS |
| Acceptance validator | `python3 scripts/acceptance/validate_e2ee_coordinator_routing_rotation.py` | PASS (production disabled) |
| Acceptance unittests | `python3 -m unittest ...routing_rotation ...schema_epoch_foundation ...protocol_key_lifecycle` | 10/10 OK |
| Acceptance-contract stage | full `acceptance-contract-tests` unittest set | **207/207 OK** (138s) |

Producer numbers (store race 502.193s; contract 1.427s; 207/207; validators green) are consistent with this reproduction.

**Commands not run (justified):** Windows `go test ./...` (pulsar-win) and macOS `swift test` (node-app). This delta modifies only four coordinator production files (`e2eecontract/contract.go`, `store/e2ee.go`, `store/e2ee_routing.go`, `store/e2ee_schema.go`) plus acceptance packets/scripts; it touches **no** `pulsar-win/` or `node-app/` file. Those platform models are byte-identical to the state the `TASK-260712-aniuyy` reviewer already exercised, so they are out of scope for this delta.

## 3. Acceptance scope assessment (brief items 1–8)

1. **Coordinator keyless — CONFIRMED.** `ProductionConfig()` returns empty `AllowedSuites` + `nil Verifier`; it rejects even the valid audit fixture as `unknown_suite` (test `...DeliveryAckSurvivesRestart` runs the production config first). Strict decoders (`DecodeCoordinatorProposal/Commit` → `decodeCoordinatorEnvelope`) reject the eight forbidden secret fields and any unknown field before routing. Stored bytes are the digest-bound public envelope only; audit stores ids/digests/reasons. No secret/plaintext column is declared (validator forbidden-column scan). No production verifier/suite/library/container is selected. `ValidateProposal` reuses the same fail-closed envelope/suite/signature/target/epoch/replay checks as `ApplyCommit` without mutating state.
2. **Exact lineage bound into snapshots — CONFIRMED.** `e2eeAirSnapshotTx` canonicalizes the sorted set of verified devices binding orbit, actor, actor-membership role + join lineage, Air-membership id/role/revision, protocol-actor id, device id, public-package digest, and verification digest. Same-device leave/rejoin (new `air_membership_id`), role change, device revoke, and actor disable each yield a distinct digest → rotation. All six lifecycle causes are covered by passing tests. The mixed-version/unsupported classification is fail-closed with no plaintext downgrade.
3. **Lazy reconciliation fail-closed + no staging race — CONFIRMED.** `reconcileE2EERotationTx` early-returns for uninitialized legacy groups and otherwise records exactly one durable `rotation.require` + revokes removed-device deliveries. `StageE2EEProtectedObject` reconciles first, rejects on a required rotation, then **re-checks inside the write transaction** (authorized member + `snapshot.Digest == target` + `sameE2EEMemberSet` + no unsupported) so a membership change landing between the reconcile and the write still fails closed with `ErrE2EERotationRequired`.
4. **Delivery targeting — CONFIRMED.** Proposals go only to prior-epoch survivors with identical lineage (`sameE2EEMemberLineage`), excluding newly joined and rejoined-different-lineage endpoints; commits deliver to the exact next `snapshot.Members`. Removed/revoked/disabled endpoints have pending deliveries revoked and are filtered from both `PendingE2EEGroupEvents` (joins `state='current'` + verified + not-revoked + actor-active + Air-joined) and `AcknowledgeE2EEGroupEvent` (requires `pending` + re-authorization). Leave subtest confirms removed peer sees 0 pending and its own commit is rejected `ErrE2EEInvalid`.
5. **Validation / CAS / concurrency / fork — CONFIRMED.** `authorizedE2EEGroupMemberTx` binds device↔protocol-actor↔verified-not-revoked↔active-actor↔joined-membership↔joined-Air↔current-member. Commit acceptance is single-winner via the epoch/commit-digest/revision/`fork_state='clean'` CAS **and** the `e2ee_one_accepted_commit_per_epoch` partial unique index; concurrent test yields accepted=1/rejected=1, final epoch 8, clean. An exact-epoch mismatched-predecessor freezes the group `forked`; a malformed-predecessor (invalid digest) is rejected **without** poisoning fork state or advancing the epoch. All verified by passing tests.
6. **Recovery / restart / cursor / ack / audit — CONFIRMED.** Immutable `(event, device)` delivery rows survive close/reopen and return byte-identical payload; cursor `(created_at > ? OR (= AND event_id > ?))` is collision-safe (same-timestamp test); acknowledgement is exact digest + revision CAS; duplicate ack → `ErrE2EEDeliveryNotFound`; replay → `ErrE2EEReplay` and is audited. All operations are bounded (member fan-out, `limit ≤ 100`).
7. **Additive / feature-off / no runtime exposure — CONFIRMED.** Four new additive tables; `e2ee_feature_state` remains `CHECK(enabled = 0)`; no legacy ACL/retention/deletion/inbox/history/media DDL or DML in the delta. No HTTP/WebSocket/protocol handler references any new routing method (dormant); `E2EEMediaEnabled` is hardcoded `false`; validator confirms `e2ee_media_v1` is absent from the five production sources and no `protocol/golden/*e2ee*` fixture exists.
8. **ACL/delete/retention reuse; deferred vs gap — CONFIRMED as deferred, not a gap.** The delta does not modify ACL/delete/retention services. Opaque-media upload/fetch HTTP runtime, client key state/crypto, history/recovery/transfer runtime, and report-evidence export are explicitly deferred to named downstream tasks (`deferredScope` + ADR "honest limits"). This is a legitimate production-dark routing foundation, not an acceptance gap for this task's defined scope.

## 4. Findings

**Critical: none. High: none. Medium: none.**

- **L1 (Low — audit fidelity, non-blocking).** `e2eeRotationReasonTx` derives `reason_code` by iterating the removed-device set in Go map order with partial precedence (`actor_disable` returns first; `device_revoke` guards `air_leave`; but `membership_change`/`air_join` can be overwritten). When several devices are removed simultaneously for *different* reasons, the recorded `reason_code` is non-deterministic among the valid causes. Impact is diagnostic only: a required rotation is always created, the required snapshot digest is exact, and every value stays within the schema enum — no effect on which snapshot rotation converges to or on any security control. Owner: this task's downstream runtime-wiring/key-state tasks. Retest: add a multi-cause removal vector and pin a canonical reason precedence, or scope the reason to "diagnostic, non-authoritative."
- **I1 (Info — verified correct, disambiguate downstream).** The snapshot classifier implements the ADR's three explicit categories: unverified/incomplete-but-present device → `unsupported` (blocks target); absent registration (`registrations == 0`) → `unsupported`; fully-revoked devices (`registrations > 0`, none non-revoked) → excluded *removed endpoint*. This correctly stops a revoked device from DoS-ing a target as "unsupported." One edge is worth a downstream test: an actor that joins an Air holding *only* revoked device rows (never a group member) is silently excluded rather than marking the target unsupported. No security or AC impact (no secret exposure; no plaintext; convergence holds), but the "member with only-revoked devices" semantics should be pinned by the key-state implementation review (EPC-005 / TASK-260712-25dzp4).
- **I2 (Info).** Prior-review tracked items (`TASK-260712-3w1cst` L1 failure-precedence linearization; I1 sentinel tripwire; I2 no test staging against a persisted `forked` group; I3 replay-state rejection un-audited; I4 `encrypted_evidence_ref` not digest-bound) are **not reopened** by this delta and remain owned downstream. The malicious-fork-freeze test partially addresses I2 for commits (a persisted `forked` group rejects a subsequent commit), but object/replay/grant/transfer staging against a persisted fork is still untested here.

## 5. Non-claims

No real-app, signed-package (MSIX/notarized macOS), hardware, or real-crypto interop evidence is claimed; all remain `not-run`/`not-run-no-selected-stack` in `EPIC-260714-th54l3`. No production library, suite, serialization, or container is selected. This review does not substitute for the external crypto implementation review `TASK-260712-1ulshp`. Any further protocol-affecting change after `e97717b` requires another delta review.

## 6. Sign-off

**APPROVE** at `e97717bfad6348279430012ecf0ce3de404eae0d`. Zero open Critical/High/Medium delta findings; L1/I1/I2 tracked with owners. The coordinator remains keyless and production-dark; the keyless routing/rotation state machine matches the acceptance criteria and the approved design. Routing task acceptance is warranted.
