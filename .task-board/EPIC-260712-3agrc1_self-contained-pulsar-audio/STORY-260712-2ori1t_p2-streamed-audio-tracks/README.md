# P2 Streamed user audio tracks

## Description
Implement long-track ingest, variants, range fetch, buffering, playback, queue, replace and synchronization.

## Scope
Implement audio_track ingest and canonical compressed variants, range/chunk serving, bounded disk cache and decoder rings, buffer-ready synchronization, metadata/progress, queue/replace, pause/seek/resume/ended and provider integration without full download or unbounded memory.

## Acceptance Criteria
B1 passes for a one-hour track on Windows and macOS with RSS, start and seek limits from section 20. Uploaded tracks synchronize across targets, survive pause/resume and cache eviction, respect quotas/rights/deletion/ACL, and never regress clip or Spotify playback.
