# P2 Air policy enforcement handoff

`TASK-260712-25862f` implements the frozen Air permission model without
weakening recipient-local controls or the immutable transmission acceptance
boundary.

## Enforcement model

- `invite` remains enforced in the transactional Air lifecycle service.
- Generic `overlay` and `interrupt` requests authorize as `overlay`.
- Generic `after_current` requests authorize as `queue`.
- Telegram track links authorize as `queue`; Telegram play-now and playlist
  replacement authorize as `replace` when self-service identity is enabled.
- App-originated external playback re-resolves the live installation and
  authorizes as `replace` before adopting the selection.
- Before Air authority cutover, existing link-authoritative behavior is
  unchanged. After cutover, only the source barycenter's current joined Air
  membership and current policy revision are authoritative.

The policy values and defaults remain those frozen in
`protocol/air-lifecycle-policy-v1.json`. Only a primary in the owner
barycenter can replace policy. Replacement uses optimistic revision matching
and now records complete old/new policy JSON, actor/orbit and commit time in
the Air audit log.

## Acceptance and local precedence

Every accepted generic transmission stores immutable `air_id`, policy
revision, operation and `allowed` result fields. Active-Air audience expansion
uses only joined barycenters with a current pointer at acceptance. A later
join, activation, leave, role change or policy change cannot add, remove,
reorder or reauthorize target rows. Exact idempotent retries return the
original accepted snapshot.

Block and DND evaluation still runs for every resolved target after Air
authorization. Consequently an allowed Air action can still produce
`blocked` or `missed_dnd` targets; Air policy has no bypass for those local
decisions. Existing capability, media ownership and installation-binding
checks remain in the same acceptance transaction.

Migrated pairwise Airs retain the legacy link's numeric scheduler domain, so
work accepted on either side of cutover stays in one FIFO. New Airs receive a
stable internal domain while public routing and runtime ownership use the
opaque Air ID.

## Verification

- `go test ./...`
- `go vet ./...`
- targeted race suite for policy, migration, Air control and app external
  playback paths
- exact previous-head transmission rollback test with the `previoushead` tag

Coverage includes allowed and denied roles, overlay and queue snapshots,
replace authorization, local DND precedence, mutation audit contents,
non-retroactive idempotent replay, SQLite immutability triggers, migrated
defaults/domain continuity, app external-playback denial and restart-safe
schema migration.
