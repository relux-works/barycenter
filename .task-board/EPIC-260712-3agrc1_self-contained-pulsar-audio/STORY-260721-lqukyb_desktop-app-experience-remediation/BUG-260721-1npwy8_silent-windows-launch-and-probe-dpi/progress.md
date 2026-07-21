## Status
development

## Review
required

## Task Class
code

## Blocked By
- (none)

## Blocks
- TASK-260721-2lv6wn

## Checklist
- [x] Hide console-session observer process windows without changing the packaged app process or session.
- [x] Assert GUI-subsystem executable and MSIX launch contract in deterministic tests.
- [x] Implement probe PerMonitorV2 fonts, geometry and WM_DPICHANGED relayout for 96/120/144/192 DPI.
- [x] Preserve capture, permission, lifecycle, evidence and package regression suites.

## Notes
2026-07-21 inline execution started from a8b2303629b6d1f7341076f6f01600a7888f347d after owner-directed autonomous consolidation. Root cause of the reported terminal window is the temporary scheduled PowerShell observer, while the signed probe already uses -H windowsgui. Scope includes hidden observer launch, executable-subsystem assertions and probe DPI/system-font remediation; no product/hardware pass is inferred.
2026-07-21 implementation complete: maintained hidden interactive observer task runner uses -WindowStyle Hidden; build/package contract parses PE headers and requires subsystem 2; probe uses PMv2, Segoe UI, responsive geometry, WM_SIZE/WM_DPICHANGED and deterministic 96/120/144/192-DPI layout tests. Local go test ./..., probe race, Windows go vet and GUI cross-build pass; file(1) identifies PE32+ GUI. Windows PowerShell/MSIX job is delegated to hosted CI; no hardware pass is inferred.

## Precondition Resources
(none)

## Outcome Resources
(none)

## Created
2026-07-21T10:56:31Z

## Last Update
2026-07-21T11:10:28Z

## Assigned To
codex-root-inline
