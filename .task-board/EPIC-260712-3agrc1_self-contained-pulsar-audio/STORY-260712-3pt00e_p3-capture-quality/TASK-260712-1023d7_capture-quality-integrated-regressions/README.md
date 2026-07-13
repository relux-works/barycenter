# Run processed-clip and live capture regressions

## Description
Exercise both platform DSP implementations through their real clip and live PTT seams before the final acoustic matrix.

## Scope
Run deterministic fixtures and hostile lifecycle tests for Windows and macOS recorded clips, local record-then-play and every live PTT platform pairing. Cover far-end-only, near-end-only, double-talk, 2 percent loss, jitter, slow recipient, route and default-device changes, Bluetooth profile change, clock drift, clipping, silence, permission revoke, cancel, lock or sleep, reconnect and feature rollback. Reconcile diagnostics, callback realtime guards, CPU, memory, cleanup and both ceiling orderings without retaining private audio.

## Acceptance Criteria
Automated and lab evidence catches bypassed DSP, echo regression, speech damage, stale reference or generation, ceiling inversion, unbounded buffers, callback blocking and misleading diagnostics. All supported workflows are green before real C3 evidence; failures mark a precise route unsupported or block promotion rather than being averaged away.
