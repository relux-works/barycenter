# Add Windows DPAPI onboarding credential client

## Description
Replace the plaintext Windows credential file with a DPAPI-backed onboarding credential bundle and add create plus join plus recover client support.

## Scope
Implement a DPAPI or Credential Locker backed store for node and control credentials and nonsecret recovery state; migrate existing pair-only installs without changing node token or ws URL; add create, join, recover and Telegram-link calls; show the one-time recovery secret in an explicit copy or save flow without silently persisting it or putting it on the clipboard indefinitely; sanitize deep-link and error logging; and expose controller hooks for the Windows onboarding UI.

## Acceptance Criteria
Windows keeps long-lived node and control credentials only in the selected protected store and removes migrated plaintext. Existing pairings survive. Create, join and recover work, recovery replaces only control credential, and the user is clearly told when an unsaved one-time secret is gone. Tests cover migration, split capability, secret nonpersistence, clipboard cleanup, redacted links and failure recovery.
