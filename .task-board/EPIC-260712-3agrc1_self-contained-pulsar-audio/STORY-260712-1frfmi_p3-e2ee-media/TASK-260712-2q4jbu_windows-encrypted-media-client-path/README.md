# Implement Windows E2EE verification recovery grant and report UX

## Description
Integrate the reviewed cryptographic state into honest Windows user flows without owning crypto or media internals.

## Scope
Show per-path encryption status, device identity or verification, membership and epoch errors, unsupported-recipient no-downgrade choice, new-device transfer, user-held recovery if selected, explicit history grants, lost-device revoke, irrecoverable-history warning and metadata-only versus decrypted-evidence report consent. Compose the reviewed key, send, playback and live components; secrets are shown only where required and never persisted by UI.

## Acceptance Criteria
Windows users can distinguish encrypted and plaintext paths before sending or speaking, verify and revoke devices, transfer current access, grant only selected history and understand unrecoverable loss. Silent downgrade and false E2EE state are impossible, report boundary consent is explicit and accessibility and secret-leak tests pass.
