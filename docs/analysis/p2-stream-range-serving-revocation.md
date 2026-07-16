# P2 authenticated stream range serving and revocation

Task: `TASK-260712-3lf8r0`

## Outcome

The coordinator now exposes a private variant resource at
`/v1/media/{media_id}/variants/{variant_id}`. `GET` and `HEAD` support full
responses and one closed, open-ended or suffix byte range with `200`, `206`
and `416` semantics. Responses include the immutable variant's strong ETag,
content length, content range where applicable, SHA-256, MIME and
`Accept-Ranges: bytes`.

Every response is private and non-cacheable (`Cache-Control: private,
no-store`) and varies on the credential and codec target headers. A matching
`If-None-Match` returns `304`; an exact strong `If-Range` continues the range,
while a stale or weak value deliberately returns the complete current object.
The object identity never changes merely to revoke access.

Production streamed-track selection remains disabled by the accepted codec
ADR. The route and storage boundary are exercised with deterministic
candidate-neutral fixtures, but this task does not advertise a codec, enable
`audio_track_v1`, or make a production variant available.

## Authorization and revocation

Stream bytes are node-only. Owner control credentials, target control
credentials, current membership and possession of a media or variant ID are
not grants. A successful open re-resolves the bearer and requires the exact
still-live installation generation in an accepted immutable transmission
target snapshot.

For the production store-backed reader, the target row, current credential
binding, blocks, reporter-local hide, media state, owner state and canonical
ready variant are checked in one immediate transaction. The immutable file
descriptor is acquired inside that transaction, so a delete, moderation
disable or variant revoke cannot commit between authorization and `open(2)`.
An already-open bounded response may finish; every later range or refill is
re-authorized and receives the same `404` as an unknown object.

A plain user report hides the media only from that reporter. It does not
revoke a neighbor's accepted target. Sender deletion, moderator deletion,
variant revocation and disabled owner actor/orbit prevent every future open.
The endpoint intentionally uses uniform not-found responses for foreign,
deleted, disabled, revoked and unknown media/variant identities.

## Storage and abuse bounds

Only immutable `stream/v1/{sha256}` storage identities are accepted. The
service opens a regular file of the persisted exact size, rejects symlinks,
and verifies that the inspected and opened object are the same file. Stream
directories are created with mode `0700`.

Only one byte range is accepted and a partial response is capped at 1 MiB.
Multiple, malformed, unsatisfied or oversized ranges return an empty `416`.
This bounds a single seek/refill and prevents overlapping or tiny-range
amplification from becoming one unmetered request.

Before any `GET` bytes or success headers are emitted, the existing actor and
orbit egress policies reserve the response. The coordinator records actual
bytes and the request outcome, then closes the reservation. Daily actual
egress and request counts survive completion; concurrent reservations return
to zero. Quota admission charges at least 1 MiB per completed response while
retaining the exact actual-byte metric, so sequential one-byte requests cannot
bypass the daily limit. Quota rejection is `429` before bytes. `HEAD`, `304`
and `416` do not consume egress.

## Compatibility

The existing Phase 1 generic clip download resource and its owner/control
path are unchanged. The new route has no schema migration and reads the
additive stream variant, accounting and immutable target rows already
accepted by the preceding P2 tasks.

## Evidence

Deterministic tests cover exact and foreign targets, node/control separation,
reporter-local hide, sender and moderator deletion, owner disable, variant
revocation, descriptor/revoke ordering, symlink refusal, `200`, `206`,
`HEAD`, matching/stale conditionals, suffix/open-ended ranges, `304`, `416`,
the 1 MiB cap, quota-before-bytes and exact egress totals:

```sh
cd coordinator
go test ./internal/store -run TestStreamVariantPersistedTargetAuthorizationAndReporterLocalHide -count=1
go test ./internal/media -run 'TestDownloadService(StreamVariant|RefusesStream)' -count=1
go test ./cmd/duet-coordinator -run TestStreamVariantHTTP -count=1
go test -race ./internal/store -run TestStreamVariantPersistedTargetAuthorizationAndReporterLocalHide -count=1
go test -race ./internal/media -run 'TestDownloadService(StreamVariant|RefusesStream)' -count=1
go test -race ./cmd/duet-coordinator -run TestStreamVariantHTTP -count=1
```

Hands-on seek/resume, audible playback and real network/hardware evidence stay
in the separate manual testing epic and are not claimed here.
