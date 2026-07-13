# P2 Air rooms and approach migration

## Description
Replace pairwise runtime links with 2-to-N Air rooms while preserving approach aliases and production state.

## Scope
Add Air and membership persistence, create/invite/join/confirm/leave/park/dissolve lifecycle, policies, routing and one-active-Air semantics for 2-to-8 Barycenters and up to 20 online Pulsars. Migrate active pairwise links and preserve approach/apart as compatibility aliases with living-air catch-up.

## Acceptance Criteria
B2-B4 Air lifecycle scenarios pass with no transitive delivery or duplicates. Leaving affects only the leaving Barycenter and preserves personal state. Offline members do not block. Active-link migration and rollback are rehearsed against production-shaped data, and legacy commands preserve their user-visible meaning.
