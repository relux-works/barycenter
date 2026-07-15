## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:45:04Z

## Last Update
2026-07-15T06:46:30Z

## Blocked By
- TASK-260712-1epb3a
- TASK-260712-2kec2s
- TASK-260712-3dqc3l
- TASK-260712-3e4p0c
- TASK-260712-1x0lot

## Blocks
- TASK-260712-2s4e9p
- TASK-260712-1xik11

## Checklist
- [x] Identify every macOS screen, menu or history row that must expose report, block or delete in phase one.
- [x] Add RU and EN labels, confirmations and error states that reuse the approved policy and moderation terminology.
- [x] Verify sender and owner permissions, hidden actions for unsupported states, and exact mapping of backend status into macOS history or receipt views.
- [x] Add targeted UI or integration coverage for the macOS moderation interactions introduced here.
- [x] Expose Report for every accessible foreign item and Delete only for owned media
- [x] Verify keyboard and VoiceOver access, repeated actions and active-media policy

## Notes
2026-07-15 strict inline kickoff from synchronized main ab09923 after PR #64. Reuse the canonical history action/backend contract and the newly frozen cross-platform moderation reason/outcome vocabulary; do not create macOS business logic forks. Automated SwiftUI/keyboard/VoiceOver semantics, EN/RU copy, authorization, repeat and offline behavior are engineering scope. Physical packaged-app keyboard and VoiceOver observation remains manual in TASK-260712-e5mfqj under EPIC-260714-th54l3.
2026-07-15 engineering candidate: canonical macOS History report/block/owner-delete/replay controls, six reasons, bounded details, confirmations, authorization recheck and privacy-safe EN/RU outcome mapping are implemented. Full Xcode suite passes 215 tests in 35 suites; release build, automated Swift acceptance, board validation and diff checks pass. Physical keyboard and VoiceOver observation remains manual in TASK-260712-e5mfqj under EPIC-260714-th54l3.
2026-07-15 accepted on exact engineering head 074e5a75826433778014af80487b779d19dec69c. Clean Swift acceptance passed with start/end dirty false and 215 tests. Hosted CI run 29395040109 passed coordinator rollback, authoritative Xcode Swift, Windows portable/cross-build and signed packaged probe. Manual real-app keyboard/VoiceOver evidence remains deferred and unclaimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-macos-ugc-controls.md](file://TASK-260712-34stvx/p1-macos-ugc-controls.md) — Phase 1 macOS UGC surface, canonical contract, accessibility semantics, and manual boundary
- [candidate-swift-acceptance-manifest.json](file://TASK-260712-34stvx/candidate-swift-acceptance-manifest.json) — Candidate repository-automated Swift acceptance manifest; dirty candidate run before exact-head clean gate
- [exact-head-swift-acceptance-manifest.json](file://TASK-260712-34stvx/exact-head-swift-acceptance-manifest.json) — Clean exact engineering head 074e5a7 Swift acceptance: 215 tests, start/end dirty false
