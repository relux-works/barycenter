## Status
done

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-15T21:43:01Z

## Blocked By
- STORY-260712-30ju1k
- STORY-260712-1i0doc

## Blocks
- TASK-260712-285pag
- TASK-260712-31rkpe
- STORY-260712-2ori1t
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
Reviewed docs/spec-self-contained-audio.md sections 8.2-8.3, 12-13, 19.2 P1.0, and 20.1-20.6 plus docs/goal-self-contained-audio.md, docs/spec.md, docs/protocol.md, docs/spec-providers.md, and the current coordinator, macOS, and Windows audio paths. Current repo state is still clip or voice oriented: macOS uses VoiceCache plus AVAudioEngine inserts, Windows mirrors that with a local WAV decoder, coordinator media remains canonical WAV, and there is no bounded-memory long-track path or stream_variants substrate yet. Created 8 development-ready tasks with one blocking foundation task, three candidate-path probe tasks, a server-variant contract task, a legal and distribution review task, a comparative evidence task, and a final ADR handoff task. Linked the Media Foundation branch to the phase-one Windows packaged baseline, linked downstream story dependencies for P2 streamed tracks and P2 acceptance, attached decomposition and cross-story notes, and linked diagrams to the tasks that consume them.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=77518, exit=0)
Root decomposition review completed. Added the missing native macOS AVFoundation or AudioToolbox branch; froze exact 5 s start, 3 s seek, 100 ms skew and 200 MiB RSS hard gates, all platform pairings, VBR and hostile fixtures, RFC range and conditional semantics, chunk integrity, target ACL, cache revocation, exact license or SBOM or CVE obligations and no-go ADR behavior. Media Foundation now waits for final signed Win10/Win11 evidence.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-3l1r1u/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p2-codec-player-spike-decomposition.md](file://STORY-260712-3l1r1u/p2-codec-player-spike-decomposition.md) — Task breakdown and execution order for the phase-two codec spike
- [p2-codec-player-spike-cross-story-deps.md](file://STORY-260712-3l1r1u/p2-codec-player-spike-cross-story-deps.md) — Cross-story dependency note for the phase-two codec spike
- [p2-codec-player-spike-components.puml](file://STORY-260712-3l1r1u/p2-codec-player-spike-components.puml) — Component diagram for the phase-two codec and player spike
- [p2-codec-player-spike-sequence.puml](file://STORY-260712-3l1r1u/p2-codec-player-spike-sequence.puml) — Sequence diagram for the phase-two codec proof flow
- [p2-root-review-amendments.md](file://STORY-260712-3l1r1u/p2-root-review-amendments.md) — Authoritative root review corrections to Phase 2 decomposition
