# Integrate the macOS capture, self-test and hotkey workflow

## Description
Assemble the reviewed macOS capture engine, offline self-test, file intake and menu-bar shortcut into one coherent Phase 1 user workflow.

## Scope
Bind the macOS shell to the TCC-gated capture engine, builtin cues and temp lifecycle, five-second record-then-play self-test, system file intake and menu-bar shortcut controller. Coordinate device and output selection, level state, visible and audible recording indications, Esc cancel, auto-stop, hidden-window behavior and typed degraded states without placing upload, file I/O or blocking work on audio callbacks.

## Acceptance Criteria
The integrated macOS workflow provides phase-one parity: builtin cue and completed five-second recording play through the production clip path, normal recording creates a bounded retryable draft, shortcut and button states agree where the sandbox-safe shortcut is supported, and denial, conflict, device loss, sleep and quit cases fail honestly with complete cleanup.
