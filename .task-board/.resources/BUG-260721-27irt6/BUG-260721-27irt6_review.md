# BUG-260721-27irt6 — Independent Review Verdict: ACCEPTED

Reviewer run: RUN-260721-c80cef. Review anchor: commit `76f09a4` on
`tracking/bug-260721-27irt6-windows-ui-stability` (HEAD confirmed == anchor).
Note: prior reviewer run RUN-260721-305602 exited 1 due to a Fable-5 API rate
limit (HTTP 429), not a real verdict; this is the first substantive review.

## Verdict
ACCEPTED → `done`. Implementation matches AC, fits the project's pure-syscall
Win32 architecture, and all deterministic gates are green on this host.

## What I verified

### Gates re-run locally (macOS host, this session)
- `go build` (GOOS=windows GOARCH=amd64 CGO_ENABLED=0 -H=windowsgui ./...): PASS
- `go vet ./...`: PASS
- `go test ./...`: PASS (all packages)
- `go test -race ./...`: PASS
- New regression tests execute and pass:
  TestWindowsDenseSectionsFitMinimumClientAtSupportedDPIs,
  TestWindowsProductionRenderIsSectionBounded,
  TestWindowsExecutableManifestEnablesModernCommonControls,
  TestWindowsProductionWindowWiresThemeAccessibilityAndFocusContracts,
  TestWindowsShellSnapshotNormalizesEveryCollectionSelection,
  TestWindowsShellStatesNeverDependOnColorAlone.

### Code review — claimed fixes are real and correct (main_window_windows.go)
- Re-entrancy guard: `render()` early-returns on `ctx.rendering`; guard set/reset
  via defer. Prevents nested full renders from native/AX callbacks.
- Bounded per-section render: each section gate calls
  `renderSectionBody(...)` then `return`, so no page falls through into
  off-screen renderers (source-order + fallthrough guarded by a test).
- Bounds caching: `move()` records `controlBounds`, skips hidden HWNDs, and
  skips already-applied `appliedBounds`; `showControl` applies cached bounds on
  show. No per-tick MoveWindow churn.
- Repaint transaction: section change hides page controls, invalidates the
  clipped parent once, and repaints only structural chrome
  (`repaintStructuralChrome`). No recursive RDW_ALLCHILDREN remains.
- Skip-unchanged text/visibility: `setText` no-ops when text is unchanged;
  `showControl` no-ops when visibility is unchanged → idle tick produces no
  repaint work.
- Crash guardrail: `mainWindowProc` recovers panics, logs message+stack, and
  returns 0 to keep the shell alive instead of tearing down AppContainer.
- Common-Controls v6 dependency added to `pulsar.exe.manifest`; layout metrics,
  min/preferred client, and dense-grid gaps retuned to fit the host at 100-200%.

### Tests strengthened, not weakened
- Color-independence test now asserts the label equals the localized status
  word (stronger than the old `[` prefix check).
- Added full collection-selection normalization coverage (11 indices) and
  no-clip layout coverage at 96/120/144/192 DPI.

### Autonomous Win10 evidence (recorded, cannot re-run here — no Windows host)
Signed MSIX 0.1.20.0; UIAutomation 240/240 (max 93 ms, p95 80 ms); direct
WM_COMMAND 240/240 (p95 88 ms); idle frame-hash count = 1 over 35 samples;
handles 347→346; zero crash/AppModel-removal/AppError events; PrintWindow
gallery across all sections. Evidence hashes recorded in the remediation doc.

## Non-blocking observations (acceptable tradeoffs, disclosed)
1. Stability logic in main_window_windows.go is `//go:build windows`, so
   crash/flicker/hang ACs are validated by the autonomous Win10 soak plus
   brittle source-string regression guards rather than behavioral unit tests —
   an inherent platform limitation, honestly bounded in the report.
2. `windowText` caps at 2000 chars; body text beyond that would defeat the
   setText skip-unchanged optimization (minor idle inefficiency, unlikely).
3. Panic recovery returns result=0 for all messages — a safe keep-alive
   fallback, though not the correct default for every Win32 message.

## Boundary
Manual subjective visual/audio and Store acceptance are correctly kept out of
scope in the separate manual-testing epic; the report does not claim manual
acceptance. Honest.
