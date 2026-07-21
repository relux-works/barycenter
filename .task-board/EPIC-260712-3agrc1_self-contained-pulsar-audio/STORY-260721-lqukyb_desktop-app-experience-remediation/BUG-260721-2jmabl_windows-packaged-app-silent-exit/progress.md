## Status
to-review

## Review
required

## Task Class
code

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [x] Capture reproducible AUMID process lifetime, exit behavior and startup artifacts on mbpro-win
- [x] Identify and repair the silent startup-exit path
- [x] Add deterministic regression tests and startup diagnostics
- [x] Build and install the repaired production-schema MSIX on mbpro-win
- [x] Record autonomous process, window, package and hash evidence
- [ ] Complete independent code review and close actionable findings

## Notes
2026-07-21: Reproduced from repeated user shortcut clicks. AppModel/TWinUI activation succeeds, AppContainer PIDs live about one second, no WER/Application Error is emitted, package status is Ok, and LocalCache/Roaming/Pulsar/pulsar.log exists at zero bytes. Treating this as an application startup exit rather than shortcut registration failure.
2026-07-21 candidate: installed package 0.1.11.0 (MSIX b8374791..., EXE 839b00a8...) on DESKTOP-3PBO632. Exact AUMID run stayed alive/visible for 31.199s in 120/120 samples; UI Automation reported responding=true, visible=true, hung=false at 192 DPI and the process remained alive after observer completion. Root cause was unbounded all-section initial render before the message pump, compounded by hidden top-level activation and broken GUI log fan-out. Manual audio/hardware acceptance remains outside this bug.

## Precondition Resources
(none)

## Outcome Resources
- [windows-packaged-launch-evidence.md](file://BUG-260721-2jmabl/windows-packaged-launch-evidence.md) — Reproduction, root cause, exact Win10 package hashes, 31-second AUMID soak and responsive visible-window evidence

## Created
2026-07-21T15:47:23Z

## Last Update
2026-07-21T16:34:19Z

## Assigned To
codex
