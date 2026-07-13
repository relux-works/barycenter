# Migrate Telegram identities and keep bot compatibility

## Description
Move the Telegram adapter onto the actor model while preserving existing orbit behavior and pair compatibility.

## Scope
Backfill existing Telegram members into actor and membership rows; route bot command authorization through the shared ActorContext; implement one-time Telegram link consume flow and merge semantics from the clarification note; preserve current pair, share, leave, revoke, and role-transfer behavior for migrated orbits and mixed legacy plus self-service installs.

## Acceptance Criteria
Existing production orbits keep the same member roles, slot ownership, and paired homes after migration; Telegram commands authorize through the same orbit plus role context as app actors; linking Telegram to an app-owned orbit never transfers identity ownership to Telegram; bot and store tests cover migrated members plus mixed legacy and self-service pair flows.
