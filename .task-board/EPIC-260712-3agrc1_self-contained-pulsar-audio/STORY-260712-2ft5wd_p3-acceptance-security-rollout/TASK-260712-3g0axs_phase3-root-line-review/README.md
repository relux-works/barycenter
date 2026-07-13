# Root-review every Phase 3 implementation diff

## Description
The primary agent performs the mandatory line-by-line review before any integrated acceptance build is frozen.

## Scope
Read every changed implementation, migration, protocol, UI, test, packaging, dependency and operational file from the Phase 3 stories and observability task. Map each hunk to the authoritative spec and task AC, inspect error and rollback paths, auth and ACL, realtime callback constraints, crypto boundaries, secrets and privacy, Store posture, concurrency, bounded resources and backwards compatibility. Review third-party versions and generated artifacts, run all unit, integration, sanitizer or race, cross-build, package and fixture checks available locally and record hardware-only gaps. Reject agent assertions unsupported by code or raw evidence. Freeze the exact commit and build hashes; any relevant later diff invalidates this review.

## Acceptance Criteria
A root-authored report lists every file and hunk reviewed, commands and results, spec and AC coverage, rejected or fixed findings and remaining external-only checks. No critical or high issue is open, no agent self-review substitutes for direct inspection and downstream reviewers and matrices use exactly the frozen reviewed hashes.
