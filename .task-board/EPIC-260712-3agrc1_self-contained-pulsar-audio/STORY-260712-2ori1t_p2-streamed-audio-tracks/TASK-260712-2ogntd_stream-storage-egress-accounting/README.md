# Implement track storage, processing and egress accounting

## Description
Turn Phase 2 quota and observability requirements into enforced, privacy-safe counters and operator controls rather than passive hooks.

## Scope
Account per actor and orbit upload starts, input bytes, canonical variant bytes, temporary processing disk, concurrent jobs, retained storage, range requests and actual egress with media, variant and outcome dimensions that do not expose content or filenames. Enforce configured upload, processing, storage and egress quotas at deterministic boundaries, with a frozen policy that does not abruptly corrupt already-started playback. Reconcile counters after crash, delete, retention and retry; expose readiness, saturation and cost metrics, alerts and admin adjustment audit; prevent range amplification or cross-tenant usage oracles.

## Acceptance Criteria
Metrics reconcile to stored and served bytes across ingest, retry, range, cache refill, delete and retention. Quotas stop new work or follow the explicit active-playback policy with stable user errors, never leak another tenant or leave counters permanently charged after cleanup. Operator dashboards and tests prove section 20.5 storage and egress gates are actually functional.
