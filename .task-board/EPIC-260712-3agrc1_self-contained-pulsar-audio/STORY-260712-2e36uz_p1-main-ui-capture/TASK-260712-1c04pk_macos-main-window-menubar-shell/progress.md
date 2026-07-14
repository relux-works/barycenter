## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-14T23:27:52Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1s6h6t
- TASK-260712-3dqc3l
- TASK-260712-3lg0ht
- TASK-260712-ut6akw

## Checklist
- [x] Define the main-window and status-menu information architecture for the Phase 1 screens and quick actions.
- [x] Implement localized shell state, keyboard navigation and VoiceOver labels for recording, DND and error states.
- [x] Ensure the main-window and menu-bar flows remain usable while unpaired, degraded or recording.

## Notes
2026-07-15 strict sequential kickoff from synchronized main e10762bf6766bc4249d2ab6bedf46c256abe496a after PR #49. Implementing the macOS main-window and menu-bar shell inline with deterministic Swift tests and static verification; no real-app, audible, VoiceOver-on-device, or physical-hardware result will be claimed.
Accepted best-effort engineering scope on exact code head 895eddfdbab91c3e4cbdf1918136a704277627dd after all four hosted jobs passed in run 29375974503. SwiftUI shell, shared EN/RU state, AppKit bridge, shortcuts, accessibility semantics, runtime health/DND/volume/route projection, unit tests, documentation and diagram are complete. No real-app, live VoiceOver, audible, microphone, packaged-device, signing/notarization or physical-hardware result is claimed; those remain in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-1c04pk/p1-main-ui-capture-components.puml) — Component diagram for the accepted macOS shell runtime and strict future capture, self-test, and data-integration seams
