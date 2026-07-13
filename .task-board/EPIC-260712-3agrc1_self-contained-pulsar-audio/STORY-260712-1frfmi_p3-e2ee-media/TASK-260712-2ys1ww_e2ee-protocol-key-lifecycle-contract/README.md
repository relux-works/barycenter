# Freeze the E2EE protocol key lifecycle and downgrade contract

## Description
Turn the reviewed threat model and passing spikes into one versioned client-owned wire and state-machine contract.

## Scope
Define device public identities and verification, group proposals and commits, epoch ordering, sender and media keys, encrypted content-key envelopes, chunk manifests, live PTT session keys, history grants, recovery or transfer packages and voluntary report evidence. Bind actor, Air, exact target snapshot, media or live session, generation, sequence, algorithm suite and expiry into authenticated data. Specify concurrent commits, offline members, restart, rollback, replay, cloned state, leave or revoke, secure deletion limits and mixed-version behavior. Unsupported targets are excluded only with explicit confirmation or the send fails; there is never silent plaintext fallback. Add canonical docs, cross-platform goldens, malformed vectors and state-model tests.

## Acceptance Criteria
Go routing mirrors, Windows and Swift implementations have one bounded canonical contract and reject unknown suite, invalid signature, tampered manifest, nonce reuse, stale or forked epoch, replay, foreign target, expired grant and downgrade. Coordinator-visible data contains no content keys. Clips, tracks, saved cues and live PTT have explicit coverage and unsupported combinations cannot be presented as encrypted.
