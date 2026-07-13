# P2 Acceptance, Capacity, and Rollout Cross-Story Dependencies

Story: `STORY-260712-1qfbiw`

## Required upstream or parallel stories

- `STORY-260712-3l1r1u` P2 codec and streaming player spike
  - Acceptance reuses the final decoder decision, memory ceilings, fixture
    corpus, and timing metrics established there.
- `STORY-260712-2ori1t` P2 streamed user audio tracks
  - Supplies the one-hour track path, B1 regression evidence, mixed-version
    streamed-track behavior, and operator rollout notes used by B1, B6,
    rollback rehearsal, and the real beta.
- `STORY-260712-3v14m9` P2 Air rooms and approach migration
  - Supplies the Air lifecycle, migration fixtures, leave behavior, and the
    synthetic load substrate required by B2-B4 and the section 20.5 scale
    gate.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox, and transport parity
  - Supplies the explicit-target ACL, inbox and replay rules, unsupported-track
    mixed-version policy, and rights or abuse revocation required by B5-B7.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - Supplies the base content-policy, reporting, listing, and operator
    conventions that phase-two rollout and rights evidence extend.

## Specific board-level blockers this story should consume

- `TASK-260712-1fpb9q` streamed-track regression evidence
  - Final automated B1 and streamed-track-specific B6 proof.
- `TASK-260712-2ubzyf` streamed-track rollout handoff
  - Final feature-flag, migration, rollback, and quota-telemetry notes for the
    long-track path.
- `TASK-260712-3nq0tq` Air lifecycle regression and rehearsal
  - Final B2-B4 automation, migration rehearsal, and synthetic load proof.
- `TASK-260712-1vklop` targets or inbox parity regressions
  - Final B5-B7 automation and mixed-version visibility proof.
- `TASK-260712-20cuna` targets or inbox rollout handoff
  - Final API, deploy-order, and downstream operator notes for explicit
    targets, inbox, and rights revocation.

## Dependency notes

- The acceptance story should own the final integrated gate, not the first
  story-level proof:
  - sibling phase-two stories keep their implementation-local regression tasks;
  - this story reruns the integrated matrix, mixed-fleet rollout, rollback, and
    real beta on the final combined system.
- Pairwise compatibility remains an inherited requirement from the phase-one
  transmission and mixer stories, but it should be proven here under the
  phase-two feature-flag and mixed-version rollout sequence instead of only by
  unit tests.
- Quota numbers are not frozen by upstream stories:
  - section 23 intentionally defers exact storage economics until telemetry
    from real usage exists;
  - this story therefore owns the final quota-calibration decision and the
    documentation update that follows it.
