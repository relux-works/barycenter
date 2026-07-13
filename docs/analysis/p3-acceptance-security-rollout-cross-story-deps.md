# P3 Security acceptance and rollout cross-story dependencies

Story: `STORY-260712-2ft5wd`

## Required upstream or parallel stories

- `STORY-260712-sskhip` P3 near-live push-to-talk
  - Acceptance consumes the final C1-C2 evidence, live session failure
    signatures, and `live_ptt` rollout notes from this story instead of
    rebuilding them ad hoc.
- `STORY-260712-3pt00e` P3 capture quality and diagnostics
  - Acceptance consumes the C3 capability matrix, degraded or unsupported
    wording, and route-based hardware evidence established there.
- `STORY-260712-1frfmi` P3 end-to-end encrypted media
  - Acceptance depends on the threat model, reviewed key lifecycle, ciphertext
    routing, evidence-copy workflow, and final C4-C6 proof from this story.
- `STORY-260712-326wd5` P3 soundboard and safe automation
  - Acceptance depends on the final schedule, revoke, audit, and no-bypass
    semantics needed for C7 and the seven-day beta prohibition checks.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - Supplies the privacy, moderation, Store-copy, and submission baseline that
    phase-three disclosures and external-review handoff extend.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox, and transport parity
  - Supplies the report, replay, delete, and target-visibility seams reused by
    encrypted evidence-copy and moderation acceptance.

## Specific board-level blockers this story should consume

- `TASK-260712-1rzqh9` live PTT latency, loss, and no-stuck-capture evidence
  - Final C1-C2 proof and rollback signatures for task 3 and task 7.
- `TASK-260712-2e80pr` capture-quality matrix and honest capability pack
  - Final C3 proof and wording for task 3 and task 9.
- `STORY-260712-1frfmi` P3 end-to-end encrypted media
  - Story-level blocker until the C4-C6 implementation, evidence, and rollout
    handoff tasks exist and can be linked more precisely.
- `STORY-260712-326wd5` P3 soundboard and safe automation
  - Story-level blocker until the C7 implementation, evidence, and rollout
    handoff tasks exist and can be linked more precisely.

## Dependency notes

- The acceptance story owns the final integrated gate, not the first local
  proof:
  - sibling phase-three stories keep their implementation-local harnesses and
    proof tasks;
  - this story reruns the combined matrix, review, rollback, and real beta on
    the final assembled system.
- `live_ptt` and `e2ee_media` must stay independently gateable:
  - acceptance therefore must test `live_ptt`-only rollout as a first-class
    path, not merely the fully enabled configuration;
  - docs and Store claims must distinguish those states explicitly.
- External review is not a comment in a summary:
  - the reviewer packet, findings log, closure state, and retest evidence must
    be durable artifacts linked from the final packet;
  - any critical or high finding that remains open is a rollout hold, not a
    soft warning.
- The seven-day beta is downstream of review closure and drill readiness:
  - do not count a beta run that predates rollback, recovery, or revoke drills;
  - the beta log must include the exact feature-flag posture and daily incident
    review.
