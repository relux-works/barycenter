# Implement macOS additive overlay, ducking and limiter

## Description
Add an independently prepared AVAudio clip branch and explicit duck controller while continuously consuming the main source-node ring.

## Scope
Implement output = limiter(main_program * duck_gain + overlay * overlay_gain + cues) before final master gain and local volume ceiling using the existing AVAudioSourceNode plus AVAudioPlayerNode graph. Prepare files off the render thread; start pre-duck at T minus 250 ms; use defaults of minus 12 dB, 250 ms attack and 600 ms release unless overridden by the frozen command; keep source-ring reads continuous; prevent post-mix clipping; ramp out cancellation and release duck; and emit sanitized ring-fill, underrun and limiter counters. Render callbacks must perform no I/O, allocation, waits or blocking locks.

## Acceptance Criteria
A3 continuity holds on macOS: a ten-second overlay starts on schedule, music remains on its timeline, post-clip position error is at most 200 ms, and there is no overflow, seek, skip or underrun burst. Gain ordering keeps local master and ceiling last, an absent main program leaves the clip at normal gain, cancellation cannot strand the graph or ducking, and repeated FIFO handoff does not leak or deadlock.
