# Freeze and implement the streamed-track protocol contract

## Description
Add generation-safe buffered-load, seek and lifecycle messages across every codec only after the codec ADR and target policy are fixed.

## Scope
Define stream_load with opaque variant manifest, ETag or integrity and start position, stream_ready with seek generation and buffered duration, stream_seek with new generation, progress, rebuffer or failure and terminal ended or cancelled semantics plus stream_track_v1. Freeze readiness threshold, deadline, coordinator-clock start, duplicate and reordered messages, late or stale generation rejection, pause and resume interaction and explicit B6 policy for Phase 1 targets. Add golden JSON, Go and Swift codecs, Windows mirror and docs while preserving clip and Spotify messages.

## Acceptance Criteria
All three codecs round-trip identical generation-safe payloads and old messages. A stale ready, seek, progress or ended event cannot affect a newer generation; nodes never start before the buffer barrier; unsupported Phase 1 nodes are visibly handled by the sender-selected policy without blocking supported targets or silent autoplay; protocol and mixed-version goldens pass.
