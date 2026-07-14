# Phase 1 Telegram, history and presence rollout handoff

- Date: 2026-07-15
- Task: `TASK-260712-1f9jtm`
- Story: `STORY-260712-34kbkn`
- Contract: `p1-history-presence-telegram-v1`

This is the durable implementation, rollout and downstream-consumer entry
point for Phase 1 Telegram/app parity, history, presence, DND and blocks. The
[frozen contract](p1-history-presence-telegram-contract-v1.md) remains
normative. This note records what is implemented, which component owns each
decision, the supported mixed-version behavior and the safe deploy/rollback
sequence.

This is best-effort engineering evidence. It does not claim a real Telegram
client, real callback rendering, audible playback, packaged app, physical
Windows/macOS device or hardware timing result. Those checks remain in manual
epic `EPIC-260714-th54l3`.

## 1. Contract and authority boundary

The common `ActorContext`, media lifecycle, transmission resolver/scheduler,
policy store and moderation service remain the sources of truth. History,
presence and Telegram are adapters and projections; none may create a second
identity, media, scheduler, DND, block, receipt or audit domain.

Application requests authenticate an existing bearer. Telegram resolves the
current linked actor again for every update and callback. Chat membership,
numeric Telegram identifiers and callback possession alone grant no authority.
Caller-supplied actor, orbit, role, slot, acceptance time, receipt, target
snapshot, force, priority, emergency or DND-bypass fields are forbidden.

Public history and block mutations use coordinator-issued `hi_`, `bl_`, `ar_`
and `or_` references with the documented scopes. They are not credentials.
Numeric actor IDs, Telegram user/file/query IDs, callback material, credential
generation, socket data, hostname, process, device, microphone/capture state,
level, local path and media URL never belong in the public projection.

## 2. Final surface inventory

### 2.1 Application HTTP

| Surface | Frozen behavior |
| --- | --- |
| `GET /v1/history` | Actor/credential-bound `all`, `sent` or `received` projection with frozen cursor pagination. Rows sort by occurrence and public history ID descending. |
| `GET /v1/history/{history_item_id}` | Authorized media/transmission detail, complete frozen target counts and only viewer-authorized target rows/reasons. Unknown and invisible IDs both return not found. |
| `POST /v1/history/{history_item_id}/actions/replay` | Creates a new transmission through current resolution and a new coordinator acceptance. It never reuses old targets or receipts and never becomes an offline inbox. |
| `.../delete`, `.../report`, `.../block_actor`, `.../block_orbit` | Strict transport-neutral commands delegated to media lifecycle, moderation and canonical block policy. Every command reauthorizes; list actions are hints, not grants. |
| `GET /v1/presence` | Own orbit plus current pairwise approach only; sorted sanitized liveness, playback, DND and capability projection. |
| `PUT /v1/presence/dnd/local` | Exact installation control only, optimistic revision and actor-scoped idempotency. |
| `PUT /v1/presence/dnd/orbit` | Current same-orbit primary, including a verified Telegram primary, through the same service. |
| `GET /v1/blocks`, `POST /v1/blocks`, `DELETE /v1/blocks/{block_id}` | Viewer-owned blocks using history-derived opaque subjects. Guessed, expired, foreign and cross-kind references collapse to documented not-found/invalid results. |

Strict JSON/query limits, idempotency syntax and response error codes remain in
the normative contract. No consumer may add permissive aliases or accept an
internal identifier because a UI happens to know it.

### 2.2 Telegram attachment matrix

Telegram calls common `SubmitMedia`; metadata is never proof.

| Telegram update | Phase 1 result |
| --- | --- |
| `voice` | Bounded download, common normalization to `voice_clip`, then an immediate durable `after_current` default. |
| `audio` with decoded duration at most 180 seconds | `audio_clip`; explicit action only, no hidden autoplay. |
| audio-signature `document` at most 180 seconds | `audio_clip`; explicit action only. |
| audio/document over 180 seconds | `track_not_available_phase1`; never truncate, relabel or enqueue a Phase 2 track. |
| non-audio document/video/animation/sticker | `attachment_not_audio`. |
| media group/multiple attachments | `media_group_not_supported_phase1` before ingest. |

The Telegram source limit is 20 MiB; common canonical output remains bounded
by 34 MiB and 180 seconds. Filename, MIME, declared duration and declared size
are hints. Bounded bytes, signature, decode/probe, duration and canonical
output determine the result. Stable errors are:

```text
telegram_download_failed | telegram_media_too_large | attachment_not_audio |
media_group_not_supported_phase1 | track_not_available_phase1 |
decode_failed | duration_mismatch | canonical_output_too_large
```

There is no Spotify fallback, queue/replace action, late autoplay or offline
inbox in this contract.

## 3. Default, inline action and callback rules

A ready voice and its default `after_current` transmission commit in one
SQLite writer transaction at the trusted Telegram acceptance time. The inline
keyboard is rendered only afterward. There is no decision window and no
synthetic `wait`; no click preserves the legacy first-after-current ordering
even when processing completes out of order.

Audio/document routes commit without a default and remain explicit-action-only.
All accepted routes use the common transmission resolver and scheduler.

`callback_data` is exactly `tg1_` plus 32 base64url characters (36 UTF-8 bytes).
Only its HMAC-SHA-256 digest is indexed. Actor/orbit/role, chat/message/update,
media ID/generation, action, delivery and audience remain server-side. Tokens
expire after 15 minutes; query outcomes are deduplicated for 24 hours.

Every click re-resolves identity and verifies actor/role/orbit, exact
chat/message, media generation and token digest. Private controls are
initiator-only. An explicitly shared control may authorize only the current
primary of the source orbit; group membership alone is insufficient.

The action vocabulary is closed:

```text
choose_overlay | choose_interrupt | choose_after_current |
choose_own_barycenter | choose_current_air |
confirm_overlay | confirm_after_current | dismiss
```

The outcome vocabulary is closed and shared with app presentation:

```text
applied | already_applied | requires_confirmation | too_late |
expired | forbidden | unsupported | failed
```

Malformed, unknown, forged and wrong-message/chat references collapse to
`expired`; wrong actor/role/orbit is `forbidden`. Exact query replay returns
the stored answer without mutation. A consumed token yields
`already_applied` only to the same authorized actor. Unknown internal outcome
values render the non-disclosing shared `callback.failed` label.

A valid terminal choice may replace the voice default only before any default
target durably reaches `playing` or `played`. Cancellation of the default and
creation of the replacement are one transaction. The replacement receives a
fresh acceptance and never inherits old FIFO position. If playback wins, the
callback is `too_late` and changes nothing. Concurrent clicks serialize to one
replacement; rollback at either injected transaction boundary leaves the
default intact.

Interrupt never silently falls back. When exact interrupt/resume is
unavailable, the service returns `requires_confirmation`, creates no
transmission and leaves the default armed. Only a second opaque callback using
the server-stored confirmation reference may choose an available overlay or
`after_current` fallback. If the default starts meanwhile, confirmation is
`too_late`.

## 4. Shared EN/RU presentation contract

`coordinator/internal/presentation` owns semantic `key`, English and Russian
copy for sender/origin/target/audience, requested/effective delivery,
downgrade, confirmation, callback outcome, media/aggregate/target state and
every frozen receipt reason. Telegram selects Russian from this catalog; app
clients select EN or RU from the same key. Neither surface maintains a private
translation dictionary.

Pairwise Air renders from safe human orbit metadata, for example `Current Air
with «Orion»` / `Текущий эфир с «Orion»`. A raw slot appears only within a safe
human target label such as `«Home», Pulsar A`; raw one-letter, composite,
numeric, typed internal and Telegram identifiers fall back to stable unknown
copy. Human metadata is whitespace-normalized and capped at 120 Unicode scalar
values.

Requested and effective delivery must be shown separately when they differ.
Mixed capability overlay is one whole `after_current` transmission with
`mandatory_target_missing_overlay_capability`; a surface must not render it as
successful overlay. Interrupt confirmation and callback outcome text use the
same catalog. Machine enums and reasons are never localized or rewritten.

## 5. History, presence and policy semantics

History is a projection over media and transmission stores. Media-only rows
show `processing`, `ready` or `error` and disappear after their first linked
transmission commits. Transmission rows preserve exact accepted ordering,
requested/effective delivery, immutable target counts, aggregate state and
authorized reasons. Content deletion/expiry removes replay/delete capability
but retains content-free receipt evidence for its policy window.

Pagination cursors are opaque, expire after 24 hours and bind actor,
credential scope, filter, limit, frozen upper bound and last key. New rows do
not shift later pages. Wrong actor, changed filter/limit, revoked credential,
expiry and malformed values return `history_cursor_invalid`.

Action visibility follows current authority and every mutation checks again.
Replay resolves current targets. Delete uses the media tombstone and durable
cancellation outbox. Report requires foreign media plus exact current-target
evidence. Actor/orbit block uses a viewer-bound subject. A sender never gains a
self-report action and an unblock never resurrects delivery.

Presence shows only own-orbit/current-pairwise nodes. After 12 seconds without
current-generation evidence, a node becomes offline with output unavailable,
playback unknown and interrupt readiness false. Last-known capabilities are
display-only, never addressing or media authority.

DND layers are `allow_all`, `messages_only` and `muted_until`. Effective policy
is the stricter active local/orbit layer; between active mutes, the later
deadline wins. Equal-severity local policy wins so remote orbit policy cannot
loosen it. Expired mute is allow-all. `messages_only` permits user voice/audio
clips but suppresses built-ins and automation. No Telegram/app action or
moderator bypass exists.

Blocks are recipient-owned. Actor/orbit block is evaluated before DND,
offline and capability. A pre-start target becomes `blocked/actor_blocked` or
`blocked/orbit_blocked`; active playback uses fade-stop
`cancelled/sender_blocked`. A viewer who does not own the block sees only
`blocked`, not its scope. Removing DND/block never requeues a terminal row.

## 6. Mixed-version compatibility

The database changes are additive. Current coordinator code retains the
legacy Telegram media/WAV bridge and Session `play_voice` path needed by the
`after_current` default and rollback-era rows. Older nodes continue their
existing `play_voice`/`solo_voice` behavior.

There is no per-target protocol split. If any online mandatory target lacks
overlay capability, the whole transmission becomes `after_current` with the
exact downgrade reason. Interrupt without complete capability creates only a
confirmation challenge. Reconnect replaces the previous capability set; it
never unions stale claims.

Phase 1 has no runtime flag that magically enables this complete surface.
Rollout therefore uses dependency order and the owning dispatcher/UI exposure
points. Operators must not document a nonexistent global switch.

## 7. Deploy and bounded exposure

1. Record coordinator/bot artifact revision and checksum, take a SQLite-safe
   backup and retain the immediate predecessor. Preserve unknown additive
   tables.
2. Deploy the coordinator first while new Telegram attachment/inline and app
   history/presence controls remain undiscoverable at their owning exposure
   points. Existing legacy voice and node playback continue.
3. Verify health, additive schema installation, current identity links,
   history/presence authorization, callback HMAC key availability and
   deterministic regression gates. Health is not an audible/hardware result.
4. Deploy capability-honest node builds according to the transmission handoff.
   A build advertises only executable capabilities. Mixed fleets use the
   whole-downgrade/confirmation rules above.
5. Expose read-only app presence/history first to a bounded cohort. Confirm
   empty/not-found behavior, redaction and cursor binding before exposing
   mutations.
6. Expose DND/block/history actions, then Telegram audio/document and inline
   callbacks. Voice default creation stays on the immediate critical path;
   keyboard delivery is asynchronous and cannot gate it.
7. Keep Store/release copy aligned with the shared labels and manual-evidence
   boundary. Do not promote CI, health or schema success into a real-client,
   audible or device claim.

## 8. Operations and observability

Monitor only privacy-safe aggregates and state:

- media processing/ready/failed backlog and common failure codes;
- pending/selected/dismissed inline-route counts and callback outcome rates;
- repeated/expired/forbidden/too-late callback rates without raw token/query
  values;
- default-to-replacement and interrupt-confirmation rates;
- transmission nonterminal rows, whole-downgrade reasons and target receipt
  outcomes;
- history cursor invalidation, authorization/not-found and action conflict
  rates;
- stale presence counts, DND revision conflicts and block lifecycle outcomes;
- cancellation outbox/scheduler watchdog backlog and coordinator reconnect
  churn.

Logs and audit rows may contain internal object references only where their
own privacy contract permits; operator dashboards and user responses must not
add Telegram identifiers, callback data, bearer material, media URL/path,
microphone/process/device details or moderation evidence.

An increase in `forbidden` can indicate stale links or attempted cross-user
use; it does not justify logging the presented token. An increase in
`too_late` is a product timing signal, not permission to reopen a decision
window. A downgrade increase must be addressed through capability rollout,
not per-target protocol splitting.

## 9. Drain-first rollback and roll-forward

Rollback starts by withdrawing new mutations at their actual owners: pause
Telegram media/inline dispatch and desktop create/action exposure while
keeping status, history and cancellation processing available. There is no
global runtime flag to substitute for this step.

1. Stop accepting new Telegram media/routes and new app mutations. Let current
   media processing reach terminal state or record its common failure.
2. Cancel eligible pending/prepared transmissions through canonical APIs and
   wait for generation-bound cancellation acknowledgements/watchdogs. Do not
   rewrite receipt rows manually.
3. Ensure inline routes have a stable pending/selected/dismissed state and no
   writer transaction is in flight. Pending keyboards may expire naturally;
   do not remint them during rollback.
4. Stop the current coordinator as the sole SQLite writer, checkpoint/back up
   the drained database and deploy the recorded predecessor.
5. Preserve additive identity, media, transmission, history cursor, opaque
   reference, policy and Telegram inline tables. The predecessor ignores
   unknown additive state; dropping it destroys roll-forward evidence.
6. Keep new surfaces withdrawn. Legacy Telegram voice and Session playback
   remain the only supported rollback-era behavior.

If drain cannot converge, restore the current coordinator rather than
improvising SQL changes. Roll-forward deploys the current coordinator while
surfaces remain withdrawn, completes schema/reconciliation, verifies preserved
media/transmission/policy/route state, reconnects nodes and then repeats the
bounded exposure order.

## 10. Exact downstream ownership

| Owner | Handoff rule |
| --- | --- |
| Identity/onboarding (`STORY-260712-2ve1c8`) | Own Telegram linking, revocation, role and `ActorContext`. Adapters re-resolve; they never authorize from raw Telegram membership. |
| Media ingest (`STORY-260712-ld674h`) | Own bounded download, signature/decode proof, canonical clip, retention and tombstone. Telegram metadata remains hints. |
| Transmission (`STORY-260712-25lysg`) | Own acceptance, immutable targets, capability/DND/block resolution, scheduling, ACL, receipts and cancellation. Inline/history calls this service. |
| Overlay/interrupt mixer (`STORY-260712-fes2jj`) | Own executable duck/fade/resume behavior and capability advertisement. Telegram/app only present exact effective mode and outcome. |
| Main UI/capture (`STORY-260712-2e36uz`) | Consume HTTP fields and shared keys, show requested/effective delivery, reason and action availability, and retain safe retry state. Never invent IDs/translations. |
| Policy/moderation (`STORY-260712-1tgryz`) | Own report evidence, moderation disable and audited enforcement. Moderation cannot broaden history/media visibility or bypass DND/block. |
| Store compliance (`STORY-260712-1i0doc`) | Consume exact EN/RU copy, redaction and mixed-version caveats. Real screenshots/client/device behavior stay manual until accepted. |
| P2 Air/targets/tracks (`STORY-260712-3v14m9`, `STORY-260712-ob1tx2`, `STORY-260712-2ori1t`) | Extend through new versioned contracts. Do not reinterpret Phase 1 callback, receipt or attachment semantics; Phase 1 history is not an inbox and Phase 1 audio/document is not a track. |

## 11. Evidence and reference index

- [Normative history/presence/Telegram contract](p1-history-presence-telegram-contract-v1.md)
- [Shared EN/RU presentation model](p1-shared-delivery-presentation-model.md)
- [Telegram callback and attachment transport](p1-telegram-callback-audio-transport.md)
- [Presence, DND and block implementation](p1-presence-dnd-block-surface.md)
- [History and receipt query](p1-transmission-history-receipt-query.md)
- [Durable inline routing](p1-telegram-inline-routing-implementation.md)
- [History replay and policy actions](p1-history-replay-policy-actions.md)
- [Accepted parity regression matrix](p1-telegram-history-presence-parity-regressions.md)
- [Transmission rollout handoff](p1-transmission-rollout-handoff.md)
- [Protocol implementation entry point](../protocol.md)
- [Deferred manual-test plan](../../.planning/260714_045154_epic-260714-th54l3.md)

The accepted engineering matrix covers ordering, callback authorization and
races, attachment proof, mixed capability, history tenant/action isolation,
presence redaction, DND/block reasons and EN/RU semantic parity. It explicitly
leaves real-client, audible, packaged-app and physical-hardware observations
unpassed.
