## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T22:46:32Z

## Blocked By
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-21ers7
- TASK-260712-3e4p0c

## Blocks
- TASK-260712-1f9jtm
- TASK-260712-wy05n6
- TASK-260712-176b74

## Checklist
- [x] Cover legacy ordering, callback flows, clip attachment errors and downgrade behavior with coordinator and bot tests.
- [x] Prove presence sanitization plus exact DND and blocked receipt reasons with deterministic fixtures.
- [x] Verify pairwise-approach target naming and app-vs-bot label parity under mixed compatibility.
- [x] Cover callback replacement versus playback-start races and no-action latency
- [x] Cover history tenant isolation, action authorization and layered DND precedence

## Notes
2026-07-15 kickoff: strict sequential inline execution started from synchronized main merge 6df7ab4 after TASK-260712-3e4p0c acceptance. Scope is deterministic cross-surface regression evidence for legacy timing/order, callback authorization and races, attachment errors, history/presence isolation, DND/block/downgrade reasons, mixed compatibility, and exact EN/RU app-versus-bot labels. Existing unit tests are evidence only after coverage mapping and root review; no real Telegram client, app, audible, physical-device or hardware result will be claimed.
2026-07-15 acceptance: exact engineering head 24a043e4794da90bccc22492269ed8fd699226a6 passed all four hosted jobs in run 29373913897. Regression mapping covers legacy FIFO and zero synthetic wait, attachment proof, forged/expired/cross-user/group callback authorization, duplicate/start/replacement races, interrupt confirmation, mixed-capability whole downgrade, history tenant/action isolation, presence redaction, layered DND/block reasons and pairwise EN/RU parity. Review removed the last private RU-only callback answer switch: app and Telegram now share callback semantic keys and exact EN/RU copy. Local full/vet/race, pinned previous-head, moderation ops, Windows test/vet/amd64+arm64 builds, Swift release, PlantUML, diff and board gates passed. No real Telegram client, app, audible playback, packaged-device or physical-hardware result is claimed; those remain in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3d0zgu/p1-telegram-history-presence-components.puml) — Component map of deterministic parity regression boundaries
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-3d0zgu/p1-telegram-history-presence-flows.puml) — Sequence map of no-action, callback race, security and mixed-fleet scenarios
- [p1-telegram-history-presence-states.puml](file://TASK-260712-3d0zgu/p1-telegram-history-presence-states.puml) — State map of callback, policy, receipt and redaction regressions
- [p1-telegram-history-presence-parity-regressions.md](file://TASK-260712-3d0zgu/p1-telegram-history-presence-parity-regressions.md) — Deterministic acceptance matrix and manual-validation boundary
