# Implement the least-privilege moderation control plane

## Description
Add report and operator decision workflows while reusing the canonical block, media-delete, credential-revoke and scheduler services rather than duplicating them.

## Scope
Add additive report and append-only moderation-audit persistence; actor-authorized report creation and safe status; rate and abuse controls; and separately authenticated least-privilege operator list, evidence-access and action APIs or CLI. Permit only accessible foreign media reports. Expose reporter, media metadata, accepted target scope and a time-limited authorized evidence copy with every access audited. Decisions are no_action, delete_media, disable_actor or disable_orbit. Delegate block to the shared recipient-control service, delete to media lifecycle, and disable to credential revocation, live WS disconnect, pending-delivery cancellation and future fetch denial. Apply reported-content retention and rollback-safe migrations.

## Acceptance Criteria
An authorized user can report each accessible foreign item without gaining new media access. An operator credential cannot act as a user credential and every evidence read and decision is attributable. Delete and disable effects propagate immediately through canonical services; repeated actions are idempotent; foreign reporters receive only privacy-safe status. Rate, tenant, revoked-operator, migration, rollback and audit-tamper tests pass, and no ordinary log contains audio, secrets or local paths.
