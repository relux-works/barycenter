# P2 Codec and Streaming Player Spike Cross-Story Dependencies

Story: `STORY-260712-3l1r1u`

## Required upstream or parallel stories

- `STORY-260712-30ju1k` P1.0 Windows Store platform spike
  - Supplies the packaged MSIX or AppContainer posture, signed probe route,
    and Windows API review discipline reused by the Media Foundation branch.
- `STORY-260712-ld674h` P1 Generic media ingest and storage
  - Owns the `media_items` lifecycle that phase 2 will extend with
    `stream_variants`; the spike can prototype independently, but the final ADR
    must name additive schema and auth hooks that fit this substrate.
- `STORY-260712-fes2jj` P1 Cross-platform overlay and interrupt mixer
  - Owns the current scheduled-start and render-safety contract. The winning
    track path must respect its no render-thread I/O, allocation, or blocking
    assumptions on both nodes.
- `STORY-260712-25lysg` P1 Transmission protocol and scheduler
  - Owns the current coordinator-time scheduling vocabulary and mixed-version
    policy surface. The spike handoff must align with its `ready` and
    `resume_at` model instead of inventing a separate timing contract.

## Downstream stories blocked or informed by this spike

- `STORY-260712-2ori1t` P2 Streamed user audio tracks
  - Direct implementation consumer. It needs the chosen decoder path, cache
    ceilings, `stream_variants` contract, and fixture corpus before coding.
- `STORY-260712-1qfbiw` P2 Acceptance, capacity and rollout
  - Reuses the same fixture pack, timing metrics, and memory criteria for B1
    plus the phase-two non-functional gates.
- `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - Consumes the chosen track path for living-air catch-up, unsupported-target
    policy, and mixed-version handling in multi-orbit airs.
- `STORY-260712-ob1tx2` P2 Explicit targets, inbox and transport parity
  - Consumes the unsupported-target and exact receipt semantics that the final
    ADR must freeze for stream tracks in mixed fleets.

## Dependency note

- The only hard board-level external dependency linked into this story is the
  phase-one Windows packaged baseline, because the Media Foundation candidate
  must prove real AppContainer compatibility instead of relying on a new ad hoc
  packaging posture.
- The ingest, mixer, and protocol stories are recorded here as authoritative
  cross-story seams, but they are not linked as additional hard blockers to
  avoid repeating the existing phase-one dependency cycle at story level.
