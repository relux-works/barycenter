# Windows tray hotkey recording controller

## Description
Own configurable toggle-record hotkey behavior and its tray-thread lifecycle without duplicating capture or upload logic.

## Scope
Register Ctrl+Shift+Space by default and user-selected alternatives with RegisterHotKey on the tray-owning UI thread. Route first press to start and second press to stop through the capture controller. Esc cancels only in a focused Pulsar recording surface; when hidden, the tray exposes an explicit Cancel action rather than installing a global bare-Esc hook. Preserve visible text and non-color tray state plus correctly sequenced cues, expose conflict and unavailable states with button fallback, and unregister on quit, lock, suspend, reconfiguration or teardown. Phase one does not depend on key-up or a low-level hook.

## Acceptance Criteria
The signed app toggles recording from the configured hotkey with the window hidden; conflict never disables button recording; focused Esc and hidden tray Cancel remove the partial capture without hijacking global Escape; repeated register, reconfigure, cancel, lock and quit cycles leave no stale registration or hidden recording; tray and window show one consistent state without UI-thread blocking.
