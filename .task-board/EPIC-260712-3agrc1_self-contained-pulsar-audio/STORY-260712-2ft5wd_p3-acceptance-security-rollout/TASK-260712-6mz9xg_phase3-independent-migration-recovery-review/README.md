# Independently review migrations rollback recovery and key-loss drills

## Description
Audit production-shaped operational evidence after the rollout rehearsal and before beta.

## Scope
Review additive migrations and backups, old and new coordinator or node ordering, independent feature flags, unknown rows, rollback preservation, capture kill, schedule kill, token and device revoke, group fork recovery, lost device, surviving-device transfer, user-held recovery if selected, irreversible history loss and restore procedures. Re-run representative commands on copied production-shaped state, inspect destructive boundaries and verify operators cannot accidentally enable unreviewed claims.

## Acceptance Criteria
No critical or high migration, rollback, data-loss, stuck-feature, secret-restoration or false-recovery finding remains. The report names exact reviewed artifacts and retests, and the beta cannot begin after any unreviewed migration or recovery change.
