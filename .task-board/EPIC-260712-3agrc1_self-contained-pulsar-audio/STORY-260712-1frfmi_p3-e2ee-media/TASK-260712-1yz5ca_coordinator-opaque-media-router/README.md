# Implement the opaque protected-media and live-frame router

## Description
Route encrypted manifests chunks envelopes and PTT frames under canonical ACLs without decrypting content.

## Scope
Extend upload, range fetch, transmission, inbox, history, cache revocation and live relay paths for versioned ciphertext. Validate only bounded public envelope and manifest fields, actor authorization, exact target snapshot, epoch, object length, hash and rate; treat encrypted chunks and live payloads as opaque. Preserve resumable upload, range and If-Range, quota, delete, report, DND and receipt semantics, isolate slow recipients and never place ciphertext blobs or secrets in ordinary logs. Deletion revokes future server access but UI and policy must not claim erasure from devices that already hold keys.

## Acceptance Criteria
Authorized clients can publish and fetch or relay only ciphertext for supported protected paths; non-targets, stale epochs, malformed sizes and unauthorized ranges fail without disclosure. Coordinator memory, queues, storage and logs are bounded, captured server state cannot play test media and legacy plaintext media remains explicitly separate behind flags with no silent downgrade.
