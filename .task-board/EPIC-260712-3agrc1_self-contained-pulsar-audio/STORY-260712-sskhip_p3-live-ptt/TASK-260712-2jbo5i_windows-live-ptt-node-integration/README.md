# Integrate Windows hold UI, sender, relay and receiver

## Description
Assemble the reviewed Windows input, capture sender and jitter receiver into one honest PTT experience without duplicating their internals.

## Scope
Bind target and Air policy selection, hold availability and Phase 1 toggle fallback, tray and main-window status, start or busy or partial receipts, local Stop, diagnostics and error recovery to the common live session model. Connect the sender and receiver components to protocol and mixer interfaces, keep session generations consistent and clean both on lock, suspend, quit, reconnect, permission revoke or feature rollback. Preserve clips, tracks, overlay, interrupt and local volume ceiling.

## Acceptance Criteria
The signed packaged Windows experience either provides reliable hold PTT or visibly uses toggle before capture. Sender and receiver never run stale or concurrently in an invalid state, target and DND policy are honest, all terminal paths clean UI and audio and Phase 1 or 2 functions remain green.
