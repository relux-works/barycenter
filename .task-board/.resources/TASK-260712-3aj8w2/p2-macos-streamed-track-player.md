# P2 macOS streamed-track candidate player

- Task: `TASK-260712-3aj8w2`
- Engineering head: `e6f0685f29e6be0dec95b0ea89b7c5463ee1206b`
- Pull request: `#158`
- Merge: `606994898a6e6873c3cc8ed330c82a236bbd3f01`
- Hosted CI: run `29487762262`, 4/4 passed

## Accepted engineering result

The macOS client has a candidate-neutral streamed-track seam. Exact
coordinator-relative requests use bearer authentication, strong `If-Range`, a
single byte range and a hard 1 MiB response ceiling. Verified chunks use
installation-secret HMAC names, same-directory atomic persistence, startup
repair, LRU refill and durable delete/disable tombstones.

Duration-independent limits are 512 MiB globally, 64 MiB per variant, 128 MiB
pinned and 1 MiB per chunk/network read. The injected decoder owns no network,
credential or disk cache and writes 48 kHz stereo float PCM into a fixed 1 MiB
SPSC ring with producer backpressure.

Exact playback/seek epochs fence ready, coordinator-scheduled start, pause,
seek, resume, progress, rebuffer, cancel and drain-before-ended. Only the
render consumer applies ring cuts. The render seam uses the lock-free ring,
preallocated atomics and caller-owned memory; source checks reject queues,
locks, waits, allocation, network, filesystem and decoder calls in that seam.

Production remains fail-closed: no decoder implementation is registered,
`NodeApp/main.swift` does not compose the candidate, and `PlayerCore` does not
advertise `stream_track_v1`. Spotify, clips, overlay, interrupt, Airfoil and
output-device composition are unchanged.

## Automated evidence

- focused macOS candidate tests: 6/6;
- full macOS package: 248 tests in 40 suites;
- release build passed;
- codec/player handoff and stream-track UI validators passed;
- hosted coordinator, node-core, Windows and signed packaged-probe jobs passed.

## Manual boundary

No selected production codec, audible packaged playback, five-second start
p95, three-second seek p95, 200 MiB process RSS, two-hour continuity, Spotify
coexistence in a running app or physical-hardware result is claimed. Those
checks remain in manual `TASK-260712-1fpb9q` under `EPIC-260714-th54l3`.

The full engineering handoff is `docs/analysis/p2-macos-streamed-track-player.md`.
