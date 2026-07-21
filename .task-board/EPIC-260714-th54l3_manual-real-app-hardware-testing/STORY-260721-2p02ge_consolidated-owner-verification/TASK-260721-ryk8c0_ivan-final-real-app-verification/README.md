# TASK-260721-ryk8c0: ivan-final-real-app-verification

## Description
Run the single final owner verification pass after the autonomous desktop engineering packet is published. Follow the supplied exact-build script once on mbpro-win and once on macOS, record only the requested screenshots/logs and return one pass/fail packet.

## Scope
Windows: normal Start-menu/MSIX launch with no terminal, permission deny/allow, built-in-mic capture, playback/duck/interrupt, hotkey fallback, picker persistence, UI scaling/keyboard/readability, suspend/lock/restart, uninstall cleanup. macOS: normal notarized launch, Keychain/onboarding, permission deny/allow, built-in-mic capture, overlay/interrupt, menu bar/hotkey, Retina UI/keyboard/VoiceOver spot checks, sleep/relaunch/cleanup. Cross-service: one real target/Air/Telegram or equivalent transport path, one streaming/live path when enabled, moderation/delete/recovery smoke, and the final passive soak defined by the handoff. Unsupported or disabled capabilities are recorded honestly, never inferred.

## Acceptance Criteria
Ivan Oparin runs only this task, against exact hashes supplied by TASK-260721-2346wf. Every checklist row has PASS, FAIL, BLOCKED or NOT_APPLICABLE plus a timestamp and requested artifact reference. Both apps launch as ordinary GUI applications, primary capture/playback and recovery paths are exercised, critical UI is legible and keyboard reachable, and cleanup is verified. A FAIL creates a focused engineering bug; no individual legacy manual task is revived unless needed for provenance.
