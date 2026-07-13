# Implement Windows long-track upload and playback UI

## Description
Render the shared Phase 2 track model in the packaged Windows app with accessible file, queue and transport controls.

## Scope
Extend brokered file pick and drag-drop to eligible long audio, show metadata and rights consent, durable resumable upload and processing progress, target and queue or replace choices, unsupported nodes, now-playing progress, pause, seek, resume, rebuffer, retry, delete and report. Use the shared model and opaque commands, never load the whole track in UI memory, preserve drafts on failure and meet keyboard, screen-reader and high-DPI requirements.

## Acceptance Criteria
Windows completes a one-hour B1 user flow without external credentials or full-file memory, renders exact server validation and progress, and controls queue, replace, pause, seek and resume accessibly. Reconnect and repeated commands do not duplicate, draft and consent rules hold, and clip or Spotify UI remains usable.
