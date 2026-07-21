## Status
development

## Review
required

## Task Class
code

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [x] Capture current Win10 crash, flicker, DPI and process-lifecycle evidence
- [x] Document root causes in message dispatch, repaint, layout and control lifecycle
- [x] Implement stable rendering and crash-safe command handling
- [x] Deliver coherent system-native Windows shell layout, typography and states
- [x] Add deterministic regression tests and pass Go race vet and Windows build checks
- [x] Build sign install and autonomously soak the package on mbpro-win
- [ ] Complete independent review and record outcome resources

## Notes
Engineering gate accepted on mbpro-win: signed MSIX 0.1.20.0 installed; UIAutomation 240/240 (max 93 ms, p95 80 ms), idle frame hashes 1, handles 347 to 346, no crash/AppModel events; direct WM_COMMAND 240/240; 150 ms PrintWindow gallery complete. Manual visual/audio acceptance remains out of scope.

## Precondition Resources
(none)

## Outcome Resources
- [BUG-260721-27irt6_windows-ui-stability-remediation.md](file://BUG-260721-27irt6/BUG-260721-27irt6_windows-ui-stability-remediation.md) — Root-cause analysis, implementation summary, signed 0.1.20.0 receipt and autonomous Win10 evidence hashes

## Created
2026-07-21T17:31:02Z

## Last Update
2026-07-21T19:44:57Z

## Assigned To
codex
