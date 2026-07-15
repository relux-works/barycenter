## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-15T00:28:51Z

## Blocked By
- STORY-260712-30ju1k
- STORY-260712-2ve1c8
- STORY-260712-ld674h
- STORY-260712-25lysg
- STORY-260712-34kbkn

## Blocks
- STORY-260712-1i0doc
- STORY-260712-ob1tx2
- STORY-260712-3v14m9
- TASK-260712-wcdz08
- TASK-260712-2egweh
- STORY-260712-2ori1t
- TASK-260712-288j4a
- TASK-260712-1yw7fo
- STORY-260712-sskhip
- STORY-260712-3pt00e

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
Decomposition completed. Created 8 development-ready tasks covering cue contract, Windows shell/capture/integration, macOS shell/capture/integration, and verification evidence. Cross-story dependencies are documented in the attached outcome resource p1-main-ui-capture-decomposition.md. Note: the board keeps unassigned tasks in backlog; they are ready to pick up once assigned.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=61333, exit=0)
Root decomposition review completed. Split platform capture, exact five-second record-then-play self-test, brokered file intake and shortcut controllers into atomic tasks; corrected self-test versus durable unsent-draft lifecycles, cue exclusion, foreground-only Escape, AppContainer gate, secure restart retry and cross-story identity, upload, transmission, history and presence dependencies. Board validates.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-2e36uz/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://STORY-260712-2e36uz/p1-main-ui-capture-components.puml) — Component diagram for story workstreams and external dependencies
- [p1-main-ui-capture-flows.puml](file://STORY-260712-2e36uz/p1-main-ui-capture-flows.puml) — Sequence diagrams for local self-test and record/send flows
- [p1-main-ui-capture-decomposition.md](file://STORY-260712-2e36uz/p1-main-ui-capture-decomposition.md) — Decomposition summary with completeness check and cross-story dependencies
- [root-reviewed-p1-main-ui-capture-decomposition.md](file://STORY-260712-2e36uz/root-reviewed-p1-main-ui-capture-decomposition.md) — Root-reviewed task split and cross-story lifecycle
- [root-reviewed-p1-main-ui-capture-components.puml](file://STORY-260712-2e36uz/root-reviewed-p1-main-ui-capture-components.puml) — Root-reviewed UI and capture task seams
- [root-reviewed-p1-main-ui-capture-flows.puml](file://STORY-260712-2e36uz/root-reviewed-p1-main-ui-capture-flows.puml) — Root-reviewed self-test and durable-draft flow
- [p1-root-review-amendments.md](file://STORY-260712-2e36uz/p1-root-review-amendments.md) — Authoritative root review corrections to Phase 1 decomposition
