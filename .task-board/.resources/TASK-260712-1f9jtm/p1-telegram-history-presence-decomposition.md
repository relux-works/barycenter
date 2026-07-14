# P1 Telegram, history and presence final decomposition

- Story: `STORY-260712-34kbkn`
- Contract: `p1-history-presence-telegram-v1`
- Final task: `TASK-260712-1f9jtm`
- Status: all eight engineering tasks accepted or in final handoff

## Accepted sequence

1. `TASK-260712-3coble` froze the HTTP, callback, attachment and policy contract.
2. `TASK-260712-1gx6mh` created the privacy-safe semantic EN/RU catalog.
3. `TASK-260712-3dmllz` added opaque callbacks and voice/audio/document transport.
4. `TASK-260712-1c1ska` implemented sanitized presence plus common DND/block mutations.
5. `TASK-260712-2hcq1g` implemented actor-scoped history and exact receipt projections.
6. `TASK-260712-21ers7` implemented durable voice defaults and atomic inline replacement.
7. `TASK-260712-3e4p0c` delegated replay/delete/report/block actions to canonical services.
8. `TASK-260712-3d0zgu` accepted deterministic parity/security regression evidence.

## Final ownership

This story owns Telegram transport adaptation, opaque callback dispatch,
shared presentation labels and the actor-scoped history/presence projection.
It does not own identity linking, media proof/lifecycle, transmission
acceptance/scheduling, mixer audio behavior, desktop UI rendering, moderation
evidence or Store/manual acceptance.

Those boundaries are frozen in
`docs/analysis/p1-telegram-history-presence-rollout-handoff.md`. Phase 1 never
authorizes from raw Telegram IDs, converts long attachments into tracks,
creates an offline inbox, splits one transmission across protocols or claims
real-client/audible/hardware evidence from deterministic tests.

## Downstream consumers

- `STORY-260712-2e36uz` consumes strict history/presence fields and shared keys.
- `STORY-260712-fes2jj` supplies executable overlay/interrupt behavior and capabilities.
- `STORY-260712-1i0doc` consumes exact privacy/copy/mixed-version evidence.
- Phase 2 Air, explicit targets/inbox and streamed tracks extend with new
  versioned contracts; they do not reinterpret this Phase 1 boundary.

## Evidence

The final regression matrix is
`docs/analysis/p1-telegram-history-presence-parity-regressions.md`. Real
Telegram rendering, packaged apps, audible playback and physical hardware stay
in `EPIC-260714-th54l3`.
