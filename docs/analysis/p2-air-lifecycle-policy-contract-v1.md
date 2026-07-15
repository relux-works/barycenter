# Phase 2 Air lifecycle and policy contract v1

- Date: 2026-07-15
- Task: `TASK-260712-17yizc`
- Story: `STORY-260712-3v14m9`
- Contract: `pulsar.air-lifecycle-policy.v1`
- Executable summary: [`protocol/air-lifecycle-policy-v1.json`](../../protocol/air-lifecycle-policy-v1.json)

This document is normative for the Phase 2 Air schema, migration, coordinator
runtime, HTTP API, Pulsar clients and Telegram aliases. It resolves the logical
Air outline in `docs/spec-self-contained-audio.md` before those components are
implemented. `MUST`, `MUST NOT`, `SHOULD` and `MAY` have their RFC 2119
meanings.

This is an engineering contract. Physical-device and listening evidence stays
in manual epic `EPIC-260714-th54l3`. Explicit target selection, target ACL
details, pagination and offline inbox behavior remain owned by
`STORY-260712-ob1tx2`; this contract only states the lifecycle boundary they
must observe.

## 1. Representation and state

### 1.1 IDs, JSON and concurrency

- Public IDs are coordinator-generated: `air_`, `aim_` (Air membership) and
  `ai_` (Air invite), each followed by a 26-character Crockford ULID. Raw
  database integers are never API identifiers.
- Authenticated requests use the existing control-token or Telegram
  `ActorContext`. Identity, role, orbit and current-Air fields supplied by a
  caller are forbidden authority inputs.
- Mutation requests use strict JSON, reject query parameters and require an
  `Idempotency-Key`. An exact retry returns the original result. Reuse with a
  different canonical body returns `409 idempotency_conflict`.
- Mutable resources expose a positive integer `revision`. The caller supplies
  the observed revision; a stale value returns `409 revision_conflict` and
  performs no partial work.
- Air, membership, invite and each affected barycenter active pointer are
  locked and changed in one transaction. Capacity and authority are rechecked
  inside that transaction. Simultaneous claims cannot overfill an Air, create
  duplicate membership or leave two active pointers.
- Timestamps are RFC 3339 UTC with millisecond precision in HTTP and Unix
  milliseconds in storage. Optional fields are omitted, never `null`.

### 1.2 Status vocabulary

Air status:

| Status | Meaning |
| --- | --- |
| `parked` | Durable room with fewer than two joined barycenters currently pointing to it; it owns no live playback |
| `active` | At least two joined barycenters point to it and one Air runtime owns their shared main/overlay controllers |
| `dissolved` | Terminal tombstone; membership cannot be restored and the ID is never reused |

Membership status:

| Status | Meaning |
| --- | --- |
| `pending_confirmation` | An invite was consumed, but the joining barycenter primary has not consented |
| `joined` | Saved membership; it does not by itself join live playback |
| `left` | Terminal membership generation; a later invitation creates a new membership ID |

Invite status is `open | consumed | expired | withdrawn`. Expiry is evaluated
from coordinator time even before a cleanup job materializes `expired`.

`Air.status` is coordinator-derived and cannot be set by an API caller. It is
`active` exactly when at least two `joined` member barycenters have
`active_air_id` equal to that Air. Otherwise a non-dissolved Air is `parked`.
Phase 2 has no empty/single-member Air GC: parked rows, policy, queue and the
audible-position snapshot remain durable until explicit dissolve.

### 1.3 Saved membership and active pointer

A barycenter MAY have many saved `joined` Air memberships but has exactly zero
or one nullable `active_air_id`. Playback routing consults that pointer, not
membership existence. A saved Air therefore remains visible and selectable
without receiving presence, main-program or overlay traffic.

Activation is an atomic compare-and-switch. The caller supplies
`expected_active_air_id` as the ID it observed, or the string `none`; the
coordinator verifies it, parks/detaches the prior runtime side, changes the
pointer, and joins/starts the destination runtime in one serialized command.
A mismatch is `409 active_air_changed`. Switching never removes either saved
membership.

Deactivation is the same compare-and-switch to `none`: it clears only the
pointer and retains saved membership. It is distinct from leave. A current
primary or companion may activate, switch or deactivate for its barycenter;
satellites may not.

If the old Air drops below two active member barycenters it parks: all
remaining Air audio fades/stops, timers are cancelled, and current main
position plus queue are persisted. A later second activation resumes through
the ordinary prepare/buffer barrier from that saved position; it never resumes
an overlay. A parked Air emits no scheduler, playback, presence-union or
catch-up effects.

## 2. Roles and default policy

Air governance is assigned to member barycenters:

- the creator barycenter is `owner` and `admin`;
- another member may be `admin` or `member` as fixed by the invite; and
- an action also requires an eligible actor role inside the acting
  barycenter. Revoked actors and `satellite` actors cannot perform Air control
  actions.

Only a current primary of the owner barycenter may replace policy, grant or
remove Air admin status, transfer ownership, or dissolve. The owner primary
cannot leave until ownership is transferred or the Air is dissolved. An Air
admin primary may issue/withdraw invites under policy and perform ordinary
playback actions; admin status does not imply policy/dissolve authority.

Policy revision starts at `1` with these defaults:

| Operation | Default | Meaning |
| --- | --- | --- |
| invite | `air_admin_primary` | current primary of owner/admin barycenters |
| overlay | `primary_companion` | current primary or companion in any active member barycenter |
| queue | `primary_companion` | current primary or companion in any active member barycenter |
| replace | `air_admin_primary` | current primary of owner/admin barycenters |

Allowed values are frozen in the executable summary. Local DND, block, node
capability, volume ceiling, media ownership and target ACL remain stronger
than Air permission. No policy has a bypass, emergency, force or remote-volume
field.

Policy replacement requires the observed `policy_revision`, records old/new
values and actor/orbit in audit, then increments the revision. A transmission
stores the accepted policy revision and authorization result. Later policy or
membership changes MUST NOT expand, remove, reorder or reauthorize its target
snapshot. Restricting policy affects only work accepted after the commit;
already accepted work is stopped only by an explicit existing cancellation
rule such as leave, dissolve, delete, block or DND. Open invites keep their
issued authority until expiry or explicit withdrawal.

## 3. Secure invitation and confirmation

### 3.1 Invite properties

An invite secret contains 256 random bits and is rendered once as an opaque
base64url code/deep link. The coordinator stores only a versioned keyed
HMAC-SHA-256, never plaintext. The raw code is forbidden from logs, audit,
metrics, URLs after the initial deep link, list/read responses and Telegram
callback data.

- fixed TTL: 15 minutes;
- single successful consume;
- bound to Air ID, issuer actor/orbit, intended Air role and policy revision;
- maximum 10 issues per actor per Air per rolling hour; and
- after 5 failed consumes per actor or source IP in one minute, further
  attempts are rate-limited without distinguishing unknown, expired, consumed
  or withdrawn codes.

All unavailable codes return the same `404 invite_unavailable`. Issue and
consume audit contains opaque invite ID, not code/hash. Creating an invite
reserves no capacity; capacity is checked both on consume and confirmation.

### 3.2 Join sequence

1. An authorized member issues an invite.
2. An authenticated actor consumes it. The actor MUST belong to a different
   barycenter. The transaction burns the invite and creates exactly one
   `pending_confirmation` membership for that barycenter.
3. The API returns a redacted preview: Air title, owner display name, intended
   Air role, policy summary, member count/capacity, and whether activation
   would switch away from another Air. It returns no member identities or
   playback metadata beyond what the invite grants.
4. A **current primary of the joining barycenter** calls confirm. Consuming a
   code, being the inviter, or being a companion is never confirmation.
5. Confirmation changes membership to `joined`. `activate` is an explicit
   required Boolean. `false` saves the Air only. `true` also performs the
   compare-and-switch from §1.3.

Inviter consent is represented by authorized invite issuance. There is no
second inviter confirmation. Decline marks the pending membership `left`; an
inviter or current Air-admin primary may also cancel a pending claim, while an
inviter may withdraw an `open` invite. These actions are idempotent. A primary
change after consume is safe because confirmation resolves the joining
barycenter's current primary at transaction time.

At most 8 joined or pending barycenters may occupy one Air. At most 20 Pulsars
may participate online in an active Air. Confirmation/activation that would
immediately exceed either limit returns respectively
`409 air_barycenter_capacity_reached` or
`409 air_online_pulsar_capacity_reached`. A Pulsar that comes online after the
20 leases are occupied remains connected to its personal barycenter but is
`capacity_deferred` for Air playback; it does not evict an admitted Pulsar and
joins when a lease is released.

## 4. HTTP application contract

Every route uses control-token auth, the common error envelope and audit rules
above. Read projections include only Airs in which the actor's barycenter has
`pending_confirmation` or `joined` membership. Foreign and guessed IDs
collapse to `404 air_not_found`.

### 4.1 Create, list and read

`POST /v1/airs`

```json
{"title":"Family air"}
```

Requires the actor to be the current primary of its barycenter. It creates a
parked Air, owner/admin `joined` membership and policy revision `1`; it does
not change `active_air_id`.

`GET /v1/airs` returns the stable current/saved read model:

```json
{
  "current_air_id":"air_01J00000000000000000000000",
  "active_pointer_revision":7,
  "saved":[{
    "air_id":"air_01J00000000000000000000000",
    "title":"Family air",
    "status":"active",
    "membership_status":"joined",
    "air_role":"owner",
    "member_count":3,
    "active_member_count":3,
    "online_pulsar_count":5,
    "capacity":{"barycenters":8,"online_pulsars":20},
    "policy_revision":2,
    "is_current":true
  }]
}
```

When there is no current Air, `current_air_id` is omitted. `saved` is sorted by
case-folded title then Air ID. `GET /v1/airs/{air_id}` adds the authorized
member-role projection, policy values and pending action for the caller; it
does not expose target/inbox or historical media detail.

### 4.2 Invite and join routes

`POST /v1/airs/{air_id}/invites`

```json
{"air_role":"member"}
```

Response `201` includes `invite_id`, `expires_at` and the raw `code` exactly
once. `air_role` is `member | admin`; only the owner primary may issue an admin
invite.

`POST /v1/air-invites/consume`

```json
{"code":"<opaque 256-bit invite>"}
```

Response `202` returns the redacted preview and `membership_revision`.

`POST /v1/airs/{air_id}/join/confirm`

```json
{
  "membership_revision":1,
  "activate":true,
  "expected_active_air_id":"none"
}
```

`expected_active_air_id` is required only when `activate=true`. Response
returns the updated Air projection. `POST .../join/decline` contains
`membership_revision`. `POST .../invites/{invite_id}/withdraw` contains the
invite revision.

### 4.3 Activate, leave, policy and dissolve

`POST /v1/airs/{air_id}/activate`

```json
{"membership_revision":3,"expected_active_air_id":"air_01JOLD00000000000000000000"}
```

The caller needs `joined` membership; any current primary or companion may
activate/switch for its barycenter. Satellite is denied. The command returns
only after the serialized runtime accepted the new authority generation.

`POST /v1/airs/{air_id}/deactivate`

```json
{"membership_revision":3,"expected_active_air_id":"air_01J00000000000000000000000"}
```

This clears `active_air_id` but retains `joined` membership. It is idempotent
when the expected pointer was already cleared by the same request; a different
current Air returns `409 active_air_changed`.

`POST /v1/airs/{air_id}/leave`

```json
{"membership_revision":3,"expected_active_air_id":"air_01J00000000000000000000000"}
```

Only a current primary may remove its barycenter. If this is current, leave
atomically clears its pointer before marking membership `left`. An owner gets
`409 owner_transfer_required`. Leave of an already-left generation is an
idempotent success to its original caller.

`PUT /v1/airs/{air_id}/members/{membership_id}/role`

```json
{"air_revision":8,"membership_revision":3,"air_role":"admin"}
```

The owner primary may replace `admin | member` on a joined non-owner
membership. Pending/left memberships and `owner` as an input are invalid.

`POST /v1/airs/{air_id}/ownership/transfer`

```json
{"air_revision":8,"membership_id":"aim_01J00000000000000000000000","membership_revision":3}
```

The owner primary may transfer to a joined membership. In one transaction the
target becomes `owner`, the prior owner becomes `admin`, Air revision advances
and both barycenters are audited. There is always exactly one owner.

`PUT /v1/airs/{air_id}/policy`

```json
{
  "policy_revision":2,
  "invite":"air_admin_primary",
  "overlay":"primary_companion",
  "queue":"primary_companion",
  "replace":"air_admin_primary"
}
```

The body is a full replacement; omitted policy fields are invalid.

`POST /v1/airs/{air_id}/dissolve`

```json
{"air_revision":9}
```

Only the current primary of the owner barycenter may dissolve. Dissolve clears
every matching active pointer, marks all memberships left and the Air
dissolved, withdraws open invites, cancels pending/preparing/scheduled work,
fade-stops active Air audio, and retains audit/tombstones. It never deletes
member barycenters or their personal session state.

## 5. Telegram callback and compatibility aliases

Telegram uses the same application service and authorization checks. Inline
buttons carry only opaque `callback_data` of the form
`air1_<32 base64url characters>` (37 ASCII bytes). The coordinator stores only
a keyed token hash bound to `telegram_user_id`, actor generation, Air ID,
operation, expected resource revision and expiry. TTL is 15 minutes; tokens are
single-use for mutations, callback query IDs are deduplicated, and the token is
rejected after actor/primary/role change. No callback carries an Air ID,
invite code, role or action as trusted plaintext.

Alias mapping is exact:

| Command | Application operation |
| --- | --- |
| `/approach` | if the caller has no current Air, create a parked Air, point the creator barycenter to it, and issue one `member` invite |
| `/approach CODE` | consume invite; create `pending_confirmation`; never activate |
| `/accept` | joining barycenter primary confirms its newest pending membership with `activate=true` |
| `/decline` | joining primary declines its newest pending membership; issuer/Air-admin primary may cancel that claim or withdraw its newest open invite |
| `/apart` | caller primary leaves its current Air; never dissolves it or removes other memberships |

If an alias is ambiguous because more than one candidate has the same newest
timestamp, the bot returns buttons instead of guessing. `/accept` that would
switch from another current Air returns a named confirmation button bound to
both pointer revisions; it never silently switches. Telegram replies use
human copy but preserve the HTTP error code in structured audit.

`/approach` requires `expected_active_air_id=none`; it never switches away
from another saved/active Air. Its create, creator-pointer and invite writes
are one idempotent transaction. The creator pointer may reference the parked
one-member Air without running audio. When the joining primary accepts with
activation, the second pointer makes it active.

The shipped pairwise behavior changes at Air cutover: the joining primary,
not the inviter, performs final confirmation; `/apart` removes only the caller
barycenter. Migration and rollout must update help/callback text atomically
with authority cutover.

## 6. Playback boundary and lifecycle races

### 6.1 Join and accepted snapshots

Membership and `active_air_id` are resolved before a transmission is accepted.
The target rows and accepted policy revision are immutable afterward. A
barycenter joining or activating later is never appended to old accepted
overlay, interrupt, queue or replace work.

After activation into a live Air, eligible Pulsars receive only current main
program catch-up: prepare/buffer, seek to the coordinator's current audible
position, then scheduled start. They do not hear an already-started or ended
overlay/interrupt and do not auto-replay anything from history/inbox. New work
accepted after activation uses the new membership/pointer snapshot.

### 6.2 Leave, switch, park and dissolve

- Leave/switch during prepare makes the leaving barycenter targets terminal
  before scheduling. Other ready targets continue.
- Leave/switch during overlay or interrupt sends a 250 ms fade-stop/cancel only
  to that barycenter; the overlay controller and main program continue for
  other active members.
- Leave/switch during a main track fade-stops that barycenter. Other members
  keep their current timeline and queue.
- If the operation leaves fewer than two active member barycenters, the Air
  parks and all remaining Air playback stops after persisting main audible
  position/queue. No one-member broadcast continues under the Air ID.
- Dissolve applies the same target cancellation but is terminal and discards
  pending Air queue execution; personal queues remain untouched.

Explicit-recipient authorization and offline inbox rows may retain history of
these outcomes, but their schemas, pagination, replay and expiry are not
defined here.

## 7. Audit and error vocabulary

Every successful or rejected mutation records a content-free audit event with
operation, actor ID/generation, acting orbit, Air/membership/invite opaque IDs,
old/new status or revision, authority generation, result code and coordinator
timestamp. It MUST NOT contain invite/callback secrets, media titles, target
lists or Telegram message text.

Stable errors:

| HTTP | Code | Meaning |
| ---: | --- | --- |
| 400 | `invalid_request` | malformed/unknown/forbidden field or self-join |
| 401 | `unauthenticated` | no live actor credential |
| 403 | `forbidden` | actor/role lacks lifecycle authority |
| 403 | `policy_denied` | actor role is valid but Air policy denies operation |
| 404 | `air_not_found` | unknown, foreign or unauthorized Air |
| 404 | `membership_not_found` | no visible membership/action generation |
| 404 | `invite_unavailable` | unknown, expired, consumed or withdrawn code |
| 409 | `idempotency_conflict` | key reused with a different request |
| 409 | `revision_conflict` | resource revision changed |
| 409 | `active_air_changed` | observed current pointer no longer matches |
| 409 | `air_dissolved` | terminal Air cannot mutate |
| 409 | `already_member` | pending/joined membership already exists |
| 409 | `membership_confirmation_required` | action requires joining-primary confirmation |
| 409 | `air_barycenter_capacity_reached` | 8 barycenter slots occupied |
| 409 | `air_online_pulsar_capacity_reached` | activation would exceed 20 online participants |
| 409 | `air_parked` | playback command addressed a non-running Air |
| 409 | `owner_transfer_required` | owner attempted leave |
| 409 | `rollback_unsafe` | link rollback requested after Air divergence |

For a request that could fail in several ways, authentication precedes strict
shape, then visibility, actor role, resource revision, policy, lifecycle,
capacity and runtime admission. Foreign IDs never reveal policy, membership or
capacity.

## 8. Authority cutover and rollback

The `air_rooms` exposure flag is not playback authority. Authority is a
persisted singleton `(mode, generation)` with modes:

1. `links_authoritative` — shipped links resolve playback; Air commands are
   disabled.
2. `airs_shadow` — additive Air rows/mappings are built and validated, but
   links alone still resolve and emit playback.
3. `airs_authoritative` — Air pointers alone resolve and emit playback; link
   rows are compatibility provenance only.
4. `rollback_hold` — Air mutations are disabled while current Air runtime is
   drained or an unsafe rollback is investigated; links still do not emit.

Cutover is one transaction after shadow validation: write migrated Air
memberships/pointers, increment generation, set `airs_authoritative`, and
commit. The serialized loop observes the new generation, drains the old
link-domain controllers, then creates Air controllers from the committed
snapshot. Every timer/effect carries authority generation and is dropped if
stale. There is no request mirroring and never a link controller plus Air
controller for the same barycenter.

Turning `air_rooms` off after cutover disables create/join/policy mutations but
does not make links authoritative. A data rollback to links is allowed only
when a validator proves zero Air divergence since shadow creation: no N>2 Air,
no saved extra memberships, no membership/pointer/policy mutation, and an
exact reversible migrated-link mapping. The reverse transaction drains Air,
increments generation and flips to `links_authoritative`. Otherwise it returns
`409 rollback_unsafe`, enters `rollback_hold`, and keeps Air data/runtime
single-authoritative. Older coordinator binaries are rollback-safe only after
they have first shipped support for reading this authority record and refusing
Air generations; blindly deploying a links-only binary is forbidden.

## 9. Downstream acceptance map

- Schema/migration must encode the enums, unique constraints, revisions,
  pointers, tombstones and authority generation above.
- Runtime must implement single-authority resolution, parked behavior,
  capacity leases and join/leave audio rules.
- Control-plane API and Telegram adapter must implement these exact routes,
  payloads, alias meanings, opaque callbacks and audit/error vocabulary.
- Policy enforcement must snapshot policy at acceptance and keep DND/block
  stronger.
- Explicit targets/inbox must consume the immutable membership/pointer snapshot
  but define their own target, ACL, pagination, replay and expiry details.

Any downstream change to a frozen enum, route, default, confirmation actor,
alias or cutover rule requires an explicit contract version; silently changing
v1 is not compatible.
