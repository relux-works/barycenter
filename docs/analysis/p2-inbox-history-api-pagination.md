# P2 inbox and history API pagination

`TASK-260712-2j5fkr` completes the HTTP/application boundary over the immutable
target and inbox foundation. It does not add a second delivery, history, ACL,
delete, cancel, or moderation authority.

## Read surfaces

- `GET /v1/inbox` uses deterministic `created_at DESC, inbox_id DESC` keyset
  pagination. The first page freezes an upper key; later inserts cannot expand
  that traversal.
- Inbox cursors are random `ic_` capabilities. Only their SHA-256 digest and
  actor, credential, current binding generation, view, limit and server-side
  page keys are stored. They expire after 24 hours and are bounded to 128 live
  cursors per actor.
- `GET /v1/inbox/{ib_...}` reauthorizes the exact current target binding.
  Forged IDs, other targets, replacement bindings, expired media, deletion and
  disablement all use the same `404 inbox_not_found` surface.
- `GET /v1/history/{hi_...}/receipts` pages the already immutable target
  snapshot. A sender sees safe target labels; a target sees only its own exact
  current binding. Receipt cursors are independent random `rc_` capabilities
  with the same 24-hour/128-capability bounds.
- Invalid, expired, cross-actor or differently parameterized page capabilities
  return `410 cursor_expired`. Reads update no scheduler, queue or playback
  authority.

Inbox/history reads expose `ib_` and `hi_` handles, display labels, media
presentation, status and allowed-action hints. They do not serialize raw
media/transmission IDs, numeric actor/orbit IDs, slots, binding generations,
node identities or bearer material. Action hints are never authorization.

## Commands

- `DELETE /v1/inbox/{ib_...}` is a locally idempotent dismiss. It neither
  deletes media nor changes another target or playback.
- `POST /v1/inbox/{ib_...}/replays` requires an idempotency key and an explicit
  delivery. The caller cannot supply media or audience identifiers. The store
  reauthenticates the current installation, resolves the inbox media, requires
  the current content-policy grant, applies normal delivery/capability policy,
  creates one new transmission to the same exact Pulsar, seals replay lineage
  and consumes the inbox item in one transaction.
- A successful replay returns only an `ir_` request capability and a `hi_`
  history capability. Exact retries return the same result; conflicting keys,
  depth greater than eight, expiry, deletion, disablement or replacement
  binding fail without creating playback.
- Existing history delete/report/block and transmission cancel routes remain
  the sole owner services for those mutations. Sender delete revokes fetch and
  inbox replay through the existing media lifecycle; eligible cancel remains
  bounded by the existing start-race policy.

Automated coverage exercises frozen pagination under a concurrent insert,
cross-actor cursor rejection, non-target detail indistinguishability,
replacement-safe target authorization, idempotent command boundaries, atomic
replay lineage, receipt audience projection and the absence of read-triggered
playback. Real-app and real-hardware validation remains in the manual test
epic.
