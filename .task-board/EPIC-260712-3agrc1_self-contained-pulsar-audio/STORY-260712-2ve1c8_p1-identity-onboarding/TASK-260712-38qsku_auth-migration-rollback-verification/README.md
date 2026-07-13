# Verify authorization, migration, and rollback behavior

## Description
Prove the story acceptance criteria with automated authorization, migration, compatibility, and rollback evidence.

## Scope
Add negative capability and authorization tests; brute-force, replay and concurrent code-consume tests; recovery secret nonpersistence and redaction checks; database migration and previous-version rollback rehearsal from the current schema and config bootstrap; and client compatibility checks for preserved pairings and control-only recovery reissue. Produce a rollout note with feature-flag order, observability and manual production checks.

## Acceptance Criteria
Tests prove node tokens cannot administer onboarding, invites, recovery or upload; plaintext recovery, invite, control and node secrets never appear in database, logs, URLs or client artifacts; one-time and concurrent semantics are exact; existing roles, slots and pair tokens survive additive migration; previous coordinator rollback tolerates new rows with the flag off; and rollout or rollback steps are reproducible.
