# P2 Telegram Air lifecycle parity

`/air` is the private-chat management surface for the common Air lifecycle.
It does not own a second room model or runtime: every mutation calls the same
transactional `store` methods used by the Pulsar HTTP API, and runtime changes
are reconciled through the existing Air control reconciler.

## User surface

- `/air` lists saved and current Airs with bilingual human labels, capacity,
  role, online Pulsars and the effective invite/overlay/queue/replace policy.
- `/air create TITLE` creates a saved Air.
- `/air join SECRET` consumes a single-use invite, deletes the secret-bearing
  source message best effort, then requires the joining Barycenter primary to
  confirm a saved join or an explicit join-and-switch action.
- Actor-bound inline controls cover activate/switch, park, leave, dissolve,
  member/admin invitation, withdrawal, decline and owner policy presets.
- `/approach`, `/accept`, `/decline` and `/apart` remain immediate compatibility
  aliases over the existing Air store and reconciler.

Air administration is rejected in group chats. Invite secrets are returned only
as a private ordinary reply; they are not included in inline prompt text,
callback data, logs or durable mutation responses.

## Callback security

Telegram sees only a random `tg1_…` value. The HMAC digest and the actor, orbit,
role, chat, message, action, Air/member/invite references and expected revisions
are durable server-side state. Claiming a callback re-resolves the current
Telegram `ActorContext`, checks every binding, expires it after 15 minutes and
atomically consumes it. Telegram query IDs are separately fenced for 24 hours.
The canonical Air mutation then re-resolves the actor inside its transaction and
checks role, policy, membership revision, active-Air expectation and capacity.

This gives forged, forwarded, stale, repeated and concurrent actions a closed
failure mode without putting private identifiers in Telegram-visible material.

## Automated evidence

- Parser coverage for list/create/join with case-preserved invite material.
- Store coverage for opaque payloads, foreign actors, role changes, expiry and
  concurrent claims (exactly one claim succeeds).
- Coordinator parity coverage for private-chat enforcement, creation, listing,
  invitation, single-use consume, joining-primary confirmation, foreign click
  rejection, repeat rejection, active runtime pointer and canonical policy
  replacement.
- Existing Air HTTP/control and approach-alias suites remain the reference for
  every lifecycle transition and legacy compatibility.

Real Telegram clients, real app installations and physical Windows/macOS audio
hardware remain in the separate manual-testing epic; no such evidence is
claimed here.
