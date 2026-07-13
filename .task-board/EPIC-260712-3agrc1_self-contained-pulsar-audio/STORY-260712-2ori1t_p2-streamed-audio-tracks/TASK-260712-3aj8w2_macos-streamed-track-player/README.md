# Implement the macOS bounded streaming provider

## Description
Integrate the ADR decoder and authenticated cache into a generation-safe main-program provider without violating AVAudio render constraints.

## Scope
Implement incremental range fetch, atomic bounded disk chunks, decoder and bounded PCM ring, readiness threshold, coordinator-time scheduled start, generation-safe pause, seek, resume, rebuffer, cancel, cache eviction or refill, delete or disable purge and drained ended. Enforce ADR cache and ring ceilings and local volume ceiling; keep network, disk, decode and locks outside render; expose sanitized buffer, fetch, decoder and audible-progress telemetry. Preserve clip, overlay, interrupt, Spotify, direct output and Airfoil behavior.

## Acceptance Criteria
macOS starts before full download with p95 at most 5 s, seeks to audio p95 at most 3 s, stays at or below 200 MiB RSS for B1 and remains duration-bounded at two hours. It rejects stale generations, survives network resets and eviction, purges revoked cache, reports ended after ring drain and passes realtime, clip, Spotify and output regressions.
