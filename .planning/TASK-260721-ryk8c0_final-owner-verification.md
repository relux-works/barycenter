# Final owner verification: one real-app pass

Owner: Ivan Oparin  
Task: `TASK-260721-ryk8c0`  
Status: `WAIT` until the release workflow supplies both a signed Windows app
candidate and a notarized macOS DMG for the source below. Do not use a terminal.

## Frozen engineering candidate

- source head: `a10808051c79cff18990d730e1a48cc4de1af138`
- product UI commits: Windows `ee09731`, macOS `b0cc1a2`
- clean automated manifest:
  `.temp/acceptance/desktop-ui-a108080/manifest.json`
- manifest SHA-256:
  `cc555975142c13d02490f723f83aa7b61cd7288604efdf593cc600b6c1317b7e`
- local result: 11/11 deterministic desktop stages passed; Windows amd64 and
  arm64 PE subsystem is GUI (`2`); Swift passed 359 tests and a release build
- exact-head hosted CI: run `29828164444` (result and signed probe hash are
  filled by the autonomous handoff before this task becomes ready)
- Finder-only UI preview bundle:
  `.temp/acceptance/desktop-ui-a108080/build/Pulsar.app`
- preview ZIP SHA-256:
  `d42b046c17ff0f39c8093b319fb3e623be6cfdae6b852cac9398b8853b3d5a0f`

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
