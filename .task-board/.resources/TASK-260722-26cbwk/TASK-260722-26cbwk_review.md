# TASK-260722-26cbwk Review — ACCEPTED

Reviewer: claude-opus-4-8 (RUN-260721-09b181). Read-only review; no code modified.

## Context
Prior reviewer RUN-260721-ccc225 recorded NO verdict — hard 429 Fable-5 credit-exhausted (`You have reached your Fable 5 limit`). This is the first substantive review of the task.

## Independent verification (Xcode 6.2.3 / DEVELOPER_DIR=/Applications/Xcode.app)
| Check | Result |
|---|---|
| `swift build` (debug) | Build complete |
| `swift test --filter DeviceInvitation|CredentialActivation|MacFirstRun|MacDeviceInvitation` | 22/22 pass, 5 suites |
| `swift test` (full) | 381/381 pass, 63 suites |
| `swift build -c release` | Build complete |
| swift-format --strict, new app-target files (4-space cfg) | CLEAN |
| swift-format --strict, DeviceInvitationPasteboard (default 2-space) | CLEAN |

Default-config lint counts on the new app-target files are 4-space-vs-2-space indentation noise; the whole app target is 4-space (existing sibling PulsarShellModel.swift = 1600 default-config issues). The unchecked global Lint-clean item is pre-existing repo-wide convention debt, not introduced by this task.

## AC coverage
- Fresh GUI launch enters Home with Create/Join directly (`mainWindow.show(section: .home)`, no `onboarding.show`; actions route to `.create`/`.join`). Telegram pairing kept as optional Settings action only.
- Authorized-primary single-flight companion invitation (primary-role probe + `beginGeneration` guard + `issueDeviceInvite(intendedRole: .companion)`).
- Deliberate copy/hide with keyboard shortcuts, visible expiry, full EN/RU failure + status copy.
- No secret sink: `@ObservationIgnored` zeroized transient bytes, redacted `description`/`Mirror`, `.privacySensitive()`+`.accessibilityHidden(true)`; source test forbids UserDefaults/Logger/NSLog/print/Codable.
- Conditional pasteboard cleanup: change-count + exact-payload guard; user replacement survives; nonisolated synchronous termination clear.
- Cancellation/relaunch safety: epoch guards + recovery-acknowledgement activation gate (`activationEligibleNodeCredentials`) prevent leak/reuse/premature activation; expiry+relaunch test proves no reuse.

## Architecture
New `NodeAppComposition` module isolates the invitation lifecycle behind protocol seams (`DeviceInvitationServicing`, `DeviceInvitationClipboard`) reusing `OnboardingService`; mirrors `MacIdentityAppComposition` epoch-guard pattern. Cleanly testable.

## Non-blocking follow-up
`DeviceInvitationPasteboardLease` overlaps substantially with existing `RecoveryPasteboardLease`; it diverges legitimately (nonisolated synchronous termination clear) but is a future consolidation candidate.

Verdict: ACCEPTED -> done.