## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-16T12:47:14Z

## Blocked By
- STORY-260712-2ori1t
- STORY-260712-3v14m9
- STORY-260712-ob1tx2
- STORY-260712-3l1r1u

## Blocks
- STORY-260712-sskhip
- STORY-260712-3pt00e
- STORY-260712-326wd5
- STORY-260712-1frfmi
- STORY-260712-2ft5wd

## Checklist
- [x] Read the authoritative specification sections and inspect the current implementation before decomposing
- [x] Create atomic implementation/research/test/documentation tasks with complete descriptions, scopes, acceptance criteria and task-specific checklists
- [x] Link all within-story dependencies and identify required cross-story dependencies in an outcome resource
- [x] Validate the decomposition, save an outcome summary and return the story to to-dev without marking implementation done
- [x] Tasks created with description and AC
- [x] Dependencies linked
- [x] Tasks are atomic — one clear deliverable each
- [x] Completeness verified — nothing forgotten
- [x] Gaps closed with blocking tasks
- [x] Diagrams drawn and linked as resources

## Notes
Reviewed docs/spec-self-contained-audio.md sections 17-20 and 23 plus docs/goal-self-contained-audio.md, docs/spec.md, docs/protocol.md, the existing P2 analysis artifacts, and the current coordinator, store, media, runbook, and acceptance surfaces. Current repo state is still phase-one shaped: docs/acceptance-run.md and docs/runbook.md stop at phase one, /healthz only reports connected-node totals, and there is no phase-two metrics, quota, rollback, or beta evidence layer yet. Created 8 development-ready tasks with one unblocked evidence-contract foundation task, one observability and quota task, four integrated acceptance or rollout tasks, one seven-day beta and quota-calibration task, and one final promotion packet task. Linked within-story dependencies, escalated the true cross-story blockers from streamed tracks, Air rooms, and explicit targets or inbox, attached decomposition and dependency resources, and linked diagrams to the tasks that consume them.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=81496, exit=0)
Root decomposition review completed. Narrowed observability to canonical metric integration, corrected B7 report-versus-authorized-removal semantics, froze exact lab/sample/build/privacy rules, strengthened all-pairing B1, resource-instrumented B2-B4, adversarial B5-B7, non-dual rollback and seven consecutive 24-hour beta rules. Added four independent reviewer tasks plus mandatory root line-by-line integration review; final acceptance and beta now wait for them.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-1qfbiw/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p2-acceptance-rollout-decomposition.md](file://STORY-260712-1qfbiw/p2-acceptance-rollout-decomposition.md) — Task breakdown and execution order for phase-two acceptance, rollout, and beta gating
- [p2-acceptance-rollout-cross-story-deps.md](file://STORY-260712-1qfbiw/p2-acceptance-rollout-cross-story-deps.md) — Cross-story dependency note for the phase-two acceptance story
- [p2-acceptance-evidence-map.puml](file://STORY-260712-1qfbiw/p2-acceptance-evidence-map.puml) — Component map for phase-two evidence flow and task ownership
- [p2-acceptance-rollout-sequence.puml](file://STORY-260712-1qfbiw/p2-acceptance-rollout-sequence.puml) — Sequence diagram for phase-two rollout, rollback, and beta progression
- [p2-acceptance-rollout-summary.md](file://STORY-260712-1qfbiw/p2-acceptance-rollout-summary.md) — Short outcome summary for the phase-two acceptance decomposition
- [p2-root-review-amendments.md](file://STORY-260712-1qfbiw/p2-root-review-amendments.md) — Authoritative root review corrections to Phase 2 decomposition
