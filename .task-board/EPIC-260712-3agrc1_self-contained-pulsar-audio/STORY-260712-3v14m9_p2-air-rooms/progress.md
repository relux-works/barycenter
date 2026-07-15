## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-15T11:54:33Z

## Blocked By
- STORY-260712-2ve1c8
- STORY-260712-25lysg
- STORY-260712-34kbkn
- STORY-260712-2e36uz
- STORY-260712-1i0doc

## Blocks
- TASK-260712-2h6snp
- TASK-260712-1fpb9q
- STORY-260712-ob1tx2
- STORY-260712-2ori1t
- STORY-260712-1qfbiw
- TASK-260712-1eva0y

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
Decomposition completed. Added 9 development-ready tasks, linked the within-story dependency graph, attached two Air-room diagrams and the story decomposition note, and validated the board. Key blocker is TASK-260712-17yizc because the approved spec leaves the exact Air lifecycle and policy contract implicit. Cross-story boundaries are recorded in p2-air-rooms-decomposition.md, especially for P1 identity and UI foundations, P1 transmission protocol, P2 streamed tracks and codec spike, P2 explicit targets and inbox, and P2 acceptance rollout.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=77520, exit=0)
Root decomposition review completed. Added missing secure Telegram Air 2-to-N lifecycle parity; separated saved membership from one active runtime, hardened single-use invites and joining-primary confirmation, made parked rooms lazy, froze join or leave media behavior and versioned policies, and strengthened deterministic link-to-Air authority cutover so rollback cannot run dual runtimes. UI now manages multiple saved Airs with disruptive-action confirmation. Linked Phase 1 identity, scheduler, callbacks, UI and Phase 2 target services; stream task links remain pending its final decomposition.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-3v14m9/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p2-air-rooms-decomposition.md](file://STORY-260712-3v14m9/p2-air-rooms-decomposition.md) — Task breakdown, execution shape, and cross-story dependency map for the Air rooms story
- [p2-air-rooms-components.puml](file://STORY-260712-3v14m9/p2-air-rooms-components.puml) — Component diagram for Air lifecycle, migration, and runtime ownership boundaries
- [p2-air-rooms-lifecycle-sequence.puml](file://STORY-260712-3v14m9/p2-air-rooms-lifecycle-sequence.puml) — Sequence diagram for Air creation, join, alias mapping, leave, and parking flow
- [p2-root-review-amendments.md](file://STORY-260712-3v14m9/p2-root-review-amendments.md) — Authoritative root review corrections to Phase 2 decomposition
