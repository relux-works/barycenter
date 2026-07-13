# TASK-260712-2u1w16 — R3 rework results

Date: 2026-07-13 (Asia/Tbilisi)

Status intent: producer handoff to root review. This evidence is not acceptance; the mandatory root full-file/hash audit and fresh independent security/migration review remain open.

## Outcome

The macOS NodeCore credential store, onboarding HTTP client, recovery state machine, explicit recovery export, and clipboard lease now implement the R3 contract while preserving the production UI boundary and the existing pair startup interface. Only the eleven task files listed under hashes changed from the independently verified R2 boundary. `node-app/Sources/NodeApp/main.swift` remains byte-identical to the frozen R2 hash.

The audit also tightened the persisted context invariant: an `active` control credential must have orbit/role context; clean-install control-only recovery state is explicitly `limited` and cannot masquerade as node credentials.

## R3 finding-to-code/test map

| Finding | Production correction | Deterministic evidence |
|---|---|---|
| R3-F1 | `RecoveryService.activeAlreadyPromoted` rejects a matching active token when the pending record is still `ever_sent=false`; neither `recover` nor `resumePending` can probe, consume, delete, or replace that impossible state. | `unsentMatchingActiveFailsClosedAtBothPublicEntryPoints` snapshots active/pending protected bytes and proves zero sends for both entry points. |
| R3-F2 | Current credential bundles, v1 migration bundles, and pending records use strict parsed schemas plus canonical re-encoding. DP/login copies reconcile only when their protected bytes are identical. | Credential and pending tests cover unknown/duplicate/trailing JSON, noncanonical numbers/escapes, key order, whitespace, and byte-different DP/login pairs; every conflict preserves both copies. |
| R3-F3 | Credential bundle v2 durably stores `ControlContextStrength.active/limited`; v1 migration derives it. Limited 403 promotion retains prior orbit/role metadata but persists limited strength. | Full and limited promotion/crash schedules cover active-save then pending-delete failure, restart cleanup with zero sends, old v1 active/limited migration, and unchanged node bytes. |
| R3-F4 | ICU UTS46 mapping is applied to the raw host before exactly one mapped ASCII root dot is removed, followed by length/label validation. | U+002E/U+3002/U+FF0E/U+FF61 single-root forms pass; multiple roots, empty labels, invalid mappings, 64-byte labels, and overlength domains fail. |
| R3-F5 | Direct export uses an injected production file-operation seam. Success requires exclusive no-follow open, complete EINTR/short-write loop, `fsync`, and one checked `close`; failures attempt truncate/close/unlink without masking the generic error. | Production-algorithm tests cover open, interrupted/short/zero/error writes, fsync, close, and cleanup failures; close is never retried and no temp/sidecar path is used. |
| R3-F6 | Node, control, pending, and Telegram result descriptions are fixed/redacted and independent of unvalidated values. | Malicious direct-initializer canaries are absent from ordinary/debug reflection and no URL/slot/token/code reaches crash-friendly descriptions. |
| R3-F7 | A pasteboard lease is retired only after atomic compare-and-clear succeeds or proves replacement. Clear errors retain the exact lease and schedule capped retries; explicit clear surfaces a retryable generic error. | Expiry-first-failure, explicit-first-failure, retry success, retry cap/delay, copy/copy, old timer, external replacement, atomic-clear race, and idempotent clear schedules pass. |

## Behavior matrix

| Flow | Request/auth | Persisted result |
|---|---|---|
| Create orbit | `POST /v1/onboarding/orbits`, no bearer, stable installation attempt ID | Validated node/control/recovery metadata bundle in protected storage; recovery secret remains in a non-Codable in-memory type and is returned once. |
| Issue/consume invite | Control bearer only for issue; no bearer for consume | Consume stores validated node/control capabilities; codes are neither persisted nor placed in a URL. |
| Context probe | Supplied node or control bearer, same canonical origin only | 200 returns full context; authenticated 403 is limited; 401 is unauthorized; 429 carries validated retry metadata. |
| Recover | No bearer; recovery secret and previously protected candidate are bounded JSON-body fields | Only control credential/context and recovery metadata change; node token, slot, orbit, and WebSocket URL bytes are preserved. |
| Rotate recovery | Control bearer and `{}` body | Only non-secret recovery metadata is persisted; one-time secret is returned in memory until explicit export/dismissal. |
| Telegram link | Control bearer | Code and bot username remain separate typed values; no Telegram deep link is constructed or persisted. |
| Explicit export/copy | Deliberate caller action only | Export contains exactly actor ID, recovery ID, and recovery secret. Copy leases clear only the unchanged exact payload; display never counts as backup. |

## Migration/crash matrix

| State or boundary | Result |
|---|---|
| Legacy DP item, legacy login item, or legacy JSON file | Read source; write distinct `onboarding-credential-bundle-v2`; exact read-back; only then delete that source. Node token/slot/WebSocket URL remain exact. |
| Canonical v1 protected bundle | Migrates to distinct v2 and verifies exact v2 representation before v1 deletion; full context maps active and context-free control maps limited. |
| Add/update/read-back failure | Returns structural storage failure and preserves a readable prior/source copy. No delete-then-add update path exists. |
| Source delete failure after verified destination | Verified destination and source both remain readable; restart converges idempotently. |
| Equivalent DP/login destination bytes | Safe update keeps the prior login copy until DP update/read-back verifies, then removes the redundant copy. |
| Any byte-different DP/login destination or pending copies | Fails closed without update/delete, including decoded-lookalike whitespace/order/unknown/duplicate variants. |
| Pair save racing control promotion | Process-wide repository mutation serialization preserves both the new node capability and recovered control capability. |
| Promotion saved, pending delete failed | Restart verifies the exact active candidate and tuple, reports persisted active/limited strength, and deletes pending without another request. |

## Recovery response matrix

| Observation after the durable send barrier | Pending/action |
|---|---|
| Consume 200 | Promote control/context, preserve node, verify protected state, delete exact pending record. |
| Consume 400/403, 429, 5xx, network, cancellation, decoder ambiguity | Retain the sent candidate; return typed rejected/rate-limited/ambiguous outcome as applicable. |
| Restart probe 200 | Promote full active context, then delete exact pending. |
| Restart probe authenticated 403 insufficient capability | Promote limited context durably, retaining protected prior orbit/role metadata when present. |
| Restart probe 401 | Retain the same tuple/token and request the existing generation's secret. |
| Probe 401 then consume 403 | Retain candidate and offer only explicit warned destructive abandon. |
| `ever_sent=false` pending matches active | Structural conflict, zero network sends, zero mutation. |
| Duplicate pending transition partially updated | Byte conflict, zero network sends, preserve both protected copies. |

## Changed files and SHA-256

These eleven files differ from the verified R2 boundary:

```text
5574ecf4377a19a37eeaa31f9ea7ca33e6eb1e9c094313858accf2198581b0cf  node-app/Sources/NodeCore/Keychain.swift
32e4009a3cf9177d23cb88fea65601f4642f2ffe0e20b51e9b587bd48440964a  node-app/Sources/NodeCore/CoordinatorOrigin.swift
c6abefdf7cfc5b0694c7aebfbe28e6de88762079d67f948246f875d2e750272f  node-app/Sources/NodeCore/OnboardingCredentials.swift
9d2429ec2dbc04ff5b2a5c934b7f87b27d969e96ba7befb5831769046e1e721a  node-app/Sources/NodeCore/OnboardingHTTPClient.swift
94135aeaaac8b1b728253498ab61e2a5220f72a5e62d54be1381a972279cd1b7  node-app/Sources/NodeCore/RecoveryService.swift
bdcaf2f478aee55eed98eade76be2f5e7220d67737094481c3366a562fc215e8  node-app/Sources/NodeCore/RecoveryExport.swift
43ec50ccec2396647cc976800adb90d7001490538dc637558aac06c434bf4c44  node-app/Tests/NodeCoreTests/CoordinatorOriginTests.swift
afa8ab8e61ac6580823a78d676891a9f10b5fea4c72e6f12181b7b62a856f779  node-app/Tests/NodeCoreTests/CredentialBundleTests.swift
6676f67cd01ef721b487dc9afd5cdc4c0b19294b2a536e04de4007323a634dbd  node-app/Tests/NodeCoreTests/OnboardingTestSupport.swift
fe914849b75e3579d27b293da10cedf0d9fd880e7fe400ac3034d159c7d47cc1  node-app/Tests/NodeCoreTests/RecoveryExportTests.swift
02b02d374ccd076823a0823a6a893d8c98000338fc5d9eef5c03f2f4f6cb4d28  node-app/Tests/NodeCoreTests/RecoveryServiceTests.swift
```

Frozen/out-of-scope R2 files were rehashed. In particular:

```text
9779a339c3dcca86f5a7b0f62bbc6f90befd6cba88fad071875fdb49b72c5a80  node-app/Sources/NodeApp/main.swift
ba2badebf20ea0e4ffc089b5a6f13b6bc349d85bd92b5888250025cc7bff631e  node-app/Sources/NodeCore/CoordinatorClient.swift
e090bc86a4d496f1887bc6391dea2f50f1246b7172af713aa90f44239b834a64  node-app/Sources/NodeCore/Credentials.swift
64a060cd7495ba996bad96da8a5988737911e3208e11eea84027c7875325d231  node-app/Sources/NodeCore/StrictJSON.swift
e368109a307162a13bb9aec4c60d95295eadf851b7d4515651a536ae58d358b4  node-app/Sources/NodeCore/OnboardingService.swift
764af667c75437e1113229be9eca1a18450de25d4cb09c33076eccccc905f992  node-app/Tests/NodeCoreTests/CredentialsTests.swift
4b2d6185fb5c4527d7e8bf2c00185659cde84b8267f9b29e96f54ac240af4e4a  node-app/Tests/NodeCoreTests/CLIPairingSourceTests.swift
4bacb318c0a7726ebbf8338d69c34c37e00d0f7da586caaaf2fec6e9dedba55c  node-app/Tests/NodeCoreTests/OnboardingHTTPClientTests.swift
80670e0426af9af415febef819773f548ce35ecc965ce73102073dfd3d426c97  node-app/Tests/NodeCoreTests/OnboardingServiceTests.swift
0cd72c6fe513631eeba05824df506bd507ba447ae668844d03a09bc709cf73b0  node-app/Tests/NodeCoreTests/PairingClientTests.swift
```

## Verification commands and results

All commands ran from the repository root unless a `node-app` working directory is shown.

| Command | Result |
|---|---|
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --filter 'CredentialBundleTests\|RecoveryServiceTests\|RecoveryExportTests\|CoordinatorOriginTests'` (`node-app`) | exit 0; 51 tests / 4 suites passed. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --filter 'CLIPairingSourceTests\|CoordinatorOriginTests\|CredentialBundleTests\|OnboardingHTTPClientTests\|PairingClientTests\|OnboardingServiceTests\|RecoveryServiceTests\|RecoveryExportTests'` (`node-app`) | exit 0; 67 tests / 8 suites passed. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` (`node-app`) | exit 0; 117 tests / 22 suites passed. |
| `for i in {1..100}; do ... swift test --skip-build --filter 'RecoveryServiceTests\|RecoveryExportTests' ...; done` (`node-app`, isolated captured run) | exit 0; 100/100 repetitions passed (28 tests per repetition). |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift build -c release` (`node-app`) | exit 0; release NodeApp linked. Existing warnings remain only in untouched `PlayerCore.swift`, `LibrespotClient.swift`, and `Protocol.swift` Sendable captures. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift-format lint --strict <11 changed Swift files>` | exit 0; no diagnostics. |
| `git diff --check` | exit 0. |
| Scoped `rg` source scans plus `strings node-app/.build/release/NodeApp` for `TASK_260712_SECRET_CANARY_R3`, direct logging primitives, and invite/link/recovery-secret URL construction | exit 0; canary exists only in its test fixture; no scoped production/binary match. |
| `task-board validate` | exit 0; board valid, no issues. |
| SHA-256 command over all 21 producer-listed R2 files | exit 0; eleven deltas are listed above and every frozen file matches R2. |

One preliminary suppressed repetition attempt exited 1 at iteration 16 while package invocations had overlapped; its output was unavailable by construction. A single diagnostic rerun passed, and the subsequent isolated output-capturing 100-iteration run passed 100/100. One preliminary generic-secret scan also matched the intentional recovery alphabet constant; the corrected distinctive synthetic-canary scan is the passing command recorded above.

The repository-standard `scripts/build-app.sh` check was not run: it queries and may unlock the developer signing Keychain and writes/signs an app bundle, so it is not a user-data-neutral gate for this task. The production `swift build -c release` check was run instead.

## Dirty-tree boundary and data-safety statement

The tree already contains concurrent coordinator, Windows, documentation, workflow, logbook, board, and prior task-resource changes. They were not edited by this R3 pass. The only R3 source/test deltas are the eleven files hashed above plus this evidence document. Existing R2 task files outside that list were preserved byte-for-byte, including the frozen `main.swift`.

All tests used injected in-memory protected stores, transports, file operations, clocks/sleepers, and pasteboards. No test accessed the developer's real Keychain, system pasteboard, or user-selected files. No live coordinator request was sent.

## Remaining review gates and risks

- Root must independently read the changed files, verify hashes, rerun the commands, and commission the fresh independent security/migration review required by R3.
- Real Security.framework and NSPasteboard runtime behavior was source-audited but deliberately not exercised against user state; independent review should retain that no-user-data boundary.
- The release build's pre-existing out-of-scope Sendable warnings remain visible and were not changed here.
- UI wiring remains owned by `TASK-260712-3dqc3l`; this task exposes typed services/outcomes only and does not modify the onboarding window.
