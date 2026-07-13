# Persist schedules principals and at-most-once execution lineage

## Description
Add additive storage for saved-cue automation state after the media lifecycle is frozen.

## Scope
Add schedules with IANA timezone and policy version, scoped principals with hashed bearer secret and revoked_at, feature and emergency-disable state, immutable execution IDs, scheduled wall and UTC instants, target snapshot reference, accepted_at, status, deny reason, retry generation, leases and audit lineage. Use compare-and-swap claims so restart, overlapping ticks, clock jumps and multiple runtime workers cannot duplicate an event. Preserve additive rollback and retention boundaries without storing plaintext tokens.

## Acceptance Criteria
Fresh, migrated and rollback fixtures preserve prior media. Repeated or skipped DST, backward or forward clock changes, crashes before and after claim and concurrent workers produce the contract-defined zero or one execution. Secrets are shown only once, stored hashed, redacted from logs and immediately revocable; every row supports attribution, cleanup and quota reconciliation.
