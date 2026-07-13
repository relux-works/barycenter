# Select and prove the group cryptography and library stack

## Description
Choose a standardized audited client-owned group-key design and exact cross-platform libraries or issue a no-go.

## Scope
Evaluate standardized group protocols and sender-key alternatives against 2..N Air membership, multiple devices, offline recipients, concurrent join or leave, forward secrecy, post-compromise behavior, device authentication and coordinator equivocation assumptions. Use only maintained audited primitives or libraries; pin exact versions, algorithm suite, canonical serialization, entropy source and platform bindings. Prototype Windows and macOS interoperability, known-answer vectors, replay and stale-epoch rejection, Store signing, license, SBOM, CVE and update obligations. The coordinator may route public state and ciphertext but never generate, unwrap or retain content secrets.

## Acceptance Criteria
A source-cited ADR selects an exact algorithm suite and libraries with cross-platform vectors, lifecycle state machine, performance and packaging evidence or blocks E2EE. It defines device identity binding, group commit ownership, nonce and key separation, recovery assumptions, anti-replay and upgrade rules. No custom cryptographic primitive or server-owned group key is permitted.
