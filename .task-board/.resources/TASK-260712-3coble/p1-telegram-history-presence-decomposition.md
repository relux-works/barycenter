# P1 Telegram Adapter, History and Presence Decomposition

## Spec slices reviewed

- `docs/spec-self-contained-audio.md` sections 4.6-4.7, 5.1-5.2, 8.1, 9.1-9.4,
  11.1-11.4, 12, 14.1-14.4, 15.2-15.5, 16, 18, 19.2-19.6
- `docs/goal-self-contained-audio.md`
- `docs/spec.md` sections 5, 8.3-8.4, 9.1-9.2, 10.1-10.3 for shipped Telegram
  behavior and legacy voice compatibility
- `docs/protocol.md` plus the current coordinator and bot implementation

## Current implementation snapshot

- Telegram is still a bespoke path:
  - `coordinator/internal/bot/bot.go` only emits text commands and `voice`
    updates.
  - `coordinator/cmd/duet-coordinator/loop.go` downloads Telegram voice,
    runs `media.Process(...)`, and directly enqueues legacy `session.KindVoice`
    items.
- There is no callback-query or inline keyboard plumbing, and no support for
  Telegram `audio` or `document` updates.
- There is no Phase 1 history/presence surface yet beyond the legacy chat texts:
  - `/home` and `/status` are text renderers over current orbit/session state.
  - coordinator HTTP still exposes only `/pair`, `/ws`, `/healthz`, and
    `/media/...`.
- Ad hoc naming helpers already exist (`peerName`, `orbitText`, `elementLabel`,
  `humanizePeers`) but they are loop-specific and do not provide one shared
  label model for app and bot parity.

## Story tasks created

1. `TASK-260712-3coble` - Clarify the Phase 1 history, presence and Telegram callback contract
2. `TASK-260712-1gx6mh` - Build the shared delivery presentation model
3. `TASK-260712-3dmllz` - Extend Telegram transport for callbacks and clip attachments
4. `TASK-260712-1c1ska` - Expose sanitized presence plus DND and block state
5. `TASK-260712-2hcq1g` - Expose transmission history and exact receipt read models
6. `TASK-260712-21ers7` - Implement Telegram inline routing actions and legacy compatibility
7. `TASK-260712-3d0zgu` - Add Telegram parity regression and sanitization coverage
8. `TASK-260712-1f9jtm` - Document Phase 1 Telegram parity and rollout handoff

## Within-story dependency graph

- `TASK-260712-1gx6mh` blocked by `TASK-260712-3coble`
- `TASK-260712-3dmllz` blocked by `TASK-260712-3coble`
- `TASK-260712-1c1ska` blocked by `TASK-260712-3coble`, `TASK-260712-1gx6mh`
- `TASK-260712-2hcq1g` blocked by `TASK-260712-3coble`, `TASK-260712-1gx6mh`
- `TASK-260712-21ers7` blocked by `TASK-260712-1gx6mh`, `TASK-260712-3dmllz`,
  `TASK-260712-1c1ska`, `TASK-260712-2hcq1g`
- `TASK-260712-3d0zgu` blocked by `TASK-260712-1c1ska`, `TASK-260712-2hcq1g`,
  `TASK-260712-21ers7`
- `TASK-260712-1f9jtm` blocked by `TASK-260712-3d0zgu`

Execution intent:

- Start by freezing the Phase 1 contract gap instead of letting every
  downstream task guess its own history/presence/callback surface.
- Then build the shared label model and Telegram transport primitives in
  parallel.
- Add the app/bot-facing presence and history surfaces once those foundations
  exist.
- Wire inline Telegram actions only after the read models and label vocabulary
  are stable.
- Finish with regression coverage, then final handoff documentation.

## Cross-story dependencies

- `STORY-260712-2ve1c8` - Identity and self-service onboarding
  - `TASK-260712-2xkyot` is the critical dependency for actor-backed Telegram
    authorization and link compatibility. This story should consume
    `ActorContext`, not raw `tg_user_id` membership lookups.
  - `TASK-260712-m5264f` is the likely owner of any control-token middleware
    reused by app-facing presence or history routes.
- `STORY-260712-ld674h` - Media ingest and storage
  - `TASK-260712-12ojcb` owns the default voice migration onto `SubmitMedia`
    while preserving legacy enqueue order.
  - `TASK-260712-2af2dp` owns the common clip pipeline used by Telegram voice
    and clip-eligible audio attachments.
- `STORY-260712-25lysg` - Transmission protocol and scheduler
  - Owns transmission rows, target receipts, exact DND/block semantics,
    capability downgrade rules, and the prepare/play lifecycle.
  - This story must not duplicate scheduler ownership; it should consume those
    states and expose humanized read models plus bot parity.
- `STORY-260712-2e36uz` - Main UI, local self-test and capture
  - The Windows/macOS integration tasks consume the presence/history/routing
    read surfaces and wording defined here.
- `STORY-260712-fes2jj` - Cross-platform overlay and interrupt mixer
  - Supplies the real runtime behavior for `overlay` and `interrupt`; this
    story surfaces those modes honestly and must expose downgrade copy when
    capabilities are absent.
- `STORY-260712-1i0doc` - Store compliance and acceptance
  - Consumes report/block wording, screenshot-ready labels, and the A5/A6/A7
    parity evidence from `TASK-260712-3d0zgu`.

## Contract gap explicitly closed

- The spec requires:
  - app-visible history and presence;
  - exact receipt reasons;
  - DND/block visibility;
  - Telegram inline actions;
  - Phase 1 compatibility behavior.
- It does not pin down the exact route/event shape for history lists,
  DND/block mutations, or Telegram callback payloads, and it spans a phase
  boundary for Telegram audio/document handling.
- `TASK-260712-3coble` is therefore a deliberate blocker task rather than an
  implicit assumption. Its output should freeze:
  - the Phase 1 clip-only attachment matrix;
  - the honest Phase 1 failure path for track-like uploads;
  - callback payload shape and lifetime;
  - history/presence query surfaces and user-visible vocabulary.

## Completeness check against story acceptance criteria

- Legacy Telegram voice staying first after the current element and preserving
  acceptance ordering is covered by `TASK-260712-21ers7` and regression-tested
  in `TASK-260712-3d0zgu`.
- Inline actions creating the same transmission semantics as the app are
  covered by `TASK-260712-3coble`, `TASK-260712-3dmllz`,
  `TASK-260712-21ers7`, and `TASK-260712-3d0zgu`.
- Presence not leaking microphone or process details is owned by
  `TASK-260712-1c1ska` and verified in `TASK-260712-3d0zgu`.
- History showing processing through played/partial/error states with exact
  receipt reasons is owned by `TASK-260712-2hcq1g` plus the shared label model
  in `TASK-260712-1gx6mh`, and proven by `TASK-260712-3d0zgu`.
- Bot/app names and target labels agreeing without raw IDs are owned by
  `TASK-260712-1gx6mh`, consumed by `TASK-260712-1c1ska`,
  `TASK-260712-2hcq1g`, `TASK-260712-21ers7`, and checked in
  `TASK-260712-3d0zgu`.

## Story boundary

This story owns:

- Telegram transport parity beyond the default ingest bridge
- humanized labels and read models
- presence/history/DND/block presentation surfaces
- bot/app wording consistency

It does not own:

- `SubmitMedia` internals
- transmission scheduling or protocol state machines
- mixer implementation
- Windows/macOS UI rendering itself
