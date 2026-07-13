# Prove C7 and publish the automation safety handoff

## Description
Produce deterministic and real-client evidence for soundboard and automation safety before Phase 3 acceptance.

## Scope
Test IANA timezone and DST skip or repeat cases, forward and backward clock jumps, crash and restart claim points, duplicate API or scheduler events, quiet hours, DND, block, Air leave, target ACL, source delete or disable, recipient local ceiling, revoke race, schedule cancel, feature and emergency disable, rate and concurrency caps, queue bounds and no-microphone invariants. Include Windows, macOS and Telegram surfaces, manual soundboard remaining available when automation is off, build hashes, sanitized artifacts and rollback steps.

## Acceptance Criteria
Rerunnable evidence proves C7 and at-most-once behavior under all frozen timing and failure cases. Revoke and kill-switch bounds pass, no missed event autoplays later, recipient local ceilings remain last, no capture path is entered and every UI or Telegram outcome reconciles with history. Hardware or seven-day items are handed to acceptance without being called passed.
