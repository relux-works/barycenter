# P2 Air Rooms and Approach Migration Decomposition

Story: `STORY-260712-3v14m9`

## Task set

1. `TASK-260712-17yizc` Freeze Air lifecycle, alias, and policy contracts
   - Blocking contract task for the missing Air control-plane details in
     `docs/spec-self-contained-audio.md` sections 11 and 13.
2. `TASK-260712-3n36ny` Add Air schema, repositories, and link migration foundation
   - Additive persistence for Airs plus active-link backfill and rollback-safe
     migration rehearsals.
3. `TASK-260712-kr64r2` Generalize shared air runtime ownership from links to Air sessions
   - Replace link-keyed `stateFor` ownership with `air_id` routing while
     preserving living-air catch-up and parked personal state.
4. `TASK-260712-2vhf80` Implement Air lifecycle services and app-facing control-plane API
   - Create, invite, join, confirm, leave, dissolve, and current-Air read
     models for Pulsar consumers.
5. `TASK-260712-25862f` Persist Air policies and enforce room-level permissions
   - Store and enforce invite, overlay, queue, and replace rights with local
     DND or block precedence.
6. `TASK-260712-2bjdlb` Map approach and apart bot flows onto Air compatibility aliases
   - Preserve current pairwise user meaning while switching runtime ownership
     and migration onto Air ids.
7. `TASK-260712-2i3u7v` Expose Air lifecycle state in the macOS Pulsar control plane
   - Integrate current Air, pending joins, lifecycle actions, and stable error
     handling into the macOS app surface.
8. `TASK-260712-31zja2` Expose Air lifecycle state in the Windows Pulsar control plane
   - Integrate current Air, pending joins, lifecycle actions, and stable error
     handling into the Windows app surface.
9. `TASK-260712-3nq0tq` Prove Air lifecycle, migration, and load regressions
   - Final proof for B2-B4 semantics, no transitive duplicates, migration and
     rollback rehearsal, and the 8-barycenter or 20-Pulsar limit.

## Execution shape

- Blocking contract: `TASK-260712-17yizc`
- Persistence path: `TASK-260712-17yizc` -> `TASK-260712-3n36ny`
- Runtime path: `TASK-260712-3n36ny` -> `TASK-260712-kr64r2`
- Control-plane path: `TASK-260712-kr64r2` + `TASK-260712-3n36ny` ->
  `TASK-260712-2vhf80`
- Policy path: `TASK-260712-2vhf80` + `TASK-260712-3n36ny` ->
  `TASK-260712-25862f`
- Compatibility path: `TASK-260712-2vhf80` + `TASK-260712-kr64r2` ->
  `TASK-260712-2bjdlb`
- Client path: API + policy + alias -> `TASK-260712-2i3u7v` and
  `TASK-260712-31zja2`
- Final proof: schema + runtime + API + policy + alias + both client tasks ->
  `TASK-260712-3nq0tq`

## Cross-story dependencies

- `STORY-260712-2ve1c8` P1 identity and onboarding
  - Air lifecycle work consumes the actor, membership, secure-token, and
    control-plane foundations created there.
- `STORY-260712-2e36uz` P1 main UI and capture
  - The macOS and Windows Air integration tasks depend on the Phase-1 window
    and tray or menu-bar shells that will display current-Air state.
- `STORY-260712-25lysg` P1 transmission protocol
  - Multi-home clip proof for B2 still depends on the scheduler, client hooks,
    and compatibility transport decomposed there.
- `STORY-260712-3l1r1u` P2 codec and streaming player spike
  - B4 track continuity and synthetic load proof depend on the chosen
    bounded-memory decoder path.
- `STORY-260712-2ori1t` P2 streamed audio tracks
  - B1 and the track half of B2-B4 remain blocked on long-track ingest,
    streaming playback, seek, and resume.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox, and transport parity
  - This Air story must not re-own target-snapshot ACLs, non-broadcast
    personal N-recipient delivery, inbox, replay, or no-late-autoplay policy.
- `STORY-260712-1qfbiw` P2 acceptance, capacity, and rollout
  - The final seven-day beta, full platform matrix, and phase gate evidence
    roll up there after story-level regressions exist.

## Completeness check

- Covered:
  - Air lifecycle and policy contract gap
  - additive schema and active-link migration
  - `air_id` runtime ownership, warmup, and no-transitive routing
  - lifecycle service and app-facing control plane
  - policy persistence and authorization
  - approach and apart compatibility aliases
  - macOS and Windows Air lifecycle integration
  - migration rehearsal, regression coverage, and 8 or 20 load proof
- Explicit gap closed with blocker:
  - exact Air lifecycle, alias, and policy contract details are not named in
    spec section 11 and are only partially implied in section 13
- Intentionally not re-owned here:
  - target-snapshot media ACLs and inbox behavior
  - streamed-track decoder and playback internals
  - parked-Air GC retention policy, which section 23 defers past the phase gate
- Diagrams attached:
  - `p2-air-rooms-components.puml`
  - `p2-air-rooms-lifecycle-sequence.puml`
