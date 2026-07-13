# Enforce media download target ACL

## Description
Implement the security boundary that authorizes generic media reads from immutable ownership and transmission-target snapshots rather than knowledge of a media ID.

## Scope
Implement generic GET authorization for owning control actors, explicitly targeted node identities and only the compatibility cases approved for mixed rollout. Resolve target access through persisted transmission target rows, reject revoked actors and nodes, deleted or expired media, and non-target or cross-orbit callers with a response that does not reveal existence. Keep long-lived credentials out of URLs and log only sanitized authorization outcomes. Add negative tests for guessed IDs, copied URLs, current-approach membership changes after acceptance, direct node/control capability confusion and legacy media access.

## Acceptance Criteria
A caller that is not the owner or a snapshotted target cannot discover or fetch media even with its exact ID or URL; later Air or approach membership changes do not expand the accepted target set; delete or actor revocation blocks new reads immediately; authorized new and legacy nodes retain only their documented compatibility access; tests prove tenant isolation and uniform not-found behavior.
