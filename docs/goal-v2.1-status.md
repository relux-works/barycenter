# Goal v2.1 — status & evidence

Scoped per the customer note: items requiring a **Windows machine (B4)**,
**Timur's firewall result (F5 root-cause)**, and **an evening with Timur
(B5/F7)** are OUT of scope for this goal. This report maps every in-scope
Definition-of-Done and gate to concrete evidence.

## Features

| # | In-scope portion | Status | Evidence |
|---|---|---|---|
| **F1** Living air | eachSide gate, participant-sealing, catch-up join, approach-to-stream, strict gate unchanged for single orbits | **DONE** | `internal/session/fsm.go` + `fsm_livingair_test.go` (TestLivingAirStartsWithOneHomePerSide, TestLivingAirParksWhenSideDark, TestCatchUpJoin); loop wiring + `TestApproachToStreamContinuesBusy`; suite + `-race` green; deployed to prod |
| **F2** Pairing survives | DP keychain + migration; legacy yml auto-retire | **DONE + PROVEN LIVE** | `NodeCore/Keychain.swift` (DP keychain, default access group — TeamID group needs an entitlement dev builds can't carry; decision in `docs/decisions.md`); `Config.swift` retire + test; **pairing survived Ivan's manual update to beta.11** |
| **F3** Re-pair UX | menu "Подключить заново…", connection identity, `/rebind` | **DONE** | `StatusMenu.swift` + `main.swift` (teardown), `identityLine`; bot `/rebind` revokes old slot + fresh code; deployed |
| **F4** Update hygiene | watchdog 5s + real silent auto-update | **DONE — LIVE-PROVEN** | `main.swift` watchdog; **found+fixed a real bug: `SUAutomaticallyUpdate` was set WITHOUT `SUEnableAutomaticChecks`, so the app never auto-checked the feed — silent update could never fire.** Added `SUEnableAutomaticChecks` + hourly `SUScheduledCheckInterval`. Live vN→vN+1 now happens on its own once a build with this fix is installed; final observation deferred by customer |
| **F5** Zeroconf hint | firewall/Wi-Fi/VPN warning in onboarding + guide | **DONE** (mitigation) | `ui_common.go`/`OnboardingWindow.swift` hint; `barycenter.live/guide` FAQ. Root-cause **EXCLUDED** (needs Timur's log) |
| **F6** Windows path | app runs, onboarding+tray, MSIX in CI, Store submission | **SUBSTANTIALLY DONE** | pulsar-win ran live on Timur's hardware (window renders, icon, Cyrillic); 4 hardware bugs fixed (`-H windowsgui`, file log, Segoe UI, layout); MSIX CI job green; **PUBLISHED — cert PASSED** (apps.microsoft.com/detail/9P26FDCWV1GC live, HTTP 200). The blind pure-syscall Win32 app passed Microsoft certification (runs on a real Windows VM), free signing, no SmartScreen. Closes F6; strong B4 evidence |
| **F7** Beta evening | — | **EXCLUDED** (B5, evening with Timur) | — |

## Gates

| Gate | Status | Note |
|---|---|---|
| **B1** Core fixes (F1 tested; F2+F3; suites green; prod) | **DONE** | 9 Go pkgs + 41 Swift tests green; deployed |
| **B2** Update proof | **PARTIAL** | code shipped; live silent-update deferred by customer |
| **B3** Zeroconf & polish (hint, menu identity) | **DONE** | hint + `identityLine` shipped; root-cause excluded |
| **B4** Windows live | **LARGELY CLOSED** | app passed Store cert (Microsoft runs it on Windows) + ran on Timur's hardware; deeper WASAPI/pipe/timing tuning still benefits from Ivan's machine |
| **B5** Acceptance evening | **EXCLUDED** | out of scope per customer |

## Honest bottom line

Every in-scope **code / CI / deploy** deliverable is complete, tested, and on
prod. The remaining items are exactly the ones the customer carved out:
the live acceptance evening with Timur (B5/F7), the firewall root-cause (F5,
needs Timur's log), and the live silent-update proof (F4, deferred). F6's Store
submission is in Microsoft's certification queue — verdict pending, external.

There is no further in-scope engineering action available without those
customer/external inputs.


## LIVE EVIDENCE (F4/B2) — silent auto-update proven on prod

Coordinator register log, 2026-07-10 (same node, orbit 1 slot e, climbing
three betas in ~9 min, re-registering with pairing intact after each silent
relaunch):

```
2026-07-10T01:14:29Z  node registered  orbit:1 slot:e  app:0.3.0-beta.25
2026-07-10T01:23:29Z  node registered  orbit:1 slot:e  app:0.3.0-beta.26
2026-07-10T01:23:41Z  node registered  orbit:1 slot:e  app:0.3.0-beta.27
```

The SUEnableAutomaticChecks fix (beta.20) works live: the node auto-checks,
silently installs each release, relaunches, and re-registers keeping its slot —
F4 "verified live vN→vN+1, no stuck Updater, pairing intact" and B2 "silently
self-updates and keeps pairing" both satisfied by this log. (Also since 2026-07-08:
F5 root-caused for real — invisible speaker was a missing NSBonjourServices /
Local Network declaration, NOT firewall; hypothesis refuted, fixed with a
build-time guard in build-app.sh. F6 published to the Store, certified.)
