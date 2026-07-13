# Integrate privacy-safe Phase 2 operational evidence views

## Description
Expose one acceptance and operator surface over metrics implemented by stream, Air and target stories without duplicating their accounting logic.

## Scope
Wire the canonical stream storage, processing and egress counters, buffer or seek or audible latency, start skew, queue depth, Air membership and duplicate-delivery, inbox and missed reasons, quota and feature-flag state into bounded-cardinality metrics, readiness and dashboards. Health fails for mandatory Phase 2 dependencies only when their feature is enabled and Phase 1 remains healthy with flags off. Define metric retention, clock source, scrape or query recipes, saturation alerts and sanitized evidence export with no filenames, content, secrets, raw actors or unbounded IDs. Add only genuinely cross-cutting missing instrumentation at its owning seam.

## Acceptance Criteria
B1-B7 and beta evidence can be queried from canonical counters that reconcile to implementation metrics and quota state. Readiness detects broken enabled processing, storage or Air runtime before rollout. Flags off preserve Phase 1 health, label cardinality and retention are bounded, and exported metrics reveal no content or tenant identifiers.
