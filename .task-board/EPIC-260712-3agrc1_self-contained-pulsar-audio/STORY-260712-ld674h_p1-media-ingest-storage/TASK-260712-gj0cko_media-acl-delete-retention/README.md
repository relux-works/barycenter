# Integrate media ACL, delete and retention lifecycle

## Description
Join target-snapshot authorization, delete cancellation and storage retention into the generic media API while preserving mixed-rollout compatibility.

## Scope
Wire generic GET and DELETE endpoints to the dedicated target ACL and retention components, media status repository and transmission cancellation interface. Ensure ready, failed, deleted and expired states are consistent across API responses, audit records, legacy WAV reads, cleanup workers and operator readiness. The integration owns no target selection policy; it consumes immutable target snapshots created by the scheduler.

## Acceptance Criteria
Authorized owners and snapshotted targets can fetch only ready media, foreign callers cannot infer existence, delete blocks new reads and cancels pending delivery before asynchronous cleanup, retention is retry-safe, and documented legacy playback remains functional during mixed rollout. End-to-end tests cover cross-orbit access, copied direct IDs, delete races, expiry races and rollback.
