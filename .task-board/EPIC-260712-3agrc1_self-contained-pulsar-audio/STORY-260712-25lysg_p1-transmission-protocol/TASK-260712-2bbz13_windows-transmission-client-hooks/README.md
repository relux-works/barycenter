# Add Windows prepare and scheduled-play client hooks

## Description
Teach the Windows node to implement the frozen clip lifecycle while keeping all audio mixing inside the mixer-owned interfaces.

## Scope
Advertise exact capabilities; on prepare, fetch with authenticated media access, verify hash and expiry, decode or open without starting playback, validate duration and report ready or a typed failure. Translate t_coord_ms through the existing synchronized coordinator clock, arm playback without blocking the hub or audio callback, reject stale scheduled starts, and emit started, ended, failed and cancelled exactly once. Route overlay and interrupt to mixer interfaces, persist local DND state and presence without mic details, clean caches after terminal state, and preserve play_voice and solo_voice.

## Acceptance Criteria
Windows never reports ready before bytes and decoder are usable, never starts a stale or cancelled transmission, and schedules against coordinator time. Duplicate or reordered prepare, play and cancel messages are idempotent. Hash, expiry, auth and decode failures map to the frozen enums. New and legacy paths pass side by side without logging secrets, local paths or audio content.
