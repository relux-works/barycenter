# TASK-260712-2u1w16 — macOS onboarding rework guard R5

The frozen R3 producer handoff is rejected. Correct all four findings below as
one narrow rework. The R1 implementation contract, R3 repair contract, and all
already accepted R3 invariants remain authoritative. Do not weaken strict
decoding, recovery crash safety, origin/capability binding, secret handling, or
pasteboard replacement safety merely to make a test pass. Do not edit the
production onboarding UI, commit, push, reset, checkout, clean, touch real
Keychain/pasteboard/user files/live network, or mark the task `done`.

Read in full before editing:

- `TASK-260712-2u1w16_independent-review-r4.md` (SHA-256
  `3b3127cb16215391d172de15b446ecf592b802d323b0810ca205fac6bf9b12d7`);
- `TASK-260712-2u1w16_rework-r3-results.md` (SHA-256
  `293ba3fc4d74ea4521fdbc7d2822b5121eb5c25cf918255932885201be11e32c`);
- the R1 and R3 precondition resources and the complete task specification.

## Starting boundary and allowed edits

Recheck these exact starting hashes before editing:

```text
e368109a307162a13bb9aec4c60d95295eadf851b7d4515651a536ae58d358b4  node-app/Sources/NodeCore/OnboardingService.swift
9d2429ec2dbc04ff5b2a5c934b7f87b27d969e96ba7befb5831769046e1e721a  node-app/Sources/NodeCore/OnboardingHTTPClient.swift
bdcaf2f478aee55eed98eade76be2f5e7220d67737094481c3366a562fc215e8  node-app/Sources/NodeCore/RecoveryExport.swift
80670e0426af9af415febef819773f548ce35ecc965ce73102073dfd3d426c97  node-app/Tests/NodeCoreTests/OnboardingServiceTests.swift
4bacb318c0a7726ebbf8338d69c34c37e00d0f7da586caaaf2fec6e9dedba55c  node-app/Tests/NodeCoreTests/OnboardingHTTPClientTests.swift
fe914849b75e3579d27b293da10cedf0d9fd880e7fe400ac3034d159c7d47cc1  node-app/Tests/NodeCoreTests/RecoveryExportTests.swift
```

Only those six source/test files may be changed. If a production requirement
genuinely cannot be met within them, stop and report the exact reason instead
of expanding scope. In particular preserve `node-app/Sources/NodeApp/main.swift`
at SHA-256
`9779a339c3dcca86f5a7b0f62bbc6f90befd6cba88fad071875fdb49b72c5a80`
and preserve every other R3 file at the hashes recorded in the R4 report.
Concurrent Windows, coordinator, Telegram, lifecycle, documentation, and board
changes are out of scope and belong to other work.

## R5-F1 — backup acknowledgement must be bound to the service origin

`OnboardingService.acknowledgeRecoveryBackup` currently compares only
`actor_id` and `recovery_id`. A service/client for origin B can therefore mark
origin A's same-tuple protected bundle as explicitly backed up. Before mutation,
require the currently loaded bundle's canonical `coordinatorOrigin` to equal
the exact `client.origin` in addition to the exact recovery tuple. A missing or
mismatched origin must fail closed with no byte/state mutation.

Add deterministic production-seam tests with two origins sharing the same
actor/recovery identifiers. Prove the wrong-origin acknowledgement fails and
preserves the bundle exactly; prove the matching-origin path changes only the
acknowledgement flag. Include missing-origin/corrupt state if representable by
the injected repository seam.

## R5-F2 — create response title must echo the submitted canonical title

`createOrbit` sends the trimmed nonempty `cleanTitle` but accepts and returns
any other nonempty server title. Bind the response to the request: the response
`title` must equal the exact trimmed string that was submitted, not merely have
valid length/shape. Case changes, added/removed whitespace, Unicode
normalization lookalikes, and any other byte/string-different title must yield
generic `invalidResponse` and must not produce credentials or recovery output.

Add a success test for the exact trimmed echo and adversarial response tests
for at least whitespace, case, and canonically equivalent-but-not-exact Unicode
variants. Use injected transport only; assert the sent request contains the
trimmed title and no secret is disclosed by the error.

## R5-F3 — exhausted automatic pasteboard cleanup must be visible and retryable

The R4 HIGH finding is mandatory. `RecoveryPasteboardLease` must retain exact
lease ownership after transient clear failures, but it may not silently stop
after the capped automatic retry schedule while the secret-bearing payload may
remain. Expose a public, generic, non-secret observable cleanup state/outcome
suited to future UI integration. It must distinguish terminal automatic cleanup
failure from a resolved/idle lease without exposing payload, recovery ID,
actor ID, secret, change count, or lease identifier. Preserve an explicit safe
manual retry path; a later successful retry must retire the exact lease and
clear the failure state. A proven external clipboard replacement must retire
the lease without clearing newer content and without reporting a false failure.

State transitions must be lease-identity safe: an old timer/retry/exhaustion
must never change the state of a newer copy. Copying a new payload resets state
for the new exact lease. Explicit clear errors remain generic and retryable;
no secret may enter descriptions, notifications, callbacks, task labels, or
metadata. Keep TTL and retry count/delays bounded and deterministic under the
existing injected sleeper/pasteboard seams; never auto-copy.

Add deterministic tests for initial failure plus all retries exhausted,
terminal visibility, exact lease retention, later manual success, later
external replacement, copy/copy with stale exhaustion, stale timer/cancel,
and value-independent/non-secret public status. Retain the existing real
`NSPasteboard` one-main-thread-closure compare-and-clear invariant.

## R5-F4 — `retry_after_seconds` representation is strict in every envelope

The R4 MEDIUM finding is mandatory. Do not collapse a present malformed value
to semantic `nil`. For every accepted non-429 error, the required
`retry_after_seconds` field must be exactly JSON `null`. A string, boolean,
object, array, zero, negative integer, fractional number, exponent form, or
overflowing integer must be `invalidResponse` even when the status/code pair is
otherwise valid. For accepted HTTP 429 `too_many_attempts`, require a bounded
positive integral JSON number and an exact canonical decimal `Retry-After`
header representing the same value; reject absent, signed, padded,
whitespace-bearing, leading-zero, fractional/exponent, overflowing, or
mismatching header/body representations.

Preserve exact key/status/code/message validation and generic error disclosure.
Add a table covering malformed values across representative 400/401/403/5xx
non-rate-limit responses and 429 body/header edge cases, including zero,
negative, fractional, exponent, overflow, leading zero, sign, whitespace, and
mismatch. Tests must call the real client decoder through injected transport.

## Verification and handoff

Read every touched file in full after the final edit. Run focused onboarding,
HTTP, recovery/export/clipboard tests; the full Swift suite; at least 100
deterministic recovery/clipboard repetitions; release build; strict scoped
formatting; `git diff --check`; scoped secret/URL/log canary scans; and
`task-board validate`. Do not touch real user state. Preserve all existing
tests and report any pre-existing warning honestly.

Write exactly one superseding outcome named
`TASK-260712-2u1w16_rework-r5-results.md`. Include exact changed-file inventory
and SHA-256 hashes; R5-F1..F4 production/test mapping; exact commands, counts,
and results; dirty-tree boundaries; and any unavailable gate. Attach it as an
outcome, set the task to `to-review`, and do not claim acceptance: root will
re-read every changed file, independently rerun tests/falsification schedules,
freeze hashes, and commission a fresh independent review.
