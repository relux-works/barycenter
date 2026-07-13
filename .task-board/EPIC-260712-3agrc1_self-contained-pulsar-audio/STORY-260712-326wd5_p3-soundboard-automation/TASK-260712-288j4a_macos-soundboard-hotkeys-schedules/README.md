# Implement macOS saved-cue soundboard and hotkeys

## Description
Add manual saved-cue selection management and triggering to the macOS window and menu bar.

## Scope
Render builtin and authorized user cues, create a cue through canonical media selection, rename, reorder and delete, choose targets and delivery policy and trigger manually or with configurable documented hotkeys. Persist only local binding preferences, detect collisions with recording and system shortcuts, provide button fallback and never request broad permissions or enter microphone capture. Show pending, partial, played, skipped and failed receipts.

## Acceptance Criteria
A macOS user can manage and trigger saved cues without Telegram or an external account. Hotkey conflicts and unavailable global registration are honest, manual controls remain usable, cue ACL and delete or disable state are enforced and no soundboard action starts capture or bypasses DND, targets or local receiver ceilings.
