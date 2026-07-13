# Build render-safe prepared-clip state on both nodes

## Description
Replace implicit one-shot voice ownership with an explicit, race-safe prepared-media state machine shared in semantics across Windows and macOS.

## Scope
Implement prepared, armed, playing, cancelling, terminal and failed clip states plus generation IDs so reordered control messages and stale timers cannot affect a newer clip. Move fetch, hash, decode or open, file cleanup and timer creation entirely to control or worker paths. Hand render callbacks preallocated immutable or atomically swapped state without blocking locks, allocation or I/O. Define common duck, overlay, limiter, interrupt and telemetry parameters, protect a playing clip from replacement by a queued clip, and retain a separately tested legacy play_voice route.

## Acceptance Criteria
Both platforms expose one documented mixer-control contract consumed by protocol hooks and platform engines. Duplicate, reordered and cancelled generations cannot start or resume. Render callbacks read only bounded prebuilt state and contain no file or network I/O, allocation, waits or blocking mutex acquisition. Legacy voice remains operational and does not share unsafe mutable ownership with the new path.
