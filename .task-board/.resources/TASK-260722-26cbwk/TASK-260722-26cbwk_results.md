Implementation evidence

- Fresh ordinary GUI bootstrap opens the native Home shell with direct Create and Join actions; Telegram pairing remains an explicit optional Settings action.
- Authorized active-primary invitation composition reuses OnboardingService and requests only the companion role. The Devices surface provides explicit generate, copy, hide, visible expiry, EN/RU feedback, keyboard shortcuts, and VoiceOver-safe semantics.
- Invitation plaintext is confined to zeroized transient model bytes and an exact-payload/change-count pasteboard lease. It is absent from shell snapshots, accessibility values, defaults, logging, and durable encoders. Hide, expiry, window close, replacement, and shutdown clear conditionally; termination closes queued writes and later copies wait behind prior cleanup.
- Newly created credentials remain activation-ineligible until recovery export and exact acknowledgement. Live recovery cannot be replaced by another Create or Join, and relaunch retains only the non-secret idempotency marker.

Validation evidence

- `swift test --filter DeviceInvitation|CredentialActivation|MacFirstRun`: exit 0, 22 tests in 5 suites.
- `swift test`: exit 0, 381 tests in 63 suites.
- `swift build -c release`: exit 0.
- Strict swift-format lint for all new invitation, composition, and focused test files: exit 0 in both four-space app scope and two-space NodeCore scope.
- `git diff --check`: exit 0.
- The first combined focused run exited 1 on a test-only wake scheduling assertion; the synchronization was corrected and both later focused runs exited 0.
- Whole legacy touched-file strict swift-format diagnostics exited 1, including a final four-space-configured run, on pre-existing style debt across main.swift, PulsarMainWindow.swift, PulsarShellModel.swift, and their existing tests. No project formatter configuration exists that makes those legacy files green without a broad unrelated rewrite; the new files and changed behavior scopes are strictly clean.
- Release compilation still reports pre-existing Swift 6 Sendable warnings in PlayerCore; the build exits 0 and this task adds no new warning.

See the 2026-07-22 TASK-260722-26cbwk LOGBOOK entry for architectural decisions and race findings.