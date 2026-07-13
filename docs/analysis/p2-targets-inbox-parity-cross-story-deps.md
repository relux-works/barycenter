# P2 Explicit Targets, Inbox, and Transport Parity Cross-Story Dependencies

Story: `STORY-260712-ob1tx2`

## Required upstream or parallel stories

- `STORY-260712-25lysg` P1 Transmission protocol and scheduler
  - Supplies the transmission, receipt, target-snapshot, and legacy fallback
    foundation this story extends into inbox and parity behavior.
- `STORY-260712-34kbkn` P1 Telegram adapter, history and presence
  - Supplies the phase-one bot ingest and history baseline that phase-two
    parity replaces with the transport-neutral command path.
- `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - Supplies `airs`, `air_members`, Air presence, and multi-barycenter target
    resolution for explicit current-Air audiences.
- `STORY-260712-2ori1t` P2 Streamed user audio tracks
  - Supplies `audio_track`, queue/replace runtime semantics, and the main
    program lifecycle that Telegram and Pulsar parity must call identically.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - Supplies the initial content-policy, reporting, and abuse-operation
    baseline that phase two must harden into fetch revocation and B7 evidence.

## Follow-on or consumer stories

- `STORY-260712-1qfbiw` P2 Acceptance, capacity and rollout
  - Consumes the final B5-B7 evidence, rollout order, and mixed-version
    operational notes.
- `STORY-260712-1frfmi` P3 End-to-end encrypted media
  - Reuses inbox, replay, report, and per-target authorization seams and must
    preserve the explicit target snapshot model rather than reintroducing
    membership-based fetch rules.

## Dependency notes

- The current story should be blocked at the story level by the phase-one
  transmission contract, phase-one Telegram baseline, Air rooms, and streamed
  track work. Without those, the task set is ready but not executable.
- Mixed-version `B6` is shared:
  - this story owns unsupported-target visibility, parity rendering, and
    explicit policy handling;
  - Air rooms and streamed tracks own the underlying runtime capability path.
