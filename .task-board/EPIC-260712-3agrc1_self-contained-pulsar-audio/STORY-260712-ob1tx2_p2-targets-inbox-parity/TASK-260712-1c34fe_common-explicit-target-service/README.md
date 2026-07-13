# Extend the common transmission service to N explicit targets

## Description
Remove one-or-both and personal-broadcast fallback by extending the Phase 1 service, not by adding a competing transport path.

## Scope
Accept only opaque target selectors authorized for the ActorContext, resolve own Barycenter, active Air and explicit Barycenter or node sets to exact deduplicated nodes at trusted acceptance time, apply include-origin and Air policy, and seal immutable snapshots. Implement the frozen targeted-track semantics and explicit mixed-version policy: compatible Phase 1 clip fallback remains visible, unsupported stream targets are surfaced and handled by the sender-selected policy without blocking supported targets or silently broadcasting. Manual replay creates a new transmission and new target snapshot. App and Telegram adapters call this same service.

## Acceptance Criteria
Personal delivery works for one or many recipients with no broadcast fallback, duplicate node or transitive Air delivery. Targets cannot be forged or enumerated and remain immutable after acceptance. Track and clip behavior follows one explicit capability policy, replay cannot inherit stale rights or ordering, and transport adapters contain no target business logic.
