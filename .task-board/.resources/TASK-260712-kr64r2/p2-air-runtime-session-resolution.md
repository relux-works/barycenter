# Phase 2 Air runtime and session-resolution handoff

- Date: 2026-07-15
- Task: `TASK-260712-kr64r2`
- Persistence predecessor: [`p2-air-schema-link-migration`](p2-air-schema-link-migration.md)

## Sole runtime owner

When `air_authority.mode = 'airs_authoritative'`, the coordinator resolves a
shared session only through `air_active_pointers` and keys the in-memory and
persisted FSM by the stable public `air_id`. Legacy negative link IDs are not
used as Air runtime, timer or voice-order keys. In `rollback_hold`, no shared
Air or link runtime is warmed. Legacy link writes are rejected while Air or
hold authority is active, preventing an alias path from resurrecting a second
delivery owner before the dedicated alias task lands.

The exact peer union is the set of joined memberships that also have a current
pointer to that same Air. Saved membership in another Air contributes no peer,
so A+B+C cannot acquire D through a transitive saved-room chain. Each physical
Pulsar is represented once as `orbit_id:slot` at the internal FSM boundary and
is converted back to one wire `NodeKey` for effects.

## Restore and lifecycle rules

Startup restores personal metadata, then instantiates only Airs whose durable
state is active with at least two current member pointers. Saved and parked Air
snapshots remain lazy. Air snapshots use `session_state_air_<public-id>` and a
playing/loading/armed/voice snapshot restores paused under the same stable Air
ID.

Every member/pointer transition advances the Air runtime revision. Delayed
ready timers, playlist/metadata/provider completions and asynchronous Telegram
voice ordering carry the dispatch-time runtime owner. Air work includes both
authority generation and Air revision, so stale work cannot enter a
replacement session. Media lifecycle cancellation visits and persists live Air
sessions as well as personal and legacy-link sessions.
Joining current members are added without changing the remaining program:

- an online joiner may load only the current main track at the estimated live
  position;
- an old voice/overlay is never replayed midstream;
- offline members do not join the current barriers and do not block playback.

On leave/deactivation the coordinator stops and removes only that orbit's
nodes, cancels their FSM participation, and immediately routes the caller to
its personal orbit. Remaining current members keep the same Air FSM and FIFO.
When fewer than two current members remain, the Air is parked, its shared
snapshot is persisted, all remaining shared nodes stop, and personal sessions
become authoritative again.

## Automated evidence

Store and loop tests cover exact active-member resolution, stable Air snapshot
restore, lazy parked rooms, no transitive membership, restart with queue,
Air-ID-scoped cross-barycenter voice ordering, main-track-only join catch-up,
caller-only leave, below-two parking, stale async completion rejection, Air
media cancellation, legacy-write rejection and fail-closed `rollback_hold`
warmup. Existing approach tests continue to cover the
legacy-authoritative compatibility branch until the later alias task maps it
onto the Air control plane.

The next control-plane task owns HTTP lifecycle endpoints and explicit runtime
refresh calls after each accepted mutation. Generic transmission schema
evolution to explicit P2 target/track domains remains with its scheduled P2
stories; this task establishes the Air-owned main program and legacy
Telegram/track order without claiming those later APIs.
