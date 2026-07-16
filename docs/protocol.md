# Protocol v1 — implementation notes

Normative source: spec ch. 8 (`docs/spec.md` v1.2). Golden files: `protocol/golden/*.json`, one per message type; contract tests on both sides decode -> re-encode -> compare against them (spec 8.7). Any protocol change lands together with the golden change in the same commit (goal invariant 5).

The additive, generation-safe `live_ptt_v1` signalling catalog, bounded binary
frame, mixed-version behavior and non-resume rules are frozen in the
[P3 live PTT wire contract](analysis/p3-live-ptt-wire-contract.md). Its
capability remains unadvertised until the later runtime and platform gates pass.
The coordinator implementation and its environment-only dark-launch boundary
are documented in the [bounded live PTT runtime handoff](analysis/p3-live-ptt-coordinator-runtime.md).

The normative Phase 3 soundboard/automation entry point, cue-only boundary,
principal scope, quiet-hours/DST rules, denial vocabulary and fail-closed
mixed-version behavior are frozen in the
[P3 automation safety contract](analysis/p3-automation-safety-contract-v1.md).
This contract adds no listener and advertises no capability by itself.

The exact additive phase-one clip-transmission, DND and presence payloads are
frozen in [`docs/analysis/p1-transmission-contract-v1.md`](analysis/p1-transmission-contract-v1.md)
and shipped in the canonical Go codec plus the Windows and Swift mirrors. The
files in `protocol/golden` remain the executable field-name contract.

The stable deploy, mixed-version, rollback and downstream-consumer entry point
is the [phase-one transmission rollout handoff](analysis/p1-transmission-rollout-handoff.md).
It does not replace the frozen contract.

The normative Phase 1 app/bot history, receipt, presence, DND/block and opaque
Telegram callback surface is frozen in the
[history/presence/Telegram contract](analysis/p1-history-presence-telegram-contract-v1.md).
Its transport-neutral RU/EN labels and privacy-safe metadata fallbacks are
defined by the [shared delivery presentation model](analysis/p1-shared-delivery-presentation-model.md).
The implemented Telegram transport boundary and automated evidence are described
in [P1 Telegram callback and audio transport](analysis/p1-telegram-callback-audio-transport.md).
The durable default/replacement transaction and inline action implementation are
described in [P1 Telegram inline routing](analysis/p1-telegram-inline-routing-implementation.md).
The implemented privacy projection and shared policy mutation boundary are
described in [P1 presence, DND and block implementation](analysis/p1-presence-dnd-block-surface.md).
The actor-scoped media/transmission projection, receipt authorization,
pagination and current-action derivation are described in
[P1 transmission history and receipt query](analysis/p1-transmission-history-receipt-query.md).
The cross-surface deterministic evidence map and its manual-validation boundary
are recorded in [P1 Telegram, history and presence parity regressions](analysis/p1-telegram-history-presence-parity-regressions.md).
The final deploy, mixed-version, drain/rollback and downstream-consumer entry
point is the [P1 Telegram, history and presence rollout handoff](analysis/p1-telegram-history-presence-rollout-handoff.md).

The normative Phase 2 create, saved-membership, active-pointer, invite,
joining-primary confirmation, policy, alias and single-authority rollback
surface is frozen in the
[Air lifecycle and policy contract](analysis/p2-air-lifecycle-policy-contract-v1.md).
Its executable enum, route, default and invariant summary is
[`protocol/air-lifecycle-policy-v1.json`](../protocol/air-lifecycle-policy-v1.json).

The shared Phase 2 Pulsar explicit-target, inbox, history, receipt-pagination
and fail-closed command model is defined by the
[Pulsar targets/inbox presentation model](analysis/p2-pulsar-targets-inbox-presentation-model.md)
and its executable
[`pulsar.targets-inbox-presentation.v1`](../protocol/pulsar-targets-inbox-presentation-v1.json)
contract. Native macOS and Windows views consume this model, while this
contract itself does not claim hands-on hardware evidence. The native
boundaries are documented in the
[macOS targets/inbox UI and command composition](analysis/p2-macos-targets-inbox-ui.md)
and [Windows targets/inbox UI and command composition](analysis/p2-windows-targets-inbox-ui.md)
handoffs. The repository-automated B5-B7 coverage and its explicit manual-test
boundary are recorded in the
[targets/inbox parity regression evidence](analysis/p2-targets-inbox-parity-regression-evidence.md).
Telegram consumes the same target and policy services through the opaque,
rollback-safe [Phase 2 Telegram explicit-target adapter](analysis/p2-telegram-explicit-target-parity.md).
The final coordinator-first deploy, mixed-version, rollback and downstream
extension rules are frozen in the
[Phase 2 targets/inbox rollout handoff](analysis/p2-targets-inbox-rollout-handoff.md).

The additive, generation-safe Phase 2 streamed-track messages, buffer barrier,
sender-selected mixed-version policy and codec no-go boundary are frozen in the
[P2 streamed-track wire contract](analysis/p2-stream-track-wire-contract-v1.md)
and its executable [`p2-stream-track-wire.v1`](../protocol/stream-track-v1.json)
contract. Landing these codecs does not advertise `stream_track_v1` in a
production node.

The shared long-file draft, processing, target-selection and playback control
surface is defined by the
[P2 streamed-track UI model](analysis/p2-stream-track-ui-model.md) and its
executable
[`pulsar.stream-track-ui-model.v1`](../protocol/pulsar-stream-track-ui-model-v1.json)
contract. It keeps upload, server processing and audible playback progress
distinct, retains unsent drafts through outages, consumes coordinator-owned
RU/EN labels and fences optimistic controls by playback and seek generation.
The [Windows streamed-track UI and bounded resumable intake](analysis/p2-windows-stream-track-ui.md)
consumes this model without weakening the production codec no-go.
The [macOS streamed-track UI and bounded resumable intake](analysis/p2-macos-stream-track-ui.md)
uses the same fail-closed projection, retains app-private drafts and requires a
separate rights confirmation for every upload attempt.
The final [streamed-track rollout, limits and operational handoff](analysis/p2-streamed-track-rollout-handoff.md)
freezes the no-go-aware rollout/rollback order, cache and quota ceilings,
operator metrics and Air/targets/inbox seams without claiming activation.
The candidate-neutral
[macOS streamed-track player](analysis/p2-macos-streamed-track-player.md)
implements the bounded range/cache/render lifecycle without registering a
production decoder. Native streamed-track views remain deferred to their
platform tasks.

This file records only the details the spec leaves to the implementation. It must never contradict the spec.

## Envelope

`{ "v": 1, "id": "msg_<ULID>", "ts": <int64 ms UTC>, "type": "<type>", "payload": { ... } }`

- `id`: `msg_` + ULID (26 chars, Crockford base32). Unique per sender, used in logs only.
- Unknown `type`: ignored with a warn log (spec 8.6). Unknown *fields* inside a known payload: rejected in contract tests (both codecs decode strictly) to catch drift early; at runtime the node/coordinator tolerate extras on decode (forward compatibility), strictness is test-only.
- Major `v` mismatch: node disconnects, error to log and chat (spec 8.1).

## Clarifications fixed by v1 (not in spec tables)

| Item | Decision |
|---|---|
| `state.playback` enum | `stopped \| loading \| paused \| playing \| voice \| wait` |
| `state.uri` | `null` when nothing is loaded |
| `state.position_ms` | audible position (spec 6.3), not daemon position |
| `welcome.session_snapshot` | `{ mode, state, current, volume }`; `current` is `null` or `{ element_id, kind, uri?, position_ms }`; `volume` is this node's 0..100 |
| `ended.reason` enum | `eof \| skipped \| error` (spec 8.4) |
| `error.code` enum | `load_failed \| track_unavailable \| media_download_failed \| audio_starvation \| librespot_restart \| device_lost \| invalid_dnd_revision`; the additive DND code is frozen by the phase-one transmission contract |
| `set_mode.mode` / snapshot `mode` | `shared \| solo` |
| Snapshot `state` | coordinator FSM state lowercase: `idle \| loading \| armed \| playing \| voice \| paused \| degraded` |
| Optional fields | `play_voice.t_coord_ms` (absent = start immediately), `error.element_id` (absent = not element-scoped). Absent = key omitted, not `null` |
| Timestamps / durations | int64 milliseconds; `t1/t2/t3` in ping/pong are sender-clock ms UTC per spec 8.5 |

## v1 additions beyond the spec ch. 8 catalog

Spec 9.1 requires `/offset` to be pushed to the node and `/offset_test` to fire
synchronized clicks, but ch. 8 lists no carrier messages. v1 adds (recorded in
UNRESOLVED_QUESTIONS as a spec v1.3 proposal, per goal invariant 6):

| type (coordinator -> node) | payload | Semantics |
|---|---|---|
| `set_offset` | `{ offset_ms }` | Set output_latency_offset_ms at runtime (spec ch. 14 calibration); node applies to all future scheduled starts |
| `offset_test` | `{ t_coord_ms, clicks, interval_ms }` | Play `clicks` clicks, first at T (coordinator clock), then every `interval_ms`; scheduled through the same T_local mechanism as resume_at |
| `load.adopt_playing` (optional) | `true` only for the initiating Pulsar | Relabel the Spotify stream that is already audible as the new element. The node sends `ready` but MUST NOT pause, clear the ring or reload the daemon. |
| `resume_at.position_ms` (optional) | audible position at `t_coord_ms` | Catch-up start: seek while paused, then resume at T. Ordinary all-paused starts omit it. |
| `external_playback` (node -> coordinator) | `{ uri, position_ms?, title? }` | In shared mode, Spotify started a track on this Pulsar outside the coordinator-owned element. `position_ms` is audible; `title` is the human `Artist — Track` label. The node ignores events whose go-librespot `play_origin` is `go-librespot`, so a stale coordinator load cannot masquerade as a phone choice. `/takeover user` keeps the initiating Pulsar playing and catches up followers; `/takeover coordinator` restores the existing element. |
| `user_pause` (node -> coordinator) | `{ element_id }` | Personal pause: the user paused THIS Pulsar in the Spotify app while the shared air was playing. The coordinator detaches only this home (barriers + subsequent elements); the broadcast continues for the others. A pause that would leave no active home degrades to the ordinary global pause. `element_id` is informational. |
| `user_resume` (node -> coordinator) | `{ element_id }` | Personal resume: play in Spotify on the same track returns this home to the air via the living-air catch-up (solo load at the live position). A different track instead goes through `external_playback` adoption. |

Seamless adoption is rollout-gated by the optional register capability
`seamless_adoption_v1`. The coordinator uses it only after every currently
participating peer in the air has announced support; mixed-version airs retain
the legacy pause/load/resume barrier. An offline peer joins later through the
ordinary load/seek/resume catch-up path and does not block adoption.

## Phase-one clip transmission, DND and presence

The application HTTP contracts and authorization boundaries are documented in
[the frozen transmission contract](analysis/p1-transmission-contract-v1.md),
[the history/receipt query](analysis/p1-transmission-history-receipt-query.md),
and [the history replay/policy command layer](analysis/p1-history-replay-policy-actions.md).

The following v1 message types are additive. Every clip lifecycle payload is
bound to `(transmission_id, generation)`; target identity comes only from the
authenticated WebSocket connection and is never accepted from a payload.

| Direction | Type | Payload fields |
|---|---|---|
| coordinator → node | `prepare_media` | `transmission_id`, `generation`, `media_id`, `kind`, `delivery`, `file_url`, `sha256`, `size_bytes`, `duration_ms`, `media_expires_at_coord_ms`, `prepare_deadline_coord_ms` |
| coordinator → node | `play_media_at` | `transmission_id`, `generation`, `t_coord_ms`, `start_deadline_coord_ms`, `delivery`; overlay adds `duck_db`, `attack_ms`, `release_ms`; interrupt adds `fade_out_ms`, `fade_in_ms` |
| coordinator → node | `cancel_media` | `transmission_id`, `generation`, `reason`, `action`, `resume_main`, `fade_ms` |
| coordinator → node | `presence_update` | `revision`, `generated_at_coord_ms`, sorted `nodes`; each node carries only the authorized availability, playback, DND and capability projection frozen by the transmission contract |
| node → coordinator | `media_ready` | `transmission_id`, `generation`, `decoded_duration_ms` |
| node → coordinator | `media_started` | `transmission_id`, `generation`, `t_first_sample_coord_ms` |
| node → coordinator | `media_ended` | `transmission_id`, `generation`, `t_last_sample_coord_ms`, `reason` |
| node → coordinator | `media_failed` | `transmission_id`, `generation`, `stage`, `code`; diagnostic text, URLs and local paths are forbidden |
| node → coordinator | `media_cancelled` | `transmission_id`, `generation`, `reason`, `action`, `main_resumed` |
| node → coordinator | `set_dnd` | `revision`, `mode`, and `muted_until_coord_ms` only for `muted_until` |

`play_media_at` never carries overlay and interrupt controls together.
`after_current` remains on the existing Session/legacy voice path and does not
receive `play_media_at` in phase one. Optional conditional fields are omitted,
not encoded as `null`. The complete enum vocabularies, timing bounds and
cross-field rules remain normative in the frozen contract linked above.

`register.capabilities` uses unique non-empty printable-ASCII strings in strict
ASCII order. Phase one defines `interrupt_resume_v1`, `media_clip_v1` and
`overlay_mix_v1` in addition to `seamless_adoption_v1`. Unknown sorted values
are retained for diagnostics but ignored by known-feature decisions. A
reconnect replaces the prior set instead of unioning it. A build advertises a
flag only after its implementation exists; therefore landing this wire codec
does not itself make either current node claim clip playback support.

`play_voice` and `solo_voice` remain registered, encoded and decoded exactly as
before. A mixed fleet can therefore downgrade a whole transmission into the
legacy Session path without splitting one transmission across protocols.

## Transport

WebSocket text frames, one JSON envelope per frame. Node initiates, first message `register`; invalid token -> close code 4401 (spec 8.2). Reconnect of the same `node_id` closes the previous connection (last-write-wins). Node heartbeat = `state` every 5 s; coordinator marks a node offline after 12 s without any message (spec 7.1 ws-hub).
