# Prove the protected-media container and local preparation path

## Description
Select the exact local encoder packaging and chunked authenticated-encryption format for clips tracks and seekable playback.

## Scope
Build signed Windows and macOS probes that normalize and encode the Phase 2 selected codec locally, encrypt independently verifiable chunks with unique nonces and manifest-bound associated data, upload resumably and fetch by authenticated range. Measure first audio, seek, skew, RSS, disk, CPU, ciphertext overhead and two-hour duration independence; prove corrupt, truncated, reordered, replayed or substituted chunks fail before decode. Avoid plaintext disk where possible, document unavoidable OS buffering and secure cleanup, and review codec and crypto licenses, SBOM and update path. Publish no-go if Store-safe local preparation cannot meet Phase 2 gates.

## Acceptance Criteria
One exact versioned container and local toolchain passes all Windows and macOS pairings, Phase 2 start, seek, skew and memory gates, range and cache rules and tamper vectors without first-run code download. Nonce derivation, chunk size, manifest authentication, resumable upload integrity and plaintext lifecycle are unambiguous, or E2EE media is blocked.
