# Add macOS Keychain onboarding credential client

## Description
Extend the macOS client from pair-only node credentials to the full onboarding credential bundle stored in Keychain.

## Scope
Implement the macOS Keychain bundle for node and control credentials and nonsecret recovery state; migrate existing file and legacy Keychain state without changing node token or ws URL; add create, join, recover and Telegram-link calls; show the one-time recovery secret in an explicit copy or save flow without silently persisting it or leaving it on the pasteboard indefinitely; sanitize deep-link and error logging; and expose service hooks for the onboarding UI.

## Acceptance Criteria
macOS keeps long-lived node and control credentials only in Keychain after migration. Existing pairings survive. Create, join and recover work, recovery replaces only control credential, and the user is clearly told when an unsaved one-time secret is gone. Tests cover migration, split capability, secret nonpersistence, pasteboard cleanup, redacted links and failure recovery.
