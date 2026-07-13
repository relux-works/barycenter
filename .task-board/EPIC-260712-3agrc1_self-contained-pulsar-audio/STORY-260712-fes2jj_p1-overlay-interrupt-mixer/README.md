# P1 Cross-platform overlay and interrupt mixer

## Description
Implement allocation-safe cross-platform overlay ducking, limiter and interrupt/resume without main timeline drift.

## Scope
Refactor Windows and macOS audio graphs to mix an independent clip branch while continuously consuming the main program. Implement allocation-safe ducking, limiter, scheduled media start, interrupt with audible-position resume, cancellation and telemetry under shared parameters.

## Acceptance Criteria
A3 and A4 pass on real Windows/macOS audio paths. Overlay causes no ring overflow, seek/skip or post-clip drift beyond specification tolerance. Interrupt resumes within tolerance. Render callbacks perform no I/O, allocation or blocking work. Deterministic mixer tests cover ramps, limiter, cancellation and 100 sequential overlays without leak/deadlock.
