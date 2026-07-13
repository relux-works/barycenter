# Independent Phase 1 migration and rollback review

## Description
Have a non-implementing reviewer audit all additive schema and state transitions against production-shaped legacy data.

## Scope
Inspect identity, media, upload, transmission, target, block, report and audit migrations plus backfills and feature flags. Exercise fresh, current-production-shaped, partially migrated, failed transaction and concurrent-worker cases; deploy the previous coordinator binary against upgraded data with flags off; verify legacy actors, roles, slots, tokens, media, queue and session snapshots; and inspect backup or restore prerequisites without altering production.

## Acceptance Criteria
A source-linked independent report and fixtures prove migrations are transactional and additive, rollback preserves legacy service and data, stale workers cannot corrupt state, and no cleanup is smuggled into rollout. Every critical or high finding is fixed and rerun before root acceptance.
