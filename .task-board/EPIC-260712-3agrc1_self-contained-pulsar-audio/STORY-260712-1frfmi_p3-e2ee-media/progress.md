## Status
development

## Assigned To
(none)

## Created
2026-07-12T15:19:46Z

## Last Update
2026-07-17T05:09:55Z

## Blocked By
- STORY-260712-1qfbiw
- STORY-260712-2ve1c8
- STORY-260712-sskhip

## Blocks
- STORY-260712-2ft5wd
- STORY-260716-1qp4gp

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
Reviewed docs/spec-self-contained-audio.md sections 6.1, 8, 11.1-11.4, 12-15.5, 18, 21.2-21.5, and 22-23; docs/goal-self-contained-audio.md; docs/spec.md; docs/protocol.md; and the current coordinator plus Windows plus macOS plaintext media path. Current seams are coordinator-side ffmpeg normalization in coordinator/internal/media/media.go, plaintext WAV storage and fetch in coordinator/internal/store/store.go and coordinator/cmd/duet-coordinator/main.go, plaintext client caches in node-app/Sources/NodeCore/VoiceCache.swift and pulsar-win/voice.go, and the absence of e2ee_media_v1 protocol, epoch, grant, or report-evidence vocabulary. Created 9 development-ready tasks with two explicit blocking design or contract tasks, full descriptions, scopes, acceptance criteria, task-specific checklists, within-story dependencies, linked diagrams, and saved decomposition plus cross-story dependency resources. Added task-linked component and sequence diagrams, validated the board, and kept live PTT, capture quality, automation, and final phase-wide rollout closure outside this story ownership.
agent completed: [analyst] solution-architect (codex) (exit=0)
agent spawned: codex (pid=88661, exit=0)
Root rejected the initial server-rotation and two-monolithic-client decomposition. The reviewed graph adds audited crypto and container spikes, an independent pre-implementation design review, client-owned group commits, an opaque router, separate per-platform key state send playback live-PTT and UX tasks, explicit irrecoverable key-loss and report-boundary semantics. No E2EE claim or implementation is accepted yet.
Owner direction 2026-07-16: split E2EE at the independent design-audit boundary. Seventeen post-audit implementation and implementation-review tasks moved to EPIC-260716-3qsztl / STORY-260716-1qp4gp. This story now owns five pre-implementation gate tasks only.
2026-07-16 correction to the E2EE split: TASK-260712-aniuyy moved to EPIC-260716-3qsztl as requested. This story now owns four audit-preparation tasks only and completes when the reproducible audit packet is ready; it does not wait for an external reviewer.
2026-07-17 strict checkpoint: threat-model TASK-260712-2e2ymn accepted on exact 847a90b and PR #227 merge 868789c after clean 12/12 and hosted 4/4. The audit-preparation story advances to container spike TASK-260712-16xmy2; only spikes are authorized. E2EE implementation, feature enablement, claims and independent review remain blocked/false/not-run.
2026-07-17 strict checkpoint: container spike TASK-260712-16xmy2 accepted through its explicit production no-go on exact 00b5bb0 after clean 16/16 and hosted run 29556420828 4/4; PR 229 merged at 478e1aa. Repository-only chunk/AAD/nonce/range/resume structure is frozen, while codec/toolchain selection, signed physical evidence, cross-platform vectors, network integration, replay state, zeroization and independent review remain open. Execution advances to TASK-260712-3er89x; E2EE remains disabled and unclaimed.

## Precondition Resources
- [spec-entry.md](file://STORY-260712-1frfmi/spec-entry.md) — Authoritative specification entry point

## Outcome Resources
- [p3-e2ee-media-components.puml](file://STORY-260712-1frfmi/p3-e2ee-media-components.puml) — Task-boundary component diagram for phase-three encrypted media trust and runtime boundaries
- [p3-e2ee-media-sequence.puml](file://STORY-260712-1frfmi/p3-e2ee-media-sequence.puml) — Sequence diagram for protected send, revoke rotation, history grant, and voluntary report evidence
- [p3-e2ee-media-decomposition.md](file://STORY-260712-1frfmi/p3-e2ee-media-decomposition.md) — Task breakdown, current seams, execution shape, and completeness check for phase-three encrypted media
- [p3-e2ee-media-cross-story-deps.md](file://STORY-260712-1frfmi/p3-e2ee-media-cross-story-deps.md) — Cross-story dependency map and boundary decisions for phase-three encrypted media
- [p3-root-review-amendments.md](file://STORY-260712-1frfmi/p3-root-review-amendments.md) — Root-reviewed client-owned E2EE architecture and review gates
