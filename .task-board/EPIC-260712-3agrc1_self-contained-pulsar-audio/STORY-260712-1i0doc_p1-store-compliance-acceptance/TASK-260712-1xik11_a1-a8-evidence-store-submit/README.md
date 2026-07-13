# Run A1-A8 gates and submit the reviewed Store build

## Description
Execute the final phase-one evidence matrix, perform root and independent high-risk reviews, and only then make the external Partner Center submission.

## Scope
From a clean provenance-tracked build, rerun coordinator, Windows, cross-build, pinned Swift, golden, migration, rollback, WACK and security suites. Execute A1-A8, Windows 10 and 11 hardware, macOS parity where applicable, p95 stop-to-audible and start-skew measurements, maximum-clip memory and 100-overlay realtime gates. Require root line-by-line review of every implementation diff plus independent security, protocol, migration and audio-render reviews with findings closed. Collect sanitized logs and real screenshots, verify public policies and coordinator availability, present the exact submission payload for the approved external-submit authority, submit or resubmit and record the certification result.

## Acceptance Criteria
Every A1-A8 and nonfunctional criterion has a build hash, method, raw sanitized artifact and pass or fail disposition; no baseline failure is waived and all mandated reviews are closed. A clean Windows reviewer completes A1 with no external account. The submitted metadata and screenshots match that build. Completion requires Store acceptance or evidence that every remaining comment is outside testability, metadata and self-contained functionality; an actual failure is never relabeled external.
