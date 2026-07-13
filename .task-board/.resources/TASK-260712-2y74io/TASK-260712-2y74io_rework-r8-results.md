# TASK-260712-2y74io — R8 lifecycle rework results

Date: 2026-07-13  
Role: developer  
Handoff state: ready for independent review and root line-by-line/hash/test audit; signed-Windows gates remain downstream.

## Outcome

R8 F25–F27 and live root directives W1–W4 are implemented on the accepted Rev16 bridge boundary. Stop/result/Release ownership is one exact `(generation, operationID, owner pointer)` transition; invalid successful prepare results fail closed without native ID-zero calls; failed Stop results are not terminal evidence; finalized release retries retain their exact authority; publication losers are waiter-owned orphan obligations; required structural evidence precedes bounded escalation; and an orphan carries a durable unique Stop producer before it becomes visible.

No capability, manifest, AppContainer, package identity, helper ABI, permission model, or production mock/fallback boundary was weakened. No commit or push was made.

## Exact changed-file inventory and SHA-256

The probe tree is untracked, so this inventory and full-file hashes—not `git diff`—define the producer scope.

| File | Current SHA-256 | Scope |
|---|---|---|
| `pulsar-win/cmd/pulsar-win-probe/coordinators.go` | `1114a0a692b981eb46c2bd5508c3ccab050addf0bd4b2fc72fa2fd7832443a5a` | Exact capture owner, Stop producer, Release gate, orphan drain, prepare/result/evidence coordinators |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle.go` | `8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68` | Generation phase admission and duplicate-prepare rejection |
| `pulsar-win/cmd/pulsar-win-probe/main_windows.go` | `36f971e8c7877c0ab80d289d59dba09356faf5bc7551be239365f460bb2e3455` | Waiter integration, exact release authority/retry, orphan cleanup, structural evidence-first escalation |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go` | `e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56` | Prior F15–F24 expectations updated to the valid orphan-release model |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go` | `7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e` | New deterministic F25–F27 and W1–W4 production-seam schedules |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go` | `99a78a3fa1e120c645930fe67ed68d0db376f34c3855e4751afb0a172bcee749` | Production wiring/static ownership assertions |
| `pulsar-win/probe-msix/README.md` | `1919528ea96be20d948b9aced81093bde152a7d1c16f23656650d3f8b2181327` | Exact lifecycle/ownership/evidence semantics and hardware boundary |
| `LOGBOOK.md` | `b2637cb0b773d1c6b937b59e005a5aac0391199322e92aa0e9317f46b3f7419d` | R8 F25–F27/W1–W4 findings in the task block only; this shared file also contains concurrent unrelated entries |

Frozen R3/R4/R5 tests remain byte-identical to the R8 starting boundary:

- `lifecycle_r3_test.go`: `805c56fceee3bc61f7506acd90fe9b11f27a6f4e096edf57246985671d7ce5c9`
- `lifecycle_r4_test.go`: `ca39aa58dfab61dacdf0923e601fe5242995b3d8b6e1c31ecaccf3a3224a0b90`
- `lifecycle_r5_test.go`: `cb2ba971e396567fcc08a78b9c088ccb20a611a276f5783c3dd42586aaed4c23`

`LOGBOOK.md` had already diverged from its frozen R8 hash when this run began because it is shared by concurrent work. No unrelated entry was reverted.

## Ownership and lifecycle invariants

| Boundary | Enforced invariant |
|---|---|
| Start/generation | A lifecycle generation phase-rejects duplicate prepare delivery before a second helper call. Pending rearm/lifecycle/abrupt gates continue to block new work. |
| Exact owner | Native identity is the immutable `(generation, operationID, owner pointer)` tuple. Operation-ID reuse alone never matches old work. |
| Stop | Stop claim precedes callback selection. `pending` always identifies the immediate callback or admitted activation/deferred callback that will publish one exact HRESULT. Terminal-first without an admitted Stop yields stable `not requested`, never fabricated `S_OK`. |
| Release | Admission covers the actual waiter-owned `CaptureRelease` invocation. Release cannot overtake an admitted activation or Stop producer. Only exact `S_OK` marks the same owner released and permits exact-pointer clear; failure retains a retry obligation. |
| Query-failure recovery | Artifact finalization/release requires exact completed Stop `S_OK`. Failed or unexpected Stop results retain native/artifact ownership and enter required-evidence-first bounded failure handling. |
| Finalized retry | Authority is derived only from observed native terminal state or that owner's exact completed Stop `S_OK`. Expected `E_ILLEGAL_METHOD_CALL` after asynchronous Stop remains a waiting retry, not success or terminal evidence. |
| Orphan owner | A successful prepare publication loser retains its exact native owner. Same-generation duplicates are rejected pre-helper; real losers are stopped once, queried to terminal by the waiter, and released through their own gate without touching the incumbent. |
| W4 linearization | `claimStop` atomically publishes `StopClaimed`, stores the callback under `stopMu`, and unlocks before `retainOrphan` publishes under `orphanMu`. A waiter can obtain the obligation only after the producer exists; until `stopResultSet` publishes, drain is `pending`. `invokeClaimedStop` consumes that callback once under `stopMu`. |
| Confirmed shutdown | The wndproc adapter touches only the atomic active owner, requests its exact nonblocking Stop before latch/wake, and never waits on orphan Stop/query. Post-confirmation ordinary query/release/finalize/evidence/cleanup remains suppressed. |
| Structural evidence | The production API orders `requiredEvidence` before `escalate`. Failed/sticky evidence suppresses the successor; confirmed shutdown suppresses both. |
| Invalid helper result | `S_OK` plus operation ID zero is rejected before publication. No Stop/query/Release receives zero; redacted required evidence and bounded fail-closed handling settle the generation without silently occupying the start gate. |

## F25–F27 and W1–W4 mapping

| Requirement | Production mapping | Deterministic executable evidence |
|---|---|---|
| F25 exact Stop/result/Release | `coordinators.go:240-565`, `375-496`, `844-925`; every Windows release call is routed through the owner gate in `main_windows.go:864-1210` | `TestR8F25ReleaseAfterStopClaimBeforeStopLockKeepsProducer`, `TestR8F25ImmediateStopCompletesBeforeTerminalRelease`, `TestR8F25DeferredActivationStopCompletesBeforeRelease`, `TestR8F25TerminalFirstMakesLaterStopStableNotRequested`, `TestR8F25ReleaseFailureRetainsExactOwnerUntilSuccessfulRetry`, `TestR8F25ConfirmedShutdownDoesNotWaitOrAdmitReleaseSuccessors`, `TestR8F25OperationIDReuseCannotReceiveOldDeferredWork` |
| F26 invalid successful prepare | `coordinators.go:968-984,1069-1125`, `lifecycle.go:383-420`, `main_windows.go:1465-1560,2081-2087` | `TestR8F26SuccessfulPrepareWithZeroIDFailsClosed`, `TestR8F26InvalidNonzeroOwnerQueryFailsClosedWithoutCleanupClaims`; native contract already asserts `CapturePrepare(...) == S_OK && capture_id != 0` in `native/pulsar-capture/tests/pulsar_capture_tests.cpp:909` |
| F27 failed Stop is not terminal | `coordinators.go:638-690`, `main_windows.go:928-974` | `TestR8F27FailedOrUnexpectedStopCannotAuthorizeCleanup` covers failure and unexpected non-`S_OK` success, with zero finalize/release/clear/settlement |
| W1 exact finalized retry authority | `coordinators.go:404-424`, `main_windows.go:864-914` | `TestR8W1FinalizedRetryUsesExactRecordedAuthority`: Stop `S_OK` → `E_ILLEGAL_METHOD_CALL` → same owner/ID retry `S_OK`; terminal-authorized retry; failed Stop cannot authorize |
| W2 publication-loser registry cleanup | `lifecycle.go:388-420`, `coordinators.go:702-835,1069-1125`, `main_windows.go:447-478` | `TestR8W2UnpublishedNativeOwnerDrainsOnWaiterWithoutTouchingIncumbent`, `TestR8W2OrphanReleaseFailureRetriesExactOwnerAndID`, `TestR8W2ConfirmedShutdownSuppressesOrphanQueryAndRelease`, `TestR8W2ConfirmationDoesNotWaitForOrphanStopOrQuery`, plus updated R6 duplicate tests |
| W3 evidence before escalation | `coordinators.go:600-634,968-984`, `main_windows.go:447-478,2081-2087` | `TestR8W3RequiredStructuralEvidencePrecedesEscalation` covers zero ID, unexpected success HRESULT, invalid query owner, orphan Stop/query/Release/clear failures, sticky evidence failure, and abrupt suppression |
| W4 orphan visibility race | `coordinators.go:248-286,722-737,1108-1112` | `TestR8W4OrphanVisibilityCarriesLiveStopProducer` proves pre-entry and in-flight drains are pending with zero query/release, then exact `Stop → terminal query → Release`; `TestR8W4ConfirmationAtOrphanPreInvocationSeam` proves confirmation returns before invocation and admits no orphan query/release |

The existing focused R3–R6 schedules continue to cover quit, forced quit/watchdog, suspend, lock, shutdown query/cancel/confirm, permission revoke/query failure, durable cleanup/rearm UI transitions, hotkey and tray teardown, evidence suppression, artifact ownership, repeated cycles, and abrupt partial recovery.

## Verification after the last Go edit

Host: macOS 15.7.4 / Darwin 24.6.0 x86_64; Go 1.26.0 darwin/amd64.

### Exact task schedules

From `pulsar-win` before concurrent module metadata became inconsistent, then repeated against the hash-matching isolated snapshot:

```bash
go test ./cmd/pulsar-win-probe -run 'TestR8|TestR6F1[5-9]|TestR6F2[0-4]|TestR5F1[2-4]|TestR4|TestR3' -count=100
go test -race ./cmd/pulsar-win-probe -run 'TestR8|TestR6F1[5-9]|TestR6F2[0-4]|TestR5F1[2-4]|TestR4|TestR3' -count=50
```

```text
PASS source worktree: probe 3.811s
PASS source worktree race: probe 4.704s
PASS isolated exact-hash repeat: probe 3.762s
PASS isolated exact-hash race repeat: probe 4.005s
```

All new schedules use channels/barriers for order; timeout branches only fail a hung test.

### Relevant host tests, race, and vet

The shared `go.mod`/`go.sum` changed during verification and became inconsistent. To avoid modifying another task's files, the exact task source was copied to `/tmp/TASK-260712-2y74io-r8-verify`, `go mod tidy` was run only there, and hashes were compared back to the worktree (all task files `MATCH`).

```bash
go test ./cmd/pulsar-win-probe ./internal/winprobe -count=1
go test -race ./cmd/pulsar-win-probe ./internal/winprobe -count=1
go vet ./cmd/pulsar-win-probe ./internal/winprobe
```

```text
PASS host: probe 0.411s; winprobe 2.010s
PASS race: probe 1.891s; winprobe 2.974s
PASS vet: no output
```

### Windows cross-validation

From the same exact-hash isolated snapshot, for both `amd64` and `arm64`:

```bash
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go vet ./cmd/pulsar-win-probe ./internal/winprobe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-probe-<arch>.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-probe-<arch>.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-winprobe-<arch>.test.exe ./internal/winprobe
file /tmp/TASK-260712-2y74io-{probe,winprobe}-*.exe
```

```text
PASS amd64 relevant vet, all-production build, probe build, probe test compile, winprobe test compile
PASS arm64 relevant vet, all-production build, probe build, probe test compile, winprobe test compile
PASS file: new outputs are PE32+ x86-64/Aarch64 Windows executables
```

### Privacy, artifact, manifest, formatting, and contract checks

```bash
go test ./internal/winprobe -run '^TestSanitizeLogEvent' -count=50
go test ./internal/winprobe -run 'TestArtifact|TestRecoverArtifacts|TestManifestValidation|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=10
xmllint --noout msix/AppxManifest.xml.in probe-msix/AppxManifest.xml.in
test -z "$(gofmt -l cmd/pulsar-win-probe/coordinators.go cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/lifecycle_r6_test.go cmd/pulsar-win-probe/lifecycle_r8_test.go cmd/pulsar-win-probe/lifecycle_source_test.go)"
if rg -n '[[:blank:]]+$' LOGBOOK.md pulsar-win/probe-msix/README.md pulsar-win/cmd/pulsar-win-probe/{coordinators.go,lifecycle.go,main_windows.go,lifecycle_r6_test.go,lifecycle_r8_test.go,lifecycle_source_test.go}; then exit 1; fi
bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
test "$(rg -c '^@startuml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(rg -c '^@enduml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
git diff --check
task-board validate
```

```text
PASS recursive privacy x50: winprobe 0.354s
PASS artifact/recovery/manifest/sandbox/helper/privacy x10: winprobe 23.528s
PASS xmllint, gofmt inventory, changed-scope whitespace, diagram delimiters, git diff --check: no output
PASS Rev16 consistency: RESULT: PASS (0 anti-patterns in normative body)
PASS task-board validate: Board is valid. No issues found.
```

## Shared-tree verification anomaly (not changed by this task)

The required module-wide commands were attempted in the live worktree after the last task edit:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

All three currently fail before task package compilation because sibling-owned module files disagree:

```text
go.mod: golang.org/x/net v0.56.0; go.sum: only v0.57.0
go.mod: golang.org/x/sys v0.46.0; go.sum: only v0.47.0
coordinator_origin.go:12:2: missing go.sum entry for golang.org/x/net/idna
internal/winprobe/artifact_rename_darwin.go:5:8: missing go.sum entry for golang.org/x/sys/unix
```

`go.mod`, `go.sum`, `coordinator_origin.go`, and `artifact_rename_darwin.go` are outside this task and were not modified. After resolving sums only in the isolated copy, the module-wide run progressed and exposed a second unrelated in-progress root-test failure, `config_test.go:62:23: undefined: newTestCredentialRepository`; the module-only copy also cannot satisfy `wire`'s repository-parent golden path. Relevant probe/winprobe suites, race, vet, and both Windows builds pass in that same resolved exact-hash snapshot.

Development anomaly corrected during W4: the existing source-presence test still required the superseded `retainOrphan` then `requestStop` text and initially failed. It now requires `publishOrphanStopProducer`, `claimStop`, and `invokeClaimedStop`, and the full focused high-count/race matrix passes.

## Residual platform gates

- `pwsh`, `cl.exe`, `makeappx.exe`, `signtool.exe`, and `plantuml` are unavailable on this macOS host.
- Native MSVC helper tests, PowerShell package tests, MSIX packaging/signing/install, WACK, and AppContainer execution were not run.
- A signed MSIX must still be exercised on Windows 10 and Windows 11 with real microphone hardware to observe actual `WM_QUERYENDSESSION`/`WM_ENDSESSION`, WTS session lock/unlock, power suspend/resume, permission revoke/restore, tray/hotkey teardown, repeated start/stop, and next-launch `.partial` recovery.
- Confirmed shutdown is intentionally OS-owned: it provides nonblocking exact-owner Stop plus bounded no-sync partial append only. It cannot promise terminal callback, Release, durable evidence sync, or helper destruction before Windows terminates the process.
- WTS registration/delivery and sandbox-specific notification availability cannot be established on macOS; failure remains honest structured blocked evidence with a Windows next action.

## Dirty-tree boundary

The repository was dirty before this run and contains extensive concurrent work in coordinator, node-app, docs, workflow, module metadata, and other pulsar-win files. Those edits were preserved. The task changed only the eight inventoried files, added one board outcome resource, and made no commit or push. Independent review must read every inventoried file in full because the probe tree is untracked.
