## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:53Z

## Last Update
2026-07-15T02:04:29Z

## Blocked By
- TASK-260712-1c04pk
- TASK-260712-30abcm

## Blocks
- TASK-260712-1s6h6t

## Checklist
- [x] Select and implement an App Sandbox compatible configurable shortcut path
- [x] Keep menu-bar, window, cue and capture-controller state consistent while hidden
- [x] Cover conflicts, unsupported global registration, sleep, quit and stale-hook cleanup without silent Accessibility expansion
- [x] Keep Escape foreground-scoped and provide hidden-window Cancel without a global bare-Esc hook

## Notes
2026-07-15 strict sequential kickoff from synchronized main merge 3f9cbdbf86ca55fb87f1c6933535c517d3a7a516 after PR #55. Implementing configurable sandbox-safe toggle recording and menu/button/cancel lifecycle inline over the accepted macOS shell and capture controller. Focused Escape remains local; real global-key conflict behavior, hidden UI and physical input observations remain manual EPIC-260714-th54l3 evidence.
2026-07-15 implementation outcome: exclusive Carbon RegisterEventHotKey controller with bounded modifier-bearing presets, typed conflict/unavailable states, generation-based stale-callback rejection, validated persistence, idempotent sleep/lock/wake/quit cleanup, foreground-only Escape and explicit hidden-window Cancel. Window/menu fallback remains independent and no Accessibility entitlement, NSEvent global monitor or event tap was added. Local 202 Swift tests, release build, app package, cue/plist/codesign and entitlement checks passed. GitHub Actions run 29383052378 passed coordinator, 202 hosted Swift tests, Windows cross-build and signed MSIX package/install/cleanup. Real keyboard, conflict, hidden UI, sleep/lock and packaged sandbox observations remain manual EPIC-260714-th54l3 evidence. Accepted engineering head 188c30d6bb899a77c23bb415602b99b62b9990f2.

## Precondition Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-ut6akw/p1-main-ui-capture-components.puml) — macOS shortcut and capture seam context

## Outcome Resources
- [p1-macos-recording-shortcut-controller.md](file://TASK-260712-ut6akw/p1-macos-recording-shortcut-controller.md) — Sandbox API decision, shortcut lifecycle, cleanup guarantees and manual-evidence boundary
- [p1-macos-recording-shortcut-lifecycle.puml](file://TASK-260712-ut6akw/p1-macos-recording-shortcut-lifecycle.puml) — Implemented shortcut, fallback, cancel and lifecycle sequence
