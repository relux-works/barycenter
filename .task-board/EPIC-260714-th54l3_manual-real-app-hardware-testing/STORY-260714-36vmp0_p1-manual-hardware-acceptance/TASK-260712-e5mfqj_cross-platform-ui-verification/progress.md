## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-15T09:56:23Z

## Blocked By
- TASK-260712-2fe5bz
- TASK-260712-3dqc3l
- TASK-260712-1vtwkl
- TASK-260712-2hodti

## Blocks
- TASK-260712-1fpb9q

## Checklist
- [ ] Add or document the test harnesses and manual probes needed for local self-test, recording, hotkey and degraded-state coverage.
- [ ] Run the Windows keyboard, screen-reader and 125, 150, 200 percent DPI matrix plus the macOS parity checks.
- [ ] Collect sanitized screenshots, logs and remaining external blockers for review and later certification evidence.
- [ ] Capture all six EN and six RU exact-build Store scenes from docs/store/phase1/screenshots.json and populate reviewed hashes
- [ ] Run WACK against the same signed MSIX in an interactive Windows session and preserve the reviewed manifest/report

## Notes
2026-07-14 scope routing: moved to EPIC-260714-th54l3 Manual real-app hardware testing. This task is deferred for hands-on execution in original sequence and no longer gates best-effort coding, unit tests or deterministic CI in EPIC-260712-3agrc1.
2026-07-15 Store asset handoff added without creating a 206th original task: this existing manual real-app task owns twelve localized exact-build screenshots and the reviewed WACK run described by docs/store/phase1. Engineering validator and templates do not claim either observation ran.

## Precondition Resources
- [phase1-store-manual-handoff.md](file://TASK-260712-e5mfqj/phase1-store-manual-handoff.md) — Exact screenshot slots, WACK procedure and fail-closed Store package handoff

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-e5mfqj/p1-main-ui-capture-components.puml) — Component diagram for task placement and dependencies
- [p1-main-ui-capture-flows.puml](file://TASK-260712-e5mfqj/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
