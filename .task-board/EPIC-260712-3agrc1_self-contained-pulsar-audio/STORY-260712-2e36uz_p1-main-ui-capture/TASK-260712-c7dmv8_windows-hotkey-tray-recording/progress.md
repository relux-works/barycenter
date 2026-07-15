## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:53Z

## Last Update
2026-07-15T02:53:43Z

## Blocked By
- TASK-260712-9i5se7
- TASK-260712-2w4gyw

## Blocks
- TASK-260712-1p8ykc

## Checklist
- [x] Register and reconfigure the toggle hotkey on the tray UI thread
- [x] Keep tray, window, cue and capture-controller state consistent while hidden
- [x] Cover conflicts, Esc, lock, suspend, quit and stale-registration cleanup
- [x] Keep Escape foreground-scoped and provide hidden-window Cancel without a global bare-Esc hook

## Notes
2026-07-15 kickoff: strict inline execution started from main 0b7ac742b6f7a263f203c7c0ff58489704b1d529 on task/task-260712-c7dmv8-windows-hotkey-tray-recording. Engineering scope covers deterministic controller/unit/source/cross-build evidence; signed real-Windows hotkey, conflict, hidden tray, lock and suspend behavior remains manual in EPIC-260714-th54l3.
Accepted 2026-07-15 on exact engineering head e70e2ea25ee7a7e335032336b6d962ec9517e230. Local Go tests/vet/race, repeated focused race, Windows amd64 build/test compile, coordinator full+pinned rollback, 202 Swift tests, board validation and diff checks passed. GitHub Actions run 29385014150 passed coordinator, node-core, pulsar-win and signed MSIX package/install/cleanup after rerunning an initial proxy.golang.org HTTP/2 module-download failure without changing the head. Physical shortcut/conflict/tray/microphone/cue/lock/suspend/AppContainer behavior remains manual in EPIC-260714-th54l3.

## Precondition Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-c7dmv8/p1-main-ui-capture-components.puml) — Windows hotkey and capture seam context

## Outcome Resources
- [TASK-260712-c7dmv8-windows-hotkey-tray-recording.puml](file://TASK-260712-c7dmv8/TASK-260712-c7dmv8-windows-hotkey-tray-recording.puml) — Owner-thread, shortcut lifecycle, capture state and UI fallback architecture
- [TASK-260712-c7dmv8-windows-hotkey-tray-recording-verification.md](file://TASK-260712-c7dmv8/TASK-260712-c7dmv8-windows-hotkey-tray-recording-verification.md) — Automated evidence and explicit manual-test boundary
