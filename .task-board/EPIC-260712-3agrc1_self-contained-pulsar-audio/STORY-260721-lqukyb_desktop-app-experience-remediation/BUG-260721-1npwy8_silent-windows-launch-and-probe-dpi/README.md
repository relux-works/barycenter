# BUG-260721-1npwy8: silent-windows-launch-and-probe-dpi

## Description
Remove visible terminal windows from autonomous Windows launch instrumentation and make the signed verification probe use an explicit GUI launch contract, PerMonitorV2 scaling, system fonts and DPI-scaled geometry.

## Scope
Update the console-session observer runner to use a hidden PowerShell window and assert the packaged executable is GUI subsystem. Update pulsar-win-probe window creation, fonts, DPI change handling, accessible labels and diagnostic identity. Preserve H00-H17 behavior and logging semantics.

## Acceptance Criteria
Starting the observer or signed probe never creates a visible console window. Binary/package checks prove GUI subsystem. Probe text and controls use system UI fonts and scaled geometry at 96/120/144/192 DPI; WM_DPICHANGED relayout is deterministic. Existing capture/lifecycle tests and MSIX contract checks remain green.
