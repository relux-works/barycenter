# P3 Near-live push-to-talk

## Description
Implement safe global hold-to-talk and progressive low-latency audio transport with fallback toggle.

## Scope
Validate global key-down/up under Store constraints, implement hold gesture with toggle fallback, progressive encoded chunks, signalling, jitter buffer, cancellation, backpressure, reconnect and loss handling, and synchronized live ducking without stale capture continuation.

## Acceptance Criteria
C1-C2 pass: 100 press/hold/release cycles do not stick across foreground, lock or quit states, and real-network mouth-to-ear p50/p95 targets hold under the specified packet loss. Reconnect never resumes an old session, fallback remains usable and main playback recovers cleanly.
