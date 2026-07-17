# P3 Telegram soundboard and automation parity

Task: `TASK-260712-uht9e2`

## Accepted engineering boundary

The Telegram adapter exposes two private-chat commands without becoming an
automation dependency:

- `/soundboard` lists the current primary's ordered, active saved cues. Cue
  selection opens a second short-lived picker for own Barycenter, current Air,
  or opaque explicit target references and for overlay or after-current
  delivery. The final click calls the same `TriggerManualSoundboard` service as
  desktop clients.
- `/automation` displays feature state, each owned schedule's enabled state and
  next canonical local occurrence. It can enable or disable the exact displayed
  schedule revision and invoke the common emergency-disable mutation.

Telegram does not receive an application bearer or automation principal
secret. The trusted adapter supplies its existing `IdentityTelegram` proof to
the common primary-only control boundary. The soundboard trigger still resolves
current membership, target references, Air policy, DND/block policy, media
state, target credentials and capabilities inside the canonical transaction.
The emergency mutation disables armed schedules through the existing control
service and remains visible in the common automation audit/history model.

## Callback safety

`callback_data` contains only a random `tg1_` capability. The server-side row is
bound to actor, orbit, role, chat, message, cue/schedule/feature revisions,
target reference and delivery choice. A claim re-resolves the Telegram actor,
atomically consumes one token and fences Telegram query IDs. Foreign,
forwarded, expired, role-changed, concurrent and repeated claims receive only a
finite generic outcome and never receive the binding. Domain failures are
finalized to the same opaque callback result vocabulary.

The schema is additive. Older binaries ignore the callback tables; new binaries
initialize them after the existing cue and automation tables. Callback-token
collision checks cover inline media, history, Air and automation namespaces.

## Automated evidence

The focused tests cover:

- command parsing and private-chat enforcement;
- opaque cue selection and explicit route callbacks;
- a Telegram-origin manual soundboard trigger reaching durable scheduler work;
- schedule enable and emergency disable through canonical services;
- current-role, chat/message, expiry, forged-token, replay and concurrent-click
  rejection;
- schedule next-run ordering across a DST fall-back fold;
- unchanged desktop-owned feature and schedule revisions when the bot prompt
  transport is unavailable.

The repository acceptance gate runs the complete Go suite, vet/race coverage,
cross-platform clients and rollback checks. No microphone/capture entry point is
called by these commands.

## Deliberately unclaimed evidence

Real Telegram delivery, callback timing, client rendering, actual audible
playback, signed application behavior and physical-device observations remain
manual work in `EPIC-260714-th54l3`. This task claims best-effort code and
automated tests only.
