# Implement one overlay scheduler per playback domain

## Description
Run prepare, barrier, FIFO and receipts in a controller orthogonal to the main Session FSM and uniquely keyed by the effective orbit or active approach.

## Scope
Create exactly one OverlayController or TransmissionScheduler for each effective playback domain, using a stable approach identity when two orbits are joined and the orbit identity otherwise. Serialize overlay and interrupt together by coordinator accepted_at plus ULID. For eligible online non-DND non-blocked targets, send prepare, wait at most three seconds from server media readiness, compute T = now + max(2*maxRTT + 250 ms, 500 ms), and send play only to ready targets. Mark exact offline, DND, blocked and not-ready receipts, reject late starts, fail if nobody is ready, and cancel non-started work on leave, apart, delete or explicit cancel. after_current and a confirmed whole-transmission fallback remain on the legacy Session FSM path; the scheduler must never invent an interrupt fallback.

## Acceptance Criteria
Two clips never overlap anywhere in the same effective playback domain even when commands originate from opposite sides of an approach. Ordering follows trusted coordinator acceptance time. Ready targets receive a coordinator-clock schedule capable of p95 skew at most 100 ms; non-ready and offline targets never late-autoplay. Pause or skip of the main program does not terminate scheduler state, unconfirmed interrupt fallback is impossible, and domain split, cancel and no-ready cases reach exact terminal receipts without orphan timers.
