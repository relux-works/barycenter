# Implement Windows saved-cue soundboard and hotkeys

## Description
Add manual saved-cue selection management and triggering to the signed Windows window and tray.

## Scope
Render builtin and authorized user cues, create a cue through canonical brokered media selection, rename, reorder and delete, choose targets and delivery policy and trigger manually or with documented AppContainer-safe registered hotkeys. Persist only local binding preferences, detect collisions with recording and system shortcuts, provide button fallback and never use an unapproved hook or enter microphone capture. Show pending, partial, played, skipped and failed receipts.

## Acceptance Criteria
A signed Windows user can manage and trigger saved cues without Telegram or an external account. Hotkey conflicts and unavailable registration are honest, manual controls remain usable, cue ACL and delete or disable state are enforced and no soundboard action starts capture or bypasses DND, targets or local receiver ceilings.
