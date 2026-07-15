## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T21:43:01Z

## Blocked By
- TASK-260712-dqdoqj
- TASK-260712-1vdlkw
- TASK-260712-ibuaxj

## Blocks
- TASK-260712-1n5fks
- TASK-260712-285pag
- TASK-260712-3lf8r0
- TASK-260712-31rkpe
- TASK-260712-2h6snp
- TASK-260712-17w78q
- TASK-260712-3aj8w2
- TASK-260712-2g3fkt

## Checklist
- [x] Choose the winning decoder and server-variant combination from the measured and legal evidence
- [x] Record rejected options and the concrete reasons they failed or lost
- [x] Define stream_variants, cache ceilings, node adapter seams, and protocol or API changes for implementation
- [x] Freeze the fixture corpus and evidence expectations that downstream implementation and acceptance stories must reuse
- [x] Hand off exact assumptions to STORY-260712-2ori1t and STORY-260712-1qfbiw without reopening the spike
- [x] Publish no-go instead of selecting a winner if any mandatory platform, format, legal or Store gate lacks a passing combination

## Notes
2026-07-15 strict-sequence start from synchronized main 739cd9f. Implementing inline outside task-board spawn workflow. Because the accepted comparative matrix has no complete passing combination, this ADR will publish an explicit production no-go rather than name a winner. It will still freeze the reusable stream_variants, authenticated range, cache/ring, adapter, integrity, seek, fixture and package assumptions so downstream engineering can proceed only behind a non-production experimental gate without reopening the evidence boundary.
Accepted as the required production no-go after engineering PR #118 merged as b253b39 (head 3e40b14). The source matrix permits no selection, so the ADR names no codec, container or complete combination and keeps the production decoder registry empty. It freezes candidate-neutral stream_variants, authenticated single-range transport, SHA-256 integrity, 512/64/128 MiB cache ceilings, 1 MiB chunks and PCM ring, generation-safe seek, 48 kHz stereo float PCM, coordinator timing gates, exact fixture hashes, range profiles, pairing/sample requirements and release obligations. Downstream engineering may build schema, state machines, collectors and test doubles only; production generation, playback, Store submission, fallback download and sandbox weakening remain false. Bundled, native and pure-Go rejected reasons are retained. Validator and negative tests passed 19/19; hosted CI run 29452694269 passed all four jobs. A reviewed later ADR may reopen only after one exact combination passes every format, gate, pairing and release obligation. Manual physical evidence remains in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p2-codec-player-spike-components.puml](file://TASK-260712-2eympi/p2-codec-player-spike-components.puml) — Accepted no-go and candidate-neutral downstream component boundary
- [player-handoff-v1.json](file://TASK-260712-2eympi/player-handoff-v1.json) — Normative production no-go and candidate-neutral implementation contract
- [adr-p2-codec-player-no-go-v1.md](file://TASK-260712-2eympi/adr-p2-codec-player-no-go-v1.md) — Accepted no-go ADR, rejected options and reopening criteria
- [validate_player_handoff.py](file://TASK-260712-2eympi/validate_player_handoff.py) — Fail-closed ADR and handoff validator
- [hosted-coordinator-manifest.json](file://TASK-260712-2eympi/hosted-coordinator-manifest.json) — Hosted run 29452694269 coordinator acceptance manifest
- [hosted-swift-manifest.json](file://TASK-260712-2eympi/hosted-swift-manifest.json) — Hosted run 29452694269 Swift acceptance manifest
- [hosted-windows-manifest.json](file://TASK-260712-2eympi/hosted-windows-manifest.json) — Hosted run 29452694269 Windows acceptance manifest
