# Approve Phase 2 target, range and rights security review

## Description
Independently review and approve the exact Phase 2 target, cursor, range, consent, replay, moderation and callback security candidate after repository and manual evidence are complete.

## Scope
Review the exact candidate commit and source hashes for opaque target selectors, immutable snapshots, cursor isolation, inbox/replay lineage, content-policy consent, conditional range/cache authorization, reporter-local effects, canonical delete/disable revocation and Telegram callbacks. Rerun representative adversarial checks and inspect manual B5-B7 plus rollout artifacts. Do not accept enumeration, stale binding, broadcast fallback, cache/refill after revoke, ambiguous consent, report-driven global denial or callback replay.

## Acceptance Criteria
An implementation-independent security reviewer names the exact commit, reruns representative adversarial checks, reviews physical/mixed-fleet and rollout evidence, records every finding and signs only after all Critical and High findings are fixed and re-reviewed. Otherwise production targets, streamed ranges and Phase 2 promotion remain blocked.
