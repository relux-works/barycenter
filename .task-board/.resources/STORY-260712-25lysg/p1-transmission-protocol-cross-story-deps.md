# P1 Transmission Protocol and Scheduler Cross-Story Dependencies

Story: `STORY-260712-25lysg`

## Required upstream or parallel stories

- `STORY-260712-2ve1c8` P1 Identity and self-service onboarding
  - Supplies the actor and control-token model that `POST /v1/transmissions`,
    `GET /v1/transmissions/{id}`, and `POST /cancel` must authorize against.
- `STORY-260712-ld674h` P1 Media ingest and storage
  - Supplies ready media rows, canonical byte storage, delete hooks, and the
    media lifecycle this story schedules and protects with target ACL.
- `STORY-260712-fes2jj` P1 Cross-platform overlay and interrupt mixer
  - Owns the real scheduled playback, ducking, interrupt-resume, and
    cancel-safe audio graph changes that the Windows and macOS client hook
    tasks call into.
- `STORY-260712-34kbkn` P1 Telegram adapter, history and presence
  - Consumes the exact receipt, presence, DND, block, and downgrade semantics
    defined here, and likely owns the user-facing mutation surfaces for block
    and DND outside the coordinator runtime core.
- `STORY-260712-2e36uz` P1 Main UI, local self-test and capture
  - Consumes the transmission create/status/cancel API plus downgrade,
    presence, and receipt data for the desktop routing and history views.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - Consumes A2, A5, and A6 evidence plus the exact block, delete, and receipt
    semantics for certification notes, policy copy, and release evidence.

## Non-blocking downstream or follow-on stories

- `STORY-260712-ob1tx2` P2 Explicit targets, inbox, and Telegram/Pulsar parity
  - Extends the phase-one target snapshot and receipt model into inbox replay
    and multi-target parity.
- `STORY-260712-3v14m9` P2 Air rooms
  - Reuses the overlay controller and transmission targeting model when
    pairwise approach becomes explicit multi-orbit Air runtime state.
