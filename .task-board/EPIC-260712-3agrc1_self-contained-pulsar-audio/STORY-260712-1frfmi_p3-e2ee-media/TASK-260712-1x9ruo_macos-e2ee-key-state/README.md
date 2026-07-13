# Implement macOS device and group key state

## Description
Own macOS E2EE device identity epoch ratchets media-key cache and transactional persistence in Keychain-backed storage.

## Scope
Use separate Keychain items and reviewed library bindings for device signing and key-agreement material, group state, current grants and bounded content-key cache. Apply OS CSPRNG, transactional persist-before-ack, rollback or clone detection, epoch fork handling, key expiry and best-effort memory clearing. Redact unified logs, telemetry and crash diagnostics and exclude secrets from preferences and sync unless the recovery design explicitly encrypts them. Expose narrow interfaces to send, playback, live and UX tasks.

## Acceptance Criteria
Supported macOS builds pass known-answer and state-machine vectors, restart and crash points never reuse a nonce or acknowledge an unpersisted epoch, revoked or cloned state fails closed and scans find no key in preferences, logs, crash artifacts or coordinator data. Keychain access, backup and irrecoverable-loss behavior match the threat model.
