# Add accessible Windows report, block and owner-delete surfaces

## Description
Expose required UGC controls on every relevant Windows history or media view using the canonical action service.

## Scope
For each accessible foreign media item expose Report with frozen reason choices and optional details plus mute or block sender; for owned media expose Delete with the active-playback policy and clear consequences. Show only authorization-allowed actions, privacy-safe status, retryable coordinator errors and RU or EN confirmations. Use keyboard and screen-reader accessible controls, shared labels and no raw IDs. Do not duplicate report, block or delete business logic in UI.

## Acceptance Criteria
A keyboard-only Store reviewer can report any accessible foreign item, block or mute its sender and delete owned media, then observe exact backend outcomes without exposing other users. Own versus foreign and terminal versus active states show correct actions. RU and EN, denial, offline, repeated-action and accessibility tests pass.
