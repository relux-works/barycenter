## Status
done

## Assigned To
codex-inline-engineer

## Created
2026-07-12T16:25:27Z

## Last Update
2026-07-17T06:54:05Z

## Blocked By
- TASK-260712-3a0cf9

## Blocks
- TASK-260712-wcdz08
- TASK-260712-2egweh
- TASK-260712-1pw1l1
- TASK-260712-265o0f
- TASK-260712-2gaswa

## Checklist
- [x] Inspect spec sections 7.3, 10.3, 11.3, 16, 17, 18 and 21.2-21.5 plus current P1 and player state code
- [x] Freeze the shared mode, degradation, indicator and ceiling vocabulary
- [x] Decide whether protocol, heartbeat or history surfaces need extension and list exact fields
- [x] Publish the C3 evidence rubric and negative-case matrix that later tasks must use
- [x] Freeze the common processor graph for clip self-test and live PTT
- [x] Define render reference route and clock ownership
- [x] Separate input AGC ceiling from receiver output ceiling
- [x] Freeze accepted degraded unsupported and fallback behavior
- [x] Freeze objective and blinded C3 matrix criteria

## Notes
2026-07-17 strict sequential inline execution started from synchronized main fd53e049c68010bc1aefb83a96ba47aa2e943f90 after the four-task E2EE audit packet completed and all post-audit E2EE work remained deferred in EPIC-260716-3qsztl. Scope is a candidate-neutral shared capture DSP and C3 contract with exact graph, state, route, timing, privacy, ceiling, fallback and evidence semantics. No AEC, NS, AGC, platform capability, audible quality, hardware or blinded-listening result may be invented; later implementation and manual epic gates remain explicit.
2026-07-17 completed on exact engineering commit 70d4cda548dc82025996b2587ac98bac6078ef49, merged by PR #236 as 5163b7fbe21f12ac57dcf2de3a7e7a66c9359c13. One normative candidate-neutral processor contract now covers recorded clip, five-second local self-test and live PTT; it freezes route/effect/health/fallback vocabulary, render-reference ownership and timing, distinct -3 dBFS input and -1 dBFS receiver ceilings, additive heartbeat fields, 14-case objective/blinded C3 rubric, privacy and rollback. Fail-closed tests reject silent bypass, ceiling conflation and invented evidence. Clean exact-head acceptance passed 16/16; hosted run 29561196208 passed 4/4. Runtime capability remains unadvertised; AEC/NS/AGC implementation, signed hardware, acoustic and blinded evidence remain not-run in EPIC-260714-th54l3.

## Precondition Resources
- [p3-capture-quality-components.puml](file://TASK-260712-1gmsvh/p3-capture-quality-components.puml) — Task seam and dependency view for the shared capture-quality contract

## Outcome Resources
- [capture-quality-v1.json](file://TASK-260712-1gmsvh/capture-quality-v1.json) — Normative capture graph, state, route, ceiling, fallback, privacy and heartbeat contract
- [capture-quality-contract-v1.json](file://TASK-260712-1gmsvh/capture-quality-contract-v1.json) — P3.2/C3 requirement mapping, objective thresholds, blinded rubric and manual not-run boundary
- [p3-capture-quality-contract-v1.md](file://TASK-260712-1gmsvh/p3-capture-quality-contract-v1.md) — Dated capture-quality decision and downstream handoff
- [p3-capture-quality-shared-graph.puml](file://TASK-260712-1gmsvh/p3-capture-quality-shared-graph.puml) — Focused diagram-as-code view of the common processor and two ceilings
