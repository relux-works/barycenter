# TASK-260712-2u1w16 — macOS onboarding rework R5 results

## Status and boundary

The six-file R5 correction is present in commit
`99aa26c3b2867decafa4415652986d16451a8409` (`checkpoint macOS onboarding
credentials`), which is contained by `main` through merge commit `0c75dcf`.
This execution pass audited the complete current contents of every R5-owned
file, reran the required falsification and build gates, and made no additional
source edit. The working branch is
`task/task-260712-2u1w16-macos-keychain-r5` at base `22f11e2`.

The only pre-handoff dirty files were the sequential execution tracker and the
task-board progress file. SwiftPM build products remain ignored. No real
Keychain, pasteboard, user file, signing identity, or live network endpoint was
used.

## Exact R5 file inventory

| SHA-256 | File |
|---|---|
| `bf0df748b368da9e1ee7bb23e5516d00199670f2ea054a3e9ec16f4a05c9479f` | `node-app/Sources/NodeCore/OnboardingService.swift` |
| `5acd9ff2ecb9e2b16dfa703cbed1cd18b6ff7343cc1f10bf5c441036dee60568` | `node-app/Sources/NodeCore/OnboardingHTTPClient.swift` |
| `79c1ef32ad64050e546aa89487f3f34f2104ec292b662f631f7bf28c16697901` | `node-app/Sources/NodeCore/RecoveryExport.swift` |
| `68967093c07ffec35519f3b7dcd8f58956c7a5edcd7b93bb408703d6c846a0c9` | `node-app/Tests/NodeCoreTests/OnboardingServiceTests.swift` |
| `095413bdc97dedd8ca568a0b5d84619474f1f569fc331bd1cb487c812a840968` | `node-app/Tests/NodeCoreTests/OnboardingHTTPClientTests.swift` |
| `623e07f141e5080f51f4bfd7064b4a553120d1e72c1f2549cb339e0add817f6c` | `node-app/Tests/NodeCoreTests/RecoveryExportTests.swift` |

The frozen UI boundary is unchanged:
`node-app/Sources/NodeApp/main.swift` =
`9779a339c3dcca86f5a7b0f62bbc6f90befd6cba88fad071875fdb49b72c5a80`.

## R5 finding map

| Finding | Production correction | Deterministic evidence |
|---|---|---|
| R5-F1 | `acknowledgeRecoveryBackup` requires exact `bundle.coordinatorOrigin == client.origin` before mutating the recovery acknowledgement flag. | Wrong-origin same-tuple bytes are preserved; matching origin changes only the flag; missing/corrupt origin state fails closed. |
| R5-F2 | `createOrbit` compares decoded response-title UTF-8 bytes with the exact trimmed submitted title, avoiding Swift canonical-equivalence acceptance. | Exact trimmed echo succeeds; whitespace, case, and Unicode-normalization variants fail generically; the injected transport sees the trimmed request. |
| R5-F3 | `RecoveryPasteboardLease` exposes generic `idle`, `leased`, and `automaticCleanupFailed` states, retains exact lease authority on exhausted retries, supports later manual retry, and guards every timer/retry by lease UUID. | Initial plus capped retry exhaustion, terminal visibility, manual recovery, external replacement, new-copy reset, stale retry/timer, and non-secret descriptions are covered. |
| R5-F4 | Error decoding distinguishes exact JSON `null` from malformed present values; 429 accepts only a bounded positive canonical decimal body value with an exact matching canonical `Retry-After` header. | Representative 400/401/403/5xx malformed scalars and 429 missing, signed, padded, whitespace, zero, negative, fractional, exponent, overflow, leading-zero, and mismatch cases call the real decoder through injected transport. |

## Verification

All commands ran on 2026-07-14. Test seams were injected and deterministic.

| Command | Result |
|---|---|
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --filter 'CLIPairingSourceTests|CoordinatorOriginTests|CredentialBundleTests|OnboardingHTTPClientTests|PairingClientTests|OnboardingServiceTests|RecoveryServiceTests|RecoveryExportTests'` (`node-app`) | exit 0; 75 tests in 8 suites passed. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` (`node-app`) | exit 0; 125 tests in 22 suites passed. |
| 100 sequential invocations of `swift test --skip-build --filter 'RecoveryServiceTests|RecoveryExportTests'` (`node-app`) | exit 0; 100/100 repetitions passed, 31 tests in 2 suites per repetition. |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift build -c release` (`node-app`) | exit 0; `NodeApp` linked in 41.63 seconds. Only pre-existing out-of-scope Sendable warnings in `PlayerCore.swift` and related protocol types remain. |
| `xcrun swift-format lint --strict` over the six R5-owned Swift files | exit 0; no diagnostics. |
| `git diff --check` | exit 0. |
| Production-source and release-binary synthetic secret canary scans | no match. |
| Scoped Telegram/recovery/invite URL or query construction scans | no match. |
| Scoped logging scan | only existing logger ownership sites outside the R5 delta; no R5 secret or raw URL logging. |
| `task-board validate` | exit 0; board valid, no issues. |

## Review disposition

A separate cold root pass re-read the six complete files and attempted to
falsify origin binding, byte-exact title echo, stale lease transitions,
terminal cleanup recovery, and error-envelope representation rules. It found
no additional defect. This pass was performed by the same executor because the
user explicitly required personal sequential execution outside the task-board
spawn workflow; it is therefore not represented as an independent review.

The R5 implementation and evidence are ready for the root acceptance decision.
