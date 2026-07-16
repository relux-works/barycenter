# P2 Telegram explicit-target parity

`TASK-260712-wt2n7m` keeps Telegram as a presentation adapter over the common
media, policy, target-resolution, transmission, inbox and moderation services.
It does not add a Telegram-owned recipient resolver, playback queue or delivery
fallback.

## File intake and rights

- Voice, audio and document updates still enter the bounded common
  `SubmitMedia` path. Telegram metadata remains an untrusted hint.
- Audio and document updates require the current version/hash content-policy
  grant and a per-upload `rights` or `права` caption. The caption is the
  Telegram equivalent of app API `rights_acknowledged=true`; it may be combined
  with `лично`, `всем` or `all`.
- The acknowledgement is a gate, not proof of ownership. The prompt explicitly
  preserves the sender's rights, permission and recording-consent duties.
- A source that needs the long-track path receives the accepted production
  codec no-go message. It is never silently converted into a short clip.

## Target and delivery picker

The adapter asks the common target-reference service for actor- and
authorization-generation-bound `trf_…` capabilities. It renders localized
Barycenter/Pulsar labels, while Telegram callback data contains only a random
`tg1_…` token. A Barycenter reference resolves to its exact current installation
set, so one explicit choice can produce an N-target immutable snapshot. The
picker also binds whether the originating Pulsar is included.

Existing voice defaults remain unchanged through the shared contract:

- immediate legacy voice uses durable `after_current` before the keyboard is
  sent;
- current-Air and personal routing are resolved by the common Air/explicit
  target services;
- audio/document updates have no hidden autoplay default.

Overlay, interrupt and after-current choices call the common transmission
service. Queue/replace values are represented by the Phase 2 callback contract,
but the currently accepted streamed-track production no-go remains
authoritative: targeted tracks fail closed as `unsupported` and cannot fall
back to clip delivery or transport-owned queue logic.

## Callback and rollback safety

Every callback is bound server-side to actor, orbit, chat, message, media
generation, action, delivery, audience, target capability, origin policy and
expiry. The common target service re-resolves Air membership and installation
generation in the same create transaction. A foreign, stale, forwarded or
re-paired target capability therefore cannot address a recipient.

Phase 2 values use the additive `telegram_inline_callback_routes_v2` companion.
Its parent callback deliberately carries the old but non-executable
`choose_own_barycenter` marker. The immediately preceding binary can open the
database, but treats a new callback as unsupported instead of reinterpreting an
explicit target or queue/replace request as a Phase 1 broadcast.

Capability errors render human names only. They never expose numeric actor,
orbit or slot authority, opaque target references, credential hashes or wire
capability identifiers. Terminal and unsupported outcomes promptly answer the
Telegram query and remove the obsolete keyboard.

## Automated evidence

Repository tests cover:

- common voice/audio/document intake and current-policy gating;
- per-upload Telegram rights acknowledgement and combined routing captions;
- opaque explicit Barycenter selection and `include_origin=false` snapshot;
- actor-bound target references and foreign-reference denial;
- additive fail-closed rollback envelope;
- targeted-track no-go without transmission creation;
- human-readable mixed-capability errors without opaque identifier leakage;
- existing callback actor/chat/message/TTL/replay/concurrency protections;
- existing Air, inbox replay/delete/report/block and legacy voice regressions.

Real Telegram clients, real recipients, packaged applications, mixed physical
fleets and audible playback remain manual-only in
`TASK-260712-3u5cdn` under `EPIC-260714-th54l3`. This task claims no such result.
