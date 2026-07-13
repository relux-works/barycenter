# macOS menu-bar hotkey recording controller

## Description
Own configurable toggle-record shortcut behavior and menu-bar lifecycle without duplicating capture or upload logic.

## Scope
Use an App Sandbox compatible global-shortcut API where available, routing first press to start and second to stop through the capture controller. Esc cancels only in a focused Pulsar recording surface; when hidden, the menu bar exposes explicit Cancel rather than a global bare-Esc hook. Preserve visible text and non-color state plus correctly sequenced cues, expose conflicts with button fallback, and unregister on quit, sleep, lock where observable, reconfiguration or teardown. Do not silently require broad Accessibility permission or a low-level event tap; if unavailable, keep menu and button recording functional.

## Acceptance Criteria
The configured shortcut toggles recording where the supported sandbox API permits it; conflicts or restrictions never disable menu or button recording; focused Esc and hidden menu Cancel work without hijacking global Escape; repeated reconfigure, cancel, sleep and quit cycles leave no stale hook or hidden recording; no unannounced Accessibility entitlement is introduced.
