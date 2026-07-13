# TASK-260712-2u1w16 — Independent security and migration review R4

Date: 2026-07-13 (Asia/Tbilisi)

## Verdict

BACK TO DEVELOPMENT

The frozen R3 implementation is not acceptable for root audit yet. The repository's focused and full test suites and release build pass, and the R3 repairs are present, but this review independently reproduced one strict error-envelope decoder defect and found one clipboard terminal-liveness/privacy defect. Both violate explicit R1/R4 invariants and require production changes plus regression tests.

Review scope was read-only. No production or workspace test source was edited. Adversarial test code was created only in a fresh `/tmp` copy. No real Keychain item, system pasteboard, user-selected file, live coordinator, commit, push, reset, checkout, or clean operation was touched.

## Severity-ranked findings

### HIGH — automatic clipboard cleanup can silently stop while retaining the recovery secret

- Location: `node-app/Sources/NodeCore/RecoveryExport.swift:257-260` and `280-289`; confirming existing test at `node-app/Tests/NodeCoreTests/RecoveryExportTests.swift:265-289`.
- Failure schedule:
  1. The user explicitly copies the three-field recovery export.
  2. The bounded TTL expires.
  3. `PasteboardAccess.clearIfUnchanged` throws on the initial clear and on each of the three capped retries.
  4. `expire` discards each error with `try?`.
  5. When `nextRetryIndex == clearRetryDelays.count`, `scheduleRetry` only sets `expiryTask = nil` and returns.
  6. The exact lease and secret-bearing pasteboard payload remain, but there is no timer, callback, public status, or result that tells the UI/user automatic cleanup exhausted.
- Violated invariant: the task requires the one-time secret not be left on the pasteboard indefinitely. R4 additionally requires terminal error visibility. Retaining the exact lease is correct, but silently stopping after the retry cap is not a safe terminal state.
- Existing evidence of the gap: `automaticClearRetriesAreCappedInCountAndDelay` intentionally observes `pasteboard.string() != nil` after all automatic attempts, then relies on a later manually initiated `clearExplicitly`. It does not prove that the terminal automatic failure is surfaced or that the UI can know a manual retry is necessary.
- Required correction: retain the exact lease after failure, expose a generic non-secret observable cleanup-failed state/outcome (or callback) when automatic attempts exhaust, and provide a safe manual retry path. Never expose the payload in the status. Add deterministic tests proving terminal failure visibility, exact lease retention, later successful retry, and preservation of externally replaced clipboard data.

Positive seam result: `SystemPasteboardAccess.clearIfUnchanged` performs change-count comparison, exact payload comparison, and clear in one main-thread closure (`RecoveryExport.swift:160-175`). It never auto-copies. The defect is terminal ownership visibility/liveness, not compare-and-clear atomicity.

### MEDIUM — malformed `retry_after_seconds` is accepted on non-rate-limit errors

- Location: `node-app/Sources/NodeCore/OnboardingHTTPClient.swift:584-600`.
- Failure schedule:
  1. A coordinator or corrupted response returns a schema-shaped HTTP 400 `invalid_request` error.
  2. `retry_after_seconds` is present with a non-null invalid scalar such as a string, boolean, object, array, zero, negative integer, or fractional number.
  3. `positiveInt` returns `nil`.
  4. The HTTP 400 branch accepts `retry == nil`, making the malformed value indistinguishable from the only valid non-rate-limit representation, JSON `null`.
  5. The client returns typed `.api(status: 400, code: .invalidRequest, retryAfterSeconds: nil)` instead of `.invalidResponse`.
- Production-seam reproduction: in an isolated `/tmp` copy, a test used the real `OnboardingHTTPClient` decoder and injected transport. Values `"17"`, `false`, `{}`, `[]`, `0`, `-1`, and `1.5` were each accepted as `.api(...)`; the test produced seven expectation failures.
- Violated invariant: R1 requires strict scalar types and exact status/code compatibility; R4 explicitly requires `null` and numeric edge-case auditing. A present malformed field must not collapse to the semantic value of JSON `null`.
- Missing coverage: current `OnboardingHTTPClientTests` checks malformed success scalars and many envelope/schema cases, but has no non-429 error test for wrong-type, nonpositive, or fractional `retry_after_seconds`.
- Required correction: distinguish absent/null/valid-positive-integer/invalid values before status validation. Non-429 accepted errors must require the field's exact allowed representation; 429 must require a bounded positive integer matching a strictly parsed canonical `Retry-After` header. Add wrong scalar, zero, negative, fractional/exponent, and overflow cases.

## Frozen-boundary verification

The R4 boundary was hashed before source inspection and again after all review commands. Every frozen file remained byte-for-byte identical:

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
9779a339c3dcca86f5a7b0f62bbc6f90befd6cba88fad071875fdb49b72c5a80  node-app/Sources/NodeApp/main.swift
```

Frozen review inputs also matched:

```text
293ba3fc4d74ea4521fdbc7d2822b5121eb5c25cf918255932885201be11e32c  TASK-260712-2u1w16_rework-r3-results.md
142f148b9cf7ef177c1ff4f9710ef2e8a955234b36fe04eca7b7648e1a55af7f  TASK-260712-2u1w16-rework-guard-r3.md
41478cc49106c11975aa39bd33954d24b683660c11015deeb8813f1ca55011d8  TASK-260712-2u1w16_independent-review-r2.md
```

## Independent falsification results

### R2/R3 repair replay

- R3-F1, unsent matching-active: both public recovery entry points reject the corrupt `ever_sent=false`/active-token match before network send and preserve both records. Source control flow and focused deterministic tests agree.
- R3-F2, exact protected schemas: credential and pending payloads use strict canonical decoding and preserve raw bytes for DP/login reconciliation. Unknown keys, duplicate keys, trailing JSON, noncanonical scalar encodings, and byte-different decoded lookalikes fail closed.
- R3-F3, limited context: active bundle persists explicit context strength. The active-save/pending-delete crash path converges with `hasLimitedContext=true` and without a duplicate recovery mutation.
- R3-F4, UTS46 root-dot order: ICU name-to-ASCII processing precedes removal of exactly one mapped root dot. Dot variants, multiple roots, empty labels, zones, IPv4/IPv6 ambiguity, userinfo, encoded hosts, ports, and loopback plaintext policy are exercised.
- R3-F5, export close: production export uses exclusive no-follow open, a complete write loop, `fsync`, exactly one checked close, and best-effort cleanup. Tests inject short/zero writes, write/fsync/close failures, and cleanup failures against the production algorithm.
- R3-F6, descriptions: ordinary/debug descriptions are fixed/value-independent for sensitive capability/material types. Direct initializer and reflection canaries are covered.
- R3-F7, lease identity: transient clear errors retain the exact lease; copy/copy, stale timer, external replacement, explicit clear, and later successful retry schedules are deterministic. The HIGH finding above remains at retry exhaustion.

### Migration and recovery crash audit

- Protected source/destination identities are distinct. Destination write/readback precedes source deletion; destructive delete-then-add fallback is absent.
- DP/login coexistence resolves only exact raw-byte identity. A differing copy or partial state fails closed and preserves sources.
- Legacy pair compatibility remains additive: node token, slot, and WebSocket URL are preserved; pair saves do not invent control authority; control-only recovery does not masquerade as node credentials.
- Recovery uses process-global scope serialization and a persisted false-to-true `ever_sent` update/readback barrier before transport send. Sent pending generations are not overwritten by new recovery IDs.
- Promotion preserves node bytes. Active-save/pending-delete crash convergence recognizes the same sent candidate and completes cleanup without a duplicate mutation.
- Probe/consume status schedules, 403 limited promotion, 401 same-token retry, 401-to-403 retention, 400/429/5xx/network/cancellation ambiguity, and explicit abandon are covered by deterministic store/transport seams.

### Origin, endpoint, JSON, and disclosure audit

- Node and control credentials remain separately typed and bearer selection is endpoint-specific. Create/join derive `/ws` from the canonical origin. Cross-origin and plaintext redirects are rejected before bearer forwarding.
- The bounded URLSession delegate rejects overflow and serializes response receipt/cancellation completion. No raw transport body, bearer, entered code, or full deep link is retained in public errors.
- Strict JSON rejects duplicate keys, unknown keys, trailing bytes, excessive depth, malformed UTF-8, and wrong success scalars. The MEDIUM finding above is a semantic validation hole after strict parsing.
- Telegram link code and bot username remain separate typed values; no code-bearing Telegram URL is constructed. Pair/WebSocket diagnostics retain origin-level context without URL path/query/fragment/userinfo.
- Source and release-binary canary scans found distinctive synthetic secrets only in intended tests, not in production strings. The release binary contains the recovery alphabet validator, which is expected and not a secret.

## Commands and results

All commands ran from `/Users/administrator/Developer/Ivan/barycenter` unless marked `/tmp`.

1. Focused NodeCore validation:

```text
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --filter 'CLIPairingSourceTests|CoordinatorOriginTests|CredentialBundleTests|OnboardingHTTPClientTests|PairingClientTests|OnboardingServiceTests|RecoveryServiceTests|RecoveryExportTests'
exit 0 — 67 tests in 8 suites passed
```

2. Full suite:

```text
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test
exit 0 — 117 tests in 22 suites passed
```

3. Deterministic recovery/clipboard stress:

```text
for run in {1..100}; do DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --skip-build --filter 'RecoveryServiceTests|RecoveryExportTests' >/dev/null || exit $?; done
exit 0 — 100/100 repetitions passed
```

4. Production build:

```text
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift build -c release
exit 0
```

5. Formatting, diff, board, and scans:

```text
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift-format lint --strict node-app/Sources/NodeCore/Keychain.swift node-app/Sources/NodeCore/CoordinatorOrigin.swift node-app/Sources/NodeCore/OnboardingCredentials.swift node-app/Sources/NodeCore/OnboardingHTTPClient.swift node-app/Sources/NodeCore/RecoveryService.swift node-app/Sources/NodeCore/RecoveryExport.swift node-app/Tests/NodeCoreTests/CoordinatorOriginTests.swift node-app/Tests/NodeCoreTests/CredentialBundleTests.swift node-app/Tests/NodeCoreTests/OnboardingTestSupport.swift node-app/Tests/NodeCoreTests/RecoveryExportTests.swift node-app/Tests/NodeCoreTests/RecoveryServiceTests.swift
exit 0

git diff --check
exit 0

task-board validate
exit 0 — board valid

if rg -n 'PAIR_CODE_CANARY|RECOVERY_SECRET_CANARY|TASK_260712_SECRET_CANARY_R3' node-app/Sources; then exit 31; fi
exit 0 — no synthetic secret canary in production source

if strings node-app/.build/release/NodeApp | rg -n 'PAIR_CODE_CANARY|RECOVERY_SECRET_CANARY|TASK_260712_SECRET_CANARY_R3'; then exit 32; fi
exit 0 — no synthetic secret canary in the release executable

if rg -n 'telegram[^\n]*(URL|url)|link_code[^\n]*(URL|url)|(URL|url)[^\n]*link_code' node-app/Sources/NodeCore; then exit 33; fi
exit 0 — no code-bearing Telegram-link URL construction

rg -n 'print\(|NSLog|Logger|logger\.|os_log|fullWebSocketURL' node-app/Sources/NodeCore
exit 0 — expected logger ownership sites only; CoordinatorClient was separately inspected for origin-only URL output
```

6. Isolated adversarial reproduction:

```text
fresh copy: /tmp/TASK-260712-2u1w16-r4.48Nmg5/node-app
swift package clean
exit 0

DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --filter 'OnboardingHTTPClientTests.nonRateLimitErrorsRejectWrongRetryAfterScalar'
exit 1 — 1 test ran; 7 malformed `retry_after_seconds` variants were accepted instead of rejected
```

The first isolated `--skip-build` attempt encountered copied SwiftPM precompiled-header paths from the workspace and was not treated as evidence. The copied package was cleaned and rebuilt before the reported reproduction. Only the `/tmp` test copy was modified.

## Architecture fit and acceptance assessment

The NodeCore layering fits the supplied onboarding sequence: secure storage and HTTP transport are injectable, onboarding methods expose typed outcomes for future UI work, recovery is control-only, and existing pair startup remains source-compatible. The implementation does not redesign the production SwiftUI onboarding window.

Most acceptance criteria are substantively implemented and the existing test/build gates are green. Acceptance nevertheless fails because strict failure decoding is incomplete and bounded pasteboard cleanup can terminate silently with the recovery secret still present. These are contract defects, not documentation-only gaps.

## Remaining gates

1. Correct both findings and add the specified deterministic tests.
2. Rerun focused/full tests, 100x recovery/clipboard schedules, release build, format, diff, and canary scans.
3. Produce a new task-scoped rework outcome with hashes.
4. Obtain a fresh independent security/migration review before root acceptance.

Because the R4 contract permits only this report as workspace review output, no direct edit was made to the repository `LOGBOOK.md`; the findings are also being persisted in task notes.
