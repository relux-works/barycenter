# P2 streamed-track wire contract v1

Task: `TASK-260712-31rkpe`

Executable contract: [`protocol/stream-track-v1.json`](../../protocol/stream-track-v1.json)
and [`protocol/golden/stream_*.json`](../../protocol/golden/).

## Safety boundary

`stream_track_v1` is an additive protocol capability, not a claim that a
production player exists. The accepted `p2-codec-player-adr-handoff.v1`
decision remains no-go: current macOS and Windows production compositions do
not advertise this capability, register a decoder, generate variants or play
streamed tracks. They explicitly reject a stream command received contrary to
capability negotiation.

The server selects one pinned canonical profile. `stream_load` carries an
opaque server-issued manifest plus a credential-free authenticated range path,
strong ETag, SHA-256 and byte size. Nodes cannot request a codec, supply a
storage key or decode the original upload. The bounded `svm1.*` manifest value
is metadata identity only: it grants no access and carries no credential.

## Messages

Every stream message is bound to `stream_id`, `playback_generation` and
`seek_generation`. Coordinator commands additionally carry a strictly
increasing `command_sequence`; node events carry a strictly increasing
`event_sequence`.

| Direction | Type | Additional purpose |
|---|---|---|
| coordinator → node | `stream_load` | Opaque pinned manifest, integrity, start position, readiness barrier/deadline and mixed-version policy. Starts a new playback generation at seek generation zero. |
| coordinator → node | `stream_resume_at` | Arms exact-generation playback at coordinator time; never valid before `stream_ready`. |
| coordinator → node | `stream_seek` | Advances seek generation exactly once and establishes a new readiness barrier. |
| coordinator → node | `stream_pause` | Pauses only the exact active generation with a bounded fade. |
| coordinator → node | `stream_cancel` | Requests terminal cancellation for the exact generation. |
| node → coordinator | `stream_ready` | Reports audible position and buffered duration for the exact generation. |
| node → coordinator | `stream_started` | Reports the coordinator-clock first audible sample. |
| node → coordinator | `stream_progress` | Reports audible, not decoder or download, position and remaining buffered duration. |
| node → coordinator | `stream_rebuffer` | Reports an empty/insufficient buffer; a fresh ready plus resume barrier is required. |
| node → coordinator | `stream_failed` | Bounded stage/code only; no URLs, paths, tokens or diagnostic prose. |
| node → coordinator | `stream_ended` | Terminal EOF/drained event for only the exact generation. |
| node → coordinator | `stream_cancelled` | Terminal cancellation acknowledgement for only the exact generation. |

## Barrier and ordering

- The frozen minimum buffer is 2,000 ms.
- Initial readiness expires 5,000 ms after load; post-seek readiness expires
  after 3,000 ms. A scheduled start has a 5,000 ms deadline.
- `stream_load` and `stream_ready` never start audio. Only an exact-generation
  `stream_resume_at`, accepted after the buffer barrier, may arm playback.
- A seek increments `seek_generation`, resets event ordering and discards all
  output from earlier seek generations, including ready, progress and terminal
  events.
- Equal sequence numbers are idempotent duplicates. Lower sequence or older
  generations are discarded. A gap, future generation or invalid phase is
  rejected rather than guessed through.
- Rebuffering requires another exact-generation ready event and a new resume
  command. Paused playback resumes through the same barrier-aware command.

The shared Go/Windows and Swift `StreamGenerationGuard` implementations freeze
these decisions and are tested against early start, duplicate load/pause,
sequence gaps, stale seek output and a terminal event from a replaced playback
generation.

## Mixed-version B6 policy

The sender explicitly chooses one policy before the target snapshot is queued:

- `require_all`: reject atomically if any frozen target lacks
  `stream_track_v1`.
- `supported_only_with_receipts`: queue only capable frozen targets and create
  a visible terminal `unsupported` receipt for each incapable target.

Unsupported nodes never receive stream messages. They do not block supported
targets under the second policy, but there is no clip/Spotify fallback, silent
success or autoplay after reconnect/upgrade. Capability changes affect only a
new sender decision and a new transmission snapshot.

## Compatibility

All 12 message types are additive under protocol major v1. The existing clip,
voice, Spotify, presence and Air goldens remain unchanged. Strict contract
tests decode and re-encode every one of the 51 goldens through coordinator Go,
the byte-identical Windows mirror and Swift.
