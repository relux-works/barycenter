# Close P1 engineering readiness and freeze the manual-test handoff

## Description
Run every automatable P1 gate and required engineering review, freeze the signed candidate and hand its unresolved real-app scenarios to EPIC-260714-th54l3.

## Scope
Rerun coordinator, Windows hosted, cross-build, pinned Swift, golden, migration, rollback, manifest, WACK where available and security suites. Close root and independent engineering reviews, verify policies and candidate metadata, index exact build hashes and map A1-A8 hardware, UI, audio and Store-observation work to the manual epic. Do not mutate Partner Center.

## Acceptance Criteria
All automatable P1 checks and reviews are green or have explicit engineering holds; the signed candidate and sanitized evidence index are reproducible; every manual gap points to EPIC-260714-th54l3. Completion authorizes P2 engineering only and makes no Store, physical-hardware or production acceptance claim.
