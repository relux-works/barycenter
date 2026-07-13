# Add generic media ingest persistence and migration scaffold

## Description
Replace the narrow legacy media row with an additive phase-one ingest model that can store upload sessions, canonical media metadata and lifecycle state without breaking existing Telegram voice playback.

## Scope
Add additive SQLite tables and repository methods for generic media items, upload sessions, idempotency, storage publication, audit and delete timestamps. Preserve legacy media rows and compatibility WAV reads. Model processing, ready, failed, deleted and expired transitions with conditional updates so workers, retries and deletion cannot publish conflicting states. Make storage keys server-generated and opaque, support atomic canonical publication and cleanup recovery, and keep migrations additive for orbit, actor, slot and legacy voice data.

## Acceptance Criteria
Fresh and migrated databases persist generic media and upload progress without losing legacy voice or pairing data. Repository state transitions reject stale workers and duplicate finalize, never expose a storage key from user input, and recover interrupted publish or cleanup safely. Migration and previous-version rollback fixtures prove legacy reads, tokens and session snapshots survive.
