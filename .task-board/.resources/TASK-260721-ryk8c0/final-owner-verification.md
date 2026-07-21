# Final owner verification: one real-app pass

Owner: Ivan Oparin  
Task: `TASK-260721-ryk8c0`  
Status: `WAIT` until the release workflow supplies both a signed Windows app
candidate and a notarized macOS DMG for the source below. Do not use a terminal.

## Frozen engineering candidate

- source head: `a7258dbfe8787a8980dc7dc6023da1e932194a57`
- product UI commits: Windows `ee09731`, macOS `b0cc1a2`
- clean automated manifest:
  `.temp/acceptance/desktop-ui-a7258db/manifest.json`
- manifest SHA-256:
  `17735d6f42371e75824689bcdc926676bb1b29dd2f63cf4dd7e897e126a6970b`
- local result: 11/11 deterministic desktop stages passed; Windows amd64 and
  arm64 PE subsystem is GUI (`2`); Swift passed 359 tests and a release build
- exact-head hosted CI: run `29828660852`; Windows, signed-package and macOS
  jobs passed (359 Swift tests in 58 suites on the hosted macOS runner)
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
