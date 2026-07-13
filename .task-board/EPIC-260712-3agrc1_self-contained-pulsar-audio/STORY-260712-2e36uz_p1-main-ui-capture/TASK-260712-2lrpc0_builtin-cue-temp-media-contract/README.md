# Builtin cue asset and local draft lifecycle contract

## Description
Provide one reviewed cue and a precise state model that distinguishes disposable self-test media from user recordings that must survive an outage.

## Scope
Add or generate a Relux Works owned or redistributable cue with provenance and package paths. Freeze separate storage classes: self-test recordings are app-private, never uploaded and deleted on close or explicit delete; active capture temp files are deleted on cancel or failed finalization; successfully finalized user recordings become durable local drafts that survive coordinator outage and app restart until confirmed upload or explicit delete; confirmed uploads remove the local draft. Define crash recovery, partial-file cleanup, file-picker copies or access tokens, filename redaction and cue sequencing so the start cue finishes before microphone samples are committed and the stop cue begins only after capture closes.

## Acceptance Criteria
Both platforms consume the same licensed cue and lifecycle state machine. A1 self-test leaves no recording after close, cancel leaves no partial capture, coordinator outage or restart never loses a finalized unsent user draft, and confirmed upload or explicit delete cleans it. Crash recovery cannot present a partial file as sendable, paths never enter logs, and recording cues are not captured into the user clip.
