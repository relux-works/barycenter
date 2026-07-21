# Reviewer Verdict — BUG-260721-1npwy8 (accepted)

## Scope reviewed
Commit 69318f9 fix(desktop): make probe launch silent and DPI aware.

## Acceptance Criteria — all met
1. Silent launch — Probe built with -H windowsgui (build-probe.ps1). PE-subsystem contract (package-contract.ps1 Get-WindowsExecutableSubsystem/Assert-ProbeGUIExecutable) asserts subsystem 2 at build time and in tests. Console-session observer runner (register-hidden-interactive-task.ps1) launches powershell.exe with -WindowStyle Hidden -NonInteractive -NoProfile -ExecutionPolicy Bypass under an Interactive/Highest scheduled-task principal; ConvertTo-HiddenTaskArgument rejects embedded double quotes (injection guard). Does not alter the packaged app process/session.
2. GUI-subsystem assertion is deterministic — package-contract.Tests.ps1 writes synthetic PE32+ fixtures and asserts subsystem 2 accepted / subsystem 3 rejected. Optional-header Subsystem offset (PE+24+68) is correct for both PE32 and PE32+.
3. DPI — SetProcessDpiAwarenessContext(PMv2 = -4) selected before any HWND creation; AdjustWindowRectExForDpi for outer size; WM_DPICHANGED relayout applies the suggested rect via SetWindowPos(SWP_NOZORDER|SWP_NOACTIVATE) then refonts+relayouts; WM_GETMINMAXINFO reports DPI-scaled minimum; Segoe UI font recreated per-DPI (updateUIFont) and deleted on close. Deterministic ui_layout_test.go covers 96/120/144/192, resize-growth, and min-clamp with no-overlap/in-bounds invariants.
4. Regression suites preserved — all Go tests pass; PowerShell package/observer contract tests wired into CI Windows job. No test/evidence depends on the changed window title (Pulsar hardware verification) or intro string.

## Architecture fit
Probe embeds no RT_MANIFEST (winres/syso targets the main app only), so the runtime PMv2 selection is the sole DPI mechanism and succeeds rather than colliding with an already-set awareness context. createWindows has a single caller, so SetProcessDpiAwarenessContext is not re-invoked. Layout logic is platform-independent (ui_layout.go, no build tag) and unit-tested on the host.

## Verification performed on review host
- go test -count=1 ./... : ok (pulsar-win, cmd/pulsar-win-probe, internal/winprobe, wire)
- GOOS=windows GOARCH=amd64 go vet ./cmd/pulsar-win-probe/ : clean
- GOOS=windows GOARCH=amd64 go build ./cmd/pulsar-win-probe/ : ok
- pwsh unavailable on macOS review host; PowerShell contract tests reviewed by inspection and execute in the hosted Windows CI job.

## Notes
Prior reviewer spawn RUN-260721-3a8700 exit=1 was an HTTP 429 Fable 5 rate-limit, not a substantive rejection.

Verdict: accepted -> done.