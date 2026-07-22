## Status
backlog

## Review
required

## Task Class
code

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [ ] Reproduce Pane and missing-pattern semantics on exact 0.1.20 packaged candidate
- [ ] Expose Button/Edit UIA control types, patterns, labels, and keyboard focus
- [ ] Add packaged Windows UI Automation regression coverage without submitting Join
- [ ] Verify native onboarding behavior and deterministic package build remain green

## Notes
Observed 2026-07-21T23:49:14Z on exact installed 0.1.20.0 package at 192 DPI. Remote readiness receipt SHA-256 5b565b293fd11e6e4a106b478700efa544610646eb19d9c7444a6f5f93f32745; canonical evidence is outcome TASK-260722-1zv67l_windows-readiness.json on TASK-260722-1zv67l. Native controls: 3003 Button, 3027 Edit with WS_TABSTOP, 3010 Button; visible/enabled and native navigation worked. UIA reports Pane/Pane/Pane, input IsKeyboardFocusable=false and ValuePattern=false, action InvokePattern=false. No invitation was entered, 3010 was not invoked, no credential was created, and no accessibility/manual PASS is claimed. This is a focused non-blocking semantics bug for the ordinary native owner flow.

## Precondition Resources
(none)

## Outcome Resources
(none)

## Created
2026-07-21T23:58:37Z

## Last Update
2026-07-22T00:07:01Z
