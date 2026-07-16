## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:25:04Z

## Last Update
2026-07-16T20:33:08Z

## Blocked By
- TASK-260712-ezdhpf
- TASK-260712-1ckdr7
- TASK-260712-3vzbbl

## Blocks
- TASK-260712-1rzqh9
- TASK-260712-39vjzd

## Checklist
- [ ] Hook the validated AppContainer-safe key-down or key-up path into the tray and capture lifecycle
- [x] Stream encoded live chunks with session generations, cancel semantics and backpressure handling
- [x] Add a bounded receiver jitter buffer and live duck or un-duck integration in the Windows audio graph
- [x] Stop capture and playback safely on release, lock, suspend, quit, permission revoke, reconnect or stale session
- [x] Add packaged tests or probes and publish remaining hardware-only evidence needs

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge 92c8b8807517921cfcc5d230dab07e4321383c67. Execute inline outside task-board spawn. Integrate accepted Windows live sender/receiver/runtime seams production-dark; do not advertise live_ptt_v1 or claim AppContainer key-down/up, packaged real-app audio or hardware evidence unless already provable from repository automation. Remaining physical evidence stays manual in TASK-260712-1rzqh9 under EPIC-260714-th54l3.
2026-07-16 engineering completion: production-dark Windows live PTT node integrates authoritative target/policy seams, atomic single-direction claims, generation-bound sender, bounded jitter receiver/mixer and a connection-bound FIFO websocket transport with 8 binary plus 16 control slots. Start/frame/end order is preserved and stale connection items are dropped. Code 100e447ab9633fc874d69e34c15f23e53ef3349a; PR #201; hosted run 29532276399 passed 4/4; main merge b4fb6f7abdf0f4f669b123afb9f3a136a0161efb. Focused live tests passed ten repeated runs, full Go vet/test/race and Windows amd64/arm64 CGO-free cross-builds passed, clean automated acceptance 12/12 at .temp/acceptance/20260716T202655Z/manifest.json. Checklist item 1 remains intentionally unchecked: RegisterHotKey supplies a toggle WM_HOTKEY, not proven AppContainer global key-down/up; no SetWindowsHookEx or production capability/composition was added. Packaged real-app hold/audio/hardware evidence stays manual in TASK-260712-1rzqh9 under EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p3-live-ptt-components.puml](file://TASK-260712-2jbo5i/p3-live-ptt-components.puml) — Task-boundary diagram for Windows live PTT integration
- [p3-live-ptt-sequence.puml](file://TASK-260712-2jbo5i/p3-live-ptt-sequence.puml) — Live session sequence that the Windows node must implement
- [p3-windows-live-ptt-node-integration.md](file://TASK-260712-2jbo5i/p3-windows-live-ptt-node-integration.md) — Engineering integration boundary, FIFO transport evidence and manual activation blockers
