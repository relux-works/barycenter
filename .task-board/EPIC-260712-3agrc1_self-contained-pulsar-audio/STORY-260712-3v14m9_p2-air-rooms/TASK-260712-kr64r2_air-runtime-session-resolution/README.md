# Make stable Air ID the sole shared-runtime owner

## Description
Generalize session routing from link IDs to one active Air per Barycenter while keeping parked rooms lazy and preventing transitive peer unions.

## Scope
Replace linkOf, negative group keys and stateFor ownership with an active-Air resolver keyed by stable Air ID. At startup restore metadata for saved and parked rooms but instantiate or warm only active runtime sessions. Build a deduplicated peer union solely from active members of that Air, never from other saved memberships. Keep one Air-scoped order and main program. Define join-in-progress: catch up an eligible track at audible position after buffer readiness, but never start an old overlay midstream. On leave, apart, dissolve or policy revocation, fade or stop the departing nodes, cancel their prepared work, revoke new access and restore their personal orbit state while remaining members continue; park when fewer than two remain.

## Acceptance Criteria
A+B+C own exactly one runtime and each target receives one command. Offline members do not block. Joining nodes catch up only the current main track by explicit living-Air rules and never hear stale overlay. A leaver stops locally without disturbing remaining nodes, saved or parked rooms consume no live session resources, and no stale link, other saved Air or restart can steal ownership or create a transitive chain.
