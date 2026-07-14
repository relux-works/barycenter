# Phase 1 media ACL, delete and retention integration

Date: 2026-07-14. Task: `TASK-260712-gj0cko`.

This note records the integration contract across the independently landed
generic download ACL, media lifecycle, Telegram compatibility bridge and the
current serial session runtime. It does not move target selection into ingest:
accepted immutable transmission targets remain owned by
`TASK-260712-1aprcb` and plug into the existing fail-closed reader.

## One logical authority, two rollout representations

`media_items` is authoritative for processing, ready, failed, deleted and
expired state. A Telegram acceptance atomically creates the same-ID legacy
`media` row and `media_legacy_wav_links` row. Startup reconciliation adds the
missing link for same-ID Telegram rows written by the immediately preceding
rollout and fails closed on conflicting manual mappings.

For linked rows:

- a generic failure, delete or expiry mirrors the non-playable state into the
  legacy row in the same SQLite writer transaction;
- `UpdateMedia` may publish a legacy ready path only while the generic item is
  ready and the legacy row has not become terminal;
- the legacy GET repository rechecks generic readiness, so an old or racing
  compatibility write cannot bypass generic revocation; and
- owning-control and exact target-snapshot generic GET rules remain unchanged.

Unknown, copied, foreign, deleted and expired generic IDs retain the same
non-disclosing `404 media_not_found` response. `TASK-260712-1aprcb` now installs
the immutable transmission-target store as the production reader. A node with
no exact accepted row remains fail-closed; owning control access is unchanged.

## Delete and delivery cancellation

The successful owner-control DELETE transaction still clears the canonical
key, closes publication/upload state, records audit, and writes the frozen
`media_lifecycle_v1` cancellation. The current coordinator installs an
at-least-once adapter to its single-threaded shared-session runtime:

- every queued copy of the media ID is removed;
- an active legacy voice is stopped;
- a cancelled in-flight voice fetch cannot schedule playback later, and each
  insertion pause completes before the following track load;
- the session advances exactly once and its new snapshot is durably saved; and
- replaying the outbox request is harmless.

The later transmission scheduler replaces this adapter through the same
interface for persisted target snapshots, prepare/play states and the final
click-free `fade_stop` wire behavior. No live membership or current-approach
rule is promoted into the generic ACL.

## Compatibility byte cleanup

`media_legacy_wav_links.cleanup_completed_at` is the durable receipt for the
mixed-rollout byte path. A terminal linked item is immediately non-readable,
while the asynchronous worker removes:

1. the legacy WAV path when it is inside the pinned canonical-media root; and
2. the deterministic private `.telegram/<media_id>.source` file.

The worker refuses paths outside the media root, final symlinks, non-regular
files and a replaced/redirected canonical directory. It fsyncs affected
directories before acknowledging the receipt. A crash after unlink retries
from `ENOENT`; only then is the historical absolute `path_wav` erased from the
legacy tombstone. Explicit deletion therefore does not retain a failed
Telegram source until its original expiry.

The existing daily legacy sweep remains for unlinked old rows. It excludes
linked rows, which are owned exclusively by the common lifecycle; this keeps
the legacy path deleter from bypassing root validation, directory fsync or the
durable cleanup receipt.

## Rollback and roll-forward

The schema change is additive and ignored by the exact previous coordinator.
That binary can open the database, mutate its legacy row and create another
same-ID Telegram pair. On roll-forward the current coordinator:

- restores missing Telegram links;
- restores generic terminal authority over stale legacy status;
- reopens a completed legacy cleanup receipt if the old binary reintroduced a
  status or path; and
- reopens the completed cancellation receipt for the same detected rollback
  mutation.

The build-tagged exact-previous-head test executes those mutations through the
real Store API from merge `0d6863c462111da6ed27f851a636e40d95100d73`.

## Operator visibility

`/healthz` now reports the shared lifecycle instance whether or not
self-service routes are enabled. In addition to canonical cleanup and
cancellation counters it exposes completed/failed legacy cleanups and the
pending legacy-cleanup backlog. Values contain no token, actor, media ID,
title, filename, local path or content.

Manual playback and real-hardware validation are intentionally not claimed by
this task; those checks live in the separate manual-test epic.
