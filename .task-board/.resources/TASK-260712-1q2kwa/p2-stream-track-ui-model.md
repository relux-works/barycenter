# Phase 2 streamed-track UI and control model

- Date: 2026-07-16
- Task: `TASK-260712-1q2kwa`
- Contract: `pulsar.stream-track-ui-model.v1`
- Localization authority: `coordinator/internal/presentation`
- macOS model: `node-app/Sources/NodeAppUI/PulsarStreamTrackModel.swift`
- Windows model: `pulsar-win/stream_track_model.go`

This task supplies one transport-neutral long-track state and command boundary
for the later native macOS and Windows views. It does not enable a production
decoder, implement those views, or claim evidence from real hardware.

## One localized state machine

The coordinator owns stable English and Russian labels for every draft phase,
playback phase, action and bounded failure code. Clients keep the complete
`{key,en,ru}` value and select only the requested language. A label whose key
does not match its typed phase or failure is discarded; arbitrary server text
never becomes failure copy.

The Swift `@MainActor @Observable` owner exposes one `Equatable` snapshot. The
Go owner exposes the same snapshot behind a read/write lock and returns deep
copies. Neither platform view needs duplicate draft, selection or playback
state. Concrete native views remain in `TASK-260712-3lximx` and
`TASK-260712-2psvhu`.

## Honest draft and progress

The candidate intake bounds are 500 MiB, two hours, 64 explicit targets, 512
UTF-8 title bytes and 2,000 report-detail bytes. The model keeps three separate
signals:

- upload progress is a byte offset over the immutable local file size;
- processing progress is a server-owned percentage; and
- playback progress is the audible position over server-confirmed duration.

No signal substitutes for another. In particular, a client MIME type, a
complete local upload or 100% processing cannot manufacture `ready`. Ready
requires server-confirmed metadata, a media ID and a playable variant manifest.

An unsent draft with retained local bytes survives `stale`, `offline`,
`coordinator_error` and a replacement projection that omits the draft. Delete
requires explicit confirmation, but the optimistic reducer still keeps the
local bytes until the coordinator confirms persisted deletion.
That confirmation must echo the exact opaque local draft ID; an omitted draft
alone is not treated as deletion evidence.

## Capability and generation fences

Every mutation is built only from an exact action in the latest `ready`
snapshot. Upload and queue/replace require current content-policy consent.
Explicit delivery requires the exact current opaque target references and the
`stream_track` capability on every selected recipient; current-Air delivery
requires a current Air option. Quota, moderation and final authorization remain
server-owned, so the client can show bounded `quota_exceeded`,
`unsupported_targets` or policy failures without guessing success.

Pause, seek and resume bind to the exact opaque stream ID and playback
generation. Seek also binds to the current seek generation. Older playback or
seek replacements are discarded, same-generation audible progress cannot move
backward, and an optimistic seek increments the seek generation. On Windows,
the final capability check and optimistic transition happen under the same
lock.

Opaque local, media, manifest, stream and target identifiers are redacted from
custom descriptions on both platforms.

## Automated evidence

- `scripts/validate_pulsar_stream_track_ui_model.py` freezes limits, enums,
  authority, durability, progress and deferred-view boundaries.
- coordinator tests compare every RU/EN label with the portable contract and
  include the expanded presentation golden digest.
- `PulsarStreamTrackModelTests` and `stream_track_model_test.go` consume the
  same contract and cover outage retention, server-owned readiness, target and
  policy gates, generation fencing, non-destructive delete and redaction.
- full coordinator and Windows suites, race tests, vet, macOS UI target builds
  and the hosted platform matrix remain required before acceptance.
