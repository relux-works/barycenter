# Implementation handoff — identity schema and auth foundation

Task: `TASK-260712-1bpog0`  
Use only after `TASK-260712-3v1k7q` has a root-approved outcome. The approved
recovery/Telegram contract and `docs/analysis/p1-root-review-amendments.md`
override older diagrams or prose.

## Scope

Implement only the coordinator identity/config/store foundation requested by
this task: additive schema and migrations, idempotent legacy backfill and
reconciliation, hash helpers, transport-neutral `ActorContext` resolution for
node/control/Telegram principals, and the `self_service_onboarding` feature
flag. Do not implement HTTP endpoints, Telegram command UX, Keychain/DPAPI
clients, media ingest, or Windows capture in this task.

## Non-negotiable invariants

- Preserve every existing `members`/`slots` row, role, owner, revoked state and
  `slots.token_hash`; old node tokens must keep authenticating through the exact
  existing SHA-256-of-canonical-string convention.
- Never mint plaintext control/recovery material during backfill. A node token
  is playback/heartbeat/scoped-media only and can never provision control or
  recovery by itself.
- All new server secrets are hash-only lowercase 64-hex `TEXT`; reproduce the
  reviewed hash vectors and use fixed-size constant-time digest comparison where
  a submitted secret is verified.
- Migrations are additive, idempotent, transaction-safe, and tolerate a
  handcrafted pre-feature database. Enable and test SQLite foreign-key
  enforcement on every connection. Preserve previous-coordinator rollback and
  the reviewed rollback→old mutation→new reconciliation behavior.
- Add the reviewed default-active orbit-status migration and database-enforced
  identity/membership uniqueness. Do not replace a database constraint with an
  application-only pre-check.
- Keep actors/memberships separate from installation credentials. The legacy
  slot row remains the sole authoritative node-token hash; no mutable duplicate.
- `ActorContext` must carry enough information to distinguish principal type,
  node versus control capability, actor, orbit, role, and active/revoked/disabled
  outcome. Role never upgrades a node credential into control capability.
- `self_service_onboarding` is false by default, represented in YAML and a
  `DUET_SELF_SERVICE_ONBOARDING` environment override with the repository's
  existing 1/other/unset convention. Feature-off behavior preserves current
  production paths.
- Do not log plaintext tokens, recovery material, codes, request-equivalent
  payloads, or token-derived values useful as credentials.

## Required tests

- Fresh database schema, reopen/idempotency, and foreign-key enforcement.
- Handcrafted old database with multiple orbits, all roles, multiple slots,
  revoked slots, primary/non-primary/orphan `paired_by`, then exact backfill
  assertions with no legacy mutation.
- Previous-binary-compatible schema tolerance and the reviewed rollback/
  reconciliation cycle.
- Exact hash vectors plus a real legacy `PairSlot`/`LookupToken` round trip
  before and after migration/reopen.
- Node/control/Telegram ActorContext success and negative matrix, including
  revoked actor, revoked slot, left membership, disabled orbit, orphan slot,
  NULL/unprovisioned control state, and node-only provisioning denial.
- Concurrent/idempotent backfill and uniqueness-conflict behavior.
- Feature flag YAML/env/default/invalid-value coverage and feature-off behavior.
- Run from `coordinator`: `go test ./...` and `go test -race ./...`.

## Delivery discipline

- Edit product/test files only inside the task scope. Do not edit planning,
  research, task-board, or unrelated source files. Do not commit.
- Record exact changed files, migration decisions, tests and results in the task
  outcome. Return the task to `to-review`, never `done`.
