# Integrate the Windows capture, self-test and hotkey workflow

## Description
Assemble the reviewed Windows capture engine, offline self-test, file intake and tray hotkey into one coherent Phase 1 user workflow.

## Scope
Bind the Windows shell to the platform-spike-backed capture engine, builtin cues and temp lifecycle, five-second record-then-play self-test, standard file intake and tray hotkey controller. Coordinate device and output selection, level state, explicit permission, visible and audible recording indications, Esc cancel, auto-stop, hidden-window behavior and typed degraded states without placing upload, file I/O or blocking work on audio callbacks.

## Acceptance Criteria
The integrated Windows workflow meets the local portions of A1 under the signed AppContainer package: builtin cue and completed five-second recording play through the production clip path, normal recording creates a bounded retryable draft, hotkey and button states agree, and denial, conflict, lock, revoke and quit cases fail honestly with complete cleanup.
