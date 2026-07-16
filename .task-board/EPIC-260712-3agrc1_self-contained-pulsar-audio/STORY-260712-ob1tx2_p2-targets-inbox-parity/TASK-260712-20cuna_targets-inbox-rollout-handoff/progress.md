## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:52Z

## Last Update
2026-07-16T04:15:44Z

## Blocked By
- TASK-260712-1vklop

## Blocks
- TASK-260712-3u5cdn
- TASK-260712-3qybi2
- TASK-260712-1kfnpu

## Checklist
- [x] Record the final API inbox receipt and rights contract
- [x] Document deploy order and mixed version rollout steps
- [x] Hand off assumptions to acceptance Air tracks and later security stories

## Notes
2026-07-16 strict-sequence start from synchronized main merge 9ee893e after TASK-260712-1vklop code PR #138 merge 029346c and tracking PR #139 merge 9ee893e; hosted runs 29470131117 and 29470338566 passed 4/4. Implementing the final contract, additive deploy/rollback order and downstream/manual handoff inline outside task-board spawn workflow without claiming real-app, hardware or mixed-fleet evidence.
2026-07-16 accepted on exact engineering head 43534c88540644f6b1477fab8c8cb1e0b3ad96f3 through PR #140, merge e51c93744094a2f1be9b67c704935cddee30f5a2, after hosted run 29470807661 passed coordinator, node-core, pulsar-win and signed packaged-probe. The versioned handoff freezes eight API/rights surfaces, seven coordinator-first rollout stages, the fail-closed mixed-version window, single-writer drain/rollback without down-migration, exact preserved tables and eleven streamed-track, acceptance and E2EE consumers. Local all-suite manifest local-task-260712-20cuna-final passed all 12 commands and 41 contract/unit tests passed. Execution status remains documented-not-executed; real B5-B7 packaged mixed-fleet acceptance and production-shaped rollout rehearsal remain manual-required in TASK-260712-3u5cdn and TASK-260712-3qybi2.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-components.puml](file://TASK-260712-20cuna/p2-targets-inbox-parity-components.puml) — Rollout and handoff context for explicit target inbox and parity work
- [Phase 2 targets and inbox rollout handoff](../../../../docs/analysis/p2-targets-inbox-rollout-handoff.md) — Final API, deploy, mixed-version, rollback and downstream contract
- [PR #140](https://github.com/relux-works/barycenter/pull/140) — Accepted engineering handoff and hosted CI provenance
