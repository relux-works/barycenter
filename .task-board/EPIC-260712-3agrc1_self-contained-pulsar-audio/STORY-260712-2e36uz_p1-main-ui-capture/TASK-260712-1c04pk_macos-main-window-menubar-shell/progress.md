## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:29:06Z

## Last Update
2026-07-14T23:20:19Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1s6h6t
- TASK-260712-3dqc3l
- TASK-260712-3lg0ht
- TASK-260712-ut6akw

## Checklist
- [ ] Define the main-window and status-menu information architecture for the Phase 1 screens and quick actions.
- [ ] Implement localized shell state, keyboard navigation and VoiceOver labels for recording, DND and error states.
- [ ] Ensure the main-window and menu-bar flows remain usable while unpaired, degraded or recording.

## Notes
2026-07-15 strict sequential kickoff from synchronized main e10762bf6766bc4249d2ab6bedf46c256abe496a after PR #49. Implementing the macOS main-window and menu-bar shell inline with deterministic Swift tests and static verification; no real-app, audible, VoiceOver-on-device, or physical-hardware result will be claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-main-ui-capture-components.puml](file://TASK-260712-1c04pk/p1-main-ui-capture-components.puml) — Component diagram for the accepted macOS shell runtime and strict future capture, self-test, and data-integration seams
