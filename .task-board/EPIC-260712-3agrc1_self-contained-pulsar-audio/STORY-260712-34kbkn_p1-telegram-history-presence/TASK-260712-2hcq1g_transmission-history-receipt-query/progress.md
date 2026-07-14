## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T21:07:17Z

## Blocked By
- TASK-260712-3coble
- TASK-260712-1gx6mh
- TASK-260712-1aprcb
- TASK-260712-1bpog0

## Blocks
- TASK-260712-21ers7
- TASK-260712-3d0zgu
- TASK-260712-3e4p0c
- TASK-260712-2fe5bz
- TASK-260712-3dqc3l

## Checklist
- [x] Build the recent-transmission query shape over media, transmissions, targets and memberships.
- [x] Map processing, ready, playing, played, partial, expired and error into stable Phase 1 client states.
- [x] Expose exact per-target reason codes and aggregate counts without inventing Phase 2 inbox behavior.
- [x] Enforce ActorContext tenant isolation, deterministic pagination and 30-day visibility
- [x] Expose requested versus effective delivery and authorization-derived action capabilities

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge 19ea2c1 after TASK-260712-1c1ska acceptance. Scope is the actor-scoped 30-day Phase 1 history/read-receipt model, deterministic pagination, exact visibility-safe target reasons, requested/effective delivery and authorization-derived actions. No Phase 2 inbox/replay behavior or manual/hardware evidence will be claimed.
2026-07-15 engineering candidate: common app/verified-Telegram history service, strict list/detail HTTP, media-only de-duplication, 30-day transmission receipts, retained failures, exact current-binding visibility, compact/full counts, requested/effective delivery, content honesty, policy-derived ordered actions, blocked-reason ownership redaction, and 24-hour actor/authorization/filter-bound digest-only cursors are implemented. Green locally: coordinator vet/full tests, focused history race x10, exact previous-HEAD suite, moderation check, Windows vet/tests/cross-build, Swift release build, board validation and diff check. Local Swift tests remain environment-blocked by the pre-existing missing Testing module; hosted macos-15 is authoritative. No manual real-app, real Telegram, audible or hardware evidence claimed.
2026-07-15 accepted engineering evidence: exact code head 742c1600aee20159a96b4a15dc20957c31edf9ed passed all four hosted jobs in run 29368167361, including coordinator pinned rollback, authoritative macOS Swift tests, portable Windows build and signed packaged probe. Best-effort code/unit/integration/static/CI scope is accepted; manual real-app, real Telegram, audible and physical-hardware evidence remains outside this task in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-2hcq1g/p1-telegram-history-presence-components.puml) — Component diagram for history and receipt query ownership
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-2hcq1g/p1-telegram-history-presence-flows.puml) — Sequence diagram for history and receipt updates across the Telegram flow
- [p1-telegram-history-presence-states.puml](file://TASK-260712-2hcq1g/p1-telegram-history-presence-states.puml) — State diagram for history and per-target receipt lifecycle
