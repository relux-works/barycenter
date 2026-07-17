## Status
development

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-17T11:07:59Z

## Blocked By
- STORY-260712-1qfbiw
- STORY-260712-1frfmi
- STORY-260712-326wd5
- STORY-260712-sskhip
- STORY-260712-3pt00e

## Blocks
- (none)

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
Reviewed docs/spec-self-contained-audio.md in full plus docs/goal-self-contained-audio.md, docs/spec.md, docs/protocol.md, the existing P3 decomposition artifacts, and the current acceptance/runbook/health surfaces. Current repo state is still phase-one shaped at the acceptance layer: docs/acceptance-run.md and docs/runbook.md stop at phase one, /healthz is limited to node counts, and there is no integrated phase-three flag posture, external-review workflow, or seven-day beta evidence layer yet. Created 9 development-ready tasks covering the gate matrix, observability, integrated C1-C3/C4-C6/C7 acceptance, external security review closure, rollout/rollback/recovery drills, the seven-day beta, and the final promotion/disclosure packet. Linked within-story dependencies, escalated concrete blockers from live PTT/capture-quality evidence plus undecomposed E2EE and automation stories, attached decomposition/cross-story/diagram resources, and validated the board.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=88662, exit=0)
Root review inserted non-delegable line-by-line implementation and final release audits plus independent realtime, automation, privacy or Store and migration or recovery reviews. The external security task now reviews the root-frozen implementation before C4-C6 acceptance, and seven beta days reset after prohibited incidents or material tested-path changes. Agent evidence cannot authorize its own release.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-2ft5wd/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p3-acceptance-evidence-map.puml](file://STORY-260712-2ft5wd/p3-acceptance-evidence-map.puml) — Component map for phase-three evidence flow and task ownership
- [p3-acceptance-rollout-sequence.puml](file://STORY-260712-2ft5wd/p3-acceptance-rollout-sequence.puml) — Sequence diagram for phase-three review, rollout, rollback, and beta progression
- [p3-acceptance-security-rollout-summary.md](file://STORY-260712-2ft5wd/p3-acceptance-security-rollout-summary.md) — Short outcome summary for the phase-three acceptance decomposition
- [p3-acceptance-security-rollout-decomposition.md](file://STORY-260712-2ft5wd/p3-acceptance-security-rollout-decomposition.md) — Task breakdown and execution order for phase-three acceptance, security review, and rollout gating
- [p3-acceptance-security-rollout-cross-story-deps.md](file://STORY-260712-2ft5wd/p3-acceptance-security-rollout-cross-story-deps.md) — Cross-story dependency note for the phase-three acceptance story
- [p3-root-review-amendments.md](file://STORY-260712-2ft5wd/p3-root-review-amendments.md) — Root-reviewed acceptance reviewer beta and release gates
