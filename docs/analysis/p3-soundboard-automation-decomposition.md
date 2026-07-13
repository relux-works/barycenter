# P3 Soundboard and Safe Automation Decomposition

> Original agent decomposition. Durable cue lifecycle, platform task splits and
> Telegram parity in `p3-root-review-amendments.md` supersede conflicting
> content below.

Story: `STORY-260712-326wd5`

## Spec slices reviewed

- `docs/spec-self-contained-audio.md` sections 2, 3, 4.7, 5, 6, 8-18, 21.2-21.5, 22-23
- `docs/goal-self-contained-audio.md`
- `docs/spec.md` and `docs/protocol.md` for current shipped transport, scheduler and compatibility constraints
- Existing decomposition and dependency notes for:
  - `STORY-260712-25lysg` P1 transmission protocol and scheduler
  - `STORY-260712-fes2jj` P1 overlay and interrupt mixer
  - `STORY-260712-34kbkn` P1 Telegram adapter, history and presence
  - `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - `STORY-260712-ob1tx2` P2 explicit targets, inbox and transport parity
  - `STORY-260712-sskhip` P3 near-live push-to-talk
  - `STORY-260712-3pt00e` P3 capture quality and diagnostics

## Current implementation snapshot

- `coordinator/cmd/duet-coordinator/main.go`
  - The coordinator only exposes `/pair`, `/media/{id}`, `/ws` and `/healthz`.
    There is no control-plane API yet for cue management, schedules, scoped
    automation tokens, revocation, or automation audit reads.
- `coordinator/internal/store/store.go`
  - SQLite currently stores `elements`, `media`, `settings` and a generic
    `events` log. There are no additive tables yet for actors, cue presets,
    schedules, automation principals, execution lineage, or quick-disable state.
- `coordinator/internal/store/orbits.go`
  - Node token hashing and immediate slot revocation already exist, which is a
    useful precedent for scoped automation-token revocation, but the actor and
    control-token model belongs to the phase-one identity story, not this one.
- `node-app/Sources/NodeCore/Protocol.swift` and `pulsar-win/wire/protocol.go`
  - The node wire contract has playback, prepared media, state, receipts and
    heartbeat seams only. There is no soundboard, automation, DND mutation, or
    automation-attribution vocabulary on the wire today.
- `node-app/Sources/NodeApp/*` and `pulsar-win/ui*.go`
  - macOS and Windows have menu/tray shells and playback status, but no
    reusable cue library, configurable cue hotkeys, schedule editor, quick
    automation disable, or automation-history surface.
- Earlier-phase stories still own critical prerequisites:
  - DND/block semantics and history baselines live in `STORY-260712-34kbkn`.
  - Explicit target snapshots and history/inbox expansion live in
    `STORY-260712-ob1tx2`.
  - Air membership and policy evaluation live in `STORY-260712-3v14m9`.
  - Local ceiling ordering and cue mixing live in `STORY-260712-fes2jj`.

## Task set

1. `Threat-model and freeze the automation surface, cue scope, and safety contract`
   - Blocking research and contract task for the unresolved spec decision in
     section 23: webhook versus local automation API, exact scope model,
     supported media kinds, quiet-hour semantics, target selectors, audit
     vocabulary, and the hard "no microphone/no bypass" boundaries.
2. `Persist cues, schedules, scoped principals, and automation execution lineage`
   - Add the additive schema and repositories for reusable builtin and user
     cues, hotkey bindings, timezone-aware schedules, scoped automation
     principals, revocation state, execution rows, and immutable audit fields.
3. `Implement cue-library, schedule-management, and scoped-token control APIs`
   - Expose authenticated CRUD and listing flows for cue presets, schedules,
     hotkey assignments, token issuance and token revocation on top of the
     approved contract.
4. `Implement coordinator automation execution, revocation, and runaway guards`
   - Build the scheduler and trigger runtime that resolves target snapshots,
     enforces timezone, quiet hours, DND, Air policy, rate limits, local volume
     ceiling ordering, cancellation, immediate revoke, and "no mic activation".
5. `Extend history, audit, and quick-disable services for automation attribution`
   - Make schedule and API-triggered cue events visible as attributable history
     with trigger source, actor or principal lineage, cancel or disable actions,
     and exact missed or blocked reasons without inventing hidden bypasses.
6. `Implement macOS soundboard, configurable cue hotkeys, and schedule controls`
   - Add the menu-bar or window surfaces for cue selection, cue CRUD, hotkey
     registration, schedule management, automation token visibility, history
     attribution, and quick-disable on macOS.
7. `Implement Windows soundboard, configurable cue hotkeys, and schedule controls`
   - Add the tray or window surfaces for cue selection, cue CRUD, hotkey
     registration, schedule management, automation token visibility, history
     attribution, and quick-disable inside the packaged Windows posture.
8. `Prove C7 automation safety and publish the operational handoff`
   - Build rerunnable regressions and evidence for timezone correctness, DND and
     quiet-hour enforcement, volume-ceiling preservation, immediate token
     revoke, runaway prevention, rate limits, cancellation, and the guarantee
     that automation never starts microphone capture.

## Execution shape

- Blocking contract first: task 1
- Persistence and API foundation: task 1 -> task 2 -> task 3
- Coordinator runtime: task 1 + task 2 + task 3 -> task 4
- Shared history and disable surface: task 1 + task 3 + task 4 -> task 5
- Platform clients:
  - task 3 + task 5 -> task 6
  - task 3 + task 5 -> task 7
- Final proof and handoff:
  - task 4 + task 5 + task 6 + task 7 -> task 8

## Cross-story dependencies

- `STORY-260712-2ve1c8` P1 identity and self-service onboarding
  - Supplies the actor, control-token and revocation baseline that scoped
    automation principals must extend instead of inventing a parallel auth path.
- `STORY-260712-25lysg` P1 transmission protocol and scheduler
  - Supplies transmission creation, prepare-ready-play semantics, cancellation,
    receipts and downgrade hygiene that automation-triggered cue delivery reuses.
- `STORY-260712-2e36uz` P1 main UI, local self-test and capture
  - Supplies the menu/tray shells, hotkey preferences, builtin-cue precedent and
    durable settings surfaces that the soundboard UI extends.
- `STORY-260712-fes2jj` P1 overlay and interrupt mixer
  - Supplies the cue branch, ducking and local ceiling ordering that scheduled
    or API-triggered cues must preserve.
- `STORY-260712-34kbkn` P1 Telegram adapter, history and presence
  - Supplies the phase-one history baseline plus DND/block visibility and exact
    reason vocabulary. Automation must extend, not fork, those surfaces.
- `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - Supplies current-Air membership, Air policy checks and 2..N routing needed
    when automation targets the active Air instead of only the local orbit.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox and transport parity
  - Supplies immutable target snapshots, ACL-safe history detail and quick
    action patterns that automation-triggered events must reuse.
- `STORY-260712-2ft5wd` P3 security acceptance and rollout
  - Consumes task 8 evidence for C7, seven-day beta safety checks, and the
    final phase-three acceptance pack.

## Gaps closed explicitly

- The authoritative spec intentionally leaves the automation surface unresolved
  until a threat-model decision. Task 1 closes that blocker instead of letting
  implementation silently choose a webhook or local API without review.
- The spec says "soundboard" and "automation" but does not freeze the media
  matrix. This decomposition explicitly treats the story as cue-class automation
  over builtin and user clips. Long streamed tracks remain owned by
  `STORY-260712-2ori1t`, avoiding an unplanned dependency on phase-two track
  queue semantics.
- The spec requires immediate revocation, attribution and quick disable, but the
  current code only has generic events and slot-token revocation precedents.
  Tasks 2 through 5 add the missing persistence, control APIs, lineage, and
  operator-facing disable flows.

## Completeness check

- Covered:
  - unresolved automation-surface threat model and contract
  - additive persistence for cues, schedules, scoped principals and audit
  - authenticated cue and schedule management APIs
  - trigger runtime with timezone, DND, Air policy, revoke and rate-limit safety
  - history and audit attribution plus quick-disable surfaces
  - macOS and Windows soundboard plus configurable hotkey UI
  - rerunnable C7 evidence and operational handoff
- Explicitly out of scope here, not forgotten:
  - live microphone capture, hold-to-talk transport and jitter playback from
    `STORY-260712-sskhip`
  - AEC, AGC, diagnostics and route capability claims from `STORY-260712-3pt00e`
  - end-to-end encryption design and rollout from `STORY-260712-1frfmi`
  - long streamed-track automation beyond cue-class clips from
    `STORY-260712-2ori1t`
- Diagrams attached:
  - `docs/diagrams/p3-soundboard-automation-components.puml`
  - `docs/diagrams/p3-soundboard-automation-sequence.puml`
