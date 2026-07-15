# P1 Telegram moderation parity

Status: implemented for `TASK-260712-dlltnr`. This handoff covers the coded
Telegram adapter and automated checks. Live Bot API, real-account, real-device
and operator-mailbox exercises remain in the separate manual-testing epic
`EPIC-260714-th54l3`.

## Surface and ownership

`/history` renders the five newest authorized history items in a private bot
chat. It uses the same history projection and the same transport-neutral
`historyactions.Service` as the authenticated app API. Telegram does not own a
second report, block, delete or replay implementation:

| Telegram control | Canonical owner | Terminal result vocabulary |
| --- | --- | --- |
| replay | transmission resolver and scheduler | `replay_accepted`, `replay_already_accepted`, `history_action_unavailable` |
| delete | media lifecycle service | `media_deleted`, `history_action_unavailable` |
| report with a frozen reason | moderation service | `report_received`, `report_already_received`, `history_action_unavailable` |
| block sender | transmission policy service | `sender_blocked`, `sender_already_blocked`, `history_action_unavailable` |

The shared presentation catalog owns exact English and Russian labels for all
buttons, six moderation reasons, directions and terminal results. Unknown
internal values fail closed to generic unavailable/failed copy. History text
uses titles, bounded display names, localized status/reason labels and public
semantics; it never renders actor, orbit, media, transmission or history IDs.

## Telegram receipt semantics

A verified Telegram member may read current transmission receipts addressed to
any current Pulsar installation in their own Barycenter. The target must still
match its immutable installation binding; joining an orbit does not make a
stale or revoked target visible. This is the same shared-history view presented
by the app, not a new Telegram-only history store.

A Telegram report records two intentionally distinct identities:

- the verified Telegram actor is the reporter and rate-limit principal;
- the exact current Pulsar target remains the immutable delivery evidence.

Databases created before this distinction are migrated transactionally from
the old `target_actor_id = reporter_actor_id` constraint. Existing reports,
foreign keys, evidence fields, indexes and the immutable-snapshot trigger are
preserved. A failure before commit restores the old table and reenables foreign
keys.

Blocking a sender from Telegram is available only to the Barycenter primary.
Because a Telegram actor has no individual playback installation, the block is
owned by the Barycenter and is enforced against its current Pulsars. App
actions keep their existing actor-scoped behavior. This is an ownership
translation at the common policy service, not a separate block list.

## Callback security and race behavior

History controls use a dedicated additive callback table because received
media does not necessarily have a Telegram upload-routing row. The transport
contract otherwise matches the established inline-routing contract:

- callback data is a random opaque `tg1_` capability; HMAC digests, never raw
  tokens, are persisted;
- each capability is bound to the current Telegram actor, orbit, role, chat,
  message, history item, action and report reason;
- capabilities expire after 15 minutes and callback-query results after 24
  hours;
- forged, expired, cross-user, cross-role, cross-chat and cross-message clicks
  cannot reach an owner service;
- owner services reauthorize the history item at execution time, so deletion,
  expiry, role transfer, revocation, an existing block or a changed receipt
  closes the action;
- a repeated Telegram query receives its durable prior result, and a second
  query against a consumed capability receives `already_applied`;
- terminal success removes the keyboard promptly; group commands and group
  callbacks cannot perform moderation actions.

Unknown callback tokens fall through to the existing upload-routing callback
namespace. A token forged for neither namespace receives the established
non-disclosing expired result.

## Compatibility boundary

The legacy voice/document intake and its default immediate
`after_current`/current-Air delivery path are unchanged. `/history` replay is a
new canonical transmission with Telegram origin, `include_origin=true`,
current-Air audience and `after_current` delivery. It never resurrects an old
target snapshot. Existing inline delivery choices, confirmation races, target
receipts and callback outcome mapping continue through their original path.

## Automated evidence

The suite covers:

- opaque minting plus forged, expired, cross-user and wrong-message rejection;
- durable query replay, consumed-token replay and unavailable-action minting;
- private-chat history rendering without raw IDs and group-chat rejection;
- Telegram report and orbit-owned sender block through the same services used
  by the app HTTP history actions;
- exact report target evidence and one-report idempotency;
- legacy moderation-schema rollback, successful migration, foreign-key health
  and snapshot immutability;
- exact closed English/Russian action, reason and result vocabularies;
- the pre-existing Telegram voice ordering, inline-routing and callback race
  regression suites.

Manual acceptance should use a real primary Telegram account and two current
Pulsar installations to confirm button layout, Bot API acknowledgement timing,
keyboard removal, operator queue visibility and live cancellation behavior.
