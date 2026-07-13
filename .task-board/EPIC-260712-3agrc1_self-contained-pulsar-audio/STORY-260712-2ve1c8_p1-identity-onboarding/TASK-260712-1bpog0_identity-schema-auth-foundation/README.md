# Add actor schema and scoped auth foundation

## Description
Introduce transport-neutral actors, memberships, and hashed credential persistence behind a shared ActorContext resolver while preserving existing orbit roles and pairings.

## Scope
Add additive schema and store migrations for actors, memberships, installation credentials, recovery hashes, invite and link codes, and audit events; backfill existing Telegram members and slot ownership into the new model without changing roles or node tokens; implement hashed secret lookup and ActorContext resolution for node, control, and Telegram identities behind feature flag self_service_onboarding.

## Acceptance Criteria
Existing databases migrate without changing orbit roles, slot ownership, or current node token validity; newly introduced server-side secrets are stored only as hashes; shared auth resolver distinguishes node and control capability and returns orbit plus role plus actor identity; rollback to the previous coordinator version tolerates the new rows when the feature flag is off.
