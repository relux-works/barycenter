# P3 End-to-end encrypted media

## Description
Threat-model and implement reviewed group media encryption with rotation, recovery and report evidence.

## Scope
Produce a threat model and reviewed group-key protocol for Orbit/Air media, local encode/normalize, ciphertext routing/storage, join/leave/revoke rotation, recovery and multi-device transfer, history grants, metadata disclosure and voluntary decrypted report evidence.

## Acceptance Criteria
C4-C6 pass after external security review closes all critical/high findings. Removed members cannot decrypt new media, new members lack history without grant, coordinator storage/traffic cannot decode test content, report evidence is explicit and voluntary, keys stay in secure storage, and E2EE claims remain feature-gated until all proof exists.
