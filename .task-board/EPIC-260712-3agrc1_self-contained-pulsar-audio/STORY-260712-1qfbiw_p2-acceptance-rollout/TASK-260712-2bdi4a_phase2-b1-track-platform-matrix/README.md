# Execute final B1 and platform compatibility acceptance

## Description
Run the approved one-hour track build across every required platform pairing and preserve Phase 1 compatibility.

## Scope
On provenance-tracked real builds use actual authenticated range serving and selected decoder to measure first audio before full download, start p95 at most 5 s, RSS at most 200 MiB, seek-to-audio p95 at most 3 s, synchronized start skew p95 at most 100 ms, pause and resume, repeated seek generations, cache eviction, network reset and drained ended on Windows-Windows, Windows-macOS and macOS-macOS. Rerun clip, overlay, interrupt, pairwise and optional internal Spotify regressions with Phase 2 flags and mixed Phase 1 nodes.

## Acceptance Criteria
B1 passes with raw sanitized timing, memory, disk, network and build artifacts for all pairings and no full download or reload. Exact p95 sample counts meet the gate, Phase 1 paths remain green and unsupported mixed targets follow the visible policy. Any miss is a fail with owner, never an inferred pass.
