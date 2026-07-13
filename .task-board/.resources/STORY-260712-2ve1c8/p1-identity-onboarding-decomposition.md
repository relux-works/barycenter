# P1 Identity and Self-Service Onboarding Decomposition

Story: `STORY-260712-2ve1c8`

## Task set

1. `TASK-260712-3v1k7q` Clarify recovery and Telegram link contracts
   - Explicit blocking research task because the authoritative spec defines the
     recovery behavior but does not define the HTTP contract, and it leaves the
     Telegram link role and conflict policy implicit.
2. `TASK-260712-1bpog0` Add actor schema and scoped auth foundation
   - Additive schema, actor backfill, hashed secret lookup, and shared
     `ActorContext`.
3. `TASK-260712-m5264f` Implement self-service onboarding and capability APIs
   - Create/join/recover/link issuance endpoints, capability middleware, rate
     limits, and audit hooks.
4. `TASK-260712-2xkyot` Migrate Telegram identities and keep bot compatibility
   - Move bot auth onto the actor model and preserve existing roles and pair
     compatibility.
5. `TASK-260712-2u1w16` Add macOS Keychain onboarding credential client
   - Secure storage and client flows for create/join/recover/link.
6. `TASK-260712-47uve0` Add Windows DPAPI onboarding credential client
   - Secure storage and client flows for create/join/recover/link.
7. `TASK-260712-38qsku` Verify authorization, migration, and rollback behavior
   - Final automated proof and rollout evidence.

## Execution shape

- Parallel start: `TASK-260712-3v1k7q` and `TASK-260712-1bpog0`
- API critical path: `TASK-260712-3v1k7q` + `TASK-260712-1bpog0` ->
  `TASK-260712-m5264f`
- Bot compatibility path: `TASK-260712-3v1k7q` + `TASK-260712-1bpog0` ->
  `TASK-260712-2xkyot`
- Platform client path: `TASK-260712-m5264f` + `TASK-260712-3v1k7q` ->
  `TASK-260712-2u1w16` and `TASK-260712-47uve0`
- Final proof: all implementation tasks -> `TASK-260712-38qsku`

## Completeness check

- Covered: actor model, scoped credentials, hashed secrets, atomic create/join,
  recovery, Telegram link, legacy Telegram migration, pair compatibility,
  secure storage on both platforms, negative auth, rollback evidence.
- Explicit gap closed with blocker: recovery API contract and Telegram link
  role/conflict policy.
- Diagrams attached:
  - `p1-identity-model.puml`
  - `p1-onboarding-flows.puml`
