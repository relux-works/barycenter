# BUG-260721-27irt6 — Windows UI stability remediation

Date: 2026-07-21
Host: `DESKTOP-3PBO632` (`mbpro-win`), Windows 10 Pro 19045
Baseline package: `ReluxWorksLLC.PulsarBarycenter_0.1.11.0_x64__q036g2bzd7ngc`

## User-visible failure

The production window flashes during refresh and navigation, then becomes
unresponsive after a section button is pressed. Windows removes the packaged
AppContainer shortly afterwards, which presents as a crash even though no Go
panic or WER crash record is emitted. The baseline UI also uses classic raised
Win32 controls, oversized typography, overlapping Home copy/actions and a
preferred client height that exceeds the 2560x1600 host at 200% scaling.

## Autonomous baseline evidence

- Host display: 2560x1600 at 192 DPI (200%).
- UI Automation invoked control `3002` (Create) from Home. The invocation
  blocked for 10,023 ms; only one step completed and the process/container was
  then removed.
- An independent `WM_COMMAND` navigation probe reproduced the same product
  path without UI Automation. Within 150 ms of command `3002`, the process was
  alive but `Responding=false`; AppModel-Runtime event 217 recorded container
  removal about 21 seconds after launch.
- `pulsar-crash.log` remained empty and no Application Error/WER crash existed,
  confirming an owner-thread hang rather than a captured Go panic.

## Root causes found in source

1. The repaired startup bounded only the initial Home render. Every non-Home
   section still walked, relabeled, enabled and showed/hidden the complete
   129-control P1/P2/P3 surface before returning to the Win32 message pump.
2. The one-second timer repeated `SetWindowText`, `ShowWindow` and button-state
   redraw work even when text and visibility had not changed.
3. Section changes hid and repainted controls one by one rather than committing
   one visual transaction.
4. Layout called `MoveWindow` for all 129 controls on every section change,
   including unchanged and hidden HWNDs. On Win10 that invalidated already
   correct sidebar pixels and left low-priority child paints queued behind the
   command handler.
5. A recursive top-level `RedrawWindow(...ALLCHILDREN|ERASE)` either blanked
   persistent chrome or made the command wait for every child, depending on
   whether the update was asynchronous or forced synchronously.
6. `pulsar.exe.manifest` did not request
   `Microsoft.Windows.Common-Controls` 6.0, leaving the production shell with
   classic control rendering.
7. The 1080x820-DIP preferred client becomes 2160x1640 physical pixels at the
   host DPI, taller than the physical display before the non-client frame.
8. Static surfaces used `WS_EX_CLIENTEDGE`, producing sunken diagnostic-tool
   panels; Home actions began before two lines of introduction text finished.

## Implemented direction

- Bound each render to exactly one active section and return before the next
  page renderer; add a re-entrancy guard for native/accessibility callbacks.
- Cache desired and applied child bounds. Do not move unchanged HWNDs, and do
  not physically move hidden pages until they become visible.
- Keep navigation/header HWNDs alive across page swaps. Repaint the clipped
  parent background synchronously and refresh only the small structural chrome
  set; never recursively erase or redraw the complete control tree.
- Skip unchanged window text and visibility. Resize repaints only controls
  whose applied bounds actually changed.
- Recover and log an unexpected main-window callback panic instead of tearing
  down the entire AppContainer.
- Enable Common Controls v6, flatten card/status surfaces, use calmer Segoe UI
  hierarchy, compact navigation, explicit text status, and active-page marker.
- Reduce unavailable Inbox/Automation and not-yet-configured streamed-track
  pages to the actions the user can actually take instead of walls of disabled
  controls; normalize stale collection selections before rendering.
- Fit the preferred client at 1040x700 DIP (minimum 900x680), including the
  dense History/Inbox/Air control grids.

## Accepted autonomous result

Installed package:

- Full name:
  `ReluxWorksLLC.PulsarBarycenter_0.1.20.0_x64__q036g2bzd7ngc`
- Signed MSIX SHA-256:
  `f74b5c8d6f8c86443f8c1b64715977be1b0183c39e7fc4dde7567c957b958348`
- Installed signed EXE SHA-256:
  `0a77f53f026b77dd6abc3b265f18a8d32744847ca23571e97ddd999cc17a0042`
- Reproducible unsigned source EXE SHA-256:
  `6b10ef90eda818ce397c78775b609ca4eeecd67ccd97c23dab5f9b2db2fffbb3`
- Signature status: `Valid`; signer
  `CN=60105954-A0D9-4E89-B32D-18AF2F423ABE`.
- Installation and launch used the product MSIX and the Explorer Desktop Shell
  default verb for `Pulsar.lnk`; no user-visible terminal is part of startup.

Deterministic gates all passed from `pulsar-win`:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-H=windowsgui' ./...`
- production `windowsgui` build with generated Win32 resources at version
  `0.1.20.0`.

Win10 product evidence:

- UI Automation navigation soak: 24 cycles across all 10 sections, `240/240`
  successful invocations; max `93 ms`, p95 `80 ms`; process running,
  responding and not hung afterward.
- Idle paint gate: `35` samples after a `500 ms` settle produced exactly one
  frame hash across several one-second refresh ticks.
- Native resource stability: handles `347 -> 346`; working set
  `54,341,632 -> 67,420,160` bytes.
- Crash evidence: callback crash log `0 -> 0` bytes; AppModel removal events
  `0`; Application Error events `0`.
- Direct `SendMessageTimeout(WM_COMMAND)` soak: `240/240`, median `59 ms`,
  p95 `88 ms`, max `106 ms`, still responding and not hung.
- DPI/window: `192 DPI`; product window `2106x1471` physical pixels, within the
  2560x1600 host display.
- Loaded control implementation:
  `WinSxS\\amd64_microsoft.windows.common-controls_..._6.0.19041.6456...`.
- A `PrintWindow(PW_RENDERFULLCONTENT)` gallery captured History, Inbox, Air,
  Automation, Settings and Home `150 ms` after each command. Every capture had
  all 10 navigation entries, one active marker, title/status and the bounded
  active-page controls. Evidence JSON SHA-256:
  `71c0f8f1555d3771b3f515dc554283b98ab46463de1a34c39f726945909eb492`;
  dense Air screenshot SHA-256:
  `5a1c86b54a2c1b5bdc1816934908c7a548701a6a023f0ecc641804282436e651`.
- Navigation-soak JSON SHA-256:
  `11ffaa9d6bfef46037e3a5677a31b991a4f51a157e216ee2271a0cac9e52b2ff`;
  final Home screenshot SHA-256:
  `c9c3fdba98606711f20a3ce55e9a1d884f451c2e8a43810e9c63b4b50f3570ef`;
  direct message-pump JSON SHA-256:
  `d19f7d58dbc39d638b5dc0e3076e7165c460af1742b9495c5cfabfab872c629e`.

## Independent review and CI portability delta

- Claude Fable 5 run `RUN-260721-305602` returned HTTP 429 with zero tokens;
  it is recorded as provider exhaustion, not a review verdict.
- Claude Opus 4.8 run `RUN-260721-c80cef` independently reviewed exact product
  commit `76f09a4`, re-ran Go test/race/vet/Windows cross-build gates, verified
  the render/repaint mechanisms and accepted the task without blocking
  findings. Verdict evidence is attached to the board as
  `BUG-260721-27irt6_review.md`.
- The first GitHub packaged-probe run then exposed a test-only portability bug:
  the source-contract assertion expected LF while Windows checkout supplied
  CRLF. The assertion now normalizes `CRLF -> LF` before matching. Production
  source and the installed `0.1.20.0` binary are unchanged by this delta.

## Verification boundary

Automated unit/source/layout checks, race/vet/cross-build, signed-package
installation and autonomous Win10 navigation/idle soak are engineering
evidence. Final subjective appearance, microphone/audible behavior and Store
acceptance remain in the separate manual-testing epic.
