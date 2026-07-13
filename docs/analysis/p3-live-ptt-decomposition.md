# P3 Near-live Push-to-talk Decomposition

> Original agent decomposition. The root-reviewed task split and dependencies
> in `p3-root-review-amendments.md` supersede conflicting content below.

Story: `STORY-260712-sskhip`

## Task set

1. `TASK-260712-9wivva` Validate Store-safe hold input path and fallback policy
   - Blocking spike for the unresolved `P3-HOTKEY` requirement in spec section
     21.2 and the key-loss safety demanded by C1.
2. `TASK-260712-3qviqc` Freeze live PTT wire contract, codec envelope and compatibility policy
   - Blocking contract task for live session messages, chunk envelope,
     `live_ptt_v1`, mixed-version behavior and old-session rejection.
3. `TASK-260712-3vzbbl` Implement coordinator live session runtime and duck scheduling
   - Coordinator-side live session ownership, target sealing, chunk fanout,
     backpressure, cancellation, reconnect safety and synchronized ducking.
4. `TASK-260712-2kj9kj` Implement macOS hold capture, live transport and jitter playback
   - macOS sender plus receiver path over the existing capture, websocket and
     audio-engine seams, with safe fallback to toggle.
5. `TASK-260712-2jbo5i` Implement Windows hold capture, live transport and jitter playback
   - Windows sender plus receiver path under packaged AppContainer constraints,
     with safe fallback to toggle.
6. `TASK-260712-1rzqh9` Prove live PTT latency, loss recovery and no-stuck-capture behavior
   - Story-level C1 and C2 evidence, fault injection, latency matrix and
     rollback-signature handoff.

## Execution shape

- Blocking spike: `TASK-260712-9wivva`
- Blocking contract: `TASK-260712-9wivva` -> `TASK-260712-3qviqc`
- Coordinator runtime: `TASK-260712-3qviqc` -> `TASK-260712-3vzbbl`
- Client paths:
  - `TASK-260712-9wivva` + `TASK-260712-3qviqc` -> `TASK-260712-2kj9kj`
  - `TASK-260712-9wivva` + `TASK-260712-3qviqc` -> `TASK-260712-2jbo5i`
- Final proof:
  - `TASK-260712-3vzbbl` + `TASK-260712-2kj9kj` + `TASK-260712-2jbo5i`
    -> `TASK-260712-1rzqh9`

## Cross-story dependencies

- `STORY-260712-30ju1k` P1 Windows packaged-app spike
  - Supplies the legal AppContainer capture or input bridge, signed-probe
    workflow and recorded lock, revoke and hidden-window limitations that the
    hold-input spike must extend rather than rediscover from scratch.
- `STORY-260712-2e36uz` P1 main UI and capture
  - Supplies the capture engines, toggle hotkey controllers, menu or tray
    shells and durable draft semantics that phase-three live capture builds on.
- `STORY-260712-25lysg` P1 transmission protocol and scheduler
  - Supplies the capability-negotiation precedent, receipt model, downgrade
    semantics, presence or DND behavior and coordinator-node contract hygiene
    that the live wire contract extends.
- `STORY-260712-fes2jj` P1 overlay and interrupt mixer
  - Supplies the existing cross-platform ducking, cancel-safe audio graph and
    render-safety seams that live ducking must reuse.
- `STORY-260712-3v14m9` P2 Air rooms
  - Supplies current-Air membership, 2..N routing and shared runtime ownership
    for sealing the live recipient set.
- `STORY-260712-2ori1t` P2 streamed audio tracks
  - Supplies the phase-two main-program player and recovery behavior that live
    PTT must duck over and restore cleanly.
- `STORY-260712-3l1r1u` P2 codec-player spike
  - Supplies codec, Store-compatibility and distribution evidence that should
    inform the live chunk codec choice instead of creating an unreviewed
    second decoder path.
- `STORY-260712-3pt00e` P3 capture quality and diagnostics
  - Parallel sibling, not a blocker for C1-C2. This story must not silently
    claim AEC, noise suppression or parity that belongs there.
- `STORY-260712-1frfmi` P3 end-to-end encrypted media
  - Parallel sibling, not a blocker for `live_ptt`. This story must leave key
    lifecycle, ciphertext routing and report-copy workflow outside its scope.
- `STORY-260712-326wd5` P3 soundboard and safe automation
  - Parallel sibling, not a blocker here. Live PTT should expose clean seams
    but should not absorb automation, schedules or webhook token policy.
- `STORY-260712-2ft5wd` P3 acceptance, security and rollout
  - Consumes the C1-C2 evidence, rollback notes and failure signatures
    produced by `TASK-260712-1rzqh9`.

## Gaps closed explicitly

- `P3-HOTKEY` was still a specification-level spike, not an implementation
  contract. `TASK-260712-9wivva` closes that gap and blocks downstream code.
- The authoritative spec names live streaming requirements but does not freeze
  exact message shapes, chunk cadence, late-join behavior or session-generation
  semantics. `TASK-260712-3qviqc` turns those into a coded contract.

## Completeness check

- Covered:
  - safe global hold-input viability and fallback policy
  - live wire contract, capability negotiation and codec envelope
  - coordinator live-session runtime, chunk fanout and duck scheduling
  - macOS sender plus receiver integration
  - Windows sender plus receiver integration
  - C1-C2 evidence, latency matrix, rollback signatures and handoff
- Intentionally not re-owned here:
  - AEC, noise suppression, AGC and capture diagnostics from `STORY-260712-3pt00e`
  - E2EE key lifecycle, moderation evidence and privacy review from `STORY-260712-1frfmi`
  - soundboard, schedules and automation safety from `STORY-260712-326wd5`
  - seven-day beta, external security review and full C3-C7 gate closure from
    `STORY-260712-2ft5wd`
- Diagrams attached:
  - `docs/diagrams/p3-live-ptt-components.puml`
  - `docs/diagrams/p3-live-ptt-sequence.puml`
