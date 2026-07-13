# TASK-260712-1bpog0 independent review — post-R2

Date: 2026-07-13  
Role: independent reviewer  
Verdict: **APPROVE / DONE FOR REVIEWER ROLE**

No substantive finding remains. The implementation matches the task acceptance
criteria, fits the existing legacy-authoritative store architecture, and passes
the independently reproduced verification matrix. This reviewer approval does
not replace the separate root line-by-line/hash audit required by the review
guard.

## Scope reviewed

Read in full:

- the complete task card, identity model, implementation guard, review guard,
  root R1–R8 guard, and root R2 composition guard;
- `docs/spec-self-contained-audio.md` sections 3 (including invariant 13), 6,
  11, 12, 18, and 19 P1.2;
- the complete accepted Rev15 contract in
  `.task-board/.resources/TASK-260712-3v1k7q/research.md` and the root amendments;
- both producer outcomes and all earlier independent/root review outcomes;
- every task-scoped production, configuration, workflow, test, and exact-old
  driver file listed by the producer, in full.

The PlantUML identity model remains a focused structural class model and the
implementation follows it without introducing a conflicting service or
deployment boundary.

## Acceptance and guard mapping

1. **Additive migration/backfill:** identity DDL is installed transactionally;
   status repair preserves SQLite schema objects and AUTOINCREMENT high water;
   narrowly authorized stale-child cleanup handles old-binary dissolution while
   unrelated FK corruption fails closed. Representative and multi-orbit legacy
   fixtures preserve roles, `paired_by`, quotas, slot generations, revoked rows,
   and live legacy node tokens.
2. **Hash-only persistence and lookup:** control, recovery, device invite, and
   Telegram link material is represented by constrained canonical SHA-256
   digests; plaintext columns are absent. Resolver and persistence paths do not
   log or persist secret plaintext.
3. **Capability and tenant separation:** token resolution counts both credential
   domains first and rejects zero, duplicate, or cross-domain matches. Node
   credentials remain playback-only; control and Telegram paths revalidate
   actor, membership, role, orbit, live slot binding, and revocation state.
   Provisioning and node minting serialize under `BEGIN IMMEDIATE` and reject
   domain conflicts. The HTTP regression proves a node token cannot read media
   from another orbit.
4. **Lifecycle and concurrency:** provisioning is a one-time generation
   transition; revoked, left, stale, satellite, and already-provisioned targets
   fail closed. Barrier-driven two-store tests cover leave/leave,
   leave/transfer, last-leave/add-member, seed bootstrap, and concurrent initial
   provisioning. Reconciliation is idempotent under concurrent opens.
5. **Feature gate and compatibility:** the resolver and identity dual-writes are
   gated by `self_service_onboarding`; unset/invalid/non-`1` environment values,
   YAML precedence, and YAML preservation are covered. Feature-off behavior
   remains legacy-authoritative and tolerates additive rows.
6. **Rollback:** the new tagged integration pins exact revision
   `e8bd240664a40b9cc78b974f3c34ad30712e2aa5` and performs two complete
   `new-on -> projection -> exact-old Store API mutation -> re-enable` cycles on
   the same database. Each cycle exercises disabled-orbit rejection plus legacy
   add/name/transfer/leave/pair/revoke/rebind/new-slot/create/delete/dissolve
   authority, verifies old-minted token behavior and one-way projected slots,
   restores the correct generation-specific quotas, and passes
   `foreign_key_check` and `integrity_check`.
7. **Migration cleanup and schema atomicity:** deterministic faults cover SQL,
   FK validation, behavior probe, pre-commit, rollback, pragma restore/read,
   joined-error, and panic cleanup paths. Late identity DDL failure rolls back
   the additive install and feature-off reopen recovers idempotently.

## Independent verification

All commands were run from `coordinator/` unless otherwise stated.

### Full uncached suite

Command: `go test -count=1 ./...`

Result: PASS (exit 0)

```text
ok  relux.works/duet/coordinator/cmd/duet-coordinator 1.304s
ok  relux.works/duet/coordinator/internal/bot 1.340s
ok  relux.works/duet/coordinator/internal/config 1.889s
ok  relux.works/duet/coordinator/internal/hub 0.968s
ok  relux.works/duet/coordinator/internal/links 2.264s
ok  relux.works/duet/coordinator/internal/media 2.543s
ok  relux.works/duet/coordinator/internal/protocol 1.959s
ok  relux.works/duet/coordinator/internal/resolver 2.008s
ok  relux.works/duet/coordinator/internal/session 2.281s
?   relux.works/duet/coordinator/internal/spotify [no test files]
ok  relux.works/duet/coordinator/internal/store 4.271s
?   relux.works/duet/coordinator/internal/ulid [no test files]
```

### Exact pinned previous implementation

Command: `go test -count=1 -tags previoushead ./internal/store -run '^TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition)$' -v`

Result: PASS (exit 0)

```text
=== RUN   TestR8ExactPreviousHEADAuthorityRoundTrip
--- PASS: TestR8ExactPreviousHEADAuthorityRoundTrip (4.38s)
=== RUN   TestR8ExactPreviousHEADTwoGenerationProjectionComposition
--- PASS: TestR8ExactPreviousHEADTwoGenerationProjectionComposition (3.08s)
PASS
ok  relux.works/duet/coordinator/internal/store 8.052s
```

### Repeated security/migration/concurrency matrix

Command: `go test -count=10 ./internal/store -run '^TestR[1-8]'`

Result: PASS (exit 0):
`ok relux.works/duet/coordinator/internal/store 13.075s`

Command: `go test -count=10 ./cmd/duet-coordinator -run '^TestNodeTokenCannotEscalateAcrossOrbitMediaAuthorization$'`

Result: PASS (exit 0):
`ok relux.works/duet/coordinator/cmd/duet-coordinator 0.659s`

Command: `go test -count=10 ./internal/config -run '^TestSelfServiceOnboarding'`

Result: PASS (exit 0):
`ok relux.works/duet/coordinator/internal/config 0.438s`

### Race, static analysis, and build

- `go test -race -count=1 ./internal/store` — PASS, exit 0,
  `ok relux.works/duet/coordinator/internal/store 14.682s`.
- `go vet ./...` — PASS, exit 0, no output.
- `go vet -tags previoushead ./internal/store` — PASS, exit 0, no output.
- `go build ./...` — PASS, exit 0, no output.

### Formatting, diff, and board

- `gofmt -l` over all task-scoped Go files — PASS, exit 0, no output.
- `git diff --check` over the task-scoped workflow/logbook/coordinator paths —
  PASS, exit 0, no output.
- `task-board validate` — PASS, exit 0: `Board is valid. No issues found.`

## Hash audit

Current task-scoped code/workflow/test hashes exactly match the producer's R2
outcome:

```text
da3fbc287e7eecd661fdf1b47ac790dcf74cacc39acc5d7ced6c06cb59e519c8  .github/workflows/ci.yml
d3918d93ca775d6f894c019f1f1c4ac65ee528fc24ceb1e913cb64660a33848a  coordinator/cmd/duet-coordinator/main.go
95c105ab5f5fde24bda7de04468959ea3e957fa86c6b0a1330d7631f0291d58a  coordinator/cmd/duet-coordinator/identity_auth_test.go
8f31e69d863d79dccb4a44f9dcf045f01f03cfe4341f32e082189184162da824  coordinator/internal/config/config.go
bc92967bcba0ab122fe81b4c82b370dced27e0a35add11bb15e35bbea5c395e4  coordinator/internal/config/config_env_test.go
ffefbf2510ce5d8d18c4aa39dbe30baaf75c5d3acbeac62359e944621ca6b34a  coordinator/internal/store/store.go
63bd1e1717fd4c964aff470ab3af05932aa66e0b8543372bc9c1a2aa25cc8450  coordinator/internal/store/orbits.go
128d3f4379a184733b620fcb062291817e456efa9210ce80f428869ec230ae2c  coordinator/internal/store/identity.go
b6bf8fd25d273473522424ed507f50d488ca6ba5856d29ef331f794e0a4269e6  coordinator/internal/store/identity_schema.go
74e7a16c543eb118ceabe52af5ccbd556326785fdfad1ed233d12efd2e9d7547  coordinator/internal/store/identity_test.go
e4d5e0c790a20981d63d2fd9dff890e34832db4387c9b6aa998b84b7c28b3d27  coordinator/internal/store/identity_security_rework_test.go
ec910b369798981a6906c8ba1385cdcbb03769f2da2964ca9891c7bafe33544f  coordinator/internal/store/identity_migration_rework_test.go
5c7cdadfa6bb8d91f654badb838567857912de45f5b2a16095abe8030fbaaf27  coordinator/internal/store/identity_rollback_rework_test.go
15e82f8b48ca805d2d3a06152c0f91f7a776c8d85d0f0e8b0e6dd91ec7cc7b5f  coordinator/internal/store/identity_previous_head_integration_test.go
5098a15a1aa00e0ed4dd34cd3b6964bb07bb41eb22062b515c187b37930e1ce2  coordinator/internal/store/orbits_concurrency_rework_test.go
51330398cb8634544e90ca8a836ee50921f1bcbc5260f0b63b783c20bfa86d32  coordinator/internal/store/testdata/previous_head_authority_test.go
```

`LOGBOOK.md` has later unrelated task additions and therefore no longer matches
the producer's R2 whole-file hash; the identity entries are intact. No
task-scoped code changed during this review.

## Residual external boundaries

- The exact previous implementation is materialized from pinned repository
  source and exercised through its real Store API; this review did not execute a
  separately archived production binary artifact.
- Deterministic transactional fault barriers were executed; physical disk-full,
  power-loss, and OS process-kill faults were not injected.
- The worktree contains substantial pre-existing unrelated changes. They were
  preserved and excluded from this verdict.

These are declared external evidence boundaries, not substantive findings
against this task's acceptance criteria.
