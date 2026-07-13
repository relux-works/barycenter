# Persist only public E2EE state ciphertext metadata and audit

## Description
Add additive repositories for encrypted media and public group state without ever persisting client secrets.

## Scope
Store device public keys and verification state, serialized public group proposals or commits, epochs and fork status, opaque content-key envelopes, encrypted manifests and object references, grants, recovery or transfer ciphertext packages, report evidence metadata and immutable audit. Use conditional transitions for concurrent commits, replay, revoke and stale workers; preserve legacy plaintext rows only for explicitly non-E2EE media. Backups contain ciphertext and public metadata only, old coordinators ignore new rows under rollback and feature flags default off.

## Acceptance Criteria
Fresh, migrated, concurrent and rollback fixtures preserve existing data and reject stale epoch, duplicate finalize, fork and revoke races. Repository and backup scans find no private device key, group secret, content key, plaintext protected media or unconsented decrypted evidence. Every transition is bounded, auditable and compatible with cryptographic state-model vectors.
