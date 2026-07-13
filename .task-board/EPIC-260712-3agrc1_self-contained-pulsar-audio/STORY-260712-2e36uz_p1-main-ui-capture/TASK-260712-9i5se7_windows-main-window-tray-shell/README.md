# Windows main window and tray shell

## Description
Build the packaged Windows main window and tray shell around the self-contained Phase 1 flows.

## Scope
Add Win32 surfaces for Create, Join, Try locally, presence summary, routing, now playing, history and settings, plus tray entry points for open, record state, DND and quit. Include RU and EN strings, keyboard navigation, non-color state indicators, screen-reader labels and layout behavior for Windows high-DPI scaling.

## Acceptance Criteria
Windows no longer reads as a Spotify-only tray app. Users can reach Create, Join and Try locally from the main window and tray, unpaired and degraded states have honest copy, and the shell remains usable at 125, 150 and 200 percent scaling without focus theft during notifications.
