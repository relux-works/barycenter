# P1 Telegram callback and audio transport

Status: implemented by `TASK-260712-3dmllz`; transport boundary only.

The Telegram adapter now emits distinct events for callback queries, `audio`
attachments, and `document` attachments. Existing `voice` behavior remains
wire-compatible. Each event carries the Telegram update/message coordinates
needed by the coordinator, while callback actions and media classification stay
outside the transport parser.

## Attachment trust boundary

Filename, MIME type, declared duration, declared size, and Telegram's update
shape are hints. They are never proof that an attachment is audio or that it is
within a limit. Audio and document events enter the same bounded Telegram
download and common `SubmitMedia` normalization path as voice clips. Actual
bytes, signature, decoded streams, duration, and canonical size decide the
outcome.

Media groups are rejected before ingest because Telegram itself proves the
message is part of a group. Other stable Telegram-facing failures are translated
only from common-ingest proof:

```text
telegram_download_failed | telegram_media_too_large | attachment_not_audio |
media_group_not_supported_phase1 | track_not_available_phase1 |
decode_failed | duration_mismatch | canonical_output_too_large
```

The Phase 1 path never truncates a long attachment or silently turns it into a
track. A decoded clip over 180 seconds returns
`track_not_available_phase1`; Phase 2 track routing remains deferred.

## Opaque callbacks

`callback_data` is exactly `tg1_` plus 32 unpadded base64url characters: 24
random bytes and 36 UTF-8 bytes total. Only an HMAC-SHA-256 digest of the token
is indexed. Actor, orbit, role, chat/message/update coordinates, media ID and
generation, action, delivery, and audience remain in the server-side binding.

The transport registry enforces the 15-minute token lifetime and 24-hour query
dedupe window. It rechecks actor, role, source orbit, chat, and message before
dispatch. Private actions are initiator-only; an explicitly shared control may
also authorize the current primary of the source orbit. Unknown, malformed,
forged, wrong-message, and wrong-chat tokens collapse to `expired`; a wrong
actor, role, or orbit returns `forbidden`.

Each Telegram callback query is answered through `answerCallbackQuery` with a
finite localized result. Terminal handlers can clear the original inline
keyboard through `editMessageReplyMarkup`. Query IDs and callback tokens are
never added to logs or rendered errors. A duplicate query returns its stored
answer without a second mutation, while a consumed token returns
`already_applied` only to the same authorized actor.

The registry stores transport references, not transmission state. The durable
inline action router and its atomic default-versus-callback mutation are owned
by `TASK-260712-21ers7`. Until that router lands, authenticated callbacks receive
the honest terminal `unsupported` answer and their stale keyboard is removed.

## Automated evidence

Package and coordinator tests cover typed attachment updates, contradictory
metadata hints reaching common ingest, media-group rejection, stable failure
translation, the 36-byte opaque shape, keyed-hash-only token lookup, forgery,
expiry, cross-actor/cross-role/cross-orbit/cross-message rejection, source-primary
authorization, query replay, consumed-token replay, prompt answers, keyboard
cleanup, and HTTP/log redaction. No real Telegram client, audible playback, or
physical hardware evidence is claimed here.
