# Self-contained Pulsar Audio

## Description
Deliver the user-approved goal in three sequential shippable phases: Store-ready clips without required Spotify or Telegram, Air rooms plus bounded-memory long audio, then near-live PTT, capture quality, E2EE and safe automation. docs/spec-self-contained-audio.md is authoritative.

## Scope
All coordinator, Windows Pulsar, macOS Pulsar, Telegram adapter, protocol, persistence, media pipeline, privacy/UGC operations, Store metadata, migrations, rollout and evidence required by specification sections 19-21. Preserve current production behavior and credentials.

## Acceptance Criteria
All A1-A8, B1-B7 and C1-C7 have reproducible evidence; every phase exit gate and non-functional gate passes in order; unit/integration/golden/migration/security/platform suites are green; Windows-Windows, Windows-macOS and macOS-macOS matrices are proven where applicable; rollback preserves data and legacy operation; docs, policies, runbook and Store listing match the implementation.
