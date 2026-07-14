# Rehearse Phase 2 rollout, authority cutover and rollback

## Description
Prove additive deployment and previous-binary recovery without dual Air or link runtimes, data loss or hidden Phase 2 commands.

## Scope
Rehearse backup or snapshot prerequisites, additive DB migration, coordinator accept-but-do-not-send, capability-aware node rollout, separate targets, Air and streamed-track flags, internal enablement, link-to-Air authority cutover, mixed-version policy and rollback. Before previous-binary rollback disable new accepts and Phase 2 flags, quiesce or safely terminate active Phase 2 work, prove only one link or Air runtime can own delivery and preserve all Phase 2 rows for later roll-forward. Fault-inject partial migration and deploy failures, verify Phase 1 pairwise clip and optional Spotify before and after and publish exact commands.

## Acceptance Criteria
Operators reproduce upgrade and rollback with build, schema and data hashes. Old binaries ignore but do not delete Phase 2 data, no two runtimes deliver, flags off suppress new commands and active-work handling is explicit. Partial failures recover, Phase 1 service remains green and a later roll-forward restores saved Air, track and inbox data.
