# TASK-260712-m5264f — mandatory independent R2 review

Date: 2026-07-13
Role: read-only security / protocol / migration reviewer

Do not accept the producer outcome as evidence by assertion. Do not edit
production code, tests, shared files, documentation, or the producer outcome;
do not commit, push, reset, or clean the dirty worktree. Review the exact live
files and compute hashes yourself.

Read the frozen Rev15 contract, root amendments, original implementation guard,
root R2 rework guard, corrected R1 review, and the R2 producer outcome. Audit
the three R2 corrections line by line:

1. Recovery rotation must retain exact old/new non-secret `recovery_id` handles
   (NULL old generation where valid) in an atomic transaction with the
   credential update and base audit. Challenge linkage/type integrity,
   collision behavior, rollback/failure injection, migration compatibility,
   and all secret/digest redaction claims.
2. Every one of the seven HTTP 429 paths must reserve first and durably record
   a typed `security.rate_limited` row before emitting 429. Audit class/domain
   separation, raw-subject hygiene, nullable versus scoped shape, rejection of
   sentinel/fabricated/mismatched actor-orbit coordinates, error propagation,
   500/no-Retry behavior, attempt consumption, and whether schema/repository
   constraints remain valid after historical orbit deletion. Explicitly decide
   whether repository-time membership validation is sufficient for historical
   audit IDs or whether the schema admits an unsafe production path.
3. App-first credential/membership mismatch must roll back reconciliation,
   re-verify and atomically quarantine the actor with audit, return a typed
   fatal error, and never return a serving Store. Audit cross-orbit, missing
   membership, quarantine failure, second reopen, explicit repair, role
   preservation, independent serving gate, feature-off/legacy behavior, and
   credential/error redaction.

Re-run focused repetitions and race tests, relevant full tests, exact
previous-head migration/rollback checks, vet, build, formatting/whitespace,
foreign-key health, and board validation. Confirm these Telegram boundary
hashes remain exact and do not review/accept Telegram consume itself:

```text
583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go
```

Attach one canonical report named
`TASK-260712-m5264f_security-review-r2.md` with exact reviewed hashes,
commands/results, findings ordered by severity with file:line evidence, and an
unambiguous verdict. Any real defect returns the task to `to-dev`; a clean
verdict leaves it `to-review` for root acceptance. No code change is authorized.
