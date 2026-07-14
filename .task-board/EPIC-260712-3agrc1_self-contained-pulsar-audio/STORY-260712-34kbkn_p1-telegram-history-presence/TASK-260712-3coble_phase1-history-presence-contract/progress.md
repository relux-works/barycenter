## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T19:04:09Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1gx6mh
- TASK-260712-3dmllz
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-3e4p0c

## Checklist
- [x] Define the Phase 1 history list and receipt-detail contract for app and bot consumers.
- [x] Define DND and block mutation semantics, visibility rules and exact reason vocabulary.
- [x] Define Telegram callback payload, callback lifetime and idempotency rules.
- [x] Freeze the Phase 1 clip-only attachment matrix and the honest errors for Phase-2-only paths.
- [x] Freeze immediate legacy default enqueue plus atomic pre-start callback replacement with a new trusted acceptance time
- [x] Freeze callback actor binding, integrity, expiry, replay defense and interrupt confirmation
- [x] Freeze history authorization, pagination, action visibility and layered local versus orbit DND

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge 8cc171f after TASK-260712-3d6cnn acceptance. Scope is a reviewed wire/read-model contract for history, receipts, DND/block ownership, Telegram callback integrity/lifetime/idempotency, clip-only attachments, interrupt confirmation, and atomic legacy default replacement. No real-app or hardware evidence is claimed.
2026-07-14 contract gate: frozen exact history list/detail authorization and stable pagination, action visibility, layered local/orbit DND, actor/orbit block refs and reasons, a 20 MiB/180 s Phase 1 Telegram clip matrix, 36-byte opaque callbacks with 15-minute lifetime and 24-hour query dedupe, actor/role/chat/message binding, interrupt confirmation, and immediate legacy default with atomic pre-start replacement/new accepted_at. JSON examples and required decisions are executable in coordinator protocol tests; full coordinator suite is green. Hardware/manual evidence is not claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3coble/p1-telegram-history-presence-components.puml) — Component diagram for contract and dependency boundaries
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-3coble/p1-telegram-history-presence-flows.puml) — Sequence diagram for the agreed Telegram media and inline-action contract
- [p1-telegram-history-presence-states.puml](file://TASK-260712-3coble/p1-telegram-history-presence-states.puml) — State diagram for contract status and receipt mapping
- [p1-telegram-history-presence-decomposition.md](file://TASK-260712-3coble/p1-telegram-history-presence-decomposition.md) — Contract-gap context and dependency map for the blocker task
- [p1-history-presence-telegram-contract-v1.md](file://TASK-260712-3coble/p1-history-presence-telegram-contract-v1.md) — Normative Phase 1 history presence DND block and Telegram callback contract
