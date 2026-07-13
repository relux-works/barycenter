# TASK-260712-1bpog0 — rework round 2 producer outcome

Date: 2026-07-13
Role: developer
Scope: root R8.1/R8.2 exact-old two-generation rollback composition correction

## Outcome

Added a deterministic `previoushead`-tagged integration that executes two full
feature-on → real rollback projection → exact pinned previous coordinator Store
mutation → feature-on reconciliation generations against the same SQLite
database. The pinned revision remains the full commit hash
`e8bd240664a40b9cc78b974f3c34ad30712e2aa5`; missing history fails the test,
there is no skip path, and each nested previous-source test has a two-minute
context deadline.

This R2 correction changes tests, CI selection, and the logbook only. It does
not alter production behavior or replace the R1–R7 implementation.

## Exact R2 changed files and SHA-256

- `.github/workflows/ci.yml`
  - `da3fbc287e7eecd661fdf1b47ac790dcf74cacc39acc5d7ced6c06cb59e519c8`
  - CI now explicitly runs both exact-previous integrations, including the new
    two-generation composition gate.
- `LOGBOOK.md`
  - `a80e3775b7df65ba3f2d7f2fb32f566967cb6df673a15a060a5f875940de0f88`
  - Records the R8.1 composition evidence and quota-generation decision.
- `coordinator/internal/store/identity_previous_head_integration_test.go`
  - `15e82f8b48ca805d2d3a06152c0f91f7a776c8d85d0f0e8b0e6dd91ec7cc7b5f`
  - Adds `TestR8ExactPreviousHEADTwoGenerationProjectionComposition`, shared
    pinned-source extraction, per-generation setup/assertions, exact-old
    subprocess invocation, reconciliation verification, and one-way projected
    slot checks.
- `coordinator/internal/store/testdata/previous_head_authority_test.go`
  - `51330398cb8634544e90ca8a836ee50921f1bcbc5260f0b63b783c20bfa86d32`
  - Adds the test injected into the exact predecessor. Every old-interval
    authorization check and mutation uses that predecessor's real Store API.

## Two complete cycle map

### Generation 1

1. Current feature-on code creates:
   - an active keep orbit with primary and companion members;
   - provisioned slot A plus a companion-owned slot B;
   - separate explicit-delete and last-member-dissolve fixture orbits;
   - a disabled-orbit fixture with a live slot, pending legacy invite, and
     quotas `5/10`.
2. Current code calls `ProjectIdentityForLegacyRollback`.
3. Before old code starts, the test proves the projection journal is pending
   with original quotas `5/10`, current quotas `0/0`, the slot is revoked, and
   the invite is burned.
4. Exact pinned previous code opens that same DB and proves the disabled orbit
   rejects through its Store API:
   - `LookupToken`;
   - `PairSlot` (`ErrLimit`);
   - `AddMember` (`ErrLimit`);
   - `ConsumeInvite` (zero result).
5. In that same exact-old interval, Store APIs exercise:
   - add member;
   - rename member;
   - transfer primary;
   - leave;
   - revoke slot;
   - same-coordinate slot rebind;
   - new slot allocation;
   - create orbit;
   - explicit `DeleteOrbit`;
   - last-member `LeaveOrbit` dissolution.
6. The old test returns only the runtime-minted rebound/new node tokens needed
   by the current-process assertions; no token is logged or stored in fixtures
   or this outcome.
7. Current feature-on reopen verifies:
   - new primary/original companion/left companion roles;
   - renamed actor and slot ownership;
   - old slot, deleted-orbit, dissolved-orbit, projected, and old control
     credentials are unauthorized;
   - old-minted rebound/new tokens are valid node-only capabilities;
   - stale installation actors are revoked and credentials removed;
   - deleted/dissolved additive children are removed;
   - the old-created orbit reconciles;
   - journal restoration returns `5/10` and marks the generation restored;
   - `foreign_key_check` and `integrity_check` pass.
8. The test sets the projected orbit active and reconciles; its projected slot
   remains revoked, credentialless, and unauthorized, proving status change
   alone cannot reverse the one-way slot projection.

### Generation 2

1. On the same database and same formerly projected orbit, current code changes
   quotas to `3/7`, explicitly re-pairs the revoked slot, issues a fresh invite,
   creates fresh active mutation fixtures, and disables the orbit again.
2. Current code calls the real rollback projection again.
3. The restored generation-one journal row is retired; the new pending journal
   captures `3/7` rather than stale generation-one `5/10`, while current quotas
   become `0/0`, the new slot is revoked, and the new invite is burned.
4. Exact pinned previous code repeats all four disabled-orbit denial checks and
   the entire legacy mutation surface listed for generation one.
5. Current feature-on reopen repeats the full role, ownership, lifecycle,
   token, node-only capability, delete/dissolve, journal, FK, and integrity
   assertions.
6. The second restored journal contains `3/7` and `restored_at`, proving the
   new generation captured/restored changed quotas rather than reusing the
   generation-one values.
7. A final status re-enable again proves the generation-two projected slot
   remains revoked and requires explicit trusted repair.

Raw SQL is used only by current test preparation/assertion for orbit status and
quota values, as permitted by the guard. It never substitutes for any mutation
or enforcement operation during either exact-old interval.

## Acceptance criteria and checklist mapping

- Existing databases / roles / slot ownership / live token validity:
  both exact-old cycles reconcile member roles, member leave state, slot
  ownership, old-token invalidation, and old-minted live-token validity. The
  full existing migration/backfill suite also passes unchanged.
- Hash-only server persistence:
  no production persistence changed. Runtime tokens are generated only in test
  temp state, passed to the bounded predecessor subprocess, and never logged,
  fixture-literalized, or included in evidence. Existing hash-only tests pass
  in the full suite.
- ActorContext capability distinction:
  both generations assert old-minted tokens resolve with exactly
  `CapabilityNode`, while revoked control and stale node authorities fail.
- Rollback tolerance / feature-off compatibility:
  the exact pinned previous implementation opens, enforces, and mutates the
  projected database twice, after which current feature-on startup reconciles
  and passes FK/integrity gates each time.
- Additive migration/backfill, legacy pair validity, feature gating, R1–R7
  security/migration/concurrency corrections:
  no production code was changed in R2; the complete uncached suite, store race
  suite, vet, tagged vet, and build all pass and retain their focused coverage.
- Architecture fit:
  the new proof reuses the existing projection API, exact pinned-source harness,
  Store API, reconciliation path, and test helpers. No alternate rollback
  emulator or test-only production branch was added.
- Evidence / logbook:
  this is a new task-scoped outcome resource; `LOGBOOK.md` records the R8.1
  composition decision.

## Commands and unabridged pass/fail summary

### Initial compile attempt (failed, corrected)

Command executed from `coordinator/`:

```text
gofmt -w coordinator/internal/store/identity_previous_head_integration_test.go coordinator/internal/store/testdata/previous_head_authority_test.go
go test -tags previoushead -count=1 ./internal/store -run '^TestR8ExactPreviousHEADTwoGenerationProjectionComposition$' -v
```

Exact result:

```text
lstat coordinator/internal/store/identity_previous_head_integration_test.go: no such file or directory
lstat coordinator/internal/store/testdata/previous_head_authority_test.go: no such file or directory
# relux.works/duet/coordinator/internal/store [relux.works/duet/coordinator/internal/store.test]
internal/store/identity_previous_head_integration_test.go:269:2: non-name fixture.oldANode on left side of :=
internal/store/identity_previous_head_integration_test.go:294:2: non-name fixture.deleteNode on left side of :=
internal/store/identity_previous_head_integration_test.go:302:2: non-name fixture.dissolveNode on left side of :=
internal/store/identity_previous_head_integration_test.go:322:2: non-name fixture.disabledNode on left side of :=
FAIL relux.works/duet/coordinator/internal/store [build failed]
FAIL
```

The field assignments were corrected to use predeclared contexts, and gofmt was
rerun with paths relative to `coordinator/`. No runtime test executed in the
failed attempt.

### Corrected focused tagged gate

```text
go test -tags previoushead -count=1 ./internal/store -run '^TestR8ExactPreviousHEADTwoGenerationProjectionComposition$' -v
```

```text
=== RUN   TestR8ExactPreviousHEADTwoGenerationProjectionComposition
--- PASS: TestR8ExactPreviousHEADTwoGenerationProjectionComposition (3.04s)
PASS
ok relux.works/duet/coordinator/internal/store 3.458s
```

### Repeated exact-old composition

```text
go test -tags previoushead -count=3 ./internal/store -run '^TestR8ExactPreviousHEADTwoGenerationProjectionComposition$'
```

```text
ok relux.works/duet/coordinator/internal/store 9.686s
```

### CI-shaped exact-old selection

```text
go test -tags previoushead -count=1 ./internal/store -run '^TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition)$'
```

```text
ok relux.works/duet/coordinator/internal/store 5.675s
```

### Full uncached coordinator suite

```text
go test -count=1 ./...
```

```text
ok relux.works/duet/coordinator/cmd/duet-coordinator 1.539s
ok relux.works/duet/coordinator/internal/bot 0.451s
ok relux.works/duet/coordinator/internal/config 1.893s
ok relux.works/duet/coordinator/internal/hub 1.216s
ok relux.works/duet/coordinator/internal/links 1.539s
ok relux.works/duet/coordinator/internal/media 2.156s
ok relux.works/duet/coordinator/internal/protocol 2.065s
ok relux.works/duet/coordinator/internal/resolver 1.669s
ok relux.works/duet/coordinator/internal/session 1.703s
? relux.works/duet/coordinator/internal/spotify [no test files]
ok relux.works/duet/coordinator/internal/store 4.085s
? relux.works/duet/coordinator/internal/ulid [no test files]
```

### Store race suite

```text
go test -race -count=1 ./internal/store
```

```text
ok relux.works/duet/coordinator/internal/store 13.073s
```

### Vet, tagged vet, and build

```text
go vet ./...
go vet -tags previoushead ./internal/store
go build ./...
```

All three exited `0` with no output.

### Formatting, diff, and board validation

```text
gofmt -l coordinator/internal/store/identity_previous_head_integration_test.go coordinator/internal/store/testdata/previous_head_authority_test.go
git diff --check
task-board validate
```

`gofmt -l` and `git diff --check` exited `0` with no output.

```text
Board is valid. No issues found.
```

### File hashes

```text
da3fbc287e7eecd661fdf1b47ac790dcf74cacc39acc5d7ced6c06cb59e519c8  .github/workflows/ci.yml
a80e3775b7df65ba3f2d7f2fb32f566967cb6df673a15a060a5f875940de0f88  LOGBOOK.md
15e82f8b48ca805d2d3a06152c0f91f7a776c8d85d0f0e8b0e6dd91ec7cc7b5f  coordinator/internal/store/identity_previous_head_integration_test.go
51330398cb8634544e90ca8a836ee50921f1bcbc5260f0b63b783c20bfa86d32  coordinator/internal/store/testdata/previous_head_authority_test.go
```

## Git status scope

The worktree was already intentionally dirty and remains uncommitted. No
commit, reset, checkout, or push was performed. R2 edits are limited to the four
hashed files above. Full status at handoff:

```text
## main...origin/main
 M .github/workflows/ci.yml
 M LOGBOOK.md
 M coordinator/cmd/duet-coordinator/main.go
 M coordinator/internal/config/config.go
 M coordinator/internal/config/config_env_test.go
 M coordinator/internal/store/orbits.go
 M coordinator/internal/store/store.go
 M docs/idea-air-rooms.md
 M docs/spec.md
 M pulsar-win/.gitignore
?? .planning/
?? .research/
?? .spec/
?? .task-board/
?? coordinator/cmd/duet-coordinator/identity_auth_test.go
?? coordinator/internal/store/identity.go
?? coordinator/internal/store/identity_migration_rework_test.go
?? coordinator/internal/store/identity_previous_head_integration_test.go
?? coordinator/internal/store/identity_rollback_rework_test.go
?? coordinator/internal/store/identity_schema.go
?? coordinator/internal/store/identity_security_rework_test.go
?? coordinator/internal/store/identity_test.go
?? coordinator/internal/store/orbits_concurrency_rework_test.go
?? coordinator/internal/store/testdata/
?? diagrams/
?? docs/analysis/
?? docs/diagrams/
?? docs/goal-self-contained-audio.md
?? docs/plans/
?? docs/spec-self-contained-audio.md
?? pulsar-win/cmd/
?? pulsar-win/internal/
?? pulsar-win/native/
?? pulsar-win/probe-msix/
?? task-board.config.json
```

## Residual risks and external boundaries

- The exact pinned previous source was executed locally through its real Store
  API, twice, with full Git history available. An external GitHub Actions run
  was not executed in this developer session.
- SQLite transaction/crash barriers remain deterministic fault-injection tests;
  no real OS power-loss or forced process-kill experiment was run here.
- This producer outcome is not acceptance. A fresh independent reviewer and a
  new root line-by-line/hash audit remain mandatory.

