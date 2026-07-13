# Implement authenticated Air lifecycle and activation APIs

## Description
Expose the frozen multi-Air control plane through ActorContext and transactional services independent of legacy link mutation.

## Scope
Implement create, saved and current list, invite issue and consume, joining-primary confirm, decline or withdraw, activate or switch, leave, park and permitted dissolve. Use high-entropy hashed single-use invites, rate limits, audit, opaque Air references, role checks, capacity of eight Barycenters and one active Air per Barycenter. Make every multi-row change transactional and idempotent under repeated or concurrent requests. Drive runtime activation and parking through the Air resolver; do not discover rooms publicly or write link rows except the compatibility adapter.

## Acceptance Criteria
Authorized actors complete lifecycle operations with stable errors; self or duplicate join, wrong confirmer, expired or replayed invite, concurrent consume, active-Air conflict, over-capacity and unauthorized policy or dissolve all fail deterministically without secret or room enumeration. Saved membership and active pointer remain consistent across restart and every accepted operation is audited.
