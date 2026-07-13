# Independently review realtime audio and capture safety

## Description
A reviewer other than the implementer audits PTT capture DSP jitter mixer and lifecycle behavior on the root-reviewed build.

## Scope
Inspect capture ownership, hotkey lost-release safeguards, encoder and decoder threads, backpressure, jitter bounds, FEC or PLC, mixer and limiter order, AEC reference and double-talk, allocations or locks in callbacks, device and route changes, DND and terminal cleanup. Reproduce targeted stress, sanitizer or race tests and real hardware cases, review C1-C3 raw artifacts and file findings with severity. Any changed code or fixture after review requires a delta review.

## Acceptance Criteria
No critical or high realtime, stuck-capture, callback, unbounded-buffer, echo or mixer finding remains. Reproduction commands, hardware identity, reviewed commit, findings and retests are attached and C1-C3 cannot start on a different build.
