# Implement self-service onboarding and capability APIs

## Description
Add the Phase 1 app-first onboarding endpoints and enforce control-token-only mutation rules across them.

## Scope
Implement transactional orbit creation, device invite create and consume, Telegram link-code issue and recovery according to the frozen contract. Mint separate node and control credentials plus one-time recovery material with required entropy, return the recovery secret only on initial creation or deliberate rotation and never log it. Add shared capability middleware, rate limits by IP and installation attempt, code attempt limits, constant-time hash validation and audit so node tokens are rejected from onboarding, invite, recovery and upload-admin surfaces while legacy pair remains node-only.

## Acceptance Criteria
Fresh create and join mint separate credentials; the recovery secret is delivered once for explicit user handling and only its hash persists server-side. Node tokens cannot administer or upload, guessed or replayed codes are rate-limited and uniformly rejected, concurrent consumes have one winner, and legacy pair plus websocket registration continue. Creation, recovery and rotation are atomic and auditable without plaintext secrets.
