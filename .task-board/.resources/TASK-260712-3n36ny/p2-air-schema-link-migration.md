# Phase 2 Air schema and link migration handoff

- Date: 2026-07-15
- Task: `TASK-260712-3n36ny`
- Contract: [`p2-air-lifecycle-policy-contract-v1`](p2-air-lifecycle-policy-contract-v1.md)

This handoff describes the additive SQLite state and the only supported
authority sequence. It is engineering evidence; physical Air playback remains
in the manual-test epic.

## Durable model

`initAirSchema` installs all Phase 2 objects in one immediate transaction:

| Object | Authority |
| --- | --- |
| `airs` | public ID, title, parked/active/dissolved state, owner and revision |
| `air_members` | saved/pending/left membership and owner/admin/member role |
| `air_active_pointers` | one row per barycenter, therefore at most one active Air |
| `air_invites` | hash-only one-time invite state and policy revision |
| `air_policies` | revisioned invite/overlay/queue/replace defaults |
| `air_audit_events` | content-free lifecycle/migration/authority audit |
| `air_legacy_link_mappings` | immutable legacy link → Air provenance |
| `air_legacy_runtime_snapshots` | immutable active-link set at each authority cutover |
| `air_authority` | singleton mode, generation and divergence counter |

There is deliberately no foreign key from additive Air rows to legacy
`orbits` or `links`. A rollback binary must be able to perform its legacy
writes without deleting unknown Phase 2 rows or being blocked by an unknown
child table. Air-owned tables retain internal foreign keys and current
repository methods validate live orbits transactionally.

Partial DDL, backfill, cutover and rollback all roll back as a unit. Unique
partial indexes enforce one live membership generation per Air/barycenter and
one joined owner; `air_active_pointers.orbit_id` is the one-active invariant.

## Deterministic backfill

Startup scans `links.state='active'` in link-ID order while no request can be
served. It fails closed if an orbit occurs in more than one active link. Each
unmapped link creates exactly:

- one parked Air;
- one joined owner membership for `orbit_a` and one joined member for
  `orbit_b`;
- the default policy row;
- one immutable mapping and audit event.

The Air and membership ULIDs use the legacy `created_at` plus SHA-256-derived
80-bit entropy over immutable link coordinates. A rolled-back attempt and its
retry therefore derive the same IDs. A committed mapping is validated on every
restart and never duplicated. Backfill does not create active pointers and
does not send audio: `links` remain the sole authority in `airs_shadow`.

## Authority sequence

```text
links_authoritative
  -> additive backfill -> airs_shadow
  -> validated atomic cutover -> airs_authoritative (generation + 1)
  -> no Air-only mutation -> atomic rollback -> links_authoritative (generation + 1)
  -> any Air-only mutation -> rollback_hold (generation + 1, fail closed)
```

Cutover revalidates/backfills current active links, requires zero pre-existing
Air pointers, writes exactly two pointers per current active link, changes the
mapped Airs to active, snapshots the legacy active-link set under the next
generation, then flips the singleton generation. Runtime consumers must attach
that generation to effects and timers in the next task. On every authoritative
restart, the immutable snapshot—not mutable Air pointers—is compared with the
legacy table, so legitimate Air-only state restores while a post-cutover old
binary write fails closed.

Normal Air create, member, active-pointer, invite and policy writes are
rejected until `airs_authoritative`. Every such write increments
`divergence_count`. A rollback is safe only with zero divergence and a complete
2:1 pointer-to-active-link mapping. Any unsafe shape commits `rollback_hold`
and returns `air rollback unsafe`; it never enables links.

## Previous-coordinator rollback

An Air-unaware coordinator may be started only while persisted authority is
legacy-owned (`links_authoritative` or its additive `airs_shadow` state). It
may then serve legacy link operations. It ignores and preserves every Phase 2
table. Before returning to an Air-aware build, startup reconciles any newly
active legacy links into new stable mappings; removed links leave their prior
Air rows parked as provenance.

It is forbidden to deploy the older binary while authority is
`airs_authoritative` or `rollback_hold`: that binary cannot read the generation
and could resurrect link delivery. If rollback returns unsafe, keep the
Air-aware binary, disable mutations and repair/forward-fix under
`rollback_hold`.

Operator inspection:

```sql
SELECT mode, generation, divergence_count FROM air_authority WHERE singleton=1;
SELECT l.id, l.orbit_a, l.orbit_b, m.air_id
FROM links l LEFT JOIN air_legacy_link_mappings m ON m.link_id=l.id
WHERE l.state='active' ORDER BY l.id;
SELECT orbit_id, air_id, revision FROM air_active_pointers ORDER BY orbit_id;
PRAGMA foreign_key_check;
PRAGMA integrity_check;
```

Never edit pointer, mapping, membership or authority rows by hand. Restore the
database backup or ship a reviewed repair command that preserves generation
ordering.

## Automated evidence

The store suite covers:

- fresh schema defaults, member confirmation, saved/current switching,
  policy revisions, invite hash storage, audit and one-active uniqueness;
- stable exactly-once link backfill across restart;
- injected failure after the first backfill and before cutover/rollback flips;
- four simultaneous links / eight barycenters with preserved legacy settings;
- safe cutover/rollback with Phase 2 rows retained;
- the exact predecessor coordinator binary breaking and creating a legacy
  link while preserving unknown Phase 2 rows, followed by current-binary
  reconciliation, cutover and rollback;
- Air-only divergence entering `rollback_hold`; and
- conflicting legacy link fixtures aborting without partial mappings.

The next runtime task owns effect-generation fencing, session restoration and
actual playback resolution. This migration does not claim dual-version live
audio evidence.
