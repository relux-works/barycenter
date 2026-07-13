# Handle lifecycle edges and clean capture shutdown

## Description
Extend the probe to stop capture cleanly on quit, suspend, session lock and permission revoke with explicit evidence logging.

## Scope
Extend the probe so quit, suspend, session lock and permission revoke all drive a clean capture shutdown, temporary artifact cleanup and hotkey unregistration. Wire whichever packaged-app notifications or callbacks the selected API exposes, and document any OS event that cannot be observed directly inside the current sandbox.

## Acceptance Criteria
For each lifecycle edge in P1.0, the probe either stops capture cleanly and logs the ordered cleanup path or records a concrete platform limitation and next action. Repeated start and stop cycles do not leave a hung process, leaked hotkey or hidden active recording session.
