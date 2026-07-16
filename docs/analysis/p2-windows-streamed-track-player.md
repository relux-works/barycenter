# P2 Windows streamed-track candidate player

Task: `TASK-260712-17w78q`

## Outcome and production boundary

The Windows client now has a candidate-neutral streamed-track cache and player
seam for deterministic engineering tests. It implements the frozen
`stream_track_v1` lifecycle without selecting a codec or weakening the
accepted codec/player ADR.

Production remains fail-closed. `main.go` does not advertise
`CapabilityStreamTrack`, the production `Player` still rejects unadvertised
stream commands, and no decoder implementation is registered. The only
decoder dependency is an injected test-target interface. A reviewed
replacement ADR must select and qualify a complete codec/container combination
before this seam may be composed into the packaged application.

## Authenticated bounded cache

`WindowsStreamHTTPRangeFetcher` derives an HTTP(S) origin only from the
configured coordinator WS(S) URL. It accepts coordinator-relative media paths,
places the node credential only in the bearer header, refuses redirects and
sends one exact `Range` plus strong `If-Range` request. Every response is
limited to 1 MiB. Authentication failures, revocation, network failure,
invalid ranges and ETag changes are returned as bounded tokens without URLs,
credentials or response bodies.

`WindowsStreamChunkCache` stores only verified immutable chunks beneath the
installation-private `stream-v1` directory. Variant and chunk filenames use
HMAC-SHA-256 over the installation secret, manifest identity, ETag and chunk
index; the persisted index contains no media ID, URL or bearer credential.
Chunk and index writes use a same-directory `.part`, file sync and rename.
Restart repair deletes partial, orphaned, symlinked, malformed, oversized and
tombstoned entries.

The frozen ceilings are enforced independently of track duration:

- 512 MiB per installation;
- 64 MiB per variant;
- 128 MiB pinned globally;
- 1 MiB per verified chunk and network read;
- unpinned least-recently-used eviction, with the current and next chunks as
  the playback pin window.

A network reset is retried once. A chunk hash mismatch deletes the candidate,
retries once and then fails closed. An ETag change invalidates the entire
variant so the coordinator must resolve a fresh manifest. Delete or disable
creates a durable HMAC-keyed tombstone, purges all local chunks and denies
refill before network access. Decoder completion additionally checks the
whole-object SHA-256 before publishing EOF.

The candidate seam intentionally rejects terminal whole-object verification
when required chunks cannot coexist under the 64 MiB per-variant ceiling.
That limitation is evidence for the current production no-go, not permission
to raise the cache bound or claim long-track readiness. A future selected
decoder design must provide a reviewed bounded whole-object proof strategy.

## Decoder, PCM and realtime boundary

The decoder adapter receives trusted manifest metadata, a verified chunk
reader, a generation and a bounded PCM writer. It owns no network client,
authorization token or disk cache. Worker-side backpressure writes 48 kHz,
stereo, interleaved float32 PCM into an exact 1 MiB single-producer,
single-consumer ring.

`ReadPCM` is the candidate render seam. It calls only the lock-free ring,
atomics and bounded arithmetic; it performs no allocation, mutex acquisition,
filesystem access, network access or decode. Local volume is clamped to
0–100, applies the existing squared gain curve and clamps output samples to
the float ceiling. Spotify, clips, overlay, interrupt and output-device code
are untouched.

## Lifecycle and generation fences

The player shares the accepted `StreamGenerationGuard` contract. Load,
scheduled resume, pause, seek, rebuffer recovery, cancel and terminal events
are checked against exact playback, seek and command generations. Replacing or
seeking cancels the old worker, increments an internal epoch and posts a ring
cut that only the render consumer applies. Decoder workers are serialized and
all PCM writes and timer callbacks recheck the epoch, so late output cannot
cross a generation boundary.

Ready requires at least 2,000 ms of PCM. Load readiness and first-audible
sample have coordinator-clock deadlines; an unsynchronized or expired clock
fails closed. Rebuffering disarms output, emits one event, waits for a new
2,000 ms threshold and requires a fresh resume. `stream_started` is emitted
only after the first rendered sample. Decoder EOF alone is not terminal:
`stream_ended(reason=eof_drained)` is emitted only after the ring reaches zero.
Audible progress advances only by rendered frames, never by downloaded or
decoded bytes.

## Automated evidence and explicit nonclaims

Deterministic tests cover authenticated exact ranges, no credential-bearing
cross-origin URL, integrity and network retry, ETag invalidation, cache hit,
LRU eviction/refill, restart repair, whole-object mismatch, durable revocation,
opaque index persistence, buffer-threshold readiness, scheduled first sample,
pause/seek/resume, stale-worker fencing, rebuffer/refill, drained completion,
clock and deadline failures, fixed duration-independent memory ceilings,
sanitized failures and the production no-go source boundary:

```sh
cd pulsar-win
go test ./... -run TestWindowsStream -count=1
go test -race ./... -run TestWindowsStream -count=1
go vet ./...
go test ./... -count=1
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build ./...
```

These checks do not claim a selected decoder, real audio, packaged playback,
one-hour/two-hour continuity, p95 start or seek latency, RSS on Windows,
Spotify coexistence in a running app or physical-device behavior. Those
observations remain in `EPIC-260714-th54l3`; production codec/player acceptance
also remains blocked by the no-go ADR.
