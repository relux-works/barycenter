# Phase 2 explicit targets, inbox and parity contract v1

- Date: 2026-07-15
- Task: `TASK-260712-2rlkp7`
- Machine contract: `acceptance/targets-inbox-contract-v1.json`
- Contract ID: `p2-targets-inbox-parity.v1`

This contract is normative for the common application service, persistence,
HTTP, Windows, macOS and Telegram work in the explicit-target/inbox story. It
extends the Phase 1 transmission, target snapshot, history, callback and
moderation models. It does not define parallel authority stores.

## Authority and create boundary

All sends continue through `POST /v1/transmissions`. The coordinator derives
identity from `ActorContext`, resolves up to 64 selectors inside the caller's
own Barycenter or current Air, deduplicates before origin filtering and writes
the transmission plus immutable target rows in one transaction. Each row
freezes orbit, slot, actor, binding generation, capability-set hash and
resolution time. Offline current bindings are included.

Later Air joins, new installations and replacement bindings cannot expand the
snapshot. Current Air membership, knowledge of an ID or an old binding does not
grant media access. A non-target—including a node in the same Air—receives the
same 404 nonexistence surface as an unknown object and its main-program state is
untouched. There is no broadcast fallback.

User media create and replay require the server-owned current content-policy
grant. Client timestamps never establish consent. Missing or stale consent is
`428 content_policy_acceptance_required`.

## Clip, track and mixed-version behavior

Phase 1 clip kinds retain `overlay`, `interrupt` and `after_current`.
`audio_track` accepts exactly `queue` or `replace`. Queue appends and replace
performs a generation-safe fade/replacement independently inside each exact
target playback domain.

Capability resolution happens before the create transaction. An online
mandatory target missing a required capability rejects the whole request with
`422 unsupported_targets`; the response contains only opaque target references
and sorted missing capability names. No partial transmission or inbox rows are
created. A Phase 1 node may receive a supported Phase 1 clip but is an explicit
unsupported target for a track. Capability loss after prepare fails only that
target with the existing receipt model and never downgrades peers.

An offline target with unknown current capability remains in the snapshot and
may receive a `missed_offline` inbox item. It never autoplays or autoqueues on
reconnect.

## Inbox creation, fields and retention

An inbox row is created in the same transaction as the first eligible terminal
target receipt. Eligibility is exact and intentionally narrow:

- the three `missed_offline` reasons;
- local or orbit DND;
- prepare deadline;
- connection loss, device unavailable or audio graph failure.

Blocked, played, cancelled and expired targets are not eligible. Media auth,
expiry, hash, decode, duration, stale-play and internal failures are not
presented as replayable misses.

The item belongs to one target snapshot row, not to an Air. Its default TTL is
30 days and actual expiry is the earliest of receipt plus TTL, media expiry and
policy retention. A new Air member never discovers it. A replaced or revoked
former target sees 404 and cannot replay it.

The common entry freezes IDs/revision, transmission and media references,
opaque target reference, media kind, requested/effective delivery, missed
status/reason, availability, creation/expiry, replay root/depth and action
hints. Source identity is a localized display label only. Receipt aggregates
reuse Phase 1 history projection, and a target sees only its exact target row.
Action hints never authorize a mutation.

## Pagination and manual replay

`GET /v1/inbox` orders by `(created_at DESC, inbox_id DESC)`, defaults to 20
and accepts 1–100 rows. The 24-hour `ic_` cursor is a random 192-bit capability;
only its SHA-256 digest is stored. It binds actor, credential scope, current
binding generation, view, limit and frozen upper/last keys. No tenant or media
identifier appears in the client token, no membership change expands a page,
and at most 128 live cursors exist per actor.

Replay is only `POST /v1/inbox/{id}/replays` after an explicit user action. It
reauthorizes the current actor/binding, media, target and content-policy grant,
then creates a new transmission with new immutable target rows. It never edits
the original receipt. Lineage keeps inbox, immediate transmission, root
transmission and depth; depth is capped at eight. Idempotency is mandatory.

`DELETE /v1/inbox/{id}` is a recipient-local dismissal. It does not delete
media or affect another target. Sender media deletion remains the existing
reauthorized media action: fetch is revoked first, pending/active playback is
cancelled, replay hints disappear and terminal receipts remain.

## Reports, quarantine and anti-denial of service

A report reuses `POST /v1/history/{id}/actions/report` and requires the
reporter's exact current target evidence. Its immediate effect is local only:
hide/unavailable, deny replay and stop that reporter's target playback; the
reporter may also use the existing block actions.

A report alone never deletes media, cancels unrelated targets, disables the
sender or Air, grants operator authority or triggers a count-based global
quarantine. This prevents reports from becoming an unreviewed global denial of
service.

Global quarantine is a distinct reversible, audited moderation-operator
decision. It denies new non-moderator fetch, delivery and replay while
preserving evidence. Reviewed media delete and actor/orbit disable remain the
existing terminal moderation decisions and enforcement paths.

## Platform and Telegram parity

Coordinator HTTP, Windows, macOS and Telegram use the same fields, enums,
authorization, errors, cursor semantics and localized presentation keys.
Telegram is an adapter over the common service; it owns no queue or inbox
state. Its callbacks contain only versioned HMAC-bound opaque action/object
IDs, actor scope, nonce and expiry—never a bearer or numeric identity. Unknown
future enums stay visibly unsupported and are never coerced into a known
action.

## Downstream implementation order

The following tasks must implement this contract rather than reinterpret it:

1. repository schema/ACL for snapshots and inbox rows;
2. common target/replay service;
3. versioned consent and history/inbox API;
4. rights, report, quarantine and disable enforcement;
5. Windows, macOS, Pulsar and Telegram presentation/parity;
6. regression evidence and rollout handoff.

Any field, enum, eligible reason, TTL, cursor binding, replay lineage or report
side effect change requires a versioned contract update before implementation.

## Verification

```sh
python3 scripts/validate_targets_inbox_contract.py
python3 -m unittest scripts/acceptance/test_acceptance.py
```
