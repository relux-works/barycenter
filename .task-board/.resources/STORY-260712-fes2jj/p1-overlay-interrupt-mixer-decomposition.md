# STORY-260712-fes2jj decomposition

## Scope anchor

This story owns the node-side mixer and playback work behind spec sections 9.2,
10.1 and 10.2:

- independent clip branches on Windows and macOS instead of the legacy
  voice-replaces-music path;
- continuous main-program consumption during overlay, with shared ducking and
  limiter behavior;
- interrupt pause and resume from audible position without ghost timers or
  timeline drift;
- render-safe clip preparation, cancellation and mixer telemetry;
- deterministic tests plus live-path A3 and A4 evidence.

Current implementation inspection drove the split:

- macOS PlayerCore.playVoice clears the ring, pauses librespot and plays the
  clip as a replacement insert;
- Windows Player.playVoice and Engine.Render do the same, and Engine.Render
  still takes a mutex on the render path;
- existing tests cover legacy voice replacement and pipe integrity, but not
  additive overlay, limiter behavior, cancellation or 100-overlay soak.

This story does **not** own coordinator ordering, target receipts, protocol
schema or ready-barrier policy beyond the node-side adoption points needed for
playback.

## Created tasks

1. TASK-260712-1hqiek - Refactor node clip control paths for render-safe prepared media state
2. TASK-260712-2zbmq4 - Add macOS prepared clip overlay branch with ducking and limiter
3. TASK-260712-1viwvi - Refactor Windows engine for additive overlay mixing, ducking and limiter
4. TASK-260712-8mwyiv - Implement macOS interrupt pause and audible-position resume
5. TASK-260712-1g6lk8 - Implement Windows interrupt pause and audible-position resume
6. TASK-260712-3d6cnn - Add deterministic overlay, interrupt and limiter regression coverage
7. TASK-260712-2hodti - Capture Windows and macOS A3 and A4 live evidence and reviewer handoff

## Within-story dependency graph

- TASK-260712-2zbmq4 blocked by TASK-260712-1hqiek
- TASK-260712-1viwvi blocked by TASK-260712-1hqiek
- TASK-260712-8mwyiv blocked by TASK-260712-1hqiek, TASK-260712-2zbmq4
- TASK-260712-1g6lk8 blocked by TASK-260712-1hqiek, TASK-260712-1viwvi
- TASK-260712-3d6cnn blocked by TASK-260712-2zbmq4, TASK-260712-1viwvi, TASK-260712-8mwyiv, TASK-260712-1g6lk8
- TASK-260712-2hodti blocked by TASK-260712-3d6cnn

Execution intent:

- Land render-safe state ownership and shared parameter carriers first.
- Build macOS and Windows overlay branches in parallel once that control
  contract exists.
- Add interrupt pause or resume separately per platform on top of the new clip
  branch.
- Finish with deterministic regression coverage, then live A3 and A4 evidence
  and reviewer handoff.

## Cross-story dependencies

- STORY-260712-25lysg - P1 Transmission protocol and scheduler
  - Owns prepare_media, media_ready, play_media_at, cancellation semantics,
    overlay FIFO ordering, capability negotiation and downgrade behavior. This
    story only adopts those commands on the nodes and must not duplicate
    coordinator ownership.
- STORY-260712-ld674h - P1 Generic media ingest and storage
  - Supplies canonical WAV media, authenticated node download, hash or duration
    metadata and delete behavior consumed by clip preparation.
- STORY-260712-30ju1k - P1.0 Windows Store platform spike
  - Supplies the named-pipe, WASAPI and AppContainer constraints that the
    Windows overlay and live-validation tasks must stay inside.
- STORY-260712-1i0doc - P1 Store compliance and acceptance
  - Consumes the Windows and macOS A3 and A4 evidence, reviewer steps and
    residual caveats produced here.
- STORY-260712-2e36uz - P1 Main UI, local self-test and capture
  - Consumes shared duck-control behavior for local recording bleed reduction
    and will surface overlay or interrupt state to the operator UI.

## Recommended implementation constraints

- Overlay must never clear the main ring or pause the provider; only interrupt
  may pause the main program.
- All download, decode, file open and hash verification work must complete
  before a clip becomes armed for play_media_at.
- Windows must remove the render-path mutex from Engine.Render; macOS must keep
  clip scheduling, cache I/O and cancellation bookkeeping off the source-node
  callback.
- Interrupt resume anchors should use audible position, meaning provider
  position minus buffered tail, and keep the spec fade timings of 250 ms out
  and 120 ms in.
- Test coverage should include cancellation during both armed and active clip
  states so stale resume or duck-release timers are caught before hardware
  validation.

## Completeness check against story AC

- Independent overlay branches with continuous main-program consumption are
  delivered by TASK-260712-2zbmq4 and TASK-260712-1viwvi.
- Interrupt pause or resume within tolerance is isolated in TASK-260712-8mwyiv and TASK-260712-1g6lk8.
- Render-thread safety, shared parameters and lock-free state ownership are
  front-loaded in TASK-260712-1hqiek.
- Deterministic ramps, limiter, cancellation and 100-overlay coverage are
  concentrated in TASK-260712-3d6cnn.
- Real Windows and macOS A3 and A4 proof and reviewer-ready evidence are
  closed by TASK-260712-2hodti.

## Workflow note

The board keeps newly created child tasks unassigned in backlog. They have full
scope, acceptance criteria, checklists and dependencies, so a developer can
pick up any unblocked task immediately after assignment.