# TASK-260712-2u1w16 — independent security/migration review R2

Reviewed: 2026-07-13 20:41 +04  
Role: reviewer  
Verdict: **back to development (`to-dev`)**

The implementation is broad and its current tests/build are green, but it does not satisfy the frozen Rev15 recovery, protected-migration, and canonical-origin invariants. Four independently reproduced findings remain. The existing test suite does not cover the failing schedules below.

## Severity-ranked findings

### HIGH — R2-F1: an `ever_sent=false` candidate matching the active bundle is accepted and deleted without credential proof

Locations:

- `node-app/Sources/NodeCore/RecoveryService.swift:455`
- `node-app/Sources/NodeCore/RecoveryService.swift:461`
- `node-app/Sources/NodeCore/RecoveryService.swift:476`
- `node-app/Sources/NodeCore/RecoveryService.swift:644`
- `node-app/Sources/NodeCore/RecoveryService.swift:656`

Failure schedule:

1. Protected active state contains control token `T` plus matching origin, actor, and recovery ID.
2. Protected pending state contains the same tuple/token with `ever_sent=false` (corruption, partial repair, or impossible-state injection).
3. `resumePending(actorID:)` calls `activeAlreadyPromoted` before checking `everSent`.
4. `activeAlreadyPromoted` checks token/identity but not `record.everSent` and returns true.
5. `cleanupPromoted` deletes the unsent pending record and reports recovery success without a probe or any separately confirmed credential.

Production-seam probe result:

```text
OUTCOME=RecoveryServiceOutcome(recovered: <redacted>)
PENDING_PRESENT=false
SEND_COUNT=0
```

Violated invariant: Rev15 permits active-bundle/pending convergence only after the verified false-to-true barrier and a successful authenticated transition. Every valid crash after promotion necessarily has `ever_sent=true`. R2 explicitly requires the `ever_sent=false` matching-active boundary to fail closed unless justified by separately confirmed authority; no such proof exists here.

Required correction: require a sent record for crash-convergence cleanup. Preserve/fail closed on an unsent matching-active state, and add a deterministic production-seam test covering both `resumePending` and `recover` entry points.

### HIGH — R2-F2: non-equivalent destination payloads are treated as equal and one Keychain copy is deleted

Locations:

- `node-app/Sources/NodeCore/Keychain.swift:210`
- `node-app/Sources/NodeCore/Keychain.swift:212`
- `node-app/Sources/NodeCore/Keychain.swift:219`
- `node-app/Sources/NodeCore/Keychain.swift:275`
- `node-app/Sources/NodeCore/Keychain.swift:284`
- `node-app/Sources/NodeCore/Keychain.swift:367`

Failure schedule:

1. The versioned DP destination contains a valid bundle.
2. The versioned login destination contains the same known fields plus an unknown protected field.
3. `JSONDecoder` silently drops the unknown field; decoded `CredentialBundle` values compare equal.
4. `readDestination` accepts the copies instead of reporting conflict.
5. A subsequent `saveBundle` updates DP and deletes the login copy, discarding the unrecognized protected state.

Production-seam probe result:

```text
ACCEPTED_NON_EQUIVALENT_DESTINATIONS
LOGIN_COPY_RETAINED=false
```

Violated invariant: the implementation guard allows automatic reconciliation only for byte-equivalent destination state and requires every protected field to compare exactly. A future/corrupt field cannot be silently discarded during migration or duplicate resolution.

Required correction: enforce the exact versioned protected schema (including rejection of unknown keys and duplicates) and compare canonical protected bytes or an equivalently complete field representation before any update/delete. Apply the same exact-record discipline to pending recovery decoding. Add DP/login tests with unknown, duplicate, and noncanonical-but-decodable fields that prove fail-closed retention.

### MEDIUM — R2-F3: a limited authenticated promotion loses its limited-context classification after a delete crash

Locations:

- `node-app/Sources/NodeCore/RecoveryService.swift:559`
- `node-app/Sources/NodeCore/RecoveryService.swift:656`
- `node-app/Sources/NodeCore/RecoveryService.swift:663`
- `node-app/Sources/NodeCore/RecoveryService.swift:687`
- `node-app/Sources/NodeCore/RecoveryService.swift:707`

Failure schedule:

1. Active state has known orbit/role for actor A and old control token.
2. Sent pending token probes as `403 insufficient_capability`.
3. `promote(... limited: true)` correctly retains the known orbit/role and saves the new active token.
4. Pending deletion fails, simulating a crash between active save and pending cleanup.
5. Restart detects the same active candidate and calls `cleanupPromoted` without another network send.
6. `cleanupPromoted` derives `hasLimitedContext` only from `orbitId == nil`; because retained metadata has an orbit, it returns false.

Production-seam probe result:

```text
FIRST_INTERRUPTED_AFTER_PROMOTION
RESTART_LIMITED=false
RETAINED_ORBIT=99
INITIAL_PROBE_SENDS=1
RESTART_SENDS=0
```

Violated invariant: Rev15 §5.1.1 requires an authenticated `403 insufficient_capability` promotion to remain reported as unavailable/limited even while retaining known protected metadata. Crash convergence must preserve that semantic result.

Required correction: persist or safely re-establish the promotion context strength without replaying the recovery mutation, and add a deterministic limited-promotion/delete-failure/restart test with pre-existing orbit metadata.

### MEDIUM — R2-F4: UTS46 dot mapping is ordered after root-dot stripping, so a mapped trailing root dot is rejected

Locations:

- `node-app/Sources/NodeCore/CoordinatorOrigin.swift:210`
- `node-app/Sources/NodeCore/CoordinatorOrigin.swift:212`
- `node-app/Sources/NodeCore/CoordinatorOrigin.swift:230`
- `node-app/Sources/NodeCore/CoordinatorOrigin.swift:240`
- `node-app/Tests/NodeCoreTests/CoordinatorOriginTests.swift:16`
- `node-app/Tests/NodeCoreTests/CoordinatorOriginTests.swift:22`

Failure schedule:

1. Parse `https://coord.example.com。`, where U+3002 is a UTS46 dot mapping.
2. `canonicalDomain` strips only an ASCII `.` before invoking ICU.
3. ICU maps U+3002 to `.` at the end of the A-label output.
4. The label validator sees an empty final label and rejects the origin.

Actual-source probe result:

```text
REJECTED
```

Violated invariant: the frozen algorithm applies the exact UTS46 profile first, then strips one trailing root dot. The tests cover an ASCII trailing dot and an interior U+3002 independently, but not their composition.

Required correction: apply the frozen ordering to the ICU result and add U+3002/U+FF0E/U+FF61 trailing-root vectors plus multiple-root-dot negative cases.

## Contract/architecture assessment

The implementation otherwise follows the supplied sequence boundary: NodeCore exposes typed create/join/recover/Telegram services; node and control capabilities are split; pairing compatibility remains additive; the node token is not used as a control bearer; recovery material is non-Codable and explicitly exported; the production pasteboard compare-and-clear is serialized on the main actor; HTTP transport is injected and bounded; and existing node startup remains source-compatible. The four findings above nevertheless fail required crash safety, migration fail-closed behavior, limited recovery reporting, and exact origin canonicalization.

No product or test source was edited during review. No real Keychain item or system pasteboard was read, written, or deleted; production implementations were inspected and all executed tests/probes used in-memory stores/transports or pure origin code.

## Independent command results

All commands ran from `node-app` unless noted.

| Command | Exit | Result |
|---|---:|---|
| `xcrun swift test --filter 'CLIPairingSourceTests|CoordinatorOriginTests|CredentialBundleTests|OnboardingHTTPClientTests|PairingClientTests|OnboardingServiceTests|RecoveryServiceTests|RecoveryExportTests'` | 0 | 53 tests in 8 suites passed |
| `xcrun swift test` | 0 | 103 tests in 22 suites passed |
| 50 repetitions of `xcrun swift test --filter 'RecoveryServiceTests|RecoveryExportTests'` | 0 | 50/50 passed |
| `xcrun swift build -c release` | 0 | production build passed |
| `xcrun swift-format lint --strict` over task-owned new/replaced source and tests | 0 | clean |
| `git diff --check -- <task file set>` from repository root | 0 | clean |
| scoped log/deep-link/secret scan with `rg` | 0 | no secret-bearing deep-link construction; only expected CLI/status logging sites found |
| actual-source U+3002 trailing-root probe | 0 | `REJECTED` (finding reproduced) |
| actual `CredentialRepository` DP/login unknown-field probe | 0 | accepted non-equivalent copies; login copy deleted |
| actual `RecoveryService` unsent/matching-active probe | 0 | recovered; pending deleted; zero sends |
| actual limited-promotion/delete-crash/restart probe | 0 | restart reported `hasLimitedContext=false` with retained orbit 99 |
| `task-board spawn status/directives "$TASK_BOARD_RUN_ID"` | 0 | run active; no operator directives |

The focused/full/repetition suites being green is not acceptance evidence for the four missing adversarial schedules; each production-seam probe falsifies an acceptance invariant the current suite does not exercise.

## Hash verification

Every producer-listed task file was read in full and independently hashed. All hashes match the producer handoff:

```text
9779a339c3dcca86f5a7b0f62bbc6f90befd6cba88fad071875fdb49b72c5a80  node-app/Sources/NodeApp/main.swift
ba2badebf20ea0e4ffc089b5a6f13b6bc349d85bd92b5888250025cc7bff631e  node-app/Sources/NodeCore/CoordinatorClient.swift
e090bc86a4d496f1887bc6391dea2f50f1246b7172af713aa90f44239b834a64  node-app/Sources/NodeCore/Credentials.swift
31c5014eca4f93a87de59f94267d583407e0a2aec44173da7e8e32dcf7384a8b  node-app/Sources/NodeCore/Keychain.swift
91800a09b0cb20e1883c803b2af9245fd41db4b0f69cdc92f264c7615d9ed35f  node-app/Sources/NodeCore/CoordinatorOrigin.swift
bde7c6ad4b75cee2cb38efce12723874eb7f52599ac5bc14046bab2b05bf2e79  node-app/Sources/NodeCore/OnboardingCredentials.swift
64a060cd7495ba996bad96da8a5988737911e3208e11eea84027c7875325d231  node-app/Sources/NodeCore/StrictJSON.swift
6e329aa52399d69d771a23db397290cc9314b19b4e8ceeb7ebbf1023d76be301  node-app/Sources/NodeCore/OnboardingHTTPClient.swift
e368109a307162a13bb9aec4c60d95295eadf851b7d4515651a536ae58d358b4  node-app/Sources/NodeCore/OnboardingService.swift
4f4e0716784a6bcbfd89cb164dd4e111c4f764d5bc29d0cf9d1e283b5f823485  node-app/Sources/NodeCore/RecoveryService.swift
bd3889c72d26aabd856b2ba34d788e14a5848af8deca34596aedd19613c0e5bc  node-app/Sources/NodeCore/RecoveryExport.swift
764af667c75437e1113229be9eca1a18450de25d4cb09c33076eccccc905f992  node-app/Tests/NodeCoreTests/CredentialsTests.swift
4b2d6185fb5c4527d7e8bf2c00185659cde84b8267f9b29e96f54ac240af4e4a  node-app/Tests/NodeCoreTests/CLIPairingSourceTests.swift
a6fc12544adb67d2ae9434e072bc8eda38cdaf8eccfd2f5cd0b9c18a7df103e1  node-app/Tests/NodeCoreTests/CoordinatorOriginTests.swift
1bf532f1a9945354c9eadb42545ce3aed24b68382bc1044a2477d00de1cb3802  node-app/Tests/NodeCoreTests/CredentialBundleTests.swift
4bacb318c0a7726ebbf8338d69c34c37e00d0f7da586caaaf2fec6e9dedba55c  node-app/Tests/NodeCoreTests/OnboardingHTTPClientTests.swift
80670e0426af9af415febef819773f548ce35ecc965ce73102073dfd3d426c97  node-app/Tests/NodeCoreTests/OnboardingServiceTests.swift
bd9053b261cad33f7eeb73485cb23905bb008cd91630d27dfcb4b012b8f27428  node-app/Tests/NodeCoreTests/OnboardingTestSupport.swift
0cd72c6fe513631eeba05824df506bd507ba447ae668844d03a09bc709cf73b0  node-app/Tests/NodeCoreTests/PairingClientTests.swift
541cd304735c0c4e1496a2677940870e98b10a7cb16c0738b03d0687ca8fb45e  node-app/Tests/NodeCoreTests/RecoveryExportTests.swift
c8a5de71e5c24494de078939786d9ae91f4002c381389ac71f707682ae12b334  node-app/Tests/NodeCoreTests/RecoveryServiceTests.swift
```

## Remaining gates

1. Correct R2-F1 through R2-F4 without weakening Rev15 or the R1/M1-M12 directives.
2. Add deterministic tests at the actual repository/recovery/origin seams for each reproduced schedule.
3. Rerun focused and full tests, repeated recovery/clipboard schedules, strict formatting, `git diff --check`, scoped privacy scans, and release build.
4. Perform a fresh independent security/migration review of the corrected hashes before acceptance.

