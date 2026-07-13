# TASK-260712-m5264f — root acceptance after R2

Date: 2026-07-13
Reviewer: root orchestrator
Verdict: **ACCEPTED**

The reviewer role briefly auto-transitioned the task to `done`; root restored it
to `to-review` and did not use that transition as acceptance. This verdict is
based on root's own line-by-line code/test audit, exact hash verification and
independent reruns, plus the separate read-only R2 review.

## Accepted corrections

- Recovery rotation captures the prior handle before mutation and writes the
  `recovery.rotated` base row plus exact nullable-old/required-new detail in the
  same `_txlock=immediate` transaction. Collision retry returns one committed
  generation; update, base/detail insert and commit remain atomic; no returned
  secret or credential digest is written to either audit table.
- Every one of the seven HTTP limiter classes reserves first and writes a typed
  `security.rate_limited` record before 429. The subject is class-domain-
  separated SHA-256; pre-identity scope is NULL/NULL; actor classes require a
  real actor-orbit membership. Audit failure returns the ordinary 500 envelope,
  no `Retry-After`, and leaves the reserved attempt consumed.
- App-first reconciliation now checks exactly one active membership in the
  credential orbit and repeats that invariant in the final serving gate. A
  typed mismatch rolls ordinary reconciliation back, re-verifies in a separate
  immediate transaction, atomically revokes/audits the actor, and refuses to
  return a Store. Quarantine failure still refuses serving and rolls its own
  revocation/audit pair back.

The rate-limit table deliberately has no orbit/actor foreign keys. Root accepts
this because audit IDs are historical and must survive later orbit deletion;
the only production writer validates the live real membership in its single
`INSERT ... SELECT WHERE EXISTS` statement, while the DDL independently locks
event/class/digest and scoped-versus-unscoped shape. Direct arbitrary SQLite
writes are not an application production path.

## Exact accepted SHA-256

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

Producer outcome SHA-256:
`41acf63c05e3adb73849860791ead3a13b14f500abf521c962fd7321e3ef61a8`.
Independent R2 review SHA-256:
`3ae3530e6dcf10c5a27bfa6b7abfbf1003289c1cb30c509055c3ed15bc0ef712`.

The sibling Telegram consume boundary remained exact:

```text
583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go
```

Telegram consume itself is not accepted here; its remaining best-effort 429
sink is the separately guarded R4 follow-up.

## Root verification

- R2 store and HTTP tests under the race detector, 10 repetitions each: PASS.
- Complete coordinator test suite uncached: PASS.
- Complete store race suite: PASS (`30.265s`); all other coordinator race
  packages also passed in the root run.
- Exact previous-HEAD authority/two-generation/Telegram reconciliation gate:
  PASS.
- `go vet ./...`, tagged previous-HEAD vet and `go build ./...`: PASS.
- Read-only gofmt, `git diff --check`, durable-sink/forbidden-field scans,
  exact hashes and `task-board validate`: PASS.
- Independent reviewer repeated focused/race/full/previous-HEAD/static gates on
  the same hashes and reported no Critical, High, Medium or Low finding.

No distributed limiter durability, external CI, or live production rollout is
claimed. The accepted Phase 1 limiter remains process-local by frozen contract.
No product code was changed during root acceptance.
