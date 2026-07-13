## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-12T17:03:16Z

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
- [ ] Project only the allowed presence fields: online/offline, output state, playback state, capability support and DND.
- [ ] Add shared DND and block mutation surfaces for app and Telegram actors.
- [ ] Verify that presence outputs and bot status texts never expose microphone or process details.
- [ ] Layer local and orbit DND so remote control cannot loosen local recipient policy
- [ ] Test muted-until clock expiry, heartbeat staleness and role-scoped block mutations

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-1c1ska/p1-telegram-history-presence-components.puml) — Component diagram for presence and DND/block surfaces
- [p1-telegram-history-presence-states.puml](file://TASK-260712-1c1ska/p1-telegram-history-presence-states.puml) — State diagram for presence, DND and blocked receipt distinctions
