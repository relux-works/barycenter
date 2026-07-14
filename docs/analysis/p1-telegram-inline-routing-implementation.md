# P1 Telegram inline routing implementation

`TASK-260712-21ers7` implements the routing portion of
`p1-history-presence-telegram-v1` without adding another media or scheduling
domain.

## Runtime boundary

- A ready Telegram voice and its default `after_current` transmission are
  committed together. The immutable transmission acceptance time is the
  trusted Telegram intake time; readiness, presence, DND, block and capability
  policy are evaluated at publication time.
- Audio and document clips register the same durable routing choice but do not
  create a default. They remain explicit-action-only.
- Every accepted route uses `CreateResolvedTransmission` policy and the common
  scheduler. `after_current` continues through the existing legacy Session FSM
  bridge; there is no Telegram scheduler.
- A callback replacement and cancellation of the not-started voice default are
  one SQLite writer transaction. If any target has begun playback, the result
  is `too_late` and neither row set changes.

## Callback boundary

- Telegram sees only `tg1_` plus 32 base64url characters. SQLite retains an
  HMAC-SHA-256 digest and server-side actor, orbit, chat, message, update,
  media-generation, action, delivery and audience bindings.
- The clicking Telegram identity is resolved again for every callback. Tokens
  expire after 15 minutes and callback query outcomes are deduplicated for 24
  hours.
- Inline controls are attached only after Telegram returns the bot prompt's
  message ID, so the minted token is bound to the exact callback message. This
  asynchronous rendering never delays the committed voice default.
- An unavailable interrupt creates a durable common confirmation challenge and
  leaves the default armed. Only a second opaque callback selecting overlay or
  `after_current` can replace it.

## Compatibility and evidence boundary

Rollback-era legacy media rows still use the pre-existing voice enqueue path.
Generic `media_items` use the durable router. Automated coverage proves opaque
storage, exact query replay, default-vs-start behavior, concurrent callback
serialization and explicit interrupt fallback. Telegram client rendering,
audible playback and real-device behavior remain manual work in the dedicated
manual-validation epic.
