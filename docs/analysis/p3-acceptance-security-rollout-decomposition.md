# P3 Security acceptance and rollout decomposition

> Original agent decomposition. Mandatory root reviews, independent reviewer
> gates and resettable beta in `p3-root-review-amendments.md` supersede
> conflicting content below.

Story: `STORY-260712-2ft5wd`

## Current implementation anchor

- `docs/acceptance-run.md`
  - Still tracks only the phase-one acceptance checklist and environment.
    There is no phase-three matrix for C1-C7, no seven-day beta log, and no
    evidence index for live PTT, encrypted media, or automation safety.
- `docs/runbook.md`
  - Still documents the duet-era node and coordinator rollout. It has no
    phase-three flag posture, no recovery or key-loss drills, no live PTT or
    encrypted-media rollout order, and no automation-disable procedure.
- `coordinator/cmd/duet-coordinator/main.go`
  - `/healthz` still reports only version, orbit count, and connected-node
    totals. It does not expose the phase-three readiness surface required for
    latency, jitter, capture-session safety, crypto posture, automation audit,
    or feature-flag review.
- `coordinator/internal/hub/hub.go`
  - The health surface still collapses runtime state to a connected-node count.
    Acceptance work therefore cannot assume the section 21.4 beta or security
    gates already have an operator-facing metric path.
- Repo-wide search for `live_ptt` and `e2ee_media`
  - The phase-three spec names those flags, but the current implementation tree
    does not yet expose an integrated feature-flag posture or rollout evidence
    layer for them.
- Existing phase-three analysis artifacts
  - `docs/analysis/p3-live-ptt-decomposition.md` and
    `docs/analysis/p3-capture-quality-decomposition.md` already split C1-C2 and
    C3 implementation proof into sibling stories.
  - `STORY-260712-1frfmi` and `STORY-260712-326wd5` are not decomposed yet, so
    C4-C6 and C7 remain explicit cross-story blockers instead of hidden
    assumptions.

## Task set

1. `Freeze the phase-three gate matrix, environments, and evidence contract`
   - Blocking foundation task that turns sections 17, 18, 21.3-21.5, 22, and
     23 plus sibling-story handoffs into one dated matrix: C1-C7 mappings,
     Windows-Windows or Windows-macOS or macOS-macOS routes, speaker or
     headphone permutations, `live_ptt`-only versus `live_ptt+e2ee_media`
     flag states, rollback fixtures, key-loss drills, external-review scope,
     beta incident rubric, and artifact naming or storage.
2. `Add phase-three observability, health, and evidence views`
   - Extends the coordinator and node operational surface beyond phase-one
     `/healthz` into the metrics and redaction-safe status views needed for
     live latency, bounded jitter, stale-session rejection, capture-effect
     state, crypto rotation or revoke, automation audit, feature-flag posture,
     and seven-day beta review.
3. `Execute C1-C3 live PTT, latency, and capture-quality acceptance`
   - Final integrated proof for hold/release safety, live latency under loss,
     route-aware echo results, honest degraded or unsupported wording, and
     live-over-main-program recovery across the supported platform pairs.
4. `Execute C4-C6 encrypted-media, privacy, and report-workflow acceptance`
   - Final integrated proof for Orbit or Air membership crypto, history grants,
     coordinator-storage privacy, truthful metadata disclosure, secure key
     storage, and voluntary decrypted evidence-copy reporting.
5. `Run the external security review and close critical or high findings`
   - Makes the external review a tracked work item with a frozen packet,
     findings triage, fix or retest loop, and explicit rollout hold policy.
6. `Execute C7 soundboard, schedule, and automation safety acceptance`
   - Final integrated proof for timezone-aware schedules, DND precedence, local
     volume ceiling, scoped automation revoke, auditability, rate limits, and
     the invariant that automation never activates capture or bypasses ACL.
7. `Rehearse phased rollout, independent gating, rollback, recovery, and key-loss drills`
   - Proves the section 18 rollout order on production-shaped data with
     additive migrations, coordinator or node rollout, internal-orbit enable,
     independent `live_ptt` and `e2ee_media` gates, downgrade or rollback, and
     reproducible capture-stop, revoke, recovery, and key-loss drills.
8. `Run the seven-day real phase-three beta and incident review`
   - Executes the real beta demanded by section 21.4 and records the only valid
     proof for the prohibited incidents: stuck capture, runaway automation, or
     key-loss.
9. `Publish the phase-three promotion packet, disclosures, and evidence index`
   - Final handoff task that updates acceptance or runbook or privacy or Store
     disclosure surfaces, maps every artifact to C1-C7 plus section 21.4 and
     section 18, and states the exact promote or hold decision.

## Execution shape

- Blocking foundation:
  - task 1
- Shared operator and evidence surface after the contract freezes:
  - task 1 -> task 2
- Integrated scenario proof after sibling evidence handoffs land:
  - task 1 + task 2 + live PTT handoff + capture-quality handoff -> task 3
  - task 1 + task 2 + E2EE handoff + compliance baseline -> task 4
  - task 1 + task 2 + automation handoff -> task 6
- Review and operational rehearsal after the scenario-level surfaces exist:
  - task 3 + task 4 + task 6 -> task 5
  - task 3 + task 4 + task 6 + task 2 -> task 7
- Real beta only after scenario gates, review closure, and drills:
  - task 5 + task 7 -> task 8
- Final promotion packet:
  - task 3 + task 4 + task 5 + task 6 + task 7 + task 8 -> task 9

## Cross-story dependencies

- `STORY-260712-sskhip` P3 near-live push-to-talk
  - Supplies the C1-C2 implementation, fault-injection evidence, rollback
    signatures, and final `live_ptt` contract consumed by task 3 and task 7.
- `STORY-260712-3pt00e` P3 capture quality and diagnostics
  - Supplies the C3 route matrix, honest capability pack, and final degraded or
    unsupported wording consumed by task 3 and task 9.
- `STORY-260712-1frfmi` P3 end-to-end encrypted media
  - Supplies the threat model, key lifecycle, ciphertext routing, voluntary
    evidence-copy flow, and secure-storage design required by task 4, task 5,
    task 7, and task 9.
- `STORY-260712-326wd5` P3 soundboard and safe automation
  - Supplies the schedule, token, audit, and revoke semantics required by task
    6, task 5, task 7, task 8, and task 9.
- `STORY-260712-1i0doc` P1 Store compliance and acceptance
  - Supplies the existing moderation mailbox, policy pages, Store-copy
    baseline, and submission conventions that phase-three disclosure updates
    must extend rather than replace.
- `STORY-260712-ob1tx2` P2 explicit targets, inbox, and transport parity
  - Supplies the final report, replay, delete, and non-target visibility seams
    that encrypted evidence-copy reporting must reuse.

## Completeness check

- Covered:
  - C1-C3 final integrated platform acceptance
  - C4-C6 encrypted-media, privacy, and evidence-copy acceptance
  - C7 automation and schedule safety
  - section 21.4 non-functional gates for jitter, reconnect safety, secure
    storage, security-review closure, disclosure updates, and seven-day beta
  - section 18 staged rollout, independent gating, rollback, and recovery drills
  - truthful `live_ptt` versus `e2ee_media` claims in docs and Store metadata
- Explicit gaps closed with blocking tasks:
  - no shared phase-three gate matrix or reviewer packet existed
  - no operator-facing phase-three observability or flag posture existed
  - external security review was a specification requirement but not a tracked
    work item
  - the final E2EE and automation stories are still undecomposed, so their
    absence is recorded as an explicit blocker instead of being hand-waved
- Intentionally not re-owned here:
  - live PTT transport implementation internals
  - capture-effect implementation internals
  - E2EE protocol and key-lifecycle implementation internals
  - soundboard or automation implementation internals
- Diagrams attached:
  - `p3-acceptance-evidence-map.puml`
  - `p3-acceptance-rollout-sequence.puml`
