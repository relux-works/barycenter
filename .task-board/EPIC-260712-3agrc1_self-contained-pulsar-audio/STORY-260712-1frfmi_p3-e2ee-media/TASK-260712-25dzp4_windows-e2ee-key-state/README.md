# Implement Windows device and group key state

## Description
Own Windows E2EE device identity epoch ratchets media-key cache and transactional persistence inside the signed package.

## Scope
Extend the reviewed DPAPI credential posture with separate protected device signing and key-agreement material, group state, current grants and bounded content-key cache. Apply reviewed library bindings, OS CSPRNG, transactional persist-before-ack, rollback or clone detection, epoch fork handling, key expiry and best-effort memory clearing. Redact Event Log, telemetry, crash dumps and diagnostics; never place secrets in config JSON or syncable preferences. Expose narrow interfaces to send, playback, live and UX tasks.

## Acceptance Criteria
Signed Windows builds pass known-answer and state-machine vectors, restart and crash points never reuse a nonce or acknowledge an unpersisted epoch, revoked or cloned state fails closed and scans find no key in config, logs, crash artifacts or coordinator data. DPAPI scope, backup and irrecoverable-loss behavior match the threat model.
