# P3 Capture quality and diagnostics

## Description
Add and verify AEC, noise suppression, AGC and cross-platform input diagnostics.

## Scope
Add platform-appropriate acoustic echo cancellation, noise suppression, automatic gain control, speaker/headphone modes, input health diagnostics and an honest capability matrix while preserving local volume ceilings and explicit capture indicators.

## Acceptance Criteria
C3 passes across the required Windows/macOS speaker and headphone matrix with no intelligible return echo in accepted conditions. Unsupported/degraded AEC is visible, capture remains bounded and cancellable, and objective plus listening-test evidence documents quality without overstating parity.
