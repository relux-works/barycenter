# P1 Identity and Self-Service Onboarding Cross-Story Dependencies

Story: `STORY-260712-2ve1c8`

## Required upstream or parallel stories

- `STORY-260712-30ju1k` P1.0 Windows Store platform spike
  - `TASK-260712-47uve0` depends on the spike choosing the Windows-safe secure
    storage mechanism and documenting any AppContainer constraints around
    DPAPI or Credential Locker usage.
- `STORY-260712-2e36uz` P1 Main UI, local self-test and capture
  - This identity story owns the credential bundle and create/join/recover
    service layer. The UI story owns the actual windows, forms, and tray
    presentation that exercise those flows.
- `STORY-260712-ld674h` P1 Generic media ingest and storage
  - The upload-admin negative authorization acceptance is only fully provable
    once media upload endpoints adopt the control-token middleware delivered by
    `TASK-260712-m5264f`.
- `STORY-260712-34kbkn` P1 Telegram adapter, history and presence
  - Telegram presence/history work should consume the actor-backed bot auth and
    link semantics from `TASK-260712-2xkyot` rather than reusing raw
    `tg_user_id` membership lookups.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - A1, A2, and A7 evidence, recovery copy, and rollout notes from
    `TASK-260712-38qsku` feed the final compliance story.

## Non-blocking adjacent stories

- `STORY-260712-25lysg` P1 Transmission protocol and scheduler
  - No direct blocker for the identity decomposition. The node/control split
    changes authorization boundaries, not the existing WebSocket register
    envelope.
- `STORY-260712-fes2jj` P1 Cross-platform overlay and interrupt mixer
  - Independent of identity work except for relying on preserved pairings and
    node token continuity after migration.
