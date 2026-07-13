# Add secure Telegram soundboard and quick-control parity

## Description
Expose saved cue triggering schedule status and emergency disable through the same actor-scoped domain commands without making Telegram mandatory.

## Scope
Add secure short-lived callbacks to list and trigger eligible cues with explicit targets and delivery policy, list schedule state and next run, disable or enable an owned schedule and invoke authorized emergency automation disable. Re-resolve ActorContext and current membership at callback execution, reject replay and stale pages, never issue or reveal automation bearer tokens and reuse canonical history, DND, block, Air, moderation and receipt services.

## Acceptance Criteria
An authorized Telegram actor can trigger the same eligible cue and perform safe quick controls with outcomes matching desktop behavior. Expired, replayed, forwarded or foreign callbacks reveal nothing and execute nothing; Telegram downtime does not affect desktop automation, and no bot action opens microphone capture or bypasses target and policy checks.
