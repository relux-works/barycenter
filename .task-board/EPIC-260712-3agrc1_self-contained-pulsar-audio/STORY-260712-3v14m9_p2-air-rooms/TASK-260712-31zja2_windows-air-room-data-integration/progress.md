## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:56Z

## Last Update
2026-07-15T15:34:21Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-2fe5bz

## Blocks
- TASK-260712-3nq0tq
- TASK-260712-cuplon

## Checklist
- [x] Integrate current Air and pending join state into Windows read models
- [x] Wire create, join, confirm, leave, and dissolve actions to the new API and stable errors
- [x] Render alias backed two party Airs without raw ids or target snapshot assumptions
- [x] Verify the dependency boundary with the phase one UI shell and the explicit targets story
- [x] Render saved and active Airs and require confirmation for disruptive switch, leave or dissolve
- [x] Verify keyboard, screen-reader and high-DPI lifecycle flows

## Notes
Strict inline execution started from synchronized main 145a902 after accepted macOS Air tracking merge. Reusing the frozen common Air API and inspecting the existing Windows packaged shell, localization, accessibility, high-DPI and test seams before implementation.
Accepted after engineering commit 8b458d8, PR #96, hosted run 29428413069 green 4/4, and merge 203bb1e. Exact final Windows automated suite passed 7/7; full repository suite passed 12/12; Go tests, vet, race, amd64 vet/build, arm64 build and blind Windows compile passed. Native EN/RU Air management covers saved/current/pending state, full lifecycle and policy presets, explicit disruptive confirmation, stable errors, secure invite redaction/expiry and keyboard/screen-reader/DPI seams without raw IDs or Phase One target/inbox coupling. One pre-existing websocket teardown flake occurred once locally; uncached retry passed and the exact test passed 10/10. No real-app, physical-hardware, live screen-reader or live high-DPI result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p2-windows-air-room-data-integration.md](file://TASK-260712-31zja2/p2-windows-air-room-data-integration.md) — Accepted implementation handoff for Windows Air room management
