# Expose authorized transmission history and exact receipts

## Description
Build an actor-scoped Phase 1 read model for media processing and online transmission outcomes without leaking other tenants or creating an inbox.

## Scope
Query media, transmissions, immutable targets and memberships to return recent items with sender and origin names, kind or title, duration, audience, requested and effective delivery, downgrade or confirmation state, aggregate counts, exact permitted target receipts and allowed actions. Enforce ActorContext and orbit membership on every list and detail query, limit target detail to the frozen visibility policy, paginate deterministically, honor deleted and expired states and the 30-day history default, and map processing, ready, playing, played, partial, expired and error without raw IDs or client-side joins.

## Acceptance Criteria
Authorized app and bot actors see stable paginated history and permitted receipts for their orbit; foreign, left or revoked actors cannot infer items or targets. Aggregate counts match target states, requested versus effective delivery is visible, expiry and deletion are honest, action capability flags match current authorization, and the model contains no Phase 2 inbox or automatic replay behavior.
