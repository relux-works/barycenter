# Implement cue schedule and scoped-principal control APIs

## Description
Expose bounded authenticated control-plane commands over the approved saved-cue and automation models.

## Scope
Add actor-authorized APIs for saved cue list or create or rename or order or delete, schedule CRUD and enable or disable, principal issue, list metadata and revoke, feature status and emergency disable. Use stable idempotency, optimistic concurrency, canonical target selectors and validation for timezone, DST policy, quiet hours and delivery mode. Return bearer secrets once in a response body, never URLs or logs; enforce narrow origin or transport rules chosen by the threat model.

## Acceptance Criteria
Authorized roles can manage only their orbit resources with least-privilege scopes. Replay and concurrent edits are deterministic, invalid or foreign media and targets reveal no data, issued secrets are unrecoverable after the one response, revoke is immediately visible and fixtures document every stable error and mixed-version state.
