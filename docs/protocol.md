# Protocol v1 — implementation notes

Normative source: spec ch. 8 (`docs/spec.md` v1.2). Golden files: `protocol/golden/*.json`, one per message type; contract tests on both sides decode -> re-encode -> compare against them (spec 8.7). Any protocol change lands together with the golden change in the same commit (goal invariant 5).

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
| `error.code` enum | `load_failed \| track_unavailable \| media_download_failed \| audio_starvation \| librespot_restart \| device_lost` (spec 8.4) |
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
| `external_playback` (node -> coordinator) | `{ uri }` | U9: in shared, the daemon started playback not belonging to the broadcast (partner's phone). Coordinator applies the takeover policy (/takeover user\|coordinator) |

## Transport

WebSocket text frames, one JSON envelope per frame. Node initiates, first message `register`; invalid token -> close code 4401 (spec 8.2). Reconnect of the same `node_id` closes the previous connection (last-write-wins). Node heartbeat = `state` every 5 s; coordinator marks a node offline after 12 s without any message (spec 7.1 ws-hub).
