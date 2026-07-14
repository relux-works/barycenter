# Phase 1 media delete and retention contract

Date: 2026-07-14. Task: `TASK-260712-1sae4q`.

This note freezes the independently deployable media-lifecycle side of the
Phase 1 contract. Immutable target rows and scheduler cancellation are owned by
the later transmission story; this task supplies their forward-only durable
interface and does not create a reverse ingest-to-scheduler dependency.

## Authorized delete boundary

`DELETE /v1/media/{media_id}` requires a live control credential. The
credential, actor, non-satellite control capability, orbit membership and media
ownership are rechecked inside the same SQLite writer transaction.

- A first owner-orbit delete returns `204 No Content`.
- Repeating that delete returns the same `204` without another outbox row.
- A well-formed unknown, foreign-orbit or already-expired ID returns the same
  `404 media_not_found` body. A copied ID is not an existence oracle.
- Missing or invalid credentials remain `401`; node-only or otherwise
  insufficient credentials remain `403`.
- Request bodies, query strings and path-shaped IDs are rejected. Tokens,
  media IDs, titles, filenames and filesystem paths are not logged on failure.

The successful transaction does all of the following or none of it:

1. changes an active `processing`, `ready` or `failed` item to `deleted`;
2. clears `storage_key`, so every generic ready-only read gate revokes new
   downloads immediately;
3. cancels any still-pending canonical publication;
4. enqueues each known canonical key for physical cleanup;
5. terminally closes an open/finalizing upload session;
6. writes one content-free audit transition; and
7. writes one `media_delivery_cancellations` outbox row.

A stale processor cannot make a terminal item ready. Publication and cleanup
share a per-storage-key critical section; deletion during the link/DB boundary
either completes before publication or makes the publisher remove its rejected
link. A process crash leaves a pending durable cleanup that converges on the
next run.

## Frozen active-delete policy

The durable interface version is `media_lifecycle_v1`:

| Target state | Required action |
|---|---|
| queued, preparing, prepared or scheduled | `cancel`; disarm every future start |
| already playing | click-free `fade_stop` |
| main program paused by an interrupt | `resume_once` through generation-safe state |

The terminal reason is `media_deleted` (or `media_expired` for retention).
There is no confirmation click, replay, reconnect start, cache refill or late
autoplay after this decision. A cancellation consumer is at-least-once and must
be idempotent by `(media_id, media_revision)`: a crash may occur after target
state changes but before the outbox receipt commits.

Until transmission persistence exists, the production sink is intentionally
unset and requests remain visible as pending. `TASK-260712-gj0cko` connects this
interface to immutable target snapshots and the scheduler after those rows and
cancel hooks land. Exact wire receipt names and ramp parameters remain owned by
`TASK-260712-51y5k9`; they may not weaken the actions above.

## Retention and cleanup

Phase 1 uses these server-side bounds:

| Data | Behavior |
|---|---|
| upload capability/session | one-hour expiry; open/finalizing sessions are failed and temp cleanup is scheduled |
| failed upload/source bytes | removed by the 15-minute maintenance loop, comfortably within the 24-hour maximum |
| ready clip bytes | `expires_at` is seven days from creation; expiry revokes reads, queues cancellation and unlinks canonical bytes |
| deleted/expired canonical bytes | asynchronous durable cleanup; DELETE does not wait for disk I/O |
| transmission/history metadata | separate 30-day policy; this worker never deletes it |
| reported evidence | separate restricted review policy, up to 30 days; this worker never deletes it |
| content-free media ingest audit | pruned after 90 days |
| media tombstone | retained for idempotency and future metadata references; contains no audio bytes or storage path |

Cleanup accepts only generated `media/v1/<sha256-shaped-random-key>` paths,
uses `Lstat`, refuses symlinks and non-regular files, unlinks, fsyncs the
canonical directory and only then acknowledges the outbox. A crash after
unlink but before acknowledgement retries from `ENOENT`. Repository checks at
both selection and acknowledgement refuse cleanup while any ready row still
references the key. Tenant-local hard-link deduplication is safe because each
media item has a distinct directory entry; unlinking one entry does not remove
the other tenant-local reference.

## Operator visibility

`/healthz` exposes content-free `media_lifecycle` counters and backlogs:
sweeps/failures, accepted deletes, expired items, completed cleanup count and
bytes, safe cleanup-state refusals, completed cancellation deliveries/failures,
pruned audit rows, and current expirable, storage-cleanup, cancellation and
temp-cleanup backlog. The health bit records whether the latest sweep completed
without an operational error and recovers after a successful retry. Metrics
contain no title, filename, actor, token, local path or audio content.

## Backup and privacy handoff

Logical deletion and fetch revocation are immediate in the live database, but
backup expiry is not instantaneous erasure. The current Litestream policy in
`docs/backup-restore.md` retains SQLite recovery points for seven days. Those
recovery points may contain a pre-delete media row and audit history; restoring
one requires running normal reconciliation/retention before serving traffic.

Litestream backs up the SQLite database, not canonical media bytes. The legacy
runbook also states that the media directory is ephemeral and is not included
in its DB backup. If an operator adds volume snapshots or any media-object
backup, its retention and deletion limitation must be published in the privacy
policy before rollout; the application must not claim that immutable backups
are erased earlier than that provider actually expires them.
