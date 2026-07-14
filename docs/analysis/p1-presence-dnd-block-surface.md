# P1 presence, DND and block implementation

Status: implemented by `TASK-260712-1c1ska` against the frozen
`p1-history-presence-telegram-v1` contract.

## Public boundary

`GET /v1/presence` projects only the caller's current installation domain:
its own orbit and, when present, the one active pairwise approach. Rows are
ordered by `(orbit_id, slot)`. A current-generation authenticated heartbeat is
online for 12 seconds; after that the row is offline with output
`unavailable`, playback `unknown`, and interrupt-resume readiness false.

The JSON allow-list is online/last-seen, output state, playback state, local
and effective DND, sorted capability names and interrupt-resume readiness. It
contains no actor id, credential, address, hostname, device/process/speaker
name, microphone/capture state, level, local path or media URL. Telegram
`/status` uses the same privacy boundary and no longer renders volume,
position, RTT, offset, speaker names or client/library versions.

`PUT /v1/presence/dnd/local` is restricted to the exact installation control
credential. `PUT /v1/presence/dnd/orbit` accepts a current same-orbit primary
through the common `ActorContext` service, including a verified Telegram
primary. Expected revision zero creates a layer; otherwise it must equal the
current revision and the commit writes `+1`. Idempotency keys and canonical
requests are persisted only as SHA-256 digests in the same writer transaction
as authorization and mutation.

`GET|POST /v1/blocks` and `DELETE /v1/blocks/{block_id}` never accept internal
integer identity. History supplies actor-bound, 24-hour `ar_` or `or_` subject
references; blocks expose stable `bl_` ids. Personal blocks belong to the
actor; orbit blocks require the current primary. App and verified Telegram
identities call the same store service. Unknown, expired, cross-viewer and
foreign identifiers collapse to not-found results.

## Enforcement and lifecycle

The existing acceptance and scheduler policy checks remain authoritative.
DND precedence is active mute, then messages-only, then allow-all. Two active
mutes choose the later deadline; equal-severity local policy wins, so remote
orbit policy cannot loosen it. Expired mute is projected and evaluated as
allow-all.

A newly restrictive layer disarms current recipient work through the
generation-aware scheduler cancellation seam. Actor and orbit blocks cancel
only matching sender work for the owned recipient nodes. Non-started targets
retain the frozen missed/blocked reasons; already active work uses the frozen
fade-stop cancellation result. Removing a block does not requeue or resurrect
terminal work.

The schema change is additive. Opaque reference, public block-id and policy
request tables point only to the existing additive transmission policy tables;
an older coordinator can ignore them during drain-first rollback. Plaintext
idempotency keys are never stored.

## Automated evidence boundary

Coordinator tests cover heartbeat freshness, stale/offline projection,
generation matching, field sanitization, exact DND replay and conflict,
viewer-scoped opaque blocks, idempotent delete, guessed-id collapse, and
shared Telegram/app role authorization. Existing transmission policy and
scheduler suites cover mute expiry/precedence and pending/active DND/block
receipt transitions.

This is best-effort code, unit and deterministic integration evidence only.
Real app behavior, audible fades, physical devices and hardware heartbeat
timing remain in the manual-test epic `EPIC-260714-th54l3`.
