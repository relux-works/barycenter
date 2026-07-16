# P2 streamed-track coordinator flow

Task: `TASK-260712-2h6snp`

## Outcome and no-go boundary

The coordinator now has a candidate-neutral main-program state machine for an
uploaded streamed track plus additive durable queue transitions. It consumes
the accepted `stream_track_v1` wire types and never calls a decoder, chooses a
codec or treats a URL as authority.

The accepted codec/player ADR still selects no production profile. Therefore
the public `queue` and `replace` transmission path remains fail-closed, nodes
do not advertise `stream_track_v1`, and this task does not enable production
playback. Deterministic tests supply candidate manifests and targets directly
to prove coordinator semantics that a future reviewed replacement ADR can
activate without redesigning the scheduler.

## Provider-neutral main program

`StreamMainProgram` parks a `MainProgramSource {kind, ref}` beneath the stream
insert. The kind may identify the legacy shared session or Spotify, but the
stream state machine does not interpret the reference and has no
Spotify-shaped branch. Drained completion restores that opaque source; replace
cancels the current stream and starts the new generation without briefly
resuming the parked source.

The existing `Session` remains the sole owner of Spotify, clip overlay,
interrupt, voice and personal-pause behavior. Stream commands use separate
effect types, state and tests. No legacy element, playlist, clip queue or
Phase 1 snapshot row is rewritten.

## Queue, replace and restart persistence

The previously accepted `stream_playback_domains` and `stream_queue_items`
remain additive and do not copy target or membership authority. New
revision-checked transitions provide:

- oldest-queued activation only while a domain is idle; direct item activation
  cannot bypass FIFO;
- immediate replace, cancelling only the active item while preserving queued
  relative order; replace from idle or against a stale current item fails;
- monotonic audible progress, rebuffer state and seek generations;
- restart fencing through a strictly newer playback generation while keeping
  the last audible position;
- exact-generation completion to `played` or `cancelled`, followed by a
  separate oldest-queued activation;
- restoration of one current item, its queue, parked main source, audible
  position and playback/seek generations after reopening SQLite.

Every multi-row replace and completion mutation is transactional. A stale
revision, playback generation, seek generation or parallel activation rolls
back rather than producing two active items.

## Buffered scheduling and ordering

One `stream_load` is emitted per capable frozen target. Every node must report
the 2,000 ms minimum buffer before the coordinator emits a synchronized
`stream_resume_at`; the start uses the largest target RTT plus the existing
500 ms scheduling margin and the frozen 5,000 ms start deadline.

Commands and events pass through the shared `StreamGenerationGuard`. Equal
duplicates and older playback/seek generations are ignored. Sequence gaps or
future generations fail closed. A seek increments the seek generation,
discards pre-seek output, rebuilds the ready barrier and may move the audible
position backward. A paused seek reaches ready but cannot auto-resume.

The persisted position is the minimum audible position of started
participants. Downloaded bytes, decoder progress and buffered-ahead samples
never advance it. A rebuffer pauses healthy targets, waits for the empty target
to become ready and creates a fresh resume barrier. Load, seek and scheduled
start timeouts obey the frozen mixed-version policy:

- `require_all` fails and cancels the whole insert;
- `supported_only_with_receipts` drops only missing/failed nodes, emits visible
  terminal receipts and lets remaining frozen targets continue.

## Air lifecycle and completion

A living-Air join loads only the new capable home at the current audible
position and arms an individual ready/resume barrier. Existing participants
continue and no previous overlay is replayed. A leave sends `stream_cancel`
only to the leaver; the shared main track continues for remaining homes.

The coordinator accepts `stream_ended` only from the started exact generation
with reason `eof_drained`. It advances after every remaining participant has
reported that terminal drained event, so decoder EOF or downloaded completion
alone cannot end the main program. A catch-up join that has not produced its
first audible sample cannot hold an already-drained program open: it is
cancelled before completion. A failure after every other participant drained
also closes the insert, and an empty living Air remains fail-paused rather than
arming a zero-target resume.

Coordinator restart is fail-paused. Restoration emits no wire command. A
user/runtime resume first advances the persisted playback generation, reloads
at the last audible position and thereby makes every pre-restart event stale.

## Evidence

The deterministic suite covers all-target and partial-capability admission,
queue/replace ordering, exact synchronized ready/start barriers, audible-min
progress, pause/seek/resume, stale pre-seek output, rebuffer recovery,
load/start timeout policy, runtime failure, Air catch-up and leave, drained
completion, invalid replacement rollback, SQLite restart and explicit legacy
Spotify/clip-session non-mutation:

```sh
cd coordinator
go test ./internal/session -run TestStreamMainProgram -count=1
go test -race ./internal/session -run TestStreamMainProgram -count=1
go test ./internal/store -run TestStreamPlayback -count=1
go test -race ./internal/store -run TestStreamPlayback -count=1
go test -tags previoushead ./internal/store -run TestMediaIngestExactPreviousHeadRollback -count=1
```

Hands-on buffering, audible timing, cache behavior, device leave/rejoin and
one-hour playback remain in the separate manual testing epic. Platform player
tasks may consume this FSM and its effects in candidate tests, but production
composition remains locked until the codec/player decision changes through a
reviewed ADR.
