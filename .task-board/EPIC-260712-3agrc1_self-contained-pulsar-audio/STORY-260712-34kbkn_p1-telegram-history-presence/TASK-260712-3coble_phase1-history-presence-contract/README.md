# Freeze Phase 1 history, presence and Telegram action contracts

## Description
Close the authorization, race and UX gaps around history, DND, block and Telegram callbacks before any surface is implemented.

## Scope
Define exact authenticated routes, events and read models for recent history and presence; pagination and 30-day retention visibility; DND local-node versus orbit policy ownership, muted-until clock rules and the invariant that remote controls cannot bypass a stricter local setting; actor and role rules for block. Define opaque integrity-protected Telegram callback state bound to the initiating actor or permitted role, its Bot API size limit, expiry, replay protection and idempotency. Freeze the clip-only attachment matrix and interrupt requires-confirmation flow. Preserve legacy voice by enqueuing default after_current as soon as processing is ready; an inline action may atomically cancel and replace that pending default only before it starts and receives a new coordinator acceptance time, otherwise it returns a stable too-late result.

## Acceptance Criteria
One reviewed contract removes all ambiguity about query authorization, target-detail visibility, DND and block ownership, callback forgery or group-click handling, default-voice races, interrupt confirmation, action lifetime and Phase-2-only errors. No raw IDs or long-lived credentials enter callback data. Legacy no-action voice adds no decision-window delay and cannot be duplicated when a callback races playback.
