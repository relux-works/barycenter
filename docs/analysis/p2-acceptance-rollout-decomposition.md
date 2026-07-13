# P2 Acceptance, Capacity, and Rollout Decomposition

Story: `STORY-260712-1qfbiw`

## Current implementation anchor

- `docs/acceptance-run.md`
  - Still tracks only the phase-one acceptance checklist and production setup.
    There is no phase-two matrix for B1-B7, section 20.5 limits, or the
    seven-day beta gate.
- `docs/runbook.md`
  - Still documents the two-home duet rollout and phase-one operations. It has
    no Air-room rollout order, streamed-track rollout flags, mixed-version
    policies, quota telemetry, or rollback procedure for phase two.
- `coordinator/cmd/duet-coordinator/main.go`
  - Exposes `/healthz`, `/pair`, and `/media/*`, but no phase-two metrics or
    readiness surface for stream buffering, seek latency, or storage or egress
    quota accounting.
- `coordinator/internal/hub/hub.go`
  - Health reporting is limited to connected-node counts, which is not enough
    for section 17 or the section 20.5 phase gate.
- `coordinator/internal/store/store.go`
  - Media persistence is still the phase-one short-media model with WAV
    retention and link-based access rules. Acceptance work must therefore
    consume the final Air, streamed-track, and target-snapshot handoffs rather
    than assume a stable phase-two store contract already exists.
- `coordinator/internal/media/media.go`
  - The current processing path is still clip-oriented and ffmpeg-to-WAV. It
    does not itself provide the long-track, range, or quota evidence the phase
    gate needs.

## Task set

1. `Prepare the phase-two gate matrix, environments, and evidence contract`
   - Blocking foundation task that freezes the exact B1-B7 matrix, platform and
     mixed-version lab roster, 8-barycenter or 20-pulsar load environment,
     beta incident rubric, fixture pack, and artifact naming or storage
     convention.
2. `Add phase-two observability, quota accounting, and operator evidence views`
   - Extends the coordinator and node operational surface from phase-one
     `/healthz` into the section 17 and 20.5 metrics required by acceptance,
     beta review, and quota calibration.
3. `Execute B1 streamed-track acceptance and the cross-platform compatibility matrix`
   - Final integrated proof for one-hour tracks, start-before-full-download,
     pause, seek, resume, cache eviction, and pairwise phase-one compatibility
     on Windows-Windows, Windows-macOS, and macOS-macOS fleets.
4. `Execute B2-B4 Air, living-air, leave, and scale acceptance`
   - Final integrated proof for multi-barycenter Air behavior, exact-once
     delivery, no transitive duplicates, living-air catch-up, leave semantics,
     and the 8-barycenter or 20-pulsar synthetic load gate.
5. `Execute B5-B7 explicit-target, mixed-version, and rights-abuse acceptance`
   - Final integrated proof for target invisibility, no broadcast fallback,
     unsupported-target policy, inbox or replay lifecycle, report or delete or
     disable revocation, and Telegram or Pulsar parity.
6. `Rehearse additive rollout, mixed-fleet migration, and rollback`
   - Proves the section 18 sequence on production-shaped data: additive DB,
     accept-but-don't-send coordinator, capability-aware node rollout, staged
     flag enablement, pairwise compatibility, and rollback that preserves
     phase-two rows and legacy service.
7. `Run the seven-day multi-home beta and calibrate quotas from telemetry`
   - Executes the real beta required by section 20.6 and turns real storage or
     egress telemetry into the phase-two quota decision that section 23 defers
     until measured usage exists.
8. `Publish the phase-two promotion packet, runbook, and evidence index`
   - Final handoff task that updates operator docs, links every artifact, maps
     them to B1-B7 plus section 20.5 and 20.6, and records the exact promotion
     or hold decision.

## Execution shape

- Blocking foundation:
  - task 1
- Early acceptance infrastructure after the contract freezes:
  - task 1 -> task 2
- Scenario proof work after upstream implementation proofs land:
  - task 1 + task 2 + streamed-track handoff -> task 3
  - task 1 + task 2 + Air handoff + streamed-track handoff -> task 4
  - task 1 + task 2 + targets or inbox handoff + streamed-track handoff ->
    task 5
- Rollout rehearsal after implementation and observability handoffs exist:
  - task 1 + task 2 + Air handoff + streamed-track handoff +
    targets or inbox handoff -> task 6
- Real beta after scenario gates and rollback rehearsal:
  - task 2 + task 3 + task 4 + task 5 + task 6 -> task 7
- Final promotion packet:
  - task 3 + task 4 + task 5 + task 6 + task 7 -> task 8

## Cross-story dependencies

- `STORY-260712-3l1r1u` P2 codec and streaming player spike
  - Supplies the canonical fixture pack, timing metrics, memory criteria, and
    Store-compatible decoder choice reused by every phase-two gate task.
- `STORY-260712-2ori1t` P2 streamed user audio tracks
  - Supplies the one-hour track path, mixed-version streamed-track behavior,
    rollout notes, and regression evidence consumed by B1, B6, rollback, and
    beta.
- `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - Supplies the Air runtime, migration fixtures, leave or living-air behavior,
    and the synthetic load basis consumed by B2-B4 and rollback rehearsal.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox, and transport parity
  - Supplies B5-B7 behavior, unsupported-target visibility, revocation rules,
    and the final mixed-version operator semantics.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - Supplies the phase-one content-policy, reporting, and operations baseline
    that phase-two abuse and quota acceptance extends rather than redefines.

## Completeness check

- Covered:
  - B1 streamed-track acceptance across both node platforms
  - B2-B4 Air lifecycle, offline catch-up, leave semantics, and exact-once
    delivery
  - B5-B7 ACL, mixed-version, inbox, and abuse gates
  - section 20.5 non-functional limits for start, seek, memory, quotas, and
    8-barycenter or 20-pulsar synthetic load
  - section 18 staged rollout and rollback preserving unknown phase-two data
  - pairwise phase-one compatibility during mixed-fleet enablement
  - seven-day real beta gate from section 20.6
  - section 23 quota calibration from real telemetry instead of guesswork
- Explicit gaps closed with blocking tasks:
  - no shared evidence contract or lab roster existed for B1-B7
  - no phase-two observability or quota surface existed beyond `/healthz`
  - exact paid or free quota numbers are intentionally deferred until real beta
    telemetry exists
- Intentionally not re-owned here:
  - decoder-path selection itself
  - Air lifecycle implementation internals
  - target-snapshot or inbox implementation internals
  - parked-Air retention policy, which stays deferred past the phase gate
- Diagrams attached:
  - `p2-acceptance-evidence-map.puml`
  - `p2-acceptance-rollout-sequence.puml`
