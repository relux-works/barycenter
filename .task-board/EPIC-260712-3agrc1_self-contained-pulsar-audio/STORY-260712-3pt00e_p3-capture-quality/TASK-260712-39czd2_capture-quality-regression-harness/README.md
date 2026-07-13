# Build the capture DSP conformance harness and fixtures

## Description
Create the deterministic software and acoustic fixture contract that both platform processors must pass during implementation.

## Scope
Provide reusable far-end-only, near-end-only, double-talk, echo-path-change, clock-drift, clipping, too-quiet, silence, route-switch and device-loss fixtures with calibrated levels and timing. Define objective echo, speech preservation, gain, clipping, latency, CPU and memory measurements plus a reproducible blinded listening method for the no-intelligible-return-echo requirement. Separate synthetic deterministic CI from real acoustic hardware runs, hash artifacts, expire any human audio promptly and never copy private user capture into fixtures.

## Acceptance Criteria
Another developer can run the same fixture set and obtain bounded pass or fail results with frozen thresholds, tolerances, hardware setup and artifact-retention rules. The harness detects a fake bypass, echo improvement that destroys near-end speech, ceiling violations, realtime blocking and non-deterministic teardown before platform work can be accepted.
