# TASK-260721-2346wf: desktop-ui-automated-acceptance-handoff

## Description
Run the maximum deterministic cross-platform UI verification available without a human and publish one exact-build handoff for final owner verification.

## Scope
Run Windows layout/DPI/source tests, Go race/vet/cross-build, signed package GUI-subsystem checks, macOS UI/model/source tests, Swift release build, localization/accessibility static checks and exact artifact provenance. Update the single owner verification task with remaining physical scenarios only.

## Acceptance Criteria
A reproducible manifest records every automated command, exact source/artifact hashes and pass/fail result. All deterministic gates pass or produce a concrete engineering defect. The final owner handoff contains one concise physical real-app checklist covering Windows and macOS without duplicate legacy tasks and makes no manual pass claims.
