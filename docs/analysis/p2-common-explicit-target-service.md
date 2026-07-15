# P2 Common Explicit-Target Service

Task: `TASK-260712-1c34fe`

## Implemented boundary

The existing Phase 1 `CreateResolvedTransmission` transaction remains the
single acceptance boundary for HTTP, history replay, and production Telegram
delivery. Phase 2 extends it instead of adding another queue or ACL model.

- `GET /v1/transmission-targets` lists only the caller's current own/Air
  domain and returns `reference`, `kind`, and a presentation label. It never
  returns actor, orbit, slot, or binding identifiers.
- An explicit `POST /v1/transmissions` audience contains one to 64 objects of
  the form `{"reference":"trf_..."}`. Numeric identity fields are rejected by
  strict JSON decoding.
- A `trf_` value is 256 random bits. SQLite stores only its SHA-256 digest plus
  the issuing ActorContext authorization hash and server-side target binding.
  References expire after 24 hours.
- Resolution rechecks credential scope, current caller domain, target
  existence, and Pulsar binding generation in the same writer transaction as
  transmission acceptance. Forged, copied, expired, outside-domain, and stale
  references all return `404 audience_not_found`.
- Selectors are sorted by opaque reference, expanded, deduplicated by exact
  node identity, and only then filtered by `include_origin`. Caller ordering is
  not a scheduler input.

## Mixed-version policy

One capability policy defines both current clips and the future streamed-track
flow:

| Media/action | Required capabilities |
|---|---|
| clip `after_current` | `media_clip_v1` |
| clip `overlay` | `media_clip_v1`, `overlay_mix_v1` |
| clip `interrupt` | `media_clip_v1`, `interrupt_resume_v1` |
| track `queue` or `replace` | `audio_track_v1`, `queue_replace_v1`, `stream_variant_v1` |

For an explicit audience, any mandatory online target missing a required
capability aborts the whole create with `422 unsupported_targets`. Details
contain only opaque references and ASCII-sorted missing capability names. No
transmission, target, request, inbox, or partial-delivery row is committed.
Offline targets retain the frozen Phase 2 rule: capability is unknown, the
exact binding may be snapshotted for a missed receipt, and it must never gain
late autoplay.

The existing Phase 1 confirmation/downgrade behavior remains available to its
non-explicit audiences. Explicit Phase 2 delivery never silently downgrades or
broadcasts.

## Telegram and rollback behavior

Verified Telegram delivery mints the same credential-bound target references
before calling the common transaction. Production Telegram events no longer
fall back to the legacy session queue when common routing fails. Direct
rollback-era rows without a transport update binding remain readable; if that
old representation cannot encode more than one personal recipient, it refuses
the action instead of changing it to `both`.

## Downstream boundary

This task intentionally does not implement streamed-track persistence/player
runtime or inbox storage. Later tasks consume the capability policy and the
existing immutable transmission targets. Inbox schema/API work must not add a
parallel target resolver, authorization model, Telegram queue, or moderation
store.

Automated coverage proves opaque reference issuance/listing, cross-credential
and stale-binding rejection, exact N-target deduplication, include-origin
filtering, atomic mixed-version rejection, clip/track policy parity, strict
HTTP errors, and the absence of the legacy personal-to-broadcast fallback.
