# Implement the bounded ephemeral live relay runtime

## Description
Relay one authorized live speaker per playback domain with isolated per-target backpressure and no media persistence.

## Scope
Authorize sender ActorContext and Air overlay policy, resolve and seal exact targets, enforce DND and block, allocate random generation state and accept bounded binary frames only after start. Relay ephemerally with per-target queues and drop or termination policy so a slow target cannot block others; cap sessions, frames, bytes, rate and lifetime; serialize against overlay and interrupt; schedule initial duck or release; surface metadata-only receipts and telemetry. End on release, watchdog, cancel, leave, disable, disconnect or restart and never resume or persist chunks. Keep feature flags and Phase 2 paths intact.

## Acceptance Criteria
Concurrent starts have one deterministic winner or busy result, every accepted frame is bounded and attributable, slow, offline or malicious targets cannot grow coordinator memory or delay peers, and chunks never enter storage or ordinary logs. Membership and recipient controls are enforced, stale or restarted sessions die, duck and main program recover and fault tests show bounded resources and no Phase 2 regression.
