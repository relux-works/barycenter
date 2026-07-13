# Build the shared long-track UI and control model

## Description
Expose one localized app model from long-file draft through processing and main-program playback, separate from platform rendering.

## Scope
Extend file metadata and durable draft state for two-hour or 500-MiB candidates, current policy consent, resumable upload and server processing progress, title or duration or variant failure, Air or explicit targets, queue or replace, unsupported recipients, now playing, audible progress, pause, seek, resume, rebuffer, ended, retry, delete and report. Use opaque IDs and canonical commands, generation-safe optimistic state and honest offline or quota errors. Never claim ready from client MIME or delete an unsent draft before confirmed upload.

## Acceptance Criteria
Windows and macOS consume identical RU and EN state, validation and action capabilities from file pick through played receipt. Processing and playback progress are distinguished, seek generations and failures are honest, local drafts survive outage, and the UI cannot bypass consent, target, queue, quota or moderation policy.
