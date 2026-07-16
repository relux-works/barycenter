# P2 streamed-track persistence handoff

Task: `TASK-260712-1n5fks`

## Boundary

The coordinator now has an additive, candidate-neutral persistence model for
long streamed audio. It extends the existing `audio_track` media kind without
changing Phase 1 media rows, legacy WAV rows, Spotify/session snapshots,
transmission targets, inbox or history authority.

The production codec decision remains **no-go** under
`p2-codec-player-adr-handoff.v1`. Candidate codecs and containers may be stored
and selected by an explicit pinned profile in tests and downstream contract
work, but `SelectProductionStreamVariant` fails closed. Enabling production
requires a reviewed replacement ADR plus a schema/repository change; there is
no runtime override.

## Additive tables

- `stream_track_metadata` retains immutable original filename, MIME,
  container, codec, byte size and SHA-256 alongside the generic media row.
- `stream_variants` stores the frozen candidate-neutral profile, codec,
  container, MIME, bitrate/rate mode, decoded sample shape, duration, size,
  strong ETag, content-addressed storage key, whole/chunk hashes and seek map.
- `stream_variant_policy` pins the accepted no-go contract and disables
  production selection.
- `stream_playback_domains` persists target-local main-program resume pointer,
  current streamed source, audible progress and playback/seek generations.
- `stream_queue_items` persists server-generated queue identities and ordering.

The new playback domain is not a target or membership authority. `target_ref`
is a scheduler-owned lookup key; authorization continues to come from the
existing immutable target snapshot and membership models.

## State and concurrency

- Variant payload is immutable; only `staged -> ready|revoked` and
  `ready -> revoked` transitions are accepted.
- Worker publication and revocation use expected revisions. A partial or stale
  worker loses the compare-and-swap and cannot publish over newer state.
- Variant selection is deterministic by `(media_id, profile)` and requires
  `ready` state.
- Seeking uses the greatest map point not after the requested time and returns
  its verified, chunk-aligned byte range.
- Queue, activation, audible progress and seeking use expected revisions.
  Playback generation rejects output from an older track; seek generation
  rejects output produced before the latest seek.
- Audible progress is monotonic within a seek generation. A seek creates a new
  generation and may move the position backward.

## Rollout and rollback

Startup creates the model after generic media ingest and before transmission.
No existing column is changed. The exact previous-coordinator fixture opens a
database containing ready variants and active playback state, mutates the
legacy media/session/orbit APIs, then rolls forward and verifies that metadata,
variant, queue, main-program pointer, audible position and generations survived.

## Downstream use

- `TASK-260712-31rkpe` can project pinned ready variant identity and generation
  into the wire contract without exposing a decoder choice or credentials.
- `TASK-260712-285pag` can use staged/publication CAS and immutable manifests;
  it must retain the no-go production policy.
- `TASK-260712-2h6snp` can restore queue and audible state and must discard late
  output when either playback or seek generation differs.
- Range serving and players must re-authorize against existing target
  snapshots on every request; these tables do not grant access.
