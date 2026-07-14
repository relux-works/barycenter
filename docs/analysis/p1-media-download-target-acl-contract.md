# Phase 1 media download target ACL contract

Date: 2026-07-14. Task: `TASK-260712-3mcof4`.

This note freezes the independently deployable authorization boundary for
canonical generic-media reads. Transmission persistence is intentionally owned
by `TASK-260712-1aprcb`; this task supplies the fail-closed reader interface it
must implement and does not create a second transmission or membership model.

## Generic HTTP boundary

`GET /v1/media/{media_id}` accepts one live bearer credential in the
`Authorization` header. A long-lived credential is never accepted in the URL,
query string, filename or redirect. Query strings, request bodies, malformed
IDs and non-GET/non-DELETE methods are rejected before storage is opened.

There are exactly two generic read cases:

1. a live control credential may read ready, unexpired media owned by its own
   orbit; or
2. a live node credential may read media only when an immutable accepted
   transmission target snapshot names the exact `(media_id, orbit_id,
   actor_id, slot)` installation identity.

A control credential is never treated as a target node even though the current
identity context also carries node capability for older control surfaces. A
node credential does not gain owner access merely by belonging to the owning
orbit: include-origin playback must be an explicit target snapshot too.
Including `actor_id` prevents a later occupant of a recycled orbit slot from
inheriting an old grant, while ordinary token rotation for the same actor and
slot does not rewrite accepted history.

## Immutable snapshot seam

`MediaTargetSnapshotReader.AllowsMediaDownload` is the only generic node-read
extension point. Its production implementation must query accepted persisted
transmission target rows. It must not derive a grant from:

- a current or former approach;
- current Air or orbit membership;
- presence, DND, history visibility or inbox visibility;
- a copied media ID, URL, hash or legacy WAV mapping; or
- a control credential belonging to a targeted orbit.

`TASK-260712-1aprcb` installs the store as the production reader. It queries
accepted persisted target rows and verifies the exact current actor, slot and
snapshotted binding generation. `TASK-260712-gj0cko` exercises the complete
reader-to-delete-to-cancellation path. Neither integration introduces a
live-membership fallback; no target row remains fail-closed while owning
control reads remain available.

Target-reader errors are reflected only as a generic internal failure and are
logged without media, actor, slot, token or local-path fields. Implementations
must make negative lookups content-free and must not log guessed media IDs.

## Live-state and non-disclosure rules

Authorization re-resolves the presented credential and requires the exact
middleware actor, orbit, role, slot and capability context. It then requires:

- an active owning orbit;
- a non-revoked media-creating actor;
- `ready` media with a non-empty generated storage key; and
- `expires_at` strictly later than the authorization time.

The service repeats the live credential, persisted target, active-block and
media-state checks under the shared canonical-storage lock. It opens and
verifies the file descriptor inside that second immediate SQLite transaction,
before the transaction releases its writer reservation. Therefore a delete,
block, terminal blocked receipt or actor revocation commits either before
authorization (and denies the read) or after the descriptor is already open;
there is no post-authorization/pre-open revocation window. A request holding an
open descriptor may finish; revocation denies every later authorization and
cleanup can unlink after the descriptor is acquired.

For a valid credential, unknown, guessed, foreign, unsnapshotted, deleted,
expired, owner-disabled and copied-URL media all return the same
`404 media_not_found` envelope. Invalid or revoked credentials return the same
credential-level `401` regardless of media ID; a live credential without the
required capability returns `403`. None of these responses contains a title,
filename, owner, target, storage key or local path.

## Canonical byte handling

The storage key must match the generated `media/v1/<64 lowercase hex>` form.
The server uses `Lstat`, requires a regular entry, opens it under the shared
publication/cleanup lock, compares the opened inode with the inspected entry
and verifies the persisted byte length. Symlinks, replaced entries, missing
bytes and metadata mismatches fail closed. Responses set the persisted MIME,
`nosniff`, a content-hash ETag and `Cache-Control: no-store`; the original title
or upload filename is never used as a server path or response filename.

## Legacy compatibility boundary

`GET /media/{legacy_id}.wav` remains a separate rollout bridge backed by the
legacy `media` table. It accepts an actual node credential, not a control
credential. Its documented owner-or-current-active-approach policy remains only
for legacy `play_voice` compatibility; it does not authorize `/v1/media` and a
legacy ID or mapping cannot be promoted into a generic grant. Once mixed-rollout
support is retired, removing that live-approach exception requires its own
versioned compatibility decision rather than silently broadening the generic
ACL.
