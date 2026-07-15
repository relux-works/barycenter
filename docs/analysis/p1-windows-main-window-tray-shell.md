# P1 Windows main-window and tray shell

- Task: `TASK-260712-9i5se7`
- Engineering boundary: native Win32 shell plus deterministic portable tests
- Manual evidence: `EPIC-260714-th54l3`

## Information architecture

The Windows executable now has one thread-safe `WindowsShell` projection and
two native presentations. The main window contains Home, Create, Join, Try
locally, History and Settings destinations. Home keeps the three primary flows
above textual presence, route and now-playing cards, then local recording/DND/
volume state and an honest history placeholder. Dedicated destinations remain
reachable even when their later capture or data integration is unavailable.

The tray exposes Open, Create, Join, Try locally, recording state, DND and
Quit before the existing pairing, Spotify help and public-policy links. A
left click opens the main window; a right click opens quick actions. The old
tray implementation combined `TPM_RETURNCMD` with an ignored return value,
which rendered commands without dispatching them; the shell removes that flag
so Win32 delivers `WM_COMMAND` to the tray owner.

Unpaired Windows launches into this shell rather than forcing an onboarding
dialog or exiting. Create/Join open the bot, Try locally explains that its later
engine is not configured, Settings stays available, and Connect in the tray
opens the existing code-entry window. Successful pairing closes the unpaired
message loop and starts the normal runtime from the newly stored credentials.

## State and accessibility

English and Russian catalogs cover every main-window and tray shell key.
Connection and recording states always carry text plus non-color tokens:
`[?]`, `[~]`, `[OK]`, `[!]`, `[MIC]` and `[REC]`. Standard Win32 `BUTTON` and
`STATIC` controls keep visible names available to Microsoft UI Automation and
Narrator; there are no owner-drawn, mouse-only controls.

The paired projection reads coordinator health, persisted local DND, bounded
volume, local route, now-playing URI and privacy-bounded presence counts from
the existing runtime. Missing history, self-test and recording integrations are
shown as unavailable rather than simulated. An active future recording keeps
Stop enabled even if ordinary capture availability drops.

Keyboard handling uses a native accelerator table and `IsDialogMessageW`, so
Tab/Shift-Tab traverse standard controls and these foreground shortcuts work:

| Shortcut | Action |
|---|---|
| `Ctrl+0` | open/restore Pulsar |
| `Ctrl+1` | Create |
| `Ctrl+2` | Join |
| `Ctrl+Shift+T` | Try locally |
| `Ctrl+Shift+R` | start/stop recording when available |
| `Ctrl+Shift+D` | toggle local DND when paired |
| `Ctrl+,` | Settings |

## DPI and focus behavior

The executable manifest remains PerMonitorV2-aware. The window also requests
PerMonitorV2 for unpackaged development builds, sizes in device-independent
units, handles `WM_DPICHANGED`, recreates fonts for the target monitor, applies
the suggested non-activating window rectangle and enforces a usable minimum
track size. Portable geometry tests cover 96, 120, 144 and 192 DPI (100, 125,
150 and 200 percent) and prove that navigation, content and status cards remain
positive, bounded and non-overlapping.

Only explicit launch/open actions show or activate the window. Periodic state
refresh updates text in place and never raises or focuses the window, so future
notifications cannot steal focus through this shell path.

## Automated and manual boundary

Go tests cover catalog completeness, stable navigation, non-color state
semantics, unpaired/degraded action availability, bounded runtime projection,
shortcut uniqueness, DPI geometry and native blind-build seams. `go vet`, the
portable tests and Windows amd64/arm64 cross-builds cover the production source.

No real packaged-app click, Narrator traversal, screen capture, notification
interaction or physical 125/150/200-percent observation is claimed. Those
checks remain in manual epic `EPIC-260714-th54l3`.
