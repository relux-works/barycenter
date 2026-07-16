# P2 macOS streamed-track candidate player

Task: `TASK-260712-3aj8w2`

## Outcome and production boundary

The macOS client now has a candidate-neutral streamed-track cache and player
seam for deterministic engineering verification. It implements the frozen
`stream_track_v1` lifecycle without selecting a codec or weakening the accepted
codec/player no-go ADR.

Production remains fail-closed. `NodeApp/main.swift` does not construct
`MacStreamCandidatePlayer`, `PlayerCore` does not advertise
`stream_track_v1`, and unadvertised stream commands are still rejected. The
only decoder dependency is `MacStreamCandidateDecoder`, an injected protocol
implemented by tests. A reviewed replacement ADR must select a complete
Windows/macOS codec and container combination before this seam can enter the
production audio graph.

## Authenticated bounded range cache

`MacStreamHTTPRangeFetcher` derives HTTP(S) only from the configured
coordinator WS(S) origin. It accepts a coordinator-relative media path, places
the node credential only in the bearer header, rejects redirects, and sends one
exact `Range` plus strong `If-Range` request. A data delegate cancels the
response at the declared or observed 1 MiB ceiling. Errors expose only bounded
stage/code tokens, never credentials, URLs, response bodies or file paths.

`MacStreamChunkCache` is an actor-owned installation-private cache. Variant and
chunk names are HMAC-SHA-256 values derived from an installation secret;
neither media identity nor URL is persisted. A chunk enters the cache only
after exact length, ETag and SHA-256 verification. Chunk and index writes fsync
their temporary file, use same-directory atomic rename, and fsync the parent
directory. Startup repair removes partial, orphaned, symlinked, malformed,
oversized and tombstoned entries.

The duration-independent ceilings match the frozen handoff:

- 512 MiB globally per installation;
- 64 MiB per variant;
- 128 MiB pinned globally;
- 1 MiB per verified chunk and per network response;
- current plus next chunk as the decoder pin window;
- unpinned least-recently-used eviction and refetch.

A network reset is retried once. A hash mismatch fails closed after one retry;
an ETag change invalidates the variant. Delete or disable writes a durable
HMAC-keyed tombstone, purges cached chunks and denies refill before network
access.

## Decoder, PCM and render boundary

The injected decoder receives trusted manifest metadata, a verified async
chunk reader and a bounded PCM writer. It owns no network client,
authorization token or cache directory. PCM is fixed at 48 kHz stereo
interleaved float32 and uses an exact 1 MiB SPSC ring. Producer backpressure
waits off the render thread rather than dropping samples.

`readPCM` is the future render seam. It only reads the lock-free ring, fixed
atomics and caller-owned memory; it does no allocation, queue synchronization,
lock acquisition, filesystem access, network access or decoding. Local volume
is clamped to 0–100, applies the existing squared gain curve and clips the
float ceiling. A source-level regression check freezes that boundary.

The existing `AudioEngine`, Spotify/librespot path, clip/overlay mixer,
interrupt controller, Airfoil bridge and output-device routing are untouched.
This is the coexistence guarantee available without real app playback and the
mechanism that keeps the candidate out of production composition.

## Lifecycle and generation fences

The player consumes the shared `StreamGenerationGuard`. Load, scheduled
resume, pause, seek, rebuffer recovery, cancel and terminal events require exact
playback, seek, command and event generations. Replacement and seek cancel the
old worker, advance an internal epoch and post a ring cut. Only the render
consumer advances the SPSC tail; even while disarmed it applies a pending cut,
allowing the replacement decoder to reuse bounded capacity without a racing
control-thread tail write. Every PCM write and timer callback checks the epoch,
so late decoder output is discarded.

Ready requires 2,000 ms of PCM. Coordinator-clock timers own ready and
first-sample deadlines. `stream_started` is emitted only after the first sample
is rendered. An underrun disarms output, emits one rebuffer event, waits for a
new readiness threshold and requires a fresh resume command. Decoder EOF alone
is not terminal: `stream_ended(reason=eof_drained)` follows only after the ring
drains. Audible position advances by rendered frames rather than downloaded or
decoded bytes.

## Automated evidence and explicit nonclaims

Deterministic tests cover exact authenticated range headers, the observed and
declared response bound, retry, integrity, cache hit, LRU eviction/refetch,
opaque persistence, durable revocation, readiness, scheduled first sample,
pause/seek/resume, generation cuts, rebuffer/refill, EOF drain, fixed memory
ceilings, production separation and render-source safety:

```sh
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer \
  swift test --package-path node-app --filter MacStreamTrackPlayerTests
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer \
  swift test --package-path node-app --filter streamTrackRenderSourceSafety
```

These tests do not claim a selected decoder, audible production playback,
packaged-app behavior, five-second/three-second p95 timing, one-hour continuity,
200 MiB process RSS, Spotify coexistence in a running app, notarization or
physical hardware behavior. Those observations remain in manual task
`TASK-260712-1fpb9q` under `EPIC-260714-th54l3`. The no-go ADR also keeps
production codec/player acceptance blocked.
