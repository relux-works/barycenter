## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-15T23:45:38Z

## Blocked By
- STORY-260712-1i0doc
- STORY-260712-25lysg
- STORY-260712-ld674h
- STORY-260712-34kbkn
- STORY-260712-2e36uz
- STORY-260712-3v14m9

## Blocks
- TASK-260712-3lf8r0
- TASK-260712-2h6snp
- TASK-260712-1fpb9q
- STORY-260712-2ori1t
- STORY-260712-1qfbiw
- TASK-260712-1eva0y
- TASK-260712-11e4e3

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
Reviewed docs/spec-self-contained-audio.md sections 4.6-4.7, 8.1, 11-15, 20.3-20.5 plus docs/goal-self-contained-audio.md, docs/spec.md, docs/protocol.md, docs/idea-air-rooms.md, and the current coordinator store session bot and media handlers. Current implementation seams are the owner-or-active-approach media ACL in GetMediaForOrbit, single-or-both targeting in targetNodes, the personal-more-than-one-recipient broadcast fallback in processMediaDone, and Telegram-specific targeting and queue logic in the coordinator loop. Created 9 development-ready tasks with one blocking clarification task, complete descriptions, scope, acceptance criteria, task checklists, within-story dependencies, story-level upstream blockers, linked diagrams, and saved decomposition plus cross-story dependency notes. Validated the story plan and the board after mutation.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=77519, exit=0)
Root decomposition review completed. Recast Phase 2 as an extension of Phase 1 target snapshots, history, secure callbacks and moderation; added versioned content-policy consent and separate Windows/macOS UI tasks. Tightened opaque authorized target selectors, targeted-track semantics blocker, one inbox item per eligible missed target, TTL/replay lineage, new-member isolation, stable pagination, zero autoplay, canonical rights revocation and mixed-version visibility. Remaining Air and stream task links will be added after those decompositions finish.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-ob1tx2/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p2-targets-inbox-parity-components.puml](file://STORY-260712-ob1tx2/p2-targets-inbox-parity-components.puml) — Component diagram for the phase two explicit target inbox and parity architecture
- [p2-targets-inbox-parity-sequence.puml](file://STORY-260712-ob1tx2/p2-targets-inbox-parity-sequence.puml) — Sequence diagram for explicit target miss and manual replay
- [p2-targets-inbox-parity-decomposition.md](file://STORY-260712-ob1tx2/p2-targets-inbox-parity-decomposition.md) — Task breakdown and execution order for the phase two explicit target inbox and parity story
- [p2-targets-inbox-parity-cross-story-deps.md](file://STORY-260712-ob1tx2/p2-targets-inbox-parity-cross-story-deps.md) — Cross story dependency note for the phase two explicit target inbox and parity story
- [p2-root-review-amendments.md](file://STORY-260712-ob1tx2/p2-root-review-amendments.md) — Authoritative root review corrections to Phase 2 decomposition
