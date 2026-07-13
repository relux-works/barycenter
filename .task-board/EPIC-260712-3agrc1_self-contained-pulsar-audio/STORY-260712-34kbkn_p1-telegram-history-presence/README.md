# P1 Telegram adapter, history and presence

## Description
Move Telegram to common ingest and expose history, presence, routing, receipts and compatibility behavior.

## Scope
Route Telegram voice and supported audio through common ingest, add inline delivery actions, preserve legacy personal/broadcast defaults and FIFO ordering, and expose app history, presence, routing, receipts, DND and block state with human labels across direct or pairwise-approach delivery.

## Acceptance Criteria
Legacy Telegram voice remains first after the current element and retains acceptance ordering. New actions create the same transmission semantics as the app. Presence never leaks microphone or process details. History shows processing through played/partial/error states, receipt reasons are exact, and bot/app names and target labels agree without raw IDs.
