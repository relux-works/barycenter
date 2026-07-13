## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-12T17:11:25Z

## Blocked By
- STORY-260712-3l1r1u
- STORY-260712-ob1tx2
- STORY-260712-3v14m9
- STORY-260712-2e36uz

## Blocks
- STORY-260712-1qfbiw

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
Reviewed docs/spec-self-contained-audio.md sections 8-15 and 20.2-20.5, docs/goal-self-contained-audio.md, docs/spec.md, docs/protocol.md, current coordinator media/session/store code, pulsar-win player/cache/engine code, node-app PlayerCore/AudioEngine/VoiceCache, and the fifo-player spike. Created 9 development-ready tasks with full scope, AC, checklists, within-story dependencies, diagram preconditions, and saved decomposition plus cross-story dependency resources. Main blockers remain generic media ingest, transmission protocol, the P2 codec spike, Air rooms, explicit targets or inbox, and the phase-one prepared-media baseline; no extra research task was created because codec uncertainty already has its own story.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=77521, exit=0)
Root decomposition review completed. Added missing shared plus Windows/macOS long-track UI tasks and enforceable storage/processing/egress accounting. Tightened exact ADR dependencies, non-speech variant processing, authenticated conditional ranges, anti-DoS report semantics, seek generations, provider-neutral main program, audible progress and drained ended, Air catch-up or leave, cache revocation, 5 s or 3 s or 100 ms or 200 MiB hard gates, all platform pairings and max-duration or network-fault evidence. Linked codec, Air, targets, consent, moderation and platform tasks; board validates.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-2ori1t/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p2-streamed-track-components.puml](file://STORY-260712-2ori1t/p2-streamed-track-components.puml) — Component diagram for the phase-two streamed-track architecture
- [p2-streamed-track-sequence.puml](file://STORY-260712-2ori1t/p2-streamed-track-sequence.puml) — Sequence diagram for track ingest, buffered start and seek
- [p2-streamed-track-decomposition.md](file://STORY-260712-2ori1t/p2-streamed-track-decomposition.md) — Task breakdown and execution order for streamed user audio tracks
- [p2-streamed-track-cross-story-deps.md](file://STORY-260712-2ori1t/p2-streamed-track-cross-story-deps.md) — Cross-story dependency note for streamed user audio tracks
- [p2-root-review-amendments.md](file://STORY-260712-2ori1t/p2-root-review-amendments.md) — Authoritative root review corrections to Phase 2 decomposition
