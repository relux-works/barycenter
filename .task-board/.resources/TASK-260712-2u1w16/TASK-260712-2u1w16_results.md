# TASK-260712-2u1w16 implementation evidence

Date: 2026-07-13

## Scope and behavior

The macOS NodeCore client now exposes a versioned protected credential bundle,
non-destructive migration, a bounded onboarding HTTP client, origin-bound UI
service hooks, a crash-safe recovery service, and explicit recovery export and
pasteboard leasing. The existing `NodeCredentials` startup and `/pair` facade
remain source-compatible. Long-lived node/control credentials and sent pending
replacement credentials are written only through the protected-store
abstraction.

| Area | Implemented behavior |
| --- | --- |
| Credential bundle | Independently optional node/control capabilities, recovery metadata, canonical coordinator origin, redacted descriptions, and a legacy node view. |
| Migration | Reads legacy DP Keychain, login Keychain, and JSON file sources; writes a distinct versioned destination; reads back and compares all fields; deletes a source only after verification; fails closed on conflicts. |
| Existing pairing | Preserves orbit, slot, node token, and WebSocket URL byte-for-byte. Pair saves merge the node capability without losing control state. The CLI exits nonzero if protected save fails and never falls back to plaintext. |
| Origin | Canonical HTTP(S) origin with ICU UTS46 non-transitional/STD3/Bidi/ContextJ processing, RFC5952 IPv6, exact port rules, no userinfo/encoded host/zone ID, and HTTPS-or-literal-loopback credential policy. |
| HTTP | Typed create, invite issue/consume, context probe, recovery consume/rotate, and Telegram-link issue operations. Requests are same-origin, non-retrying, redirect-rejecting, bounded while streaming, strict JSON/media/status decoded, and redact bodies, bearers, codes, URLs, and transport strings from errors. |
| Recovery | Per `(origin, actor)` process-wide serialization; protected unsent→sent read-back barrier before transport; exact duplicate reconciliation; restart probe; atomic control-only promotion preserving node bytes; pending retention for ambiguity; crash convergence after promotion. |
| One-time material | `RecoverySecret` is deliberately non-Codable and redacted/non-printable. Explicit export contains exactly actor ID, recovery ID, and canonical recovery secret. Clipboard copy is never automatic and uses a maximum 300-second lease with atomic conditional clear. |
| UI hooks | `OnboardingService` provides create/join/invite/rotate/Telegram/probe/backup-acknowledgement operations and binds every stored bearer to the client's exact canonical origin. Raw bearer-taking HTTP methods are module-internal. |
| Logging | Coordinator diagnostics retain origin and safe protocol type only; invite, Telegram, response-body, bearer, path/query/fragment, and transport-error details are not logged. |

The production SwiftUI onboarding window was intentionally not edited; its
data binding is owned by `TASK-260712-3dqc3l`.

## Migration crash matrix

| Boundary / failure | Result |
| --- | --- |
| Source read/decode fails | Source is unchanged; migration fails closed. |
| Destination add/update fails | Source and any prior-good destination copy remain intact. No delete-then-add update is used. |
| Destination read-back fails or differs | Source remains; migration reports verification failure. |
| DP destination reports missing entitlement | A distinct versioned login destination is used; it is never confused with the legacy login source. |
| Equivalent DP + login destinations exist | The secondary verified copy remains until the authoritative update reads back exactly; only then is it deleted. |
| Destination/source values conflict | All copies are preserved and the load fails closed. |
| Source deletion fails after destination verification | Both readable copies remain; restart converges idempotently. |
| Crash after pending false write | Exact unsent record remains and may be removed only before the send gate. |
| Crash/partial failure while duplicate pending items transition true | No transport send occurs until every duplicate is reconciled and read back as `ever_sent=true`. |

All migration and Keychain tests use `MemoryProtectedStore`; the developer's
real Keychain and credential files were not read, added, updated, or deleted by
the test suite.

## Recovery response matrix

| Operation/result | Protected-state result |
| --- | --- |
| Consume 200 | Atomically promote candidate control/context, preserve node, verify active bundle, then delete exact pending state. |
| Consume 400 after send | Retain sent candidate as ambiguous. |
| Consume 403 credential invalid | Retain candidate; after a preceding probe 401, expose explicit destructive-abandon flow without deleting. |
| Consume 429 | Retain candidate and return typed retry seconds. |
| Consume 5xx/network/cancellation/decode ambiguity | Retain candidate; cancellation after the send gate cannot erase or replace it. |
| Restart probe 200 | Promote returned actor context and delete exact pending state. |
| Restart probe 403 insufficient capability | Promote the authenticated candidate with limited protected context and delete pending state. |
| Restart probe 401 | Retain the same tuple/token and request the recovery secret; no new token is generated. |
| Restart probe 429/5xx/network | Retain candidate and report rate limit/ambiguity. |
| Crash after active save but before pending delete | Restart detects exact origin/actor/recovery/token identity, verifies the active bundle, and deletes pending without another request. |
| Mismatched actor/context or corrupt promoted metadata | Fail structurally and retain pending state. |

## Root review directives M1-M12

| Directive | Production/test evidence |
| --- | --- |
| M1 | Canonical ASCII recovery/human-code normalization and exact canonical send/export are covered by `recoveryAndHumanCodeNormalizationIsASCIIOnlyAndCanonical` and `groupedRecoveryInputIsSentCanonicalAndConcurrentEncodingIsIndependent`. |
| M2 | `CoordinatorOrigin` has validating raw-value and custom Codable paths; malicious bundle/pending decoding tests fail closed. |
| M3 | Per-request bounded URLSession delegate, early Content-Length cancellation, chunked N+1 cancellation, cancellation mapping, exact JSON media type, and independent encoders are covered in `OnboardingHTTPClientTests`. |
| M4 | Cross-instance credential locking, per-operation codecs, structural validation, duplicate-destination retention, and stale writer tests are covered in `CredentialBundleTests` and `RecoveryServiceTests`. |
| M5 | ICU UTS46 with STD3/Bidi/ContextJ and non-transitional flags covers invalid A-labels, deviations, Unicode dot mapping, IPv4/IPv6, and frozen vectors in `CoordinatorOriginTests`. |
| M6 | Origin-bound authenticated hooks and exact promotion identity checks have zero-send wrong-origin and malicious response/crash tests. |
| M7 | Actor-serialized pasteboard lease, positive/capped TTL, main-thread system pasteboard access, and deterministic competing-copy/clear/expiry tests are in `RecoveryExportTests`. |
| M8 | Every exact pending duplicate transitions and verifies true before send; scope release is awaited; endpoint-specific response semantics and hard response cap are tested. The post-audit partial-transition restart case additionally proves zero transport until duplicate reconciliation. |
| M9 | Legacy `/pair` uses the bounded no-redirect transport, same-origin/media/status checks, strict credential validation, generic errors, and deterministic redirect/overflow/malformed tests. |
| M10 | Exact one-lowercase-letter response slots, cancellation/release ordering without tombstones, and gated long-tail stream/early Content-Length cancellation have deterministic tests. |
| M11 | Public no-secret `resumePending`, redacted public wrappers, nonzero CLI protected-save failure, bounded JSON depth, module-internal bearer methods, and public origin-bound service paths are tested. The permitted scope expansion is only `NodeApp/main.swift` pairing-save failure lines. |
| M12 | `PasteboardAccess.clearIfUnchanged` performs count+payload+clear atomically; the system implementation uses one main-thread closure and the fake one lock. Adversarial replacement, exact-owner, gated ordering, and concurrent contender tests pass. |

## Verification

Every command below exited 0 from `node-app` unless another directory is stated.

| Command | Result |
| --- | --- |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --filter 'CLIPairingSourceTests\|CoordinatorOriginTests\|CredentialBundleTests\|OnboardingHTTPClientTests\|PairingClientTests\|OnboardingServiceTests\|RecoveryServiceTests\|RecoveryExportTests'` | 53 tests in 8 suites passed. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` | 103 tests in 22 suites passed. |
| `for run in {1..20}; do DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --skip-build --filter 'RecoveryServiceTests\|RecoveryExportTests' >/dev/null || exit $?; done` | 20/20 deterministic recovery and clipboard repetitions passed. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift build -c release` | Production `NodeApp` linked successfully. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift-format lint --strict ...` | All new/replaced scoped NodeCore and test files passed strict lint. Existing baseline-formatted `CoordinatorClient.swift`, `CredentialsTests.swift`, and `main.swift` were not bulk-reformatted. |
| `git diff --check` from repository root | Passed. |
| Synthetic canary absence scan | `TASK_260712_SECRET_CANARY_9f3a` absent from `node-app/Sources` and `node-app/Tests`. |
| Public bearer API scan | No public raw `issueDeviceInvite`, `probe`, `rotateRecovery`, or `issueTelegramLink` methods in `OnboardingHTTPClient.swift`. |
| Secret/URL/log scans | Raw URL/transport-error logging and secret-bearing path/query construction absent; expected secret reveals are limited to bounded JSON request bodies and explicit export payload construction. |

The release build reports pre-existing Swift concurrency warnings in
`PlayerCore.swift`/protocol payload types. Those files are outside this task
and were not modified. No repository-standard signed/notarized app command was
run because signing/deployment would mutate external state; the production
release executable build above is the scoped compile validation.

## Changed file SHA-256

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

## Remaining risks and review focus

- Production Security.framework and NSPasteboard calls were compiled but not
  exercised against real user state; deterministic fakes cover failure and
  concurrency boundaries without touching the developer's Keychain/clipboard.
- Review should independently compare the accepted coordinator response shapes
  and canonical-origin vectors against the frozen Rev15 contract.
- Review should line-audit migration source/destination identities, pending
  false→true reconciliation, and recovery promotion cleanup before acceptance.
- The SwiftUI onboarding task must preserve the exact RU/EN unrecoverable-loss
  warning and invoke copy/save/backup acknowledgement only from explicit user
  actions.
