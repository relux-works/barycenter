# Add transmission persistence and target-snapshot ACL integration

## Description
Add the durable transmission state model and connect immutable target snapshots to the generic media authorization boundary.

## Scope
Add additive schema and repository methods for transmissions, transmission_targets, receipt timestamps and reasons, accepted_at plus ULID ordering, expires_at, effective delivery and downgrade reason, block rows and the DND lookup hooks required by the frozen contract. Persist offline as well as online eligible targets at coordinator acceptance. Integrate the media authorization service through those rows so later leave, apart or membership changes cannot expand access. Preserve existing elements, media rows, pairings and session snapshots during upgrade and rollback.

## Acceptance Criteria
Fresh and migrated databases represent every phase-one transmission and target state without changing legacy state. Target snapshots include the exact accepted audience and remain immutable except for lifecycle status fields. The generic media GET boundary can authorize against them without link-time membership, direct IDs cannot bypass the snapshot, and additive migration plus previous-version rollback rehearsals pass.
