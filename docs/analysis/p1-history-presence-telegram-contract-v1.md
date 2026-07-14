# Phase 1 history, presence and Telegram action contract v1

- Date: 2026-07-14
- Task: `TASK-260712-3coble`
- Story: `STORY-260712-34kbkn`
- Contract version: `p1-history-presence-telegram-v1`

This document is normative for the Phase 1 history and receipt read models,
presence and DND/block mutations, and Telegram inline routing. It specializes
the accepted identity, media and transmission contracts; it does not replace
their authorization, lifecycle, retention, capability or receipt rules.

`MUST`, `MUST NOT`, `SHOULD` and `MAY` have their RFC 2119 meanings. This is an
automatable engineering contract. Real-app and physical-device acceptance
remains in `EPIC-260714-th54l3`.

## 1. Shared invariants

### 1.1 Authority and representation

- Every app request resolves the existing bearer credential to one active
  `ActorContext`. Telegram resolves the clicking user again at callback time;
  chat membership and a Telegram numeric ID alone grant nothing.
- Caller-supplied actor, role, orbit, slot, acceptance time, receipt, DND
  ownership or block ownership fields are forbidden.
- JSON requests are strict: unknown, duplicate and explicit-null fields,
  trailing values and bodies over 8 KiB are `400 invalid_request`. Optional
  fields are omitted. HTTP timestamps are RFC 3339 UTC with milliseconds.
- Public actor and source-orbit subject references use viewer-scoped opaque
  values beginning `ar_` and `or_`. They are not credentials and cannot be
  used outside the viewer's authorized domain. Numeric actor IDs are never
  returned; numeric orbit IDs remain display/routing context, never block
  mutation authority.
- Public history IDs are `hi_` plus a 26-character Crockford ULID. Block IDs
  are `bl_` plus the same ULID shape. The coordinator creates both.
- Visible names are UTF-8, whitespace-normalized and capped at 120 Unicode
  scalar values. Presentation may localize labels, never enums or reasons.

### 1.2 Sources of truth

The media repository owns processing, ready, failed, delete and expiry state.
The transmission repository owns accepted ordering, requested/effective
delivery, target snapshots and receipts. Presence owns liveness and mirrored
node-local DND; orbit policy owns orbit DND and blocks. History is a projection
over those stores and cannot mutate or reinterpret them.

`docs/analysis/p1-transmission-contract-v1.md` remains authoritative for the
aggregate/target vocabulary, block-before-DND-before-offline precedence,
effective DND, cancellation and interrupt confirmation, 30-day receipt
retention, media-byte revocation and trusted acceptance order.

## 2. Phase 1 history HTTP API

### 2.1 Routes and pagination

```text
GET /v1/history?view=all&limit=30&cursor=opaque
GET /v1/history/{history_item_id}
```

`view` is optional: `all`, `sent`, or `received`; default `all`. `limit` is
optional, defaults to 30, and is 1 through 100. `cursor` is absent on the first
page. Unknown/repeated parameters are `400 invalid_request`; detail accepts no
query.

Rows sort by `(occurred_at DESC, history_item_id DESC)`. Page one freezes an
upper bound and authorization/filter fingerprint. `next_cursor` is a server-
issued AEAD-protected opaque value bound to actor, credential scope, view,
limit, upper bound and last key, expiring after 24 hours. It cannot contain
readable actor, orbit, transmission or media IDs. Wrong-actor, changed-filter,
malformed, expired and post-revocation cursors all return `400
history_cursor_invalid`. Newer inserts do not shift subsequent pages and a
retained row appears at most once.

```json
{
  "contract":"p1-history-presence-telegram-v1",
  "items":[
    {
      "history_item_id":"hi_01J00000000000000000000000",
      "item_kind":"transmission",
      "direction":"sent",
      "occurred_at":"2026-07-14T07:10:11.123Z",
      "media":{
        "media_id":"m_01J00000000000000000000000",
        "kind":"voice_clip","title":"Voice message",
        "duration_ms":4200,"content_available":true
      },
      "sender":{
        "actor_ref":"ar_opaque-viewer-scoped",
        "display_name":"Ivan","source_orbit_id":42,
        "source_orbit_ref":"or_opaque-viewer-scoped"
      },
      "audience":{"kind":"current_air","target_count":2},
      "requested_delivery":"overlay","effective_delivery":"overlay",
      "status":"played","reason_code":"completed",
      "target_counts":{"played":2,"other":0},
      "actions":["delete","replay"]
    }
  ],
  "next_cursor":"opaque-or-omitted"
}
```

`next_cursor` is omitted on the final page. `direction` is `sent` for source-
orbit visibility, `received` for target-only visibility, and
`sent_and_received` when both apply. Compact `target_counts` has exactly
`played` and `other`; detail contains the complete frozen counts. A blocked
sender sees only `blocked`, never the recipient or block scope.

### 2.2 Media-only items

An owner-visible media row exists while ingest has no transmission. It is
hidden once the first linked transmission commits, preventing duplicate media
and transmission rows for one action.

```json
{
  "history_item_id":"hi_01J00000000000000000000001",
  "item_kind":"media","direction":"sent",
  "occurred_at":"2026-07-14T07:09:00.000Z",
  "media":{
    "media_id":"m_01J00000000000000000000001",
    "kind":"audio_clip","title":"Doorbell",
    "duration_ms":1800,"content_available":true
  },
  "status":"ready","actions":["delete","replay"]
}
```

Media status is exactly `processing`, `ready`, or `error`. `error` requires a
sanitized common media `reason_code`; `processing` omits duration and content
availability and exposes only `delete`. This lets app and bot present
processing through ready without inventing receipts. Failed media follows
media retention, not 30-day transmission retention.

### 2.3 Transmission detail and receipt authorization

```json
{
  "contract":"p1-history-presence-telegram-v1",
  "history_item_id":"hi_01J00000000000000000000000",
  "item_kind":"transmission","direction":"sent",
  "transmission_id":"tr_01J00000000000000000000000",
  "occurred_at":"2026-07-14T07:10:11.123Z",
  "accepted_at":"2026-07-14T07:10:11.123Z",
  "expires_at":"2026-07-14T07:15:11.123Z",
  "requested_delivery":"overlay","effective_delivery":"after_current",
  "downgrade_reason":"mandatory_target_missing_overlay_capability",
  "status":"partial","reason_code":"partial_delivery",
  "target_counts":{
    "accepted":0,"preparing":0,"ready":0,"scheduled":0,"playing":0,
    "cancelling":0,"played":1,"missed_offline":1,"missed_dnd":0,
    "missed_not_ready":0,"blocked":0,"failed":0,"cancelled":0,"expired":0
  },
  "targets":[{
    "orbit_id":42,"slot":"a","status":"played","reason_code":"completed",
    "started_at":"2026-07-14T07:10:12.621Z",
    "ended_at":"2026-07-14T07:10:16.812Z"
  }],
  "actions":["delete","replay"]
}
```

Target omission rules are transmission contract section 4. Creator/current
source primary sees all rows. Other current source control actors see
aggregates plus rows bound to themselves. A recipient sees only its exact
current actor/slot/binding snapshot. Approach membership alone grants no
history. Unknown and unauthorized IDs both return `404 history_not_found`.

For a public blocked row, `reason_code` is omitted unless the viewer owns that
recipient block. No response includes credential generation, connection ID,
IP, hostname, process/microphone state, path, media URL, Telegram identifier,
callback material or moderation-only evidence.

### 2.4 Retention and action visibility

Transmission history is visible for 30 days from `accepted_at`. Delete/expiry
sets `content_available=false` and removes `delete` and `replay`, while
content-free receipt evidence remains. Exact ordered action vocabulary:

```text
cancel | delete | replay | report | block_actor | block_orbit | unblock
```

Actions are hints, not grants; mutations reauthorize. `cancel` is only for the
creator/current source primary while `can_cancel`. `delete` is for current
media owner/control. `replay` requires current create authority and available
content. `report` requires creator or exact target evidence access.
`block_actor`/`block_orbit` require a received foreign subject and no matching
block; `unblock` requires block ownership.

Replay always creates a new transmission with a new trusted `accepted_at` and
current audience/capability/DND/block checks. It never reuses a receipt or
autoplays a missed delivery. Phase 1 has no offline inbox.

## 3. Presence, DND and block HTTP API

### 3.1 Sanitized presence

`GET /v1/presence` accepts no query and returns only the caller's own orbit and
current pairwise approach domain.

```json
{
  "contract":"p1-history-presence-telegram-v1",
  "revision":91,"generated_at":"2026-07-14T07:10:14.000Z",
  "orbit_dnd":{"mode":"allow_all","revision":4},
  "nodes":[{
    "orbit_id":42,"slot":"b","online":true,
    "last_seen_at":"2026-07-14T07:10:13.800Z",
    "output_state":"ready","playback_state":"main",
    "local_dnd":{"mode":"messages_only","revision":8},
    "effective_dnd":{"mode":"messages_only","source":"local"},
    "capabilities":["interrupt_resume_v1","media_clip_v1","overlay_mix_v1"],
    "interrupt_resume_ready":true
  }]
}
```

Nodes sort by `(orbit_id, slot)` and capabilities by ASCII. Effective `source`
is `none`, `local`, `orbit`, or `local_and_orbit`. An active mute carries
`muted_until`, omitted otherwise. Offline and sanitization behavior is exactly
transmission section 7.3. Presence is never download/addressing authority.

### 3.2 DND mutation and precedence

```text
PUT /v1/presence/dnd/local
PUT /v1/presence/dnd/orbit
```

```json
{"expected_revision":8,"mode":"muted_until","muted_until":"2026-07-14T11:00:00.000Z"}
```

Mode is `allow_all`, `messages_only`, or `muted_until`. `muted_until` is
required only for that mode, after coordinator now and at most 30 days ahead.
`expected_revision=0` is only for an absent layer; otherwise it equals current
revision and server commits +1. Both routes require an `Idempotency-Key` with
the transmission-contract syntax and actor scope. Exact key/body retry returns
the same response. Stale/different writes are `409 dnd_revision_conflict` with
current sanitized layer. Success returns the changed layer:

```json
{
  "scope":"local","mode":"muted_until",
  "muted_until":"2026-07-14T11:00:00.000Z",
  "revision":9,"changed":true
}
```

Local requires exact installation control and can alter only its own slot; it
is the HTTP equivalent of generation-safe `set_dnd`. Orbit requires current
same-orbit primary; verified Telegram primary calls the same service.
Companion, satellite, approach peer and sender are denied.

Effective DND is the stricter layer: active `muted_until` > `messages_only` >
`allow_all`; between mutes, later timestamp wins. Expired mute is `allow_all`.
`messages_only` permits user-authored `voice_clip`/`audio_clip` but suppresses
built-ins, automation and later track controls. No request or callback has
force, priority, emergency or DND bypass. Remote state may tighten but never
loosen local DND.

The public reason vocabulary is fixed: a suppressed non-started target is
`missed_dnd/local_dnd` or `missed_dnd/orbit_dnd`; tightening DND during active
playback is `cancelled/dnd_enabled`. A sender may see the reason category but
cannot mutate or bypass the recipient layer that produced it.

### 3.3 Block mutation

```text
GET /v1/blocks
POST /v1/blocks
DELETE /v1/blocks/{block_id}
```

Create requires `Idempotency-Key` and an authorized history subject:

```json
{"scope":"actor","subject_ref":"ar_opaque-viewer-scoped"}
```

Scope is `actor` or `orbit`. An actor owns personal recipient blocks; current
primary may also own its recipient-orbit blocks. Approach membership, sender
and moderator roles grant no block ownership. Exact retry/already-active same-
owner returns `200 reused=true`; first create is `201`. Out-of-domain subjects
collapse to `404 block_subject_not_found`.

`scope=actor` requires the `ar_` reference and `scope=orbit` requires the `or_` reference
from that same authorized history projection. Cross-kind
references are `400 invalid_request`; numeric IDs are never accepted here.

List returns only blocks owned/administered by the current actor context,
ordered `(created_at DESC, block_id DESC)`:

```json
{
  "blocks":[{
    "block_id":"bl_01J00000000000000000000000",
    "scope":"actor","subject_ref":"ar_opaque-viewer-scoped",
    "display_name":"Sender","created_at":"2026-07-14T07:11:00.000Z",
    "revision":1
  }]
}
```

Create returns the same row plus `reused`; delete returns
`{"block_id":"...","changed":true}`. Display names are presentation only and
cannot be sent back as selectors.

Delete is idempotent for its owner (`changed=false` after first success).
Another owner's, unknown and guessed IDs return `404 block_not_found`. A new
block uses frozen acceptance/pre-start/active fade-stop behavior. Unblock never
resurrects, requeues or replays a terminal transmission.

The exact public outcomes are `blocked/actor_blocked` or
`blocked/orbit_blocked` before playback and `cancelled/sender_blocked` for an
active fade-stop. Sender-facing blocked detail omits which scope caused it.

## 4. Telegram attachment matrix

Telegram ingest calls the common media service and never owns a second media
or scheduler state machine.

| Update | Phase 1 result |
| --- | --- |
| `voice` | `voice_clip` after bounded download and common normalization |
| `audio`, probed <=180 s | `audio_clip` |
| audio-signature `document`, probed <=180 s | `audio_clip` |
| audio/audio-document >180 s | `track_not_available_phase1`, never truncate/relabel |
| non-audio document/video/animation/sticker | `attachment_not_audio` |
| media group/multiple attachments | `media_group_not_supported_phase1` |

Telegram source is capped at 20 MiB; common canonical output remains 34 MiB
and 180 seconds. MIME, extension, declared duration/size are hints; bounded
bytes, signature and decoded probe decide. A clip over 60 seconds can use
interrupt/after-current, while overlay returns `overlay_duration_exceeded`.

Stable sanitized failures:

```text
telegram_download_failed | telegram_media_too_large | attachment_not_audio |
media_group_not_supported_phase1 | track_not_available_phase1 |
decode_failed | duration_mismatch | canonical_output_too_large
```

Phase-2 track, queue, replace, offline inbox and late autoplay are never
offered. There is no Spotify fallback and no Telegram file ID in an error.

## 5. Opaque Telegram callback contract

### 5.1 Token and binding

Telegram `callback_data` is opaque `tg1_` plus 32 unpadded base64url characters
(192 random bits): 36 UTF-8 bytes, below Telegram's 64-byte limit. Coordinator
stores only a keyed token hash and server-side row. No raw actor, orbit, chat,
message, media, transmission, delivery or credential value is encoded.

The row binds initiating actor/source orbit; authorization mode
`initiator_only` or explicit `source_primary`; Telegram chat/message and
original update; media ID/generation and optional legacy default; one exact
action/step and canonical options; creation, 15-minute expiry, use state and
outcome digest.

Private buttons are always initiator-only. Group buttons may allow current
source primary only on an explicitly shared-orbit control surface. Group
membership is insufficient. Every click resolves actor again and verifies
identity, role, orbit, chat/message, media generation and token hash in
constant time. Tokens/query IDs are redacted from logs and errors.

### 5.2 Replay and answer vocabulary

Telegram callback query IDs are deduplicated for 24 hours. Same query returns
stored answer without another mutation. A terminal token is consumed with its
mutation. Another query to a consumed token returns `already_applied` only to
the same authorized actor. Wrong actor/role/orbit is `forbidden`; expired or
superseded is `expired`; malformed, unknown, wrong-chat/message and forged all
collapse to `expired` so the token is not an oracle.

```text
applied | already_applied | requires_confirmation | too_late |
expired | forbidden | unsupported | failed
```

`answerCallbackQuery` is prompt for every result. Presentation localizes text;
the code is audited. `failed` is non-disclosing and retryable only while the
token remains unused.

### 5.3 Actions and interrupt confirmation

```text
choose_overlay | choose_interrupt | choose_after_current |
choose_own_barycenter | choose_current_air |
confirm_overlay | confirm_after_current | dismiss
```

Audience buttons mint another short-lived server-side row; they carry no IDs.
Interrupt calls the ordinary service. If exact interrupt is available,
replacement commits. On `requires_confirmation`, no transmission is created
and legacy default remains queued. Bot replaces keyboard with exact available
fallback tokens. Confirmation rechecks authority/capability and uses the
server-stored transmission confirmation token, never callback data.

If default starts while confirmation is pending, confirm is `too_late` and
schedules nothing. `dismiss` consumes only UI and leaves default untouched.

## 6. Legacy default and callback race

### 6.1 Immediate default

When Telegram media becomes ready, one transaction immediately creates the
legacy `after_current` transmission using existing personal/broadcast default,
records trusted `accepted_at`, and creates routing-choice generation 1. Inline
keyboard rendering/delivery is not on this critical path. There is no decision window;
no click means unchanged legacy order.

### 6.2 Atomic pre-start replacement

A valid terminal choice locks routing-choice and default transmission. It may
replace only while no default target durably committed `playing` or `played`.
One transaction consumes token/generation, commits default sender cancellation
as existing `cancelled/sender_cancelled`, creates exactly one replacement via
ordinary service, assigns a new coordinator `accepted_at`, and stores private
replacement links for history/audit without changing receipt enums.

Old FIFO position is never inherited. Choosing existing default mode/audience
is `already_applied` and preserves acceptance time. If replacement wins before
a late node start, existing cancel/start handling disarms or fade-stops stale
default. If start commits first, callback is `too_late`, cancels nothing and
creates nothing. Thus a race produces one audible transmission, never both.

Concurrent clicks serialize on choice generation: one applies, others get
`already_applied` with sanitized outcome. Restart restores unexpired durable
choices, creates no second default and never extends expiry.

## 7. Error and audit map

| HTTP | Code | Meaning |
| --- | --- | --- |
| 400 | `invalid_request` | strict JSON/query/ID failure |
| 400 | `history_cursor_invalid` | cursor malformed/expired/mismatched |
| 401 | `unauthorized` | bearer absent/invalid |
| 403 | `insufficient_capability` | wrong role/scope |
| 404 | `history_not_found` | item unknown or invisible |
| 404 | `block_subject_not_found` | subject outside authorized history |
| 404 | `block_not_found` | block unknown/invisible/other owner |
| 409 | `dnd_revision_conflict` | optimistic revision mismatch |
| 409 | `transmission_state_conflict` | action became too late |
| 422 | `track_not_available_phase1` | Phase 2 track required |
| 429 | `too_many_attempts` | actor/orbit rate limit |
| 500 | `internal_error` | sanitized server failure |

Every DND/block mutation and callback decision appends actor, scope, action,
result, object generation and coordinator time to privacy-safe audit. It never
records raw callback data, Telegram file/query IDs, bearer material, media
URL/path, microphone state or private transport cause.

## 8. Downstream conformance

Downstream tasks implement, in order: shared localized presentation; Telegram
callback/audio/document transport; presence/DND/block surface; history and
receipt queries; durable inline routing; policy actions; parity regressions;
and rollout handoff. Tests must prove forgery, replay, expiry, group click,
restart, default/start race, stable pagination, DND precedence and
sanitization.

No task may add readable callback fields, decision delay before default,
inherited FIFO time, group-member authority, raw actor IDs, late autoplay, DND
bypass, receipt relabeling or a second Telegram scheduler.
