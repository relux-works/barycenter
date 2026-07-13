# P1 Identity and self-service onboarding

## Description
Build app actors, scoped credentials and self-service create/join/recovery without Telegram dependency.

## Scope
Add transport-neutral actors and memberships, separate node and control capabilities, hashed server credentials, secure Windows/macOS storage, atomic orbit creation, device invites, Telegram linking and recovery. Migrate existing Telegram members and preserve production pairings with additive schema and rollback compatibility.

## Acceptance Criteria
A fresh app creates and controls an orbit without Telegram, joins by invite and recovers credentials as specified. Node tokens cannot administer or upload as control actors. Secrets are hashed server-side and stored in DPAPI/Keychain. Negative authorization, migration and rollback tests pass without changing existing orbit roles or pairing tokens.
