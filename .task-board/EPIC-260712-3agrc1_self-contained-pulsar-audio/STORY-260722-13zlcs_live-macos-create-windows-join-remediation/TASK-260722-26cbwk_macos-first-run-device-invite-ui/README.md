# TASK-260722-26cbwk: macos-first-run-device-invite-ui

## Description
Complete the macOS production shell so a fresh ordinary launch enters the accountless Create/Join experience directly and an activated primary can issue a one-time device invitation for another installation.

## Scope
Reuse OnboardingService and its control-capability boundary. Add a main-shell invitation surface with companion role, explicit generate, copy and hide actions, visible expiry, auto-clear pasteboard behavior, cancellation-safe async state and no secret persistence or logging. Keep recovery export mandatory before activating a newly created Barycenter. Preserve optional Telegram pairing without making its legacy window the primary first-run path. Add model, composition, SwiftUI source and lifecycle tests; preserve EN/RU, keyboard and VoiceOver semantics.

## Acceptance Criteria
A fresh macOS GUI launch visibly exposes Create and Join without requiring the status menu or Telegram. After safe Create plus recovery export, an authorized primary can generate exactly one device invitation, see and copy it deliberately, hide it, and receive localized expiry/error status. The code is never written to logs, UserDefaults, model snapshots after hide, accessibility values or unrelated persistence; pasteboard cleanup is conditional. Cancellation/relaunch cannot leak or reuse one-time material. Swift focused/full tests and release build pass.
