# Add Air persistence and transactional link-to-Air migration

## Description
Introduce additive durable Air state and a rollback-safe authority cutover from active links without dual-runtime delivery.

## Scope
Add Air, member, invite-hash, policy, audit and per-Barycenter active-Air pointer repositories with separate saved membership and runtime activation state. Backfill each active link exactly once into a stable two-member Air and record deterministic legacy-link to Air mapping. Use feature flags and a documented authority sequence so links remain authoritative before cutover, Air becomes solely authoritative after cutover, alias writes remain coherent and rollback cannot run both runtimes. Preserve new Phase 2 rows when an older coordinator serves legacy behavior, rehearse partial or failed migration and concurrent lifecycle changes on production-shaped fixtures.

## Acceptance Criteria
Fresh and upgraded databases restore saved, parked and active state with a transactional one-active invariant. Every link maps to one two-member Air without duplicate membership or delivery. Partial migration rolls back atomically, restart does not repeat backfill, and the previous coordinator can provide documented legacy service without deleting Phase 2 data or resurrecting a parallel link runtime.
