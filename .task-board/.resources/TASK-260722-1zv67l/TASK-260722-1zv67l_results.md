# TASK-260722-1zv67l automated readiness handoff

## Verdict and evidence boundary

The deterministic Mac-create/Windows-join readiness boundary passes. The coordinator routes, exact installed candidates, full Mac test/build/package boundary, ordinary Mac launch, and installed Windows Join surface were all rechecked. The real Create, recovery export, invitation generation/copy, invitation entry, and Join action were not performed. Manual evidence is `not-run`; manual and hardware PASS remain false and no owner checklist row was checked.

The owner handoff is reduced to exactly two ordinary app screens: Mac Create/save recovery/generate and copy one invitation, then Windows paste the invitation/Join securely/report the visible result. It contains no terminal command and requests no duplicate manual task.

## Exact live boundary

- Coordinator: `https://barycenter.relux.works`, version `git-3565c1e1ca0511168026ec2ba72440d23fb1317f`, status `ok`, 3 orbits, 0 connected nodes. GET-only route probes returned Create `400`, device invite `400`, consume invite `400`, and unknown control `404`.
- Mac: `/Applications/Pulsar.app`, `works.relux.pulsar`, version `0.3.0` build `946`, x86_64, source `fb807e1caa40ebb7d206d983e234b626f4457945`. Local authority `duet-nodeapp`, certificate SHA-1 `40DF8747F4232A938A57313718A71748A553388D`, CDHash `020f0a58bdfebb8371fb07bc070787b7615a9450`. This is a local signed, non-notarized candidate.
- Windows: host `DESKTOP-3PBO632`, OS `10.0.19045.0`, package `ReluxWorksLLC.PulsarBarycenter_0.1.20.0_x64__q036g2bzd7ngc`, status `Ok`, Developer signature, source `76f09a4d8be693d57cd5d47b9b9e5ac06196519c`, accepted hosted CI run `29863591495`. This supersedes the older `0.1.11.0` owner receipt.

### Published SHA-256 values

| Candidate component | SHA-256 |
| --- | --- |
| Mac `NodeApp` | `a862bfd563ef9956527ad5704e290966b8d8922cea3dbdd54cee2097f53fbabd` |
| Mac `go-librespot` | `a6a6808104129b18e2b660526e4d44c8d1731d89f2e62ea6a2cce30e09c7d61f` |
| Mac `Info.plist` | `885b001d33a76ccf95e554e568594d9ae6037459592c45692dbf5d48ca429308` |
| Mac review archive | `87313d3a64821aebf76b4e8d993041819cd7f9f3df20082d7f95c6383cad6c67` |
| Windows accepted package archive receipt | `f74b5c8d6f8c86443f8c1b64715977be1b0183c39e7fc4dde7567c957b958348` |
| Windows `pulsar-win-amd64.exe` | `0a77f53f026b77dd6abc3b265f18a8d32744847ca23571e97ddd999cc17a0042` |
| Windows `go-librespot.exe` | `1967b76fc6e8e91763cea10c1cac1bb5f97cdb08a6100bdb27c9a01470cf84ca` |
| Windows `pulsar-capture.dll` | `8c1657d035ab738559c91c4c8468d6a4ba663a80dc96aab8951cc4c2d3b52c2f` |

The three installed Windows component hashes were recomputed during this run. The package archive was not present on the Windows host and was not falsely described as rehashed; its value is pinned from the accepted install receipt.

## Ordinary GUI evidence

- Mac was quit normally and opened with LaunchServices. `open /Applications/Pulsar.app` exited `0`; process `77965` had parent PID `1`, finished launching, remained visible and unhidden, exposed one `1120×812` product window, and passed a one-second process sample with exit `0`. No new `NodeApp` diagnostic report appeared. The onboarding Keychain service was absent and `~/duet/node.yml` was absent, so this remains a first-run candidate.
- Windows was launched from the Desktop shortcut in interactive session 1. Process `13244` had parent `explorer.exe`, was responding and not hung, and exposed a visible `2106×1471` window at 192 DPI. There were no new callback crash bytes, Application crash events, or AppModel removal events. The protected and legacy credential files remained absent.
- The safe Windows probe clicked only native Join navigation control `3003` to expose the Join screen. It located native invitation `Edit` `3027` and native action `Button` `3010`, all visible/enabled, with the input carrying `WS_TABSTOP`. It never entered an invitation and never clicked the Join action. Remote probe result: exit `0`, raw remote receipt SHA-256 `5b565b293fd11e6e4a106b478700efa544610646eb19d9c7444a6f5f93f32745`, 3994 bytes; executed probe SHA-256 `eac6da77962356858921e6be7950fe447546460c1732fa34965ef4337747d9e4`.
- The temporary Windows scheduled task and probe directory were removed. A later independent read-only SSH postcondition exited `0` and confirmed the same package, responsive session-1 process, and absent credentials.

## Code and tests

New acceptance infrastructure:

- `scripts/acceptance/windows_create_join_readiness.ps1`: fail-closed installed-package, component-hash, ordinary-launch, crash, credential, and non-submitting Join-surface probe.
- `scripts/acceptance/validate_live_create_join_readiness.py`: exact manifest validator for coordinator, candidates, owner scope, and no-manual-PASS boundary.
- `scripts/acceptance/test_live_create_join_readiness.py`: seven regression tests covering the valid manifest, candidate/route drift, manual-PASS rejection, real-Join rejection, exact two-screen/no-terminal scope, UIA anomaly disclosure, and source-level non-submission guarantees.

Validation commands and actual exits:

| Gate | Exit | Result |
| --- | ---: | --- |
| Coordinator health plus four GET-only route probes | `0` each | Exact health/version and `400/400/400/404` contract |
| `xcrun swift test` (full suite, concise confirmation rerun) | `0` | Green |
| focused `MacFirstRunLifecycleSourceTests.freshLaunchUsesMainShell` | `0` | 1 test passed |
| `xcrun swift build -c release` | `0` | Green |
| deep/strict codesign verification | `0` | Green |
| pinned identifier/certificate codesign requirement | `0` | Green |
| corrected installed Mac hash command | `0` | All exact values reproduced |
| Mac archive `unzip -t` | `0` | No compressed-data errors |
| source-commit/product-diff provenance | `0` | Accepted Mac/Windows commits present; Mac product tree unchanged after accepted source |
| ordinary Mac launch/window/process/sample/crash gates | `0` each | Green |
| final Windows scheduled probe result | `0` | Green native readiness; no submit |
| Windows temporary task/probe-directory cleanup postconditions | `0` | Removed |
| Windows final package/process/credential postcondition | `0` | Green |
| `python3 -m unittest scripts.acceptance.test_live_create_join_readiness` | `0` | 7 tests passed |
| `python3 -m py_compile ...` | `0` | Green |
| readiness manifest validator | `0` | Green |
| executed PowerShell probe hash pin | `0` | Exact match |
| `git diff --check` | `0` | Green |

Honest non-green boundaries and corrected attempts:

- `spctl --assess` exited `3` and reported `rejected`, the expected result for the explicitly local, non-notarized Mac candidate. This is not reported as a passing distribution gate.
- `security find-generic-password -s works.relux.pulsar` exited `44` before and after launch because no onboarding credential exists. This is the intended unpaired precondition, not a generic command pass.
- The first local Mac hash command exited `1` because it used the wrong `Contents/Resources/go-librespot` path. The corrected `Contents/MacOS/go-librespot` command was run independently and exited `0` with the accepted hash.
- Early Windows probe iterations exited nonzero while discovering Windows PowerShell 5 source-encoding behavior and the UI Automation semantics described below. They were never represented as green. The safe native-control gate was then encoded explicitly and its independent scheduled run exited `0`.
- The initial SSH alias attempt exited `255` because it selected the wrong user/too many agent keys; the corrected `admin@mbpro-win` connection pinned the intended agent key. A hung `scp` transfer was cancelled with exit `130`; the source was then transferred with a bounded encoded transport and hash-verified before execution.
- Mac XCUITest/screenshot capture was not treated as a green gate because this host lacks the needed UI recording permission. The required ordinary GUI boundary was inspected directly through the running application, CoreGraphics window state, process parentage, sampling, and crash reports. No subjective, VoiceOver, audio, or hardware result is inferred.

## Routed finding

The current Windows controls are natively usable but expose incorrect UI Automation semantics: controls `3003`, `3027`, and `3010` report `ControlType.Pane`; the input reports no `ValuePattern` and not keyboard-focusable; the action reports no `InvokePattern`. This does not force-fit the no-terminal owner pass because the native `Button`/`Edit` controls are visible, enabled, keyboard-tabbed, and the safe navigation click worked. It is disclosed in the manifest and routed as `BUG-260722-224lo9` under desktop-app-experience remediation. No assistive-accessibility PASS is claimed.

## Evidence files

- Readiness manifest: SHA-256 `a9fd01a4d8b6081b3a2e5cada2f02e22dedb8180b3b162507539153248df57ec`.
- Canonical Windows receipt plus immutable remote-receipt metadata: SHA-256 `16c440503f37884dd796db103e81262c688eb366c7997e1599fb25d4cccdec5c`.
- Two-screen owner handoff: SHA-256 `0392312b6a07ecf4ca7b6299b461f8aff580b1e33925bc0ea03076fd038b72d6`.

The owner task remains in `backlog`, all six pre-existing manual rows remain unchecked, and manual evidence remains `not-run`.
