## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T20:34:35Z

## Blocked By
- TASK-260712-3coble
- TASK-260712-1gx6mh
- TASK-260712-51y5k9
- TASK-260712-2xkyot

## Blocks
- TASK-260712-21ers7
- TASK-260712-3d0zgu
- TASK-260712-3e4p0c
- TASK-260712-2fe5bz
- TASK-260712-3dqc3l
- TASK-260712-25862f

## Checklist
- [x] Project only the allowed presence fields: online/offline, output state, playback state, capability support and DND.
- [x] Add shared DND and block mutation surfaces for app and Telegram actors.
- [x] Verify that presence outputs and bot status texts never expose microphone or process details.
- [x] Layer local and orbit DND so remote control cannot loosen local recipient policy
- [x] Test muted-until clock expiry, heartbeat staleness and role-scoped block mutations

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge 4f026a0 after TASK-260712-3dmllz acceptance. Scope is privacy-safe heartbeat presence, layered local/orbit DND and role-scoped actor/orbit block surfaces with exact scheduler reasons. Execution remains outside task-board spawn workflow; no manual, real-app or hardware evidence will be claimed.
2026-07-14 engineering gate: added privacy-allowlisted GET /v1/presence with current-generation 12-second liveness, sanitized heartbeat playback/output state and deterministic capability ordering; atomic expected-revision DND with digest-only actor-scoped idempotency; viewer-scoped ar_/or_ subject refs and bl_ block IDs; app/verified-Telegram shared ActorContext services; generation-safe pending/active DND and sender-block cancellation; and privacy-safe Telegram /status. Added HTTP/store/hub tests, implementation handoff and updated component/state diagrams. Coordinator vet/full/race, exact previous-head rollback, Windows vet/tests, Swift release build, board validation and diff checks are green. No real-app, audible, physical-device or hardware evidence is claimed; it remains in EPIC-260714-th54l3.
Exact engineering head a65fc659e3ae389484163723aa63a3806f4b986d passed all four hosted CI jobs in run 29365735642, including authoritative macOS Swift tests and the signed packaged Windows probe. Engineering acceptance is complete; physical/manual evidence remains unclaimed.
Tracking head f4f718b3cd54143d9a5c9d6c5a1e39fe46d724d0 passed all four hosted jobs in run 29365971499; PR #44 landed at merge 19ea2c1fd58dd066cbe0a217dd28972a4ff77a6b.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-1c1ska/p1-telegram-history-presence-components.puml) — Component diagram for presence and DND/block surfaces
- [p1-telegram-history-presence-states.puml](file://TASK-260712-1c1ska/p1-telegram-history-presence-states.puml) — State diagram for presence, DND and blocked receipt distinctions
