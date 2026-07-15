# P2 stream variants, authenticated ranges and bounded cache contract

Task: `TASK-260712-dqdoqj`

Contract: `p2-stream-variants-range-cache.v1`

Parent rubric: `p2-codec-spike-rubric.v1`

## Outcome

All decoder candidates now consume the same content-addressed variant manifest,
authenticated single-range transport and installation-private chunk cache. The
contract is frozen in
`acceptance/codec-spike/stream-contract-v1.json`; the executable reference is
`scripts/codec_spike/stream_contract.py`.

This is deliberately a spike substrate. It materializes real SQLite
`stream_variants` rows and exercises the complete wire/cache behavior, but it
does not migrate the production coordinator database or select a decoder. The
production schema and processing pipeline remain owned by the later streamed
track tasks; they must preserve this contract or publish a reviewed versioned
replacement.

## Original upload versus canonical variants

The original upload remains under the media retention, rights, evidence and
deletion policy. It is not a decoder input merely because its container is
known. Candidate probes consume content-addressed canonical variants generated
from the exact frozen fixture lock.

The spike intentionally keeps MP3, AAC-LC in fast-start M4A and ADTS, and Opus
in Ogg available. That avoids choosing a codec before the license, packaging,
platform and comparative evidence tasks. Each variant row records:

- media and variant identities, purpose, codec, container, MIME and rate mode;
- bitrate, decoded sample shape, duration and byte size;
- whole-object SHA-256, strong ETag and `stream/v1/{sha256}` storage key;
- a contiguous SHA-256 chunk manifest and a monotonic chunk-aligned VBR seek
  map;
- staged/ready/revoked state, revision and publish/revoke timestamps.

`materialize_catalog` refuses absolute paths, nested paths, symlinks, missing
files and lock/hash mismatches before it creates the SQLite catalog. The table
has uniqueness and state constraints, and `PRAGMA integrity_check` must pass.

## HTTP range contract

The production route shape is
`/v1/media/{media_id}/variants/{variant_id}`. The repository range harness uses
the equivalent fixture route so every candidate can run without changing the
production service during the spike.

Every GET or HEAD requires both the bearer header and the exact opaque target
snapshot binding. A bearer never appears in a URL. Missing, guessed,
cross-target, unauthorized and production-revoked objects all resolve to the
same 404 surface. The harness-only `revoked` fault profile may return 410 so a
candidate can prove that it treats either terminal response as non-refillable;
that is not the production disclosure contract.

Only one RFC byte range is accepted. Closed, open-ended and suffix ranges,
206/`Content-Range`/`Content-Length`, unsatisfiable 416, strong ETag,
`If-Range`, `If-None-Match`, GET and HEAD are deterministic. An `If-Range`
mismatch returns the full 200 object; the range client rejects that response,
invalidates the old ETag namespace and requires manifest re-resolution rather
than mixing versions.

Successful responses use `private, no-store`, vary on Authorization and the
opaque target header, and carry `nosniff`, the whole-object digest and exact
length. The app cache is explicit; no browser, proxy or shared HTTP cache is an
authority. The request log records method, fixture, profile, range, status and
byte counts but no bearer or target value.

## Integrity and deterministic seek

The maximum chunk and network read are both 1 MiB. A chunk is checked against
its manifest SHA-256 before it becomes cache-visible or reaches a decoder.
Whole-object identity is bound by the strong ETag and storage key. A chunk
failure removes the atomic file; a second failure rejects the candidate.

Seek points are monotonic `{timeMS, offset}` pairs, start at zero, are at most
10 seconds apart and always point to the first byte of a verified chunk. A seek
chooses the greatest point not after the requested time. Container probes may
refine the proportional reference map with parsed MP3 frames, MP4 sample tables
or Ogg pages, but cannot weaken alignment, spacing or integrity.

## App-private cache

The frozen default ceilings are:

| Limit | Bytes |
|---|---:|
| installation cache | 512 MiB |
| one namespace/variant | 64 MiB |
| simultaneously pinned chunks | 128 MiB |
| one chunk / one network read | 1 MiB |

These ceilings are independent of media duration, so a two-hour object cannot
cause a full download or duration-proportional RAM/disk growth. The reference
cache provides:

- HMAC-SHA-256 keys over installation secret, authorization namespace,
  variant, ETag and chunk index; filenames reveal none of those identifiers;
- same-directory temporary writes, file fsync, atomic rename and directory
  fsync;
- SQLite LRU state reconciled against files, sizes and hashes on restart;
- unpinned LRU eviction with both global and per-variant hard bounds;
- concurrent reader pins with a separate hard pin budget;
- corruption removal and orphan/partial-file cleanup;
- namespace or variant invalidation that immediately denies new opens, removes
  unpinned bytes, tombstones pinned bytes and records a durable no-refill ledger
  across restart after delete or actor disable.

An already-open descriptor may finish its bounded chunk read; it grants no new
open and disappears when the final pin closes. That is the portable Windows
and POSIX behavior. Playback cancellation remains the scheduler's separate
responsibility.

## Reproduction

Validate the frozen contract and run the reference tests:

```sh
python3 scripts/codec_spike/stream_contract.py --validate
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/codec_spike/test_codec_spike.py
```

After generating the exact fixture corpus from the parent task, materialize the
candidate input catalog:

```sh
python3 scripts/codec_spike/stream_contract.py \
  --fixtures .temp/codec-fixtures \
  --lock .temp/codec-fixtures.lock.json \
  --database .temp/stream-variants.sqlite3
```

Every decoder candidate receives the same fixture-lock SHA-256, catalog bytes,
variant manifests, target/bearer range behavior and cache limits.

## Evidence and non-claims

Repository tests cover manifest and SQLite materialization, exact ranges and
conditionals, uniform cross-target denial, ETag rotation, chunk integrity,
HMAC namespace separation, bounded eviction, restart reuse, pinned-reader
invalidation, corruption repair and oversize rejection.

This task does not claim actual decoder viability, audible playback, real
hardware timing, licensing suitability, Store/AppContainer acceptance,
production variant generation or final codec selection. Those claims remain
with the ordered candidate, license, comparative evidence and ADR tasks.
