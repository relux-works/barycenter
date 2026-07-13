# TASK-260712-m5264f — R2 rework results

Date: 2026-07-13  
Role: developer  
Board handoff target: `to-review`

## Result

Implemented all three mandatory R2 corrections against the combined dirty
worktree without committing, pushing, resetting, or editing the sibling
Telegram-consume implementation.

### R2-F1 — exact recovery-rotation audit

- Added additive `recovery_rotation_audit_details`, keyed to the existing base
  audit row. It stores only the nullable prior `recovery_id` and required new
  `recovery_id`, both constrained to the frozen non-secret `rec_` handle shape.
- `RotateRecovery` now retains the old handle before the credential update and
  inserts the base event plus exact transition detail in the same immediate
  transaction. Base/detail/update/commit failure rolls the generation back and
  returns no replacement material.
- The node credential is not rewritten. Collision retry produces one committed
  rotation and one detail row. Tests scan both audit tables for returned
  plaintext credentials and their credential digests.

### R2-F2 — durable rate-limit audit

- Added typed `rate_limit_audit_events` with stable classes for all seven HTTP
  limiters and the future in-process Telegram-consume class. It records only
  event type, class, a class-domain-separated SHA-256 subject digest, optional
  verified actor/orbit scope, and timestamp.
- The schema and repository allow exactly two shapes: pre-identity classes are
  fully unscoped; recovery-rotation and Telegram-link-issue are fully scoped to
  a real actor-orbit membership. Partial, fabricated, mismatched, and
  class-incompatible scopes fail without a row.
- The existing process-local reservation order and rolling-window logic remain
  unchanged. A rejected reservation must durably audit before the handler emits
  `429` and `Retry-After`; persistence failure emits the ordinary internal-error
  envelope, no `429`, and no `Retry-After`, while the reserved attempt remains
  consumed.
- `RecordRateLimitAudit` is a shared error-returning Store method suitable for
  the sibling in-process Telegram limiter. The sibling was deliberately not
  changed in this task.

### R2-F3 — app-first alignment quarantine

- The app-first reconciliation shortcut preserves its accepted role only after
  proving exactly one active membership whose orbit matches
  `installation_credentials.slot_orbit_id`.
- The final serving gate independently checks every installation credential for
  the same alignment.
- A typed violation rolls back ordinary reconciliation, then a separate
  immediate transaction re-verifies the violation, revokes the actor, and
  records `identity.alignment_quarantined`. Startup returns the fatal alignment
  error and no Store. Quarantine failure is joined into the startup error and
  its transaction rolls back; service still does not start.
- Real close/reopen fixtures prove foreign-orbit and missing-membership failure,
  durable quarantine, unchanged credential hashes, repeated failure before
  repair, explicit repair, and successful reopen with the original app-first
  role.

## Endpoint / acceptance-criteria mapping

| Surface | Contract and R2 evidence |
| --- | --- |
| `POST /v1/onboarding/orbits` | Separate node/control/recovery material, hash-only persistence, no-store transport contract, node bearer rejection, source-IP and installation-attempt durable limiter classes. |
| `POST /v1/device-invites` | Shared control middleware plus role/lifecycle transaction recheck; node and satellite negative coverage. |
| `POST /v1/device-invites/consume` | Uniform code failure, source-IP limiter audit, independent-connection single winner, capacity/revoked-slot reuse. |
| `POST /v1/recovery/consume` | Fixed-shape/dummy-hash validation, source-IP and recovery-ID limiter audit, replay/idempotency and concurrent serialization. |
| `POST /v1/recovery/rotate` | Actor-scoped limiter audit, exact old/new durable rotation detail, rollback injection, collision retry, one-time response, unchanged node credential. |
| `GET /v1/actor/context` | Shared `ActorContext`, capability/lifecycle classification, exact errors and node playback-only behavior. |
| `POST /v1/telegram-links` | Control plus role authorization, actor-scoped limiter audit, hash-only one-time code issue, no secret URL. Public Telegram consume remains absent. |
| Upload administration and legacy pair/websocket | Existing capability matrix rejects node tokens from upload administration; feature-off, legacy pair, and websocket registration compatibility suites remain green. |

Primary production-path tests include
`TestR2RecoveryRotationAuditRetainsExactTransition`,
`TestR2FirstRecoveryRotationRecordsNullPriorGeneration`,
`TestR2RecoveryRotationAuditFailuresRollbackCredential`,
`TestR2RecoveryRotationCollisionRetriesWithoutExtraAudit`,
`TestR2EveryHTTPRateLimitClassPersistsDurableAudit`,
`TestR2RateLimitAuditFailureSuppresses429AndRetryAfter`,
`TestR2AppFirstAlignmentViolationQuarantinesAndRefusesServing`,
`TestR2AppFirstMissingMembershipIsQuarantined`, and
`TestR2AlignmentQuarantineFailureStillRefusesServing`. The full existing
capability, lifecycle, concurrency, rollback, legacy, websocket, and Telegram
compatibility suites were also executed.

## Exact post-edit SHA-256

```text
840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
8c2d5544a75cc09eb6b9b3980e91096a6ba8ef46e093c8fa12f847bc4f45cf2a  coordinator/internal/store/identity_migration_rework_test.go
08d8d49e269701a03bb4bbcf5be49f6e9fd71a54aa00a9fabc6f1fa96c566ec0  coordinator/internal/store/onboarding_rework_r2_test.go
d0c969f388d2b4138918c3e07490216c99c99f8565d4b90a39cb9238c53a1d1e  coordinator/cmd/duet-coordinator/onboarding.go
8b7e8582a7de081653e778e5d88fb6ba0db7858d5c813bdf3a40f4166ab7c350  coordinator/cmd/duet-coordinator/onboarding_rework_r2_test.go
```

Untouched sibling Telegram-consume boundary (matches the pre-edit capture):

```text
583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go
```

## Commands and results

All commands were run locally from `coordinator/` unless the command explicitly
uses repository-root paths.

### Corrected failures

1. Initial HTTP-focused command:

   ```bash
   gofmt -w cmd/duet-coordinator/onboarding_rework_r2_test.go && go test -count=1 ./cmd/duet-coordinator -run '^TestR2' -v
   ```

   Initial result: **FAIL** in the new create/source-IP,
   create/installation-attempt, and audit-failure cases. The production code
   was not implicated: the new test helper modeled an external peer without a
   TLS connection and one assertion searched for a small numeric actor ID that
   also occurred in SQLite's numeric error code. The helper now supplies TLS,
   and the scoped failure fixture allocates a high actor ID. Re-running the exact
   command: **PASS**; all seven class subtests and both failure scopes passed.

2. Root live audit finding R2-F2a: the first repository version accepted
   half-scoped/class-incompatible positive identifiers. The repository and DDL
   now enforce both-or-neither, class-specific shapes, and real actor-membership
   coordinates. Negative repository and direct-schema tests were added.

### Passing final validation ledger

```bash
gofmt -w internal/store/onboarding_rework_r2_test.go && go test -count=1 ./internal/store -run '^TestR2' -v
# PASS

gofmt -w internal/store/security_audit.go internal/store/onboarding_rework_r2_test.go && go test -count=1 ./internal/store -run '^TestR2' -v && go test -count=1 ./cmd/duet-coordinator -run '^TestR2' -v
# PASS

go test -count=10 ./internal/store -run '^TestR2' && go test -count=10 ./cmd/duet-coordinator -run '^TestR2'
# PASS (10/10 each)

go test -count=1 ./internal/store -run 'Test.*(Identity|Onboard|Recovery|Invite|Telegram|Rollback|Migration|Provision|Capability|Legacy|OrbitStatus|R2)' && go test -count=1 ./cmd/duet-coordinator -run 'Test.*(Identity|Onboard|Recovery|Invite|Telegram|Capability|Legacy|WebSocket|R2)' && go test -count=1 -tags previoushead ./internal/store -run '^Test(R8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition)|Telegram.*Previous)' -v
# PASS; exact predecessor authority round trip, two-generation projection, and Telegram reconciliation all passed

go test -count=3 ./internal/store -run '^TestR2' && go test -count=3 ./cmd/duet-coordinator -run '^TestR2'
# PASS after the final schema scope constraint

go test -count=1 -tags previoushead ./internal/store -run '^Test(R8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition)|Telegram.*Previous)' && go test -count=1 ./... && go test -race -count=1 ./... && go vet ./... && go vet -tags previoushead ./internal/store && go build ./...
# PASS; previous-head 8.293s, full uncached store 6.229s, final race store 32.817s; every package, vet target, and build target passed

go vet ./... && go vet -tags previoushead ./internal/store && go build ./...
# PASS
```

Final repository-root validation command:

```bash
set -e
if rg -n 'LogEvent' coordinator/cmd/duet-coordinator/onboarding.go coordinator/internal/store/security_audit.go; then echo 'forbidden best-effort audit sink found'; exit 1; fi
audit_schema=$(sed -n '121,176p' coordinator/internal/store/identity_schema.go)
if print -r -- "$audit_schema" | rg -n 'node_token|control_token|recovery_secret|code_hash|invite_code|link_code|telegram_user_id|installation_attempt_id|raw_ip'; then echo 'forbidden plaintext audit field found'; exit 1; fi
files=(coordinator/internal/store/identity.go coordinator/internal/store/identity_schema.go coordinator/internal/store/onboarding.go coordinator/internal/store/security_audit.go coordinator/internal/store/identity_migration_rework_test.go coordinator/internal/store/onboarding_rework_r2_test.go coordinator/cmd/duet-coordinator/onboarding.go coordinator/cmd/duet-coordinator/onboarding_rework_r2_test.go)
unformatted=$(gofmt -l "${files[@]}")
[[ -z "$unformatted" ]]
git diff --check
for f in "${files[@]}"; do out=$(git diff --no-index --check /dev/null "$f" 2>&1 || true); [[ -z "$out" ]]; done
expected=(
'583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go'
'a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go'
'efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go'
'3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go'
)
actual=("${(@f)$(shasum -a 256 coordinator/internal/store/identity_telegram.go coordinator/internal/store/identity_telegram_test.go coordinator/internal/store/identity_telegram_previous_head_test.go coordinator/cmd/duet-coordinator/telegram_identity_test.go)}")
[[ "${(j:\n:)actual}" == "${(j:\n:)expected}" ]]
echo 'secret-field, durable-sink, formatting, whitespace, and Telegram boundary checks passed'
```

Result: **PASS** — secret-field, durable-sink, formatting, whitespace, and
Telegram boundary checks passed.

`task-board validate` is recorded on the attached board resource after resource
attachment, before the status handoff.

## Migration / rollback notes

- Both new tables are additive; `audit_events` is not rebuilt. The rotation
  detail table references its base event; the rate-limit security log remains
  durable and nullable-scoped without fabricated foreign-key sentinels.
- The previous binary ignores the new tables. Exact pinned previous-head
  authority/projection round trips and current re-reconciliation pass.
- Startup quarantine is intentionally not a silent authority repair. Ordinary
  reconciliation rolls back, the violation is re-read in a separate immediate
  transaction, and the process refuses serving even if quarantine persistence
  itself fails.
- Phase 1 limiter durability is single-process only; no distributed durability
  is claimed.

## Dirty-worktree and external boundaries

- The worktree was already substantially dirty with accepted identity,
  onboarding, Telegram, coordinator, documentation, CI, and Windows work. No
  unrelated change was reset, committed, pushed, or reformatted.
- R2 production edits are confined to the shared identity/onboarding files and
  the new typed audit repository listed in the hash inventory. R2 tests are
  confined to the two `onboarding_rework_r2_test.go` files plus the additive
  schema table-existence expectation in `identity_migration_rework_test.go`.
- `identity_telegram.go` and all named sibling Telegram-consume tests are an
  external changing boundary and were byte-for-byte untouched. Their acceptance
  is not claimed here. The combined current suite nevertheless passes.
- No external CI or distributed limiter was exercised or claimed. Local tests
  use the production SQLite repository, HTTP handlers, independent database
  connections, real production startup/reopen path, and exact pinned previous
  binary where specified.

Fresh independent security/protocol/migration review and root line-by-line,
hash, and test audit remain required before downstream acceptance.
