# Extend Phase 1 target snapshots with inbox and replay lineage

## Description
Add only the Phase 2 persistence needed for N-target delivery and manual missed-item handling on top of the existing immutable target ACL.

## Scope
Extend Phase 1 transmission_targets and history indexes for arbitrary deduplicated node targets, and add inbox item state, per-target ownership, expiry bounded by media expiry, replay lineage, consumed or dismissed timestamps and revocation projection. Preserve accepted target identity across leave while preventing new members from inheriting old items. Reuse media and range-fetch ACL through the immutable snapshot and canonical deleted or disabled state. Add additive migrations and previous-version rollback fixtures without a second membership-based authorization path.

## Acceptance Criteria
Fresh and upgraded data supports N targets and exactly one inbox item per eligible missed target, stable replay lineage and deterministic pagination. Current Air membership cannot grant old access, leaving cannot broaden access, delete or disable revokes future fetch and replay, and Phase 1 targets, receipts, media and pairwise compatibility remain intact under rollback.
