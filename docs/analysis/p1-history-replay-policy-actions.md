# P1 history replay and policy actions

- Task: `TASK-260712-3e4p0c`
- Contract: `p1-history-presence-telegram-v1`
- Scope: automated coordinator behavior; no manual app or hardware claim

Phase 1 history now has one transport-neutral command service for application
bearers and verified Telegram identities. The history projection remains a
hint surface. Every command resolves the current `ActorContext` again and then
delegates the mutation to its canonical owner service.

## App HTTP surface

All commands use `POST /v1/history/{history_item_id}/actions/{action}`, reject
query parameters and strict-decode one JSON object. Unknown history IDs and
unknown action paths do not disclose internal media or actor identifiers.

| Action | Body | Idempotency | Owner service |
| --- | --- | --- | --- |
| `replay` | `audience`, `delivery`, optional `include_origin` and interrupt fallback confirmation | required `Idempotency-Key` | common transmission resolver |
| `delete` | `{}` | media tombstone is intrinsically idempotent | media lifecycle and cancellation outbox |
| `report` | `reason`, `details` | one report per reporter/media | moderation control plane |
| `block_actor` | `{}` | required `Idempotency-Key` | transmission block policy |
| `block_orbit` | `{}` | required `Idempotency-Key` | transmission block policy |

`cancel` continues to use the transmission endpoint. `unblock` continues to
use the public block ID returned by the block list, so history does not invent
an ambiguous “remove every matching block” operation.

## Replay boundary

The client never supplies `media_id`, `accepted_at`, an old target, or a
receipt ID. The command service obtains media only from the authorized history
item. The coordinator assigns a new trusted acceptance time and invokes the
same resolver used by ordinary transmissions. Audience membership, explicit
selectors, target bindings, presence, capability, DND and block policy are
therefore evaluated again and a new immutable target snapshot is committed.

A retry with the same actor/key/request returns the already-created
transmission even if the media was deleted after acceptance. A different key
after delete or expiry cannot create anything. This preserves idempotent
response replay without reviving content or turning history into an offline
inbox.

## Delegated policy and audit

- Delete uses the media terminal transition, lifecycle audit rows and durable
  delivery-cancellation outbox; the lifecycle worker is signalled normally.
- Report uses the existing exact-target evidence check, per-reporter/media
  uniqueness, rate limit, evidence snapshot and append-only moderation audit.
- Block mints a viewer-bound subject reference and uses the existing
  actor/orbit role policy plus policy-request idempotency ledger. New blocks
  trigger the established cancellation enforcement path.
- Replay is evidenced by its actor-scoped idempotency record, transmission row
  and immutable target rows; there is no second history-action ledger carrying
  duplicate business state.

Telegram owners can replay and delete their own available Telegram media
through the same service. They do not receive an app bearer or installation
slot, and current role/media ownership checks still apply. Revoked actors,
departed members, node-only credentials, foreign viewers and stale direct IDs
fail before a mutation owner commits.

## Automated evidence

Store, service and HTTP tests cover fresh acceptance and target resolution,
same-key replay, changed-request conflict, delete/replay races, repeated delete,
deleted and expired media, exact report evidence, repeated reports, actor and
orbit blocks, block retries after the action hint changes to `unblock`, verified
Telegram ownership, strict request parsing and revoked/foreign actors. Manual
real-client, audible and hardware validation remains exclusively in
`EPIC-260714-th54l3`.
