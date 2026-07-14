## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T21:48:42Z

## Blocked By
- TASK-260712-1gx6mh
- TASK-260712-3dmllz
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-12ojcb
- TASK-260712-2qpp6w

## Blocks
- TASK-260712-3d0zgu

## Checklist
- [x] Show inline delivery actions and audience choices for clip-eligible Telegram media after processing completes.
- [x] Preserve the acceptance-time FIFO and default after_current path when the user takes no action.
- [x] Surface visible capability downgrades and reuse the shared presentation model for all bot wording.
- [x] Create the legacy default immediately at media readiness without waiting for a callback window
- [x] Atomically replace only a not-started default and require explicit interrupt fallback confirmation

## Notes
2026-07-15 kickoff: strict sequential inline execution started from synchronized main merge 77cf82f after TASK-260712-2hcq1g acceptance. Scope is Telegram inline audience/delivery routing with secure opaque callback compatibility and legacy no-action behavior. No later history replay task, Phase 2 inbox, manual real-client, audible or hardware evidence will be claimed.
2026-07-15 engineering candidate: durable voice-default plus route transaction, explicit-only audio/document clips, exact message-bound tg1 opaque callbacks with HMAC digests and 24-hour query replay, fresh Telegram ActorContext authorization, common target/DND/block/capability resolution, atomic not-started replacement, start-first too_late, whole-transmission overlay downgrade presentation, and second interrupt fallback confirmation are implemented. Green locally: coordinator vet/full tests/full race, focused routing race x5, fault-injection rollback, legacy FIFO regressions, bot HTTP keyboard contract, Windows vet/native tests/cross-compile, Swift release build, PlantUML render validation, board validation and diff check. Local Swift tests remain environment-blocked by the pre-existing missing Testing module. No manual real-app, Telegram-client, audible or hardware evidence is claimed; that remains in EPIC-260714-th54l3.
2026-07-15 accepted engineering evidence: exact code head 8fc47cf75b0f1ba521e80bd9d8a42885edacb217 passed all four hosted jobs in run 29370460972, including coordinator pinned rollback, authoritative macOS Swift tests, portable Windows build and signed packaged probe. Best-effort code/unit/integration/static/CI scope is accepted; manual real-app, Telegram-client, audible and physical-hardware evidence remains outside this task in EPIC-260714-th54l3. PR #46 tracking CI and merge remain.
2026-07-15 delivery: tracking head a9c6defb8def8aea277e24ece687ce9377c1e150 passed all four hosted jobs in run 29370645888; PR #46 merged to main at 912d08018cccda0589d5de7356bb8af8a20fd6f1. Strict execution advanced to TASK-260712-3e4p0c.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-21ers7/p1-telegram-history-presence-components.puml) — Component diagram for inline routing and legacy compatibility
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-21ers7/p1-telegram-history-presence-flows.puml) — Sequence diagram for inline routing and legacy fallback behavior
