# Final owner verification: one real-app pass

Owner: Ivan Oparin  
Task: `TASK-260721-ryk8c0`  
Status: `WINDOWS READY / FINAL WAIT`. The exact developer-signed Windows
hardware-test candidate below is installed and can be exercised without a
terminal. Final cross-platform acceptance still waits for a notarized macOS DMG
and no Store/EV-signing claim is made.

## Frozen engineering candidate

- source head: `a7258dbfe8787a8980dc7dc6023da1e932194a57`
- product UI commits: Windows `ee09731`, macOS `b0cc1a2`
- clean automated manifest:
  `.temp/acceptance/desktop-ui-a7258db/manifest.json`
- manifest SHA-256:
  `17735d6f42371e75824689bcdc926676bb1b29dd2f63cf4dd7e897e126a6970b`
- local result: 11/11 deterministic desktop stages passed; Windows amd64 and
  arm64 PE subsystem is GUI (`2`); Swift passed 359 tests and a release build
- final tracking-head hosted CI: run `29829090013`; all four jobs passed,
  including Windows, signed-package, coordinator and macOS (359 Swift tests in
  58 suites). Its product source is identical to frozen source head `a7258db`.
- signed diagnostic probe (not the production app):
  `.temp/ci-29828660852-msix/PulsarProbe-0.1.0.0-x64-signed.msix`
- signed diagnostic probe SHA-256:
  `42081733678469a97d065ef0c0950c7f481b3e63ccfc5b528dce96a50fac8994`
- Finder-only UI preview bundle:
  `.temp/acceptance/desktop-ui-a7258db/build/Pulsar.app`
- preview ZIP SHA-256:
  `8ba061ca4ad551596d43d06ed60945056c725b5792fae0b392651d76a7818e27`

The local preview is ad-hoc signed and has no bundled `go-librespot`; it is
useful for a Finder UI smoke only and cannot satisfy the production launch,
notarization, update, integration or TCC persistence rows.

## Installed Windows 10 candidate

- host/account: `DESKTOP-3PBO632` / `admin`, Windows `10.0.19045`
- source/tracking head: `f6c9b47dfebbca5feb9d533cb91bef45cb7d82b3`
  (product source is the accepted `a7258db` candidate)
- package: `ReluxWorksLLC.PulsarBarycenter_0.1.1.0_x64__q036g2bzd7ngc`
- package SHA-256:
  `eaa01ad6de70bf020a9ff4f145045003a93a475ae8711e62e30b532531f79d4a`
- installed GUI EXE SHA-256:
  `accd11d545ff89aa0ce106b1599771c588be9e90ba32a5f0530868bae5f43d28`
- installed `go-librespot.exe` SHA-256:
  `ffe82704be5671629a00bdea3915e40aa4e723b4a45417325da41dd90f8d9402`
- installed `pulsar-capture.dll` SHA-256:
  `b5ca5c1c110023532269772ff25e5072b1f64ab4c5758ab55be0478fd803db94`
- package state: `Ok`; signature: valid local `Developer`; signer key is
  non-exportable and the certificate expires after 30 days
- local test signer thumbprint:
  `ce7d47f761bb659314ad659e7dc9042a3994c4cd`; row 6 cleanup must remove this
  package and certificate from Current User `My`/`TrustedPeople` and Local
  Machine `Root`
- runtime: `packagedClassicApp` / `appContainer`; declared capabilities include
  `microphone`, internet client/server and private-network client/server
- launch: `Pulsar` is registered in Start and the Desktop shortcut activates
  `ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc!Pulsar`
- installer did not launch the app, so no visible-window, microphone, audio or
  manual PASS is inferred from installation

This package is suitable for the Windows 10 functional/hardware rows in this
checklist. It is intentionally local-test signed and cannot prove Store/EV
production signing or submission readiness.

## One checklist

Record each row as `PASS`, `FAIL`, `BLOCKED` or `NOT_APPLICABLE`, with local
time and one screenshot/log reference. A `FAIL` returns to engineering as one
focused bug; do not reopen the old manual tasks.

| # | Check | Windows 10 `mbpro-win` | macOS |
|---|---|---|---|
| 1 | Install the supplied exact candidate and launch it from Start/Finder. It opens as an ordinary GUI app with no terminal window. |  |  |
| 2 | With the built-in microphone, exercise permission deny, then allow, record five seconds, play it back and confirm recording can be cancelled/stopped. |  |  |
| 3 | Confirm critical text and controls are sharp, unclipped and keyboard reachable. On Windows check 100% and one non-default scale; on macOS check Retina plus one short VoiceOver traversal. |  |  |
| 4 | Exercise one real target/Air/Telegram route and one stream/live path if enabled; confirm no duplicate playback. Mark disabled capabilities `NOT_APPLICABLE`, never inferred. |  |  |
| 5 | Smoke report/delete, recovery export, app restart and sleep/lock recovery; verify the app does not silently lose the selected input or leave capture active. |  |  |
| 6 | Quit/uninstall the test candidate and verify capture stopped and temporary test media/package state was cleaned. |  |  |

## Return packet

Return this completed table plus:

- the exact Windows package hash and macOS DMG hash shown before install;
- one Windows screenshot and one macOS screenshot of the main window;
- the first failing step and visible message for any `FAIL`;
- start/end timestamps and the longest uninterrupted passive run you actually
  observed (no minimum duration is invented).

No Store, hardware, audible-quality, Narrator, VoiceOver or production result
is accepted until it is recorded here.
