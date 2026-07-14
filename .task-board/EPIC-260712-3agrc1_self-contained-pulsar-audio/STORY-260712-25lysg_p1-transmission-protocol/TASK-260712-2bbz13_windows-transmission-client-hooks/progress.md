## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T10:21:24Z

## Blocked By
- TASK-260712-1g70av

## Blocks
- TASK-260712-2qc27p
- TASK-260712-3d6cnn

## Checklist
- [ ] Announce clip capabilities and implement prepare/download/hash/decode-ready flow
- [ ] Emit transmission lifecycle and DND or presence messages from the player path
- [ ] Keep legacy play_voice and solo_voice working while routing scheduled play through mixer hooks
- [ ] Use synchronized coordinator time and reject stale, duplicate or cancelled scheduled starts
- [ ] Keep prepare I/O and scheduling out of render and hub blocking paths

## Notes
Strict inline execution started from synchronized main merge 0b54899073e4dc4948b248f7c77666e7151f5459 on branch task/task-260712-2bbz13-windows-transmission-client-hooks. Scope is best-effort Windows coding and deterministic unit, integration, race and cross-build verification of the frozen clip lifecycle. No packaged-app, audible-output, Windows 10 or Windows 11 physical-hardware result will be claimed; those stay in manual epic EPIC-260714-th54l3.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-2bbz13/p1-transmission-scheduler-sequence.puml) — Windows client flow for prepare, ready, play, and cancel

## Outcome Resources
(none)
