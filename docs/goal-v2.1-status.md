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
| **F4** Update hygiene | watchdog 5s + SUAutomaticallyUpdate | **CODE SHIPPED** | `main.swift` watchdog, `build-app.sh` SUAutomaticallyUpdate. Live silent vN→vN+1 proof **deferred by customer** ("автоапдейт протестим потом"); F2's survival already proven by the manual update |
| **F5** Zeroconf hint | firewall/Wi-Fi/VPN warning in onboarding + guide | **DONE** (mitigation) | `ui_common.go`/`OnboardingWindow.swift` hint; `barycenter.live/guide` FAQ. Root-cause **EXCLUDED** (needs Timur's log) |
| **F6** Windows path | app runs, onboarding+tray, MSIX in CI, Store submission | **SUBSTANTIALLY DONE** | pulsar-win ran live on Timur's hardware (window renders, icon, Cyrillic); 4 hardware bugs fixed (`-H windowsgui`, file log, Segoe UI, layout); MSIX CI job green; **submitted to Store, "In certification"**. Full live flow + cert verdict pending (B4/cert) |
| **F7** Beta evening | — | **EXCLUDED** (B5, evening with Timur) | — |

## Gates

| Gate | Status | Note |
|---|---|---|
| **B1** Core fixes (F1 tested; F2+F3; suites green; prod) | **DONE** | 9 Go pkgs + 41 Swift tests green; deployed |
| **B2** Update proof | **PARTIAL** | code shipped; live silent-update deferred by customer |
| **B3** Zeroconf & polish (hint, menu identity) | **DONE** | hint + `identityLine` shipped; root-cause excluded |
| **B4** Windows live | **IN PROGRESS / customer-gated** | app proven on hardware; full flow + fixes ongoing; needs the Windows machine |
| **B5** Acceptance evening | **EXCLUDED** | out of scope per customer |

## Honest bottom line

Every in-scope **code / CI / deploy** deliverable is complete, tested, and on
prod. The remaining items are exactly the ones the customer carved out:
the live acceptance evening with Timur (B5/F7), the firewall root-cause (F5,
needs Timur's log), and the live silent-update proof (F4, deferred). F6's Store
submission is in Microsoft's certification queue — verdict pending, external.

There is no further in-scope engineering action available without those
customer/external inputs.
