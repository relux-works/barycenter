## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T22:59:11Z

## Blocked By
- TASK-260712-3d0zgu

## Blocks
- TASK-260712-1xik11
- TASK-260712-uht9e2

## Checklist
- [x] Document the attachment matrix, inline fallback rules and shared label vocabulary.
- [x] Record mixed legacy/new compatibility notes and the boundaries to the transmission and UI stories.
- [x] Publish the final handoff note together with the story diagrams and decomposition summary.

## Notes
2026-07-15 kickoff: strict sequential inline execution started from synchronized main merge b6e49cb after TASK-260712-3d0zgu acceptance. The handoff will consolidate only accepted deterministic contracts and exact cross-story ownership, link the regression matrix, freeze rollout/rollback and observability caveats, and explicitly exclude raw Telegram identifiers, Phase 2 tracks/offline inbox, real-client, audible, packaged-app and physical-hardware claims.
2026-07-15 acceptance: exact engineering head 14d3d5a6f99a614f4886d42246fd33a61a51459d passed all four hosted jobs in run 29374582024. The final durable handoff freezes application HTTP, Telegram attachment/callback/default rules, shared EN/RU presentation, history/presence/DND/block semantics, mixed-version whole downgrade, deploy/exposure order, privacy-safe observability, drain-first rollback and exact upstream/downstream ownership. The obsolete pre-implementation decomposition and three story diagrams were replaced and an executable protocol/doc guard prevents silent loss of critical exclusions. Local coordinator full/vet/presentation-race, pinned previous-head, Windows test/vet/amd64+arm64 builds, Swift release, PlantUML, diff and board gates passed. No real Telegram client, app, audible playback, packaged-device or physical-hardware result is claimed; those remain in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-1f9jtm/p1-telegram-history-presence-components.puml) — Final component ownership and cross-story handoff map
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-1f9jtm/p1-telegram-history-presence-flows.puml) — Deploy, runtime callback and drain-first rollback sequence
- [p1-telegram-history-presence-states.puml](file://TASK-260712-1f9jtm/p1-telegram-history-presence-states.puml) — Voice route and bounded rollout exposure states
- [p1-telegram-history-presence-decomposition.md](file://TASK-260712-1f9jtm/p1-telegram-history-presence-decomposition.md) — Final accepted story decomposition and downstream ownership
- [p1-telegram-history-presence-rollout-handoff.md](file://TASK-260712-1f9jtm/p1-telegram-history-presence-rollout-handoff.md) — Durable Phase 1 parity rollout, rollback and consumer handoff
