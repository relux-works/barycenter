# P1 Transmission protocol and scheduler

## Description
Add clip transmission data, protocol negotiation, prepare barrier, receipts, targeting, DND and block semantics.

## Scope
Implement media transmissions, explicit target snapshots, target receipts, HTTP endpoints, prepare/ready/play/cancel protocol, capability negotiation, an overlay controller orthogonal to Session FSM, ordering, partial readiness, DND/block and phase-one offline semantics. Keep after_current on the legacy session path where required.

## Acceptance Criteria
Protocol additions ship in all three codecs with golden and compatibility tests. Ready targets start within the specified barrier and skew target, late/offline/DND targets receive exact non-autoplay receipts, two overlays never overlap, ordering follows coordinator acceptance time, direct media IDs cannot bypass target ACL, and legacy nodes downgrade visibly to after_current.
