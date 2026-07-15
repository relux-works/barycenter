## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-15T00:54:40Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1p8ykc
- TASK-260712-2fe5bz
- TASK-260712-25at8b
- TASK-260712-c7dmv8

## Checklist
- [x] Define the main-window and tray information architecture around the Phase 1 screens and quick actions.
- [x] Implement localized shell state, keyboard navigation and accessibility labels for recording, DND and error states.
- [x] Ensure the window and tray layout remain usable at 125, 150 and 200 percent scaling.

## Notes
2026-07-15 strict sequential kickoff from synchronized main merge 44c4bf3b60d9f4b29cda7639f7f2a1e775356025 after PR #52. Implementing the packaged Windows main-window/tray shell inline with honest unpaired/degraded copy, EN/RU catalog, keyboard/screen-reader semantics and deterministic DPI layout contracts. Real packaged-app, Narrator and physical 125/150/200 percent observations remain manual EPIC-260714-th54l3 evidence.
2026-07-15 implementation outcome: added a shared EN/RU WindowsShell projection; native Win32 Home/Create/Join/Try locally/History/Settings window; direct tray Open/Create/Join/Try/record/DND/Quit commands; honest unpaired startup and later-capability placeholders; persisted DND, route, presence and now-playing projection; textual non-color state tokens; Tab/dialog navigation and Ctrl accelerators; PerMonitorV2 font/layout handling with 100/125/150/200 percent geometry tests. Also repaired the existing inert tray menu caused by ignored TPM_RETURNCMD results. No real packaged-app/Narrator/DPI hardware result is claimed; manual evidence remains EPIC-260714-th54l3.
Accepted engineering head 90450979463dcf96035cc4278a79f4d528b0778f: after CI exposed Windows-target vet violations on the first head, RtlMoveMemory message payload handling fixed them; rerun 29380174085 passed all four jobs including portable Windows tests/cross-build and signed MSIX build/install/cleanup. Local Go race, amd64/arm64 builds, coordinator tests and Swift release passed. PlantUML CLI is unavailable locally; outcome source remains reviewable diagram code. Real packaged UI/Narrator/DPI hardware evidence remains only in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-9i5se7/p1-main-ui-capture-components.puml) — Implemented shared Windows shell projection, native main-window/tray commands, accessibility and PerMonitorV2 layout
