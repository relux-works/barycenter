# Land E2EE audit-gate board split

## Description
Move the independent cryptographic design audit, all sixteen E2EE implementation/evidence tasks and the external implementation review into a separate deferred epic without changing application code.

## Scope
Keep the four audit-packet preparation tasks in STORY-260712-1frfmi. Move TASK-260712-aniuyy, sixteen implementation/evidence tasks and TASK-260712-1ulshp into EPIC-260716-3qsztl. Make the deferred epic depend on the main engineering epic and every implementation task depend directly on the independent design audit. Remove deferred E2EE work from the main final engineering cycle.

## Acceptance Criteria
The main E2EE story contains exactly four preparation tasks; the deferred story contains the audit, sixteen implementation/evidence tasks and the external implementation review; no implementation task can start before TASK-260712-aniuyy passes; the main non-E2EE cycle does not depend on deferred E2EE work; task-board validate passes; the commit contains board files only.
