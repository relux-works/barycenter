# Implement at-most-once automation runtime and kill switches

## Description
Evaluate schedules and authenticated triggers safely through the canonical transmission service.

## Scope
Claim due executions atomically, resolve and seal canonical targets, recheck current Air, DND, block, quiet hours, feature flag, principal revoke and media disable before transmission creation, then record accepted_at and exact outcomes. Apply the frozen no-catch-up, DST, idempotency, retry, queue and concurrency rules, per-principal and per-orbit limits, pending cancellation and global emergency disable. The coordinator never opens a microphone and does not claim to enforce receiver local volume; every recipient mixer retains its own output ceiling last.

## Acceptance Criteria
Crash, restart, duplicate tick, duplicate API idempotency key, clock change and concurrent-worker tests create at most one eligible transmission. Revoked, disabled, DND, blocked, quiet-hour, invalid-media or stale-target events create none and are audited exactly. A slow or malicious principal cannot grow memory or queues, kill switches stop new events within the frozen bound and standard recipient ceilings remain authoritative.
