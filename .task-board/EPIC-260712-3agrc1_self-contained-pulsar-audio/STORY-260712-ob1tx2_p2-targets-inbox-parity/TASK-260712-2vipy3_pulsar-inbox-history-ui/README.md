# Build the shared Phase 2 target and inbox presentation model

## Description
Provide transport-neutral localized state and commands for platform UIs without implementing Windows and macOS views in one task.

## Scope
Extend shared RU and EN models with active Air, permitted opaque target choices and capabilities, include-origin, targeted-track policy, consent requirement, requested and effective delivery, inbox items, TTL, receipt pagination, replay, dismiss, delete, report and mute action capabilities. Keep all authorization server-derived, represent loading, stale, offline and coordinator-error states honestly, and expose no raw IDs or non-target metadata.

## Acceptance Criteria
Windows and macOS can render the complete Phase 2 flow from the same view model and command interfaces. Labels and allowed actions match Telegram semantics, capability and expiry changes update deterministically, and a UI cannot construct a target or action not returned by the server. Before implementation continues, the task branch must fetch origin and be rebased onto the latest origin/main; conflicts must be resolved without dropping unrelated board changes, and git merge-base --is-ancestor origin/main HEAD must succeed before review handoff.
