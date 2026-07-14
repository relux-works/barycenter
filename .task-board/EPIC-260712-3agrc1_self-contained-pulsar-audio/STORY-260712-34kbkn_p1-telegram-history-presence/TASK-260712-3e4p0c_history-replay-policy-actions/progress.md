## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:52:48Z

## Last Update
2026-07-14T22:30:48Z

## Blocked By
- TASK-260712-3coble
- TASK-260712-2hcq1g
- TASK-260712-1c1ska
- TASK-260712-2qpp6w
- TASK-260712-gj0cko
- TASK-260712-2kec2s

## Blocks
- TASK-260712-3d0zgu
- TASK-260712-pbfz37
- TASK-260712-34stvx
- TASK-260712-dlltnr
- TASK-260712-wt2n7m

## Checklist
- [x] Create replay as an explicit new transmission with new targets and accepted_at
- [x] Delegate delete, report and mute to owner services with ActorContext and audit enforcement
- [x] Cover expired, deleted, revoked, duplicate and racing actions

## Notes
2026-07-15 kickoff: strict sequential inline execution started from synchronized main merge 912d080 after TASK-260712-21ers7 acceptance. Scope is one ActorContext-scoped history command layer for explicit replay, owner-policy delete, moderation report, and block/mute delegation with idempotency, audit, revocation and race coverage. No Phase 2 inbox autoplay or manual real-client, audible, app, or hardware evidence will be claimed.
2026-07-15 engineering candidate: one transport-neutral history command service now backs app bearer and verified Telegram consumers. Replay accepts no client media ID or acceptance time, creates a fresh common-resolver transmission with newly resolved targets, DND/block/capability policy and actor-scoped idempotency; same-key retries survive later deletion without reviving content. Delete delegates to the audited media tombstone plus durable cancellation outbox; report delegates to exact-target moderation evidence/rate-limit/audit; actor/orbit mute delegates to viewer-bound block policy and existing cancellation enforcement. Tests cover fresh target-set changes, expired/deleted media, delete/replay race, repeated delete/replay/report/block, changed-request conflict, verified Telegram owner actions, revoked/foreign callers, strict HTTP parsing, outbox and audit evidence. Green locally: coordinator vet/full tests, focused full race, pinned previous-head compatibility, moderation ops check, Windows vet/native tests/windows cross-test compile, Swift release build, both PlantUML renders, and diff check. No manual real-client, audible, hardware or Phase 2 inbox evidence is claimed; it remains in EPIC-260714-th54l3.
2026-07-15 accepted engineering evidence: exact code head 04f2b20c33b9af464e155b720f45838f70497ade passed all four hosted jobs in run 29372823415, including coordinator vet/full/pinned rollback, authoritative macOS Swift tests, portable Windows build and signed packaged probe. Best-effort code/unit/integration/static/CI scope is accepted; manual real-client, audible and physical-hardware evidence remains outside this task in EPIC-260714-th54l3. PR #47 tracking CI and merge remain.
2026-07-15 delivery: tracking head 75cbb5b9f4ab1077691baa6d0c900ff2d208f343 passed all four hosted jobs in run 29373039104. PR #47 merged exact accepted scope to main as 6df7ab4932e0b3fc8629ce0a92f924d34c78b557. Strict execution advanced to TASK-260712-3d0zgu from synchronized main.

## Precondition Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3e4p0c/p1-telegram-history-presence-components.puml) — History command ownership and delegation boundaries
- [p1-telegram-history-presence-states.puml](file://TASK-260712-3e4p0c/p1-telegram-history-presence-states.puml) — History action authorization, delegation, and idempotency states

## Outcome Resources
- [TASK-260712-3e4p0c_history-replay-policy-actions.md](file://TASK-260712-3e4p0c/TASK-260712-3e4p0c_history-replay-policy-actions.md) — HTTP contract, transport-neutral orchestration, owner-service delegation, idempotency and automated evidence
