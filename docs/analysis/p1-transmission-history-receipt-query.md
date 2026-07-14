# P1 transmission history and receipt query

- Task: `TASK-260712-2hcq1g`
- Contract: `p1-history-presence-telegram-v1`
- Scope: automated Phase 1 read model; no manual-device acceptance

The coordinator now exposes the frozen Phase 1 history contract through
`GET /v1/history` and `GET /v1/history/{history_item_id}`. Both routes resolve
the bearer again to the current `ActorContext`; the same store query accepts a
verified Telegram identity so bot routing does not maintain a second history
model.

## Projection and authorization

History is derived from the existing media, transmission, immutable target and
current identity stores:

- owner-visible unlinked media appears as `processing`, `ready`, or `error`;
  it disappears from the media-only projection as soon as its first
  transmission commits;
- transmission rows use trusted `accepted_at`, preserve requested/effective
  delivery and downgrade reason, and retain receipt evidence for 30 days;
- the creator/current source primary using control authority receives every
  target row, another current source control actor receives aggregate counts
  plus only its exact current target binding, and a target-only actor receives
  only that binding;
- a foreign actor receives an empty list and the same `404 history_not_found`
  detail result as an unknown ID. Revoked credentials fail before projection;
- a node-only bearer never gains the source/media control projection; it can
  observe only the exact receipt bound to its own current installation;
- failed media follows media retention instead of the transmission window.

List target counts contain exactly `played` and `other`. Detail counts contain
all frozen target states. A blocked target's actor/orbit reason is redacted
unless the viewer owns the matching active recipient block.

## Pagination and actions

Rows sort by `(occurred_at DESC, history_item_id DESC)`. The first page freezes
its upper key. A 24-hour random `hc_` capability binds the next page to the
actor, credential plus current authorization scope, view, limit, upper key and
last key. Only its SHA-256 digest is stored; tenant and media identifiers never
appear in the client token. Expired state is removed opportunistically and at
most 128 live cursor rows are retained per actor.

Action hints are recomputed for every query in the contract order:
`cancel`, `delete`, `replay`, `report`, `block_actor`, `block_orbit`, `unblock`.
They reflect current control capability, media availability, target evidence,
role and active block ownership. Report is offered only for foreign media with
an exact current target binding, matching the moderation evidence boundary;
senders do not receive a self-report action for their own media. Hints are not mutation grants; every mutation
reauthorizes. Replay remains only a hint for the separate create-transmission
flow—history never queues, autoplays, or creates a Phase 2 inbox item.

## Automated evidence

Store and HTTP tests cover media lifecycle projection, app and Telegram
identity parity, deterministic pagination, cursor misuse and expiry, 30-day
retention, source/recipient/foreign visibility, aggregate and exact receipts,
requested/effective delivery, ordered actions, strict query parsing and
response redaction. Real-app and real-hardware validation remains exclusively
in `EPIC-260714-th54l3`.
