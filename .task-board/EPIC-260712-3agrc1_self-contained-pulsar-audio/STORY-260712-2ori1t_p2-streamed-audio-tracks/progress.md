## Status
development

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-16T08:19:07Z

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
2026-07-16 strict progress: TASK-260712-1n5fks started inline from tracking PR #141 merge b7bc2b4 after hosted run 29471003186 passed 4/4. Implementing only candidate-neutral schema, integrity/seek metadata and restart-safe queue/progress persistence permitted by the accepted production no-go ADR.
2026-07-16 strict checkpoint: TASK-260712-1n5fks accepted on exact head b64a671 through PR #142, merge 5478006, after hosted run 29471845396 passed 4/4. Candidate-neutral persistence and exact previous-binary rollback are green; production codec/player remains no-go. Strict story execution advances to TASK-260712-31rkpe.
2026-07-16 strict progress: TASK-260712-31rkpe started inline from tracking PR #143 merge d26cb26 after hosted run 29472071694 passed 4/4. Implementing shared/Go/Swift/Windows wire payloads, generation semantics and mixed-version policy without enabling production codecs or claiming real-app playback.
2026-07-16 strict checkpoint: TASK-260712-31rkpe accepted on exact head ea2d6d4 through PR #144, merge 0b9fc7d, after hosted run 29473326227 passed 4/4. All three codecs share 51 goldens and generation/timing/mixed-version invariants; production stream_track_v1 advertisement remains disabled. Strict story execution advances to TASK-260712-2ogntd.
2026-07-16 strict progress: TASK-260712-2ogntd started inline from tracking PR #145 merge 188b503 after hosted run 29473524803 passed 4/4. Auditing and implementing storage, processing, retained-byte and actual-egress counters, deterministic quota boundaries, reconciliation and privacy-safe operator surfaces without enabling production streamed playback.
2026-07-16 strict checkpoint: TASK-260712-2ogntd accepted on exact head 00a2697 through PR #146, merge 15ebd3d, after hosted run 29475162175 passed 4/4. Authoritative actor/orbit storage, processing and actual-egress projections, deterministic admission/reconciliation and authenticated audited operator surfaces are green; production streamed playback remains disabled. Strict story execution advances to TASK-260712-285pag.
2026-07-16 strict progress: TASK-260712-285pag started inline from tracking PR #147 merge 6e53606 after hosted run 29475408660 passed 4/4. Implementing secure audio_track intake, limits, probe/metadata, cleanup and test-only candidate pipeline while preserving the accepted production codec no-go and clip/Telegram behavior.
2026-07-16 strict checkpoint: TASK-260712-285pag accepted on exact head 7b30755 through PR #148, merge 4749a76, after hosted run 29476335634 passed 4/4. Consent-gated bounded track intake now preserves probed metadata and processing accounting, returns the codec ADR's stable no-go and publishes no variant or generated WAV. Strict story execution advances to TASK-260712-3lf8r0.
2026-07-16 strict progress: TASK-260712-3lf8r0 started inline from merge 4749a76 after hosted run 29476335634 passed 4/4. Implementing target-snapshot-authorized range/conditional transport, revocation and bounded actual-egress accounting without enabling production playback.
2026-07-16 strict checkpoint: TASK-260712-3lf8r0 accepted on exact head 52bf876 through PR #150, merge cf3a33a, after hosted run 29478459982 passed 4/4. Private conditional ranges now re-authorize exact target generations, enforce reporter-local versus global revocation, reject unsafe storage and cap both response and tiny-request amplification while metering exact bytes. Strict story execution advances to TASK-260712-2h6snp; production selection and hands-on playback remain unclaimed.
2026-07-16 strict progress: TASK-260712-2h6snp started inline from merge cf3a33a after hosted run 29478459982 passed 4/4. Implementing candidate-neutral queue/replace, buffer-ready, audible progress, seek/rebuffer generations, leave and restart behavior without enabling production playback.
2026-07-16 strict checkpoint: TASK-260712-2h6snp accepted on exact head 020c9e9 through PR #152, merge d427f82, after hosted run 29480661409 passed 4/4. Provider-neutral main-source orchestration, FIFO queue/replace persistence, exact-generation buffered scheduling, audible progress, rebuffer/restart, Air join/leave and ring-drained completion are green; production stream_track_v1 remains disabled and hands-on playback unclaimed. Strict story execution advances to TASK-260712-17w78q.
2026-07-16 strict progress: TASK-260712-17w78q started inline from merge d427f82 after hosted run 29480661409 passed 4/4. Implementing bounded Windows range cache, incremental candidate decoder/ring integration and generation-safe load/seek/pause/resume/ended behavior in code and deterministic tests without enabling the production capability or claiming real-hardware performance.
2026-07-16 strict checkpoint: TASK-260712-17w78q accepted on exact head a7bfeb7 through PR #154, merge feabd2e, after hosted run 29482823224 passed 4/4. Authenticated opaque range caching, fixed disk/PCM bounds, integrity and durable revoke handling, exact-generation lifecycle and drained completion are green; production stream_track_v1 remains disabled and hands-on Windows performance is unclaimed. Strict story execution advances to TASK-260712-1q2kwa.
2026-07-16 strict progress: TASK-260712-1q2kwa started inline from merge feabd2e after hosted run 29482823224 passed 4/4. Implementing the shared RU/EN long-track draft, processing, canonical target, queue/replace and generation-safe playback presentation model without enabling production decoding or claiming real-app behavior.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-2ori1t/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p2-streamed-track-components.puml](file://STORY-260712-2ori1t/p2-streamed-track-components.puml) — Component diagram for the phase-two streamed-track architecture
- [p2-streamed-track-sequence.puml](file://STORY-260712-2ori1t/p2-streamed-track-sequence.puml) — Sequence diagram for track ingest, buffered start and seek
- [p2-streamed-track-decomposition.md](file://STORY-260712-2ori1t/p2-streamed-track-decomposition.md) — Task breakdown and execution order for streamed user audio tracks
- [p2-streamed-track-cross-story-deps.md](file://STORY-260712-2ori1t/p2-streamed-track-cross-story-deps.md) — Cross-story dependency note for streamed user audio tracks
- [p2-root-review-amendments.md](file://STORY-260712-2ori1t/p2-root-review-amendments.md) — Authoritative root review corrections to Phase 2 decomposition
