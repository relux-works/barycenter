# Execute final B2-B4 Air and capacity acceptance

## Description
Run integrated Air, streaming, target and leave behavior at real and synthetic scale with exact duplicate and resource instrumentation.

## Scope
Use at least A+B+C and five real or representative Pulsars for exact-once clip and track target delivery, no transitive paths, offline member nonblocking, living-track catch-up to audible position, no stale overlay autoplay, join during media, leave during prepare and playback with leaver fade or stop and remaining continuation. Run eight Barycenters and twenty Pulsars while measuring coordinator CPU, RSS, SQLite latency, hub queues, timer or goroutine counts, bandwidth, command duplicates and recovery. Exercise mixed platforms and feature flags.

## Acceptance Criteria
B2-B4 pass with per-target command and receipt correlation proving exactly once, no transitive delivery, correct catch-up and isolated leave. Synthetic capacity stays within the frozen resource and latency thresholds with no lost or duplicate delivery, or records the exact failing seam. Artifacts are sufficient for rollback and operator review.
