# Prove mixer determinism and realtime safety

## Description
Build automated cross-platform evidence for the exact gain graph, scheduling, continuity, cancellation and interrupt behavior.

## Scope
Add Go and Swift fixtures for sample-accurate attack and release ramps, pre-duck timing, gain order, limiter ceiling and hit counters, main-ring consumption, absence of main input, cancel ramps, generation races, interrupt anchor and resume drift, stale timers and 100 sequential overlays. Add allocation or lock instrumentation appropriate to each render path and fail tests when callback I/O, heap allocation, blocking waits or forbidden mutexes are introduced. Include bounded memory and leak checks for the maximum phase-one clip.

## Acceptance Criteria
Deterministic tests fail on a wrong ramp, clipping, ring stall, greater-than-200-ms overlay drift, greater-than-500-ms interrupt drift, stale resume, cancellation leak or gain-ceiling bypass. Both platforms complete 100 overlays without deadlock or growth, enforce realtime callback guards, and map each automated part of A3 and A4 to evidence while naming the hardware-only remainder.
