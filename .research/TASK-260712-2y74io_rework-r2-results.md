# TASK-260712-2y74io — lifecycle rework R2 producer outcome

## Handoff state

Developer implementation and host-verifiable checks are ready for independent
review. This is not signed-Windows acceptance: native MSVC/MSIX execution and
the Windows 10/11 hardware matrix remain mandatory gates.

No commit or push was created. No manifest capability, package identity,
AppContainer boundary, helper ABI, or production mock path was changed.

## Exact changed-file inventory

The task-owned source tree was already untracked before this rework, so Git
cannot reconstruct a baseline diff for those files. Each current production
file listed below was inspected in full; the independent reviewer must do the
same rather than relying on `git diff`.

| File | Rework scope | SHA-256 at handoff |
| --- | --- | --- |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle.go` | Generation/gate lifecycle coordinator; exact settlement; signal history; non-droppable terminal intent; exit, helper, timer, evidence-retry, permission-query, and owned-resource coordinators | `fe948dd433258e603159052ad5a785425844afe68e3a4a493e3f2937d224a326` |
| `pulsar-win/cmd/pulsar-win-probe/main_windows.go` | Production coordinator wiring; waiter-only runtime permission checks; generation-carrying continuations; non-droppable quit; exact capture cleanup and bounded hard exit | `a19ed77402dc1291442ab5d7932851520ee22a6c0eb73be5c0201c76af0b6bb8` |
| `pulsar-win/cmd/pulsar-win-probe/window_windows.go` | Generation-aware UI messages; shutdown-pending gate; persisted cancel/confirm signals; retry-safe tray removal; exit/timer transitions | `91c6145f0ab114e7ed38f6ff5188019ec57abc1c8951c89dc32eca27aeb2cde3` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_test.go` | Existing ordering expectations updated for explicit tray-removal stage and persisted shutdown cancellation | `d6df215f601d7b5af2f91986f8e27b468c1a8d588e2cb8ab8216a85b6943ac05` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_rework_test.go` | Deterministic production-coordinator regressions mapped to R1–R10 | `b48b1bccb5a042d0693b1952c0a95380704d99ce65b55c3673fd207580accb88` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go` | AST guard that limits `CapPermissionCheck` to pre-waiter initialization and waiter runtime paths | `1a8903a5e9a614a14491c428d03107bd6e7b2cbcf1b2a14c72e5260ae41b1b7a` |
| `pulsar-win/probe-msix/README.md` | Accurate generation, signal, cleanup, hard-deadline, tray retry, and platform-limit documentation | `186b8c04b82e749dac9c7b816a909614cb60f1aefa26f010f843be75b89e83a2` |
| `LOGBOOK.md` | Three R1–R10 ownership/exit decisions added to the existing task entry; unrelated concurrent entries preserved | `22c1cda7a78cb576fed831f2ab6ac02c5741ee9f2821fb9ad941b01133045380` |

The hashes above were recomputed after the final formatting/verification pass.
Review should still recompute them before relying on the values.

## Production invariants

| Boundary | Enforced invariant |
| --- | --- |
| Start gates | Quit, query/confirmed shutdown, suspend, session lock, permission revoke, and an unfinished idle-cleanup run synchronously block new capture, picker, discovery, and rearm work. Shutdown cancellation clears only the shutdown flag; the active cleanup run continues blocking work until `idle`. |
| Capture identity | An explicit Record allocates one monotonic generation. Lifecycle entry closes its gate and binds the exact current generation and helper operation ID under the same mutex. Settlement targets only that generation; N+1 cannot advance N. |
| Lost interleavings | A lifecycle run snapshots the current terminal/artifact phase and replays already-observed state after its own stop stage. Release before registration produces an honest no-capture run; registration before release receives the exact release. |
| Async continuations | Waiter permission results and MTA-ready messages carry generation (and operation ID for activation). Prepare/activate callbacks run while the coordinator holds the gate boundary; stale messages are logged no-ops. Picker and discovery initiation use the same gated production seam. |
| Permission ownership | After waiter startup, all `CapPermissionCheck` calls run through the waiter-owned serialized coordinator. The only direct call is pre-waiter initialization/recovery. Query/read/take/release ownership in the accepted Rev16 bridge remains unchanged. |
| Quit delivery | Graceful quit is coordinator state, not a bounded-channel command. Every waiter loop checks it before ordinary commands, so queue saturation and `SetEvent` failure cannot lose terminal intent. Repeated quit begins cooperative cleanup once. |
| Resource order | Capture terminal → owned temporary artifact absent/finalized → capture release → permission unsubscribe → hotkey unregister → WTS unregister → helper destroy → tray delete → evidence sync → process-exit-ready. Tray ownership clears only after successful `NIM_DELETE`. |
| Exit states | `running → graceful-pending → force-committed` or `quit-committed`. Successful `CapDestroy` does not defeat the 30-second watchdog. Only irrevocable `WM_QUIT` commit defeats it. The force callback has no blocking log/sync. |
| Retry failure | `SetTimer==0` during terminal cleanup immediately commits force exit; idle hotkey retry failure escalates to graceful quit. Evidence sync has a tested finite retry budget and commits quit without a last blocking sync after exhaustion. |
| Evidence | The latest OS signal and ordered signal history are persisted on the run. Cleanup rows contain cleanup ID/order/stage, edge/mode/reason, exact capture generation/operation ID, and honest artifact/resource postconditions without paths, secrets, or audio content. |

## R1–R10 correction and deterministic test map

| Finding | Correction | Deterministic tests |
| --- | --- | --- |
| R1 | Atomic gate + generation/run binding; exact generation settlement; phase replay | `TestR1LifecycleRegistrationAndReleaseShareGenerationBoundary`, `TestR1GenerationNPlusOneCannotSettleGenerationN`, `TestR1RegistrationReplaysAlreadyObservedGenerationState`, `TestR1OverlappingEdgesBindAndSettleOneGeneration`, `TestR1QuitWithoutCaptureClosesCompetingStartGate` |
| R2 | Generation-carrying permission/capture messages and gate-held prepare/activate callbacks | `TestR2StalePermissionAndCaptureContinuationsAreSuppressed` (quit/suspend/lock/shutdown × permission-ready/capture-ready), `TestR2ContinuationBeforeLifecycleMayInvokeExactlyOnce` |
| R3 | `WM_QUERYENDSESSION` immediately sets `shutdownPending`; cancel requires ordered idle cleanup; confirm keeps terminal gate | `TestR3ShutdownQueryGateCancelAndConfirm` (query/start, cancel, confirm with/without capture) |
| R4 | Quit intent removed from bounded ordinary queue and consumed exactly once by waiter polling | `TestR4TerminalIntentSurvivesSaturatedOrdinaryQueueAndWakeFailure` |
| R5 | Hard deadline survives destroy/sync; no blocking hard-exit work; timer fallback and sync retry budget are production coordinators | `TestR5HardExitRemainsArmedAfterHelperDestroyAndDuringSyncStall`, `TestR5RetryTimerFailureHasImmediateFallback`, `TestR5RepeatedEvidenceSyncFailuresExhaustBoundedRetry` |
| R6 | Runtime permission calls route only through waiter coordinator; source call-site guard | `TestR6RuntimePermissionQueriesAreWaiterOwnedAndSerialized`, `TestR6CapPermissionCheckCallSitesRemainWaiterOwned` |
| R7 | Helper initialization lifetime extracted to atomic production state | `TestR7HelperLifetimeIsRaceSafe` plus host race suite |
| R8 | Repeated signal modifies stored latest value, count, and ordered history | `TestR8RepeatedSignalsPersistLatestAndOrderedHistory` (query→confirm, repeated suspend, repeated lock) |
| R9 | Owned resource clears only after delete callback succeeds | `TestR9OwnedResourceRetriesDeleteBeforeClearingOwnership` |
| R10 | Tests drive the portable coordinators used directly by Windows production code, including 100 real coordinator cycles | `TestR10ProductionCoordinatorSurvivesRepeatedStartStopCycles`; all tests above; Windows cross compilation |

## Acceptance-criteria and checklist mapping

| AC/checklist item | Evidence in this handoff |
| --- | --- |
| Quit and explicit app shutdown release capture resources | Non-droppable graceful intent cancels permission/enumeration/picker, stops the bound capture, waits for waiter-owned terminal/artifact/release, unsubscribes permission, unregisters hotkey/WTS, destroys helper, deletes tray, syncs evidence, then posts quit. Force paths remain fail-closed and bounded. |
| Suspend and session lock | Production `WM_POWERBROADCAST/PBT_APMSUSPEND` and registered `WM_WTSSESSION_CHANGE/WTS_SESSION_LOCK` close gates, bind/stop the exact generation, log exact/latest/history signals, remove hotkey after cleanup, and require an observed resume/unlock plus waiter permission query to rearm. Capture never auto-restarts. |
| Permission revoke | Waiter-owned `AccessChanged+CheckAccess` closes the permission gate, stops the exact generation with `CAP_REASON_PERMISSION_REVOKE`, disposes artifacts before release/idle, logs ordered cleanup, and rearms only after an allowed waiter result. |
| Shutdown query/confirm/cancel | Query establishes the synchronous start barrier and returns without blocking. Cancel records the new signal and completes idle cleanup. Confirm retains terminal disposition, performs direct nonblocking stop/hotkey release, and logs the honest abrupt OS handoff limitation. |
| Repeated cycles | The production coordinator runs 100 start/prepare/lock/terminal/artifact/release/unlock/idle cycles with no active run or generation left. Focused R1–R10 tests passed 20 repetitions. |
| Architecture | Accepted Rev16 UI/waiter/native ownership is preserved; runtime `CapPermissionCheck` is waiter-only; helper ABI and manifest are unchanged; no production mock or sandbox weakening was introduced. |
| Tests/lint/build | All host tests, race, vet, Windows cross-build/vet/test compilation, manifest XML, bridge static consistency, whitespace, and Git diff checks passed. |

## Exact verification commands and unabridged pass/fail summary

All commands ran from the repository root unless the block starts with
`cd pulsar-win`. There were no failing commands in the final verification pass.

### Formatting and focused deterministic repetition

```bash
cd pulsar-win
gofmt -w cmd/pulsar-win-probe/lifecycle.go \
  cmd/pulsar-win-probe/lifecycle_test.go \
  cmd/pulsar-win-probe/lifecycle_rework_test.go \
  cmd/pulsar-win-probe/lifecycle_source_test.go \
  cmd/pulsar-win-probe/main_windows.go \
  cmd/pulsar-win-probe/window_windows.go
go test -count=20 ./cmd/pulsar-win-probe -run 'TestR[1-9]'
```

Exact summary:

```text
ok   relux.works/duet/pulsar-win/cmd/pulsar-win-probe   0.864s
```

The regex includes `TestR10...` because it begins with `TestR1`; every R1–R10
test therefore ran 20 times.

### Full host tests

```bash
cd pulsar-win
go test -count=1 ./...
```

Exact summary:

```text
ok   relux.works/duet/pulsar-win                         2.940s
ok   relux.works/duet/pulsar-win/cmd/pulsar-win-probe   0.805s
ok   relux.works/duet/pulsar-win/internal/winprobe      2.417s
ok   relux.works/duet/pulsar-win/wire                   1.472s
```

### Full host race suite

```bash
cd pulsar-win
go test -race -count=1 ./...
```

Exact summary:

```text
ok   relux.works/duet/pulsar-win                         4.019s
ok   relux.works/duet/pulsar-win/cmd/pulsar-win-probe   1.765s
ok   relux.works/duet/pulsar-win/internal/winprobe      3.567s
ok   relux.works/duet/pulsar-win/wire                   2.525s
```

### Vet, Windows cross-build, and Windows test compilation

```bash
cd pulsar-win
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c \
  ./cmd/pulsar-win-probe -o /tmp/TASK-260712-2y74io-probe.test.exe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c \
  ./internal/winprobe -o /tmp/TASK-260712-2y74io-winprobe.test.exe
xmllint --noout probe-msix/AppxManifest.xml.in
```

Exact summary: no stdout/stderr; every command exited 0. The Windows binaries
were compiled, not executed on macOS.

### Focused production artifact/manifest checks

```bash
cd pulsar-win
go test -count=1 -v ./internal/winprobe \
  -run 'TestArtifactWriterAbortRetriesOwnedCleanupPostcondition|TestManifest'
```

Exact summary:

```text
--- PASS: TestArtifactWriterAbortRetriesOwnedCleanupPostcondition (0.00s)
--- PASS: TestManifestValidationRequiresExactReviewedCapabilitySet (0.00s)
    --- PASS: TestManifestValidationRequiresExactReviewedCapabilitySet/unexpected_capability (0.00s)
    --- PASS: TestManifestValidationRequiresExactReviewedCapabilitySet/missing_reviewed_network_capability (0.00s)
PASS
ok   relux.works/duet/pulsar-win/internal/winprobe   0.357s
```

### Accepted bridge, diagram, whitespace, diff, and board checks

```bash
bash .research/root-checks/windows-consistency-check.sh \
  .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
test "$(grep -c '^@startuml' docs/diagrams/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(grep -c '^@enduml' docs/diagrams/p1-windows-store-spike-lifecycle.puml)" -eq 1
! rg -n '[[:blank:]]+$' \
  pulsar-win/cmd/pulsar-win-probe/lifecycle.go \
  pulsar-win/cmd/pulsar-win-probe/lifecycle_test.go \
  pulsar-win/cmd/pulsar-win-probe/lifecycle_rework_test.go \
  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go \
  pulsar-win/cmd/pulsar-win-probe/main_windows.go \
  pulsar-win/cmd/pulsar-win-probe/window_windows.go \
  pulsar-win/probe-msix/README.md
git diff --check
task-board validate
```

Exact summary: the bridge checker printed every named guard as `PASS` and
ended `RESULT: PASS (0 anti-patterns in normative body)`; the diagram,
whitespace, and Git diff checks had no output and exited 0; board validation
printed `Board is valid. No issues found.`

The earlier contention-associated `TestSupervisorSpawnRestartStop` anomaly
reported by root did not reproduce in either final full host suite. This does
not erase the earlier observation; no lifecycle claim depends on that test.

## Required checks not run and residual platform gaps

- `probe-msix/build-probe.ps1`, `native-command.Tests.ps1`, native
  `pulsar-capture` MSVC/C++ tests, MakeAppx staging, signing, and WACK were not
  run. This host is Darwin and has no `pwsh`, CMake, Visual Studio Windows
  toolchain, Windows SDK, or MakeAppx. `/usr/bin/clang++` is a macOS compiler
  and cannot validate the C++/WinRT helper.
- The generated Windows `.test.exe` binaries were not executed because macOS
  cannot run them.
- PlantUML rendering was not run because no `plantuml` executable or jar is
  installed. The authoritative diagram delimiters and repository whitespace
  were checked.
- AppContainer delivery of WTS lock/unlock and power notifications, global
  hotkey behavior, real permission revoke timing, microphone capture, suspend,
  sign-out/shutdown budget, repeated process exit, and fail-closed recovery
  still require the signed MSIX on real Windows 10 and Windows 11 hardware.
- A confirmed `WM_ENDSESSION` remains an OS-owned abrupt boundary. The probe
  requests nonblocking stop, wakes one best-effort waiter drain, and unregisters
  the hotkey, but cannot guarantee terminal callback, helper destruction, or
  durable log/file sync before Windows terminates it. The evidence explicitly
  records this limitation and the next recovery inspection.

## Git diff/status scope

At evidence capture:

```text
 M LOGBOOK.md
?? pulsar-win/cmd/pulsar-win-probe/
?? pulsar-win/probe-msix/README.md
```

`git diff --numstat` can only report the tracked `LOGBOOK.md` (`22 0`) because
the probe and MSIX trees are untracked. Those 22 lines include pre-existing
concurrent task entries; this rework added only the three decision lines under
`TASK-260712-2y74io`. The broader worktree contains unrelated modified and
untracked coordinator/spec/research files owned by other work. None was reset,
overwritten, committed, or pushed. `git diff --check` passed for the entire
tracked worktree.
