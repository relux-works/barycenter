# Implement macOS long-track upload and playback UI

## Description
Render the shared Phase 2 track model in the macOS app with accessible file, queue and transport controls.

## Scope
Extend file pick and drag-drop to eligible long audio, show metadata and rights consent, durable resumable upload and processing progress, target and queue or replace choices, unsupported nodes, now-playing progress, pause, seek, resume, rebuffer, retry, delete and report. Use the shared model and opaque commands, never load the whole track in UI memory, preserve drafts on failure and meet keyboard and VoiceOver requirements.

## Acceptance Criteria
macOS completes a one-hour B1 parity flow without external credentials or full-file memory, renders exact validation and progress, and controls queue, replace, pause, seek and resume accessibly. Reconnect and repeated commands do not duplicate, draft and consent rules hold, and clip or Spotify UI remains usable.
