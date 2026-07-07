# UIPROBE — Windows UI checklist (goal v2.1 F6)

`ui_windows.go` (onboarding window + tray) is written blind: it compiles for
GOOS=windows and the portable helpers are unit-tested, but the Win32 message
loop, window rendering and tray were never executed. Run these on real
hardware and note what breaks — each item maps to a spike gate.

## Setup

1. Build: `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o pulsar-win.exe .`
2. Put `go-librespot.exe` (the fork's Windows daemon) beside `pulsar-win.exe`.
3. Launch `pulsar-win.exe` with no arguments while UNPAIRED (no
   `%APPDATA%\Pulsar\credentials.json`).

## Onboarding window

- [ ] A window titled "Pulsar" appears, ~440×380, centered-ish.
- [ ] Title "Пульсар", the 3-step intro, and the Wi-Fi/firewall hint render;
      Cyrillic is not mojibake (STATIC uses CRLF — confirm line breaks show).
- [ ] The code field uppercases input and drops spaces/dashes as you type is
      NOT wired at the control level (only on submit) — typing lowercase is
      fine; verify submit still accepts it (normalizePairCode runs on submit).
- [ ] Get a code from @barycenter_bot `/create` or `/pair`, type it, click
      "Подключить". On success the window closes and the app continues to the
      shell; `credentials.json` is written.
- [ ] A bad/expired code shows the error text under the button and re-enables
      submit ("Подключить" restored from "Подключаю…").
- [ ] Closing the window (X) exits with code 2 (unpaired), not a crash.

## Tray

- [ ] After pairing, a tray icon appears (currently no custom icon is set via
      NIF_ICON — it may be blank/default; TODO: embed an .ico and pass hIcon).
- [ ] Left- or right-click opens a popup menu.
- [ ] The menu shows: "Барицентр: в сети/переподключение…" (grayed), the
      identity line "host · дом slot" (grayed), separator, "Подключить
      заново…", separator, "Выйти".
- [ ] "Выйти" shuts the app down cleanly (ws/supervisor/player stop, daemon
      exits, tray icon removed).
- [ ] "Подключить заново…" opens the onboarding window; on a successful new
      pairing the app exits (best-effort: relaunch to reconnect — in-place
      restart is a follow-up).

## Known blind risks

- Tray icon has no image yet (NIF_ICON not set) — likely invisible or default.
  Fix: bundle Assets\StoreLogo as an .ico, LoadImageW, set hIcon + NIF_ICON.
- STATIC controls may clip long Cyrillic lines; widths are guesses.
- Message-only window (HWND_MESSAGE) for the tray — confirm it still receives
  Shell_NotifyIcon callbacks (some sources require a normal hidden window).
- No DPI awareness: on hi-DPI the layout may be small. Add a manifest DPI hint.
- `curOnboarding`/`curTray` package globals assume a single window at a time —
  true today (onboarding OR tray, never both).
