# Execute final C1-C3 live and capture-quality acceptance

## Description
Run C1-C3 only on the exact root-reviewed and independently realtime-reviewed build.

## Scope
Rerun 100 hold or release cycles, lost-release and lifecycle matrix, two real home networks, Windows to Windows, Windows to macOS and macOS to macOS, 2 percent loss, p50 800 ms and p95 1500 ms, bounded jitter, speaker and headphone far-end, near-end and double-talk, honest degraded routes and main-program recovery. Test both ordinary live_ptt and the E2EE live path when claimed, preserve raw sanitized artifacts and never average away a failed platform or environment.

## Acceptance Criteria
C1-C3 pass with exact thresholds and reproducible hardware evidence or fail with a named blocker. No stuck capture, callback violation, unintelligible loss result, echo claim mismatch, stale session or main-program regression occurs, and capability wording matches each tested flag and route.
