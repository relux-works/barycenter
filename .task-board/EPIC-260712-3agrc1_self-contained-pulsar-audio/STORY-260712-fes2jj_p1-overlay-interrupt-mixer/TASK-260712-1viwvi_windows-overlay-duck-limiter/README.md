# Implement Windows additive overlay, ducking and limiter

## Description
Replace voice-replaces-music rendering with a separately prepared clip branch while continuously consuming the main WASAPI ring.

## Scope
Implement output = limiter(main_program * duck_gain + overlay * overlay_gain + cues) before final master gain and local volume ceiling. Arm clip buffers off the render thread; start pre-duck at T minus 250 ms; use defaults of minus 12 dB, 250 ms attack and 600 ms release unless the frozen command overrides them; keep main ring reads continuous even in silence; apply a post-mix limiter that prevents clipping; ramp out cancellation and release duck safely; and emit sanitized ring-fill, underrun and limiter counters. The WASAPI callback must perform no I/O, allocation, waits or blocking locks.

## Acceptance Criteria
A3 continuity holds on Windows: a ten-second overlay starts on schedule, music remains on its timeline, post-clip position error is at most 200 ms, and there is no overflow, seek, skip or underrun burst. Gain ordering keeps local master and ceiling last, an absent main program leaves the clip at normal gain, cancellation cannot click or strand ducking, and repeated FIFO handoff does not leak or deadlock.
