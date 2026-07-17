# Approve Phase 3 independent migration and recovery review

## Description
Obtain implementation-independent Phase 3 migration and recovery approval for the exact root-reviewed candidate after signed rollout, rollback and recovery drill evidence exists.

## Scope
Select a qualified reviewer who implemented none of the reviewed schema, rollback, identity recovery, feature-disable, revoke, backup or restore paths. The reviewer names the exact root-reviewed commit, reruns representative repository migration and exact-previous-head checks, consumes signed TASK-260712-30xwu2 real-app and production-shaped drill artifacts, verifies independent feature kills, lost-device and surviving-device recovery, deferred E2EE fork/key-loss boundaries, copied-state restore and irreversible-loss wording, and signs or rejects the candidate. Any affected code, schema, feature flag, backup procedure, fixture or runtime-config delta reopens review.

## Acceptance Criteria
An implementation-independent reviewer records identity, independence, exact commit and artifact hashes; signed rollout/rollback/recovery evidence binds one reviewed build; additive migrations, exact predecessor rollback, capture and automation kills, revoke and recovery generation fencing, copied-state restore and honest irreversible-loss states pass; every Critical and High finding is fixed and independently re-reviewed; deferred E2EE paths remain disabled rather than simulated. Otherwise NF-migration-recovery-review, NF-rollout-recovery, beta and Phase 3 promotion remain blocked.
