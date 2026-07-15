## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-15T03:15:28Z

## Blocked By
- TASK-260712-2lrpc0
- TASK-260712-1c04pk
- TASK-260712-30abcm
- TASK-260712-3lg0ht
- TASK-260712-ut6akw

## Blocks
- TASK-260712-3dqc3l

## Checklist
- [x] Implement explicit TCC-gated capture, device selection, level metering and the local loopback path.
- [x] Wire toggle recording, start and stop cues, Esc cancel, temp-file cleanup and clean stop behavior around app lifecycle events.
- [x] Integrate the configurable global hotkey plus file picker and drag-drop with clear fallback behavior when the hotkey is unavailable.

## Notes
2026-07-15 kickoff: strict inline execution started from synchronized main 707593ecf43c6ad31a9c60676940ff7f8a941e34 on task/task-260712-1s6h6t-macos-local-capture-self-test. Scope is deterministic integration of already accepted macOS capture, self-test/file intake, shell and shortcut seams; real TCC dialogs, microphone/routes, audible cues, physical shortcut, Finder drag/drop, sleep and signed-app observations remain manual in EPIC-260714-th54l3.
2026-07-15 accepted on exact engineering head f8e9db9. One app composition now binds accountless and paired production audio, TCC capture, persisted input/hotkey, exact five-second self-test, file intake, serialized normal-recording cues, durable drafts, typed shell states and idempotent lifecycle cleanup. Local gates passed: 205 Swift tests, coordinator vet/full/moderation/pinned rollback, Windows vet/full/cross-build, release app packaging/canonical cue/plist/codesign, board validation and diff checks. GitHub Actions run 29385946438 passed all four jobs including signed MSIX build/install/cleanup. No physical microphone, TCC UI, audible route/cue, Finder, real hotkey conflict, sleep/session or signed production-app observation is claimed; all remain in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-1s6h6t/p1-main-ui-capture-components.puml) — Component diagram for task placement and dependencies
- [p1-main-ui-capture-flows.puml](file://TASK-260712-1s6h6t/p1-main-ui-capture-flows.puml) — Flow diagram for local self-test and record/send behavior
- [p1-macos-local-capture-self-test-integration.md](file://TASK-260712-1s6h6t/p1-macos-local-capture-self-test-integration.md) — Accepted application composition, lifecycle, local-only and manual-boundary handoff
- [p1-macos-capture-workflow-components.puml](file://TASK-260712-1s6h6t/p1-macos-capture-workflow-components.puml) — Integrated macOS capture component ownership diagram
- [p1-macos-capture-workflow-sequence.puml](file://TASK-260712-1s6h6t/p1-macos-capture-workflow-sequence.puml) — Normal recording and exact self-test sequence diagram
