# Freeze hard gates, fixtures and the streaming proof harness

## Description
Turn section 20.2 and B1 performance requirements into one candidate-neutral, artifact-producing protocol before prototypes begin.

## Scope
Define MP3, AAC and Opus fixtures across supported containers, one-hour and two-hour CBR or VBR, seek-index edge cases, corrupt, truncated, hostile, no-range, slow, reset and cache-eviction cases. Pin expected sample format and duration. Define authenticated HTTP range and fault harness, RSS and disk-cache measurements, first-audio and seek-to-audio p95 sampling, coordinator-clock skew for Windows to Windows, Windows to macOS and macOS to macOS, pause and resume scripts, real versus synthetic hardware, package architectures and evidence format. Set hard gates of track start p95 at most 5 s, seek p95 at most 3 s, skew p95 at most 100 ms, per-node RSS at most 200 MiB for B1 and memory independent of duration. Shortlist exact versioned candidates including native macOS.

## Acceptance Criteria
Every section 20.2 proof and relevant B1 or 20.5 threshold maps to a reproducible fixture, sample count, command and artifact. Candidate tasks share identical inputs and cannot move gates after seeing results. The shortlist, supported containers, hardware and failure criteria are explicit, and security or decoder crashes count as rejection.
