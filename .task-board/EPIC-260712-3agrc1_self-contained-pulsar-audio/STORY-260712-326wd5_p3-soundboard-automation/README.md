# P3 Soundboard and safe automation

## Description
Add safe cues, schedules and scoped automation that respect DND, volume ceilings and audit.

## Scope
Implement reusable licensed/user cues, configurable hotkeys, timezone-aware schedules and a scoped webhook or local automation surface. Integrate history, revocation, rate limits, DND, target policy, local volume ceiling and audit without any emergency bypass.

## Acceptance Criteria
C7 passes. Scheduled and API-triggered media obey timezone, DND, ACL and volume constraints, every event is attributable and cancellable, token revoke is immediate, runaway/rate-limit tests pass and automation cannot activate microphone or bypass recipient controls.
