# P2 target inbox store and ACL

- Date: 2026-07-16
- Task: `TASK-260712-2bk0vy`
- Contract: `p2-targets-inbox-parity.v1`

## Result

The Phase 1 `transmission_targets` snapshot remains the only recipient media
authority. It now also freezes a canonical capability-set digest and resolution
time. Existing orbit, actor, slot and `binding_paired_at` fields are the binding
generation; no membership-derived recipient table was added.

`transmission_inbox_items` has one unique row per immutable target tuple. The
row is inserted in the same SQLite writer transaction as the first eligible
terminal receipt. Only the contract's three offline reasons, two DND reasons,
prepare deadline, connection loss, device unavailable and audio-graph failure
are eligible. Receipt retries and restart backfill converge through the unique
tuple without duplicate items.

The item freezes media/delivery/receipt data, exact binding ownership, expiry,
availability, dismissal/consumption timestamps and revocation state. Expiry is
the earlier of 30 days after the receipt and canonical media expiry. Replay
lineage is stored separately against the new transmission and consumes the
original item without mutating its receipt. A missed replay inherits root and
depth; depth is constrained to eight.

Repository pagination is deterministic on `(created_at DESC, inbox_id DESC)`
with a frozen upper key. Reads revalidate the current actor/slot binding against
credentials and slots, but intentionally do not join memberships or Air state.
Later members see no old rows, and a replacement binding receives the same
nonexistence result and no media bytes.

Canonical media delete/expiry and actor/orbit moderation disable project
`unavailable` into inbox rows in the same transaction as the authority change.
Generic media download continues through `AllowsMediaDownload` and its exact
target snapshot plus current binding, block, actor, orbit and media-state checks.

## Migration and rollback

The schema is additive. Current startup backfills capability digests,
`resolved_at_ms`, scheduler companions and eligible historical receipts before
reinstalling the immutable target trigger. New target columns have rollback-safe
defaults, so the immediately preceding coordinator can continue inserting and
updating Phase 1 rows. A later current startup deterministically reconciles such
rollback-era rows and creates any missing eligible inbox projection.

## Verification

- all nine eligible receipt pairs and representative ineligible failures;
- exactly-once receipt retry;
- N-row keyset pagination with a frozen upper bound;
- non-target and replacement-binding noninheritance;
- snapshot-based media denial after binding replacement;
- canonical delete revocation;
- stable replay root/depth and original-item consumption;
- additive old-shape migration and receipt backfill;
- full Store and coordinator HTTP suites;
- exact previous-head rollback fixture and hosted CI before acceptance.

The public inbox/history routes, cursor issuance, dismissal/replay HTTP actions
and versioned policy consent remain assigned to their downstream tasks.
