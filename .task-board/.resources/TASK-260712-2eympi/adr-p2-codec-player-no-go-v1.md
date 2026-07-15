# ADR: P2 codec/player production no-go and candidate-neutral handoff

- Status: accepted no-go
- Date: 2026-07-15
- Task: `TASK-260712-2eympi`
- Contract: `acceptance/codec-spike/player-handoff-v1.json`
- Comparative evidence: `acceptance/codec-spike/comparative-matrix-v1.json`

## Decision

No codec/player combination is selected for production. Phase 2 production
media generation, playback, Store submission and decoder fallback remain
blocked. The production decoder registry is empty.

This is the only result permitted by the frozen selection rule: one complete
Windows plus macOS combination must pass every required format, hard gate and
Windows-Windows, Windows-macOS and macOS-macOS pairing. The comparative matrix
contains no such combination. A format or platform failure and a missing
measurement are both non-pass outcomes; neither can be averaged away.

Candidate-neutral downstream engineering may proceed. It is limited to schema,
authenticated range transport, integrity, bounded cache, scheduler, adapter
interfaces and deterministic test doubles. It cannot register a production
decoder or emit a production-ready variant until a reviewed later ADR replaces
this no-go.

## Rejected combinations

`bundled-ffmpeg-8.1.2-v1` on both platforms is the only probe that decodes and
seeks all six smoke fixtures on hosted Windows amd64 and macOS ARM64. It is
still rejected because the probe consumes prepared local files rather than the
end-to-end authenticated range/cache path, lacks the 30-sample/two-hour pairing
matrix, lacks Windows ARM64 native decode evidence, and has no production
signing, notarization, isolation, release SBOM/advisory or counsel closure.

The Media Foundation plus native macOS combination is rejected because Windows
returns `0xC00D36C4` for Ogg/Opus and macOS requests at least the complete source
before first PCM. A server-canonical AAC-only concept is not selected: it would
be a new exact candidate and still needs a streaming macOS implementation plus
the complete matrix.

The pure-Go combination is rejected because its audited GPL-2.0-only AAC module
cannot enter the approved static proprietary distribution, MP3 random seek
requires a full source scan, and the Ogg reader exposes no random-seek API.

## Frozen implementation seam

The machine-readable handoff copies and validates the authoritative values from
the range/cache contract and rubric. Downstream code must consume them without
inventing a codec choice:

- `stream_variants` remains content-addressed and immutable after publish. The
  original upload is never automatically a decoder input. Candidate codec and
  container enums exist for schema and test fixtures only; production variant
  selection is disabled.
- GET/HEAD use the exact authenticated single-range route. Authorization and the
  immutable target snapshot are checked on every request; credentials never
  enter URLs. Missing, unauthorized and revoked objects retain the uniform 404
  surface.
- A verified chunk is at most 1 MiB. SHA-256 chunk and whole-object integrity is
  mandatory before decode; mixed ETags invalidate the namespace. Seek maps are
  monotonic, chunk-aligned, at most 10 seconds apart and select the greatest
  point not after the requested time.
- The installation cache remains 512 MiB globally, 64 MiB per variant and 128
  MiB pinned, with HMAC-derived opaque keys, atomic persistence, restart repair,
  LRU eviction and durable no-refill after revocation.
- A decoder adapter owns no network, authorization or disk cache. It writes
  48 kHz stereo interleaved float PCM to a bounded 1 MiB ring from a worker.
  The render callback only reads and never calls the decoder, allocates, waits
  or takes a blocking lock.
- Prepare, ready, arm, pause, new-generation seek, resume, drain and cancel are
  the stable operations. Late output from an old generation is discarded.
  Scheduling uses coordinator monotonic milliseconds and preserves 5,000 ms
  start, 3,000 ms seek-to-audio and 100 ms skew p95 gates.

The six checked-in smoke fixture hashes, all eight range fault profiles, three
pairings, three warmups plus 30 measured samples, long-duration RSS gates and
the full release-obligation list are frozen in the handoff contract. Downstream
tests must reuse them.

## Downstream consequences

`STORY-260712-2ori1t` may implement `stream_variants`, the wire protocol,
storage/egress accounting, range/revocation, cache, coordinator state machines,
UI models and fake decoder adapters. Any code path that marks a variant ready
for a production decoder must fail closed while the registry is empty.

`STORY-260712-1qfbiw` may build observability and acceptance collectors against
the same contract. Production codec/player acceptance stays blocked and real
packaged pairing evidence remains in `EPIC-260714-th54l3`.

No downstream task may silently choose bundled FFmpeg, native frameworks, pure
Go, a new codec, runtime download or sandbox exception. Such a change reopens
the spike and requires a versioned matrix plus replacement ADR.

## Reopening the decision

A replacement ADR needs one exact Windows/macOS combination that decodes every
required format and passes every hard gate and pairing. It must also close the
exact source/build receipt, runtime SBOM, vulnerability, corresponding-source,
notice, all-architecture signing, macOS notarization, sandbox/no-download and
AAC counsel gates. The new matrix must preserve raw failure evidence and keep
score averaging forbidden.

## Verification

```sh
python3 scripts/codec_spike/validate_comparative_matrix.py
python3 scripts/codec_spike/validate_player_handoff.py
python3 -m unittest scripts/codec_spike/test_codec_spike.py
```
