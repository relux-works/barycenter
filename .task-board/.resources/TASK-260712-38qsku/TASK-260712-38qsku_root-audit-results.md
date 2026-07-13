# TASK-260712-38qsku — root audit and acceptance results

Date: 2026-07-14 (Asia/Tbilisi)
Task: authorization, migration, compatibility, and rollback verification
Base: `c4951968ee5e5dc40a985bac3e8684befd019343`

## Disposition

Accepted as an automated auth/migration/rollback checkpoint. The task closes
the code and reproducibility gaps found during the audit: physical SQLite
artifact leakage, exact-old config bootstrap compatibility, a safe callable
rollback projection, and environment-only deployment wiring. No production
downgrade, real user credential, or native hardware action was performed.

## Findings closed on the task branch

| Severity | Finding | Resolution and regression |
|---|---|---|
| High | `ProjectIdentityForLegacyRollback` had no operator-callable production path, so the documented fail-closed downgrade could not be reproduced without ad-hoc code/SQL. | Added `duet-coordinator --project-identity-rollback`: one-shot, atomic/idempotent, feature-off store open, no HTTP service. Tests prove repeated execution and the exact legacy token/`PairSlot`/`AddMember` barriers. |
| High | The pinned predecessor uses `yaml.Decoder.KnownFields(true)` and rejects the current `self_service_onboarding:` YAML field. A database-only rollback rehearsal could pass while the old binary failed to boot. | Added a strict pre-mutation decode against the exact predecessor YAML shape, including YAML-merge coverage; added an injected test executed inside the exact old source tree. Rollout templates now keep the flag environment-only. |
| Medium | Hash-only column checks did not prove plaintext was absent from physical SQLite sidecars/residual artifacts. | Added a production-Store lifecycle test covering create, invite consume, Telegram link consume, recovery rotation and recovery consume, then scanning DB/WAL/SHM/journal bytes before and after close for every returned plaintext credential. |
| Medium | Compose and native systemd deployment paths did not expose the environment-only flag consistently. | Wired the flag through Compose/Coolify and a mode-`0600` systemd environment file without adding the incompatible YAML field. Added shell/YAML/template validation. |
| Medium | A surface-only YAML guard could miss a current-only field supplied through a merge key. | Replaced it during cold review with strict decoding into the full pinned predecessor schema; direct and merged forms are rejected before the database is opened. |

## Acceptance mapping

| Acceptance criterion | Evidence |
|---|---|
| Node tokens cannot administer onboarding, invites, recovery, Telegram links, or future upload administration. | `TestCapabilityMiddlewareRejectsNodeAcrossAdministrationSurfaces`, full role matrix, and the production `withControl` middleware used by the synthetic upload-admin surface. |
| Recovery, invite/link, control and node plaintext never reaches database, logs, URLs, or client artifacts. | New physical SQLite artifact test; existing hash-only schema/lookup tests; coordinator log and no-secret-URL tests; macOS and Windows protected-storage/redaction/redirect suites. |
| Brute-force, replay, and concurrent one-time semantics are exact. | Durable rate-limit audit suites; device invite replay and concurrent single-winner tests; recovery idempotent same-tuple and serialization tests; Telegram expiry/replay/concurrent consume/LRU-bound limiter tests. |
| Existing roles, slots, and pair tokens survive additive migration. | Representative legacy migration, Telegram role/slot context preservation, legacy bootstrap, feature-off compatibility, and full current regression suites. |
| Previous coordinator tolerates current rows with the flag off. | Exact pinned source extraction runs its real Store APIs through one and two projection generations, then current reconciliation; the new exact-old config bootstrap test covers process startup. |
| Rollout and rollback are reproducible. | Production one-shot projection command, strict config preflight, deployment wiring, CI exact-old gate, and attached operator runbook with SQL/manual gates and emergency containment. |
| Client pairing and control-only recovery compatibility are preserved. | macOS targeted 68-test credential/recovery matrix and full 125-test suite; Windows protected repository, legacy migration, control-only recovery and privacy focus x20 plus full/race suites. |

## Final verification matrix

All commands below passed on the final production/test logic.

| Command | Result |
|---|---|
| Coordinator focused auth/migration/rollback suites, `-count=20` | PASS: command package 3.673s; store package 15.496s |
| Exact predecessor tagged matrix (`AuthorityRoundTrip`, two generations, config bootstrap) | PASS: 7.779s on final pass |
| `cd coordinator && go vet ./... && go test -count=1 ./...` | PASS; store 6.324s on final uncached pass |
| `cd coordinator && go test -race -count=1 ./...` | PASS; store 33.454s on final pass |
| `cd coordinator && go build ./... && go mod verify` | PASS; all modules verified |
| Config predecessor guard x50 and rollback command x20 | PASS |
| macOS targeted credential/recovery matrix | PASS: 68 tests / 6 suites |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` | PASS: 125 tests / 22 suites |
| Windows compatibility/privacy focus, `-count=20` | PASS |
| `cd pulsar-win && go vet ./... && go test -count=1 ./...` | PASS: all four packages |
| `cd pulsar-win && go test -race -count=1 ./...` | PASS: all four packages |
| `cd pulsar-win && go build ./... && go mod verify` | PASS; all modules verified |
| `bash -n`, YAML/Compose parsing, env/service contract grep, `git diff --check` | PASS |

The default Command Line Tools Swift driver on this host lacks the packaged
`Testing` module despite reporting Swift 6.2.3. The accepted macOS commands use
the installed Xcode toolchain explicitly; the first CLT-only attempt was a
toolchain selection failure, not a test failure.

## Final changed-file SHA-256 inventory

| SHA-256 | File |
|---|---|
| `b225a564f15661af3055912166a00a642f839cde1c24d2f2ea654aedfe2df58a` | `.github/workflows/ci.yml` |
| `913e2c27591bd7b9915d6908e84fd5c03287d294148909ad91df5e2780bfd343` | `coordinator/cmd/duet-coordinator/main.go` |
| `7208f540407ab2082d85f03cc8968ae4a9895d235aec1235e213e8ae159c52c9` | `coordinator/cmd/duet-coordinator/identity_rollback_command_test.go` |
| `fde46a1efb4663acc9637a5a6be5c2a1b8ba5d2c3aa2577735df3c7b1d18e2ce` | `coordinator/internal/config/config.go` |
| `7204ca65a337c77d69ebfc0e02684742fbd40289a4485e36eb66210ef1dfceaf` | `coordinator/internal/config/config_env_test.go` |
| `dc9b9001e1555c2d8dfd90eb57f4428bf2b52303056fda1bf6f86ed903972956` | `coordinator/internal/store/auth_migration_acceptance_test.go` |
| `711d6008976af36b1c08f81661de0dccda31e0a5fbc5fced77ab8f6e4dc1f643` | `coordinator/internal/store/identity_previous_head_integration_test.go` |
| `c2812156bc7f64b49fabee16ff4836f0b5b2744167ac0f70dbe64f47e774a0d7` | `coordinator/internal/store/testdata/previous_head_config_test.go` |
| `64f2ec9bc9f4251785b13605776d530d2161491b58133672c0bdea43a6781e27` | `deploy/coolify.env.example` |
| `3f85e681daadaf98acb7a7ec074da2e2358d9adb7d40ef6b821fdf36a4b42cdf` | `deploy/coordinator.container.yml` |
| `46810122d755d018af2cab81ae4f8b6ea076edd6fcbfb3b0178035f8f10ec4c0` | `deploy/coordinator.env.example` |
| `ca44ce47a2894b2689ad0a4914df33776de8f3de10fd76db81cfc91060224a94` | `deploy/coordinator.yml.template` |
| `b854689972e61091db5008ad6351ed62d58b6a2797b63c6294848844b9ba5877` | `deploy/duet-coordinator.service` |
| `a64cf8994b9b9dbb52fe69e54a2d5e256d842de23c75913a449d66fbfa90ca56` | `deploy/install-coordinator.sh` |
| `50035998629463dee4db41069ea88a7f2fcc27aa2222394ede30a64a83d788e2` | `docker-compose.yml` |

## CI and changed behavior

CI now executes all three exact-predecessor gates with full Git history. The
normal current config continues to support YAML and env precedence, but the
rollback command deliberately accepts only the predecessor-neutral YAML source.
The feature flag is present in Compose/Coolify and native systemd env examples;
it remains absent from both coordinator YAML templates.

## Rollback semantics retained

The projection journals original quotas, sets disabled-orbit legacy quotas to
zero, revokes legacy slots, and burns legacy/new invite surfaces before the old
binary starts. Re-enable restores quota journals but never revives projected
slots; explicit re-pair/re-provision remains mandatory. Emergency rollback
without projection keeps ingress closed and tenants offline.

## Evidence boundary

- No live production database, ingress, Litestream repository, or previous
  production coordinator was changed.
- No real Keychain, DPAPI, NTFS, HWND clipboard, signed MSIX, or Windows 10/11
  hardware claim is made here. Native Windows/package evidence remains in the
  later strict-sequence tasks.
- Review and remediation were performed by the same strict-sequential executor
  because the user required self-execution outside the task-board workflow.
  This is a cold second pass and root audit, not an independent-party security
  attestation.
