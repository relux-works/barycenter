# TASK-260712-2y74io — R10 lifecycle rework results

Date: 2026-07-13  
Role: developer  
Handoff state: ready for independent review and root full-file/hash/test audit. Signed-Windows gates remain downstream.

## Outcome

R10-F28/F29 and live root findings F30-F32 are implemented without changing the accepted Rev16 helper ABI, manifest capabilities, AppContainer boundary, package identity, or confirmed `WM_ENDSESSION` wndproc contract. Ordinary work now has operation-level admission: every native query/read/cancel/release, artifact callback, lifecycle mutation, UI publication, evidence operation, and separately dangerous successor obtains a fresh atomic pre-close permit immediately before its callback.

The confirmed wndproc remains nonblocking and byte-identical in `window_windows.go`: it closes the start gate, requests the exact active owner's one-shot Stop, publishes the confirmation latch, wakes the waiter, and returns. Only the existing bounded no-sync append to the owned `.partial` is admitted after confirmation. Windows and next-launch recovery own remaining process resources.

No commit or push was made.

## Exact changed-file inventory and SHA-256

The probe and docs trees are untracked in this worktree, so full-file hashes—not `git diff`—define producer scope.

| File | SHA-256 | Scope |
|---|---|---|
| `pulsar-win/cmd/pulsar-win-probe/coordinators.go` | `0806f508eaf2df9ef95ea9d701af95d6c6f49965e1a9730339bf81dfd71dad05` | Operation permit, split fail-closed query operations, Stop/Finalize/Release sequencing, split structural evidence/escalation, split prepare/suppressed continuations, orphan post-Release completion gate |
| `pulsar-win/cmd/pulsar-win-probe/main_windows.go` | `51a0e03dbdc06e1f4fa26de761103f943be14ef411e81a2f4cdf1e3aece1639a` | Production waiter/UI integration; self-gated helper, state, log, enqueue, lifecycle, artifact, Release, retry, and UI transitions |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r10_test.go` | `a78b3b91603c560b678520c695817723647c6210bfa3f6b386508b7ace3c233b` | Deterministic R10 F28-F32 production-seam schedules and Release call-site audit |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go` | `d8369ed22d334598cf967277e68a180d2ec678638a9476b6b164db473a74ca8d` | Updated production wiring assertions for centralized permission, Release, watchdog, and operation admission |
| `pulsar-win/probe-msix/README.md` | `b0c0ab8b1e62e00f2c6179a536f08b1b744e5751c3f4e4fe1684c6c8a86ae47b` | Operation-level abrupt semantics and F30-F32 successor suppression |
| `docs/diagrams/p1-windows-store-spike-lifecycle.puml` | `01ffa997d7a7b9e8867d3cb5a15da662c4bbc937b6a428ff0a20be9e73bd8cd2` | Truthful graceful/cancelled/confirmed lifecycle branches |
| board resource `p1-windows-store-spike-lifecycle.puml` | `01ffa997d7a7b9e8867d3cb5a15da662c4bbc937b6a428ff0a20be9e73bd8cd2` | Updated through `task-board resource update`; byte-identical to docs diagram |
| `LOGBOOK.md` | `0fd813b23a48191385a0ffaf1a1ee95f08af5b0663c334ec222ec89801494515` | F28-F32 task decisions and the corrected Windows-only build anomaly; shared file contains unrelated concurrent entries |

Preserved frozen files:

| File | SHA-256 |
|---|---|
| `lifecycle.go` | `8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68` |
| `window_windows.go` | `97a65fab63958e2b84814312d8b06e6e1f81a91cc31a616c7d58ae27072580a1` |
| `lifecycle_r6_test.go` | `e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56` |
| `lifecycle_r8_test.go` | `7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e` |

## Invariants and production mapping

| Boundary | Enforced invariant | Production location |
|---|---|---|
| Operation permit | The acquire load observing `closing=false` is the permit linearization point. A callback may return after confirmation, but every successor acquires another permit. | `coordinators.go:189-232` |
| Query-failure cleanup | Stop, artifact finalization/abort, and exact-owner Release have independent permits. A pre-close Stop result never authorizes a not-yet-admitted cleanup callback. | `coordinators.go:708-765`; `main_windows.go:1077-1583` |
| Native owner | Release covers the actual helper call; successful Release may publish its one-shot result after the latch but cannot clear owner/app state or settle lifecycle without fresh permits. | `main_windows.go:484-490,1077-1583` |
| Artifact ownership | Create/write/periodic sync/finalize/abort and retry are separately admitted. Confirmation leaves the `.partial` recovery-owned instead of running ordinary cleanup. | `main_windows.go:1077-1583,2188-2259` |
| Permission/helper/picker | Every waiter-owned query, cancel, result take, release, handle read/close, state publication, and UI post is independently admitted. | `main_windows.go:489-1075,1585-2186` |
| Lifecycle/evidence/UI | Lifecycle advance/replay, log/logAsync, enqueue/wake, durable UI publication, and graceful watchdog admission are self-gated. | `main_windows.go:2322-2425,2695-2825` |
| Structural failure | Required evidence and graceful escalation use separate permits. Evidence admitted before close may return, but escalation cannot start after the latch. | `coordinators.go:651-681` |
| Prepare/activation continuation | Failure callback, diagnose, lifecycle query, settlement, activation evidence, enqueue, stop, and quit cannot share a stale outer permit. | `coordinators.go:1068-1190,1256-1270`; `main_windows.go:1830-2086` |
| Orphan owner | Query/terminal/Release and later orphan clear/evidence/settlement each have a distinct permit. A Release returning after confirmation leaves the orphan recovery-owned. | `coordinators.go:853-900,1155-1190`; `main_windows.go:504-549` |
| Confirmed shutdown | No lifecycle/global mutex, ordinary cleanup, evidence, resource release, helper destroy, or UI work was added to the wndproc. `window_windows.go` stayed byte-identical. | `coordinators.go:1001-1024`; frozen `window_windows.go` |

## Requirement-to-test mapping

| Requirement / schedule | Deterministic test evidence |
|---|---|
| F28.1 Stop blocks, confirmation returns, zero Finalize/Release | `TestR10F28StopBoundariesRejectLaterFinalizeAndRelease/query_failure_stop_blocks_then_confirmation` |
| F28.2 confirmation after Stop result, before Finalize permit | same test, `confirmation_after_stop_publication_before_finalize_permit` |
| F28.3 admitted Finalize finishes; zero Release/clear/PASS/UI/evidence | `TestR10F28FinalizeAndReleaseHaveIndependentPermits/admitted_finalize_may_finish_but_release_is_rejected` |
| F28.4 confirmation immediately before Release admission | same test, `confirmation_immediately_before_release_permit` |
| F28.5 admitted Release finishes once; zero post-latch successors | same test, `admitted_release_returns_once_without_post-latch_successors` |
| F28.6 admitted `CaptureGetResult`/`CaptureRead` cannot authorize successors | `TestR10F28AdmittedQueryOrReadCannotAuthorizeSuccessors` |
| F28.7 finalized Release and artifact retry reject late callbacks | `TestR10F28RetryAndAllCallbackSeamsRespectConfirmation` |
| F28.8 confirmation stays bounded for permission/artifact/release/evidence/UI/hotkey/tray/destroy callbacks | same test, `bounded-*` subtests |
| F28.9 graceful/cancelled shutdown retains ordinary cleanup and restored start gate | `TestR10F28GracefulAndCancelledShutdownPathsRemainOperational`; prior R3-R8 lifecycle suites |
| F28.10 exact owner and operation-ID reuse | `TestR10F28OperationIDReuseKeepsExactOwnerIdentity`; prior F25-F27 suites |
| F30 blocked required evidence cannot escalate | `TestR10F30BlockedStructuralEvidenceCannotEscalateAfterLatch` through `handleInvalidNativeOwner` |
| F31 blocked evidence/log-class continuations suppress failure/enqueue/lifecycle/UI/log successors | `TestR10F31BlockedContinuationCannotSmuggleSuccessors` |
| F32 orphan Release returning after latch retains orphan and suppresses clear/evidence/settlement/UI | `TestR10F32OrphanReleaseCannotClearOrSettleAfterLatch` |
| Every production `CaptureRelease` call site is exact-owner and operation gated | `TestR10CaptureReleaseCallSitesHaveExactOwnerAndOperationGates` uses AST call enumeration plus executable coordinator schedules |
| F29 truthful diagrams | Both diagram hashes match; delimiter/static checks pass; confirmed branch contains exact Stop → latch/wake → bounded append → waiter/OS recovery handoff only |

All ordering tests use channels/barriers. Timeouts only fail a hung test.

## Verification after the last code edit

Host: macOS/Darwin x86_64; Go 1.26.0 darwin/amd64.

### Focused lifecycle schedules

```bash
go test ./cmd/pulsar-win-probe -run 'TestR10|TestR8|TestR6F1[5-9]|TestR6F2[0-4]|TestR5F1[2-4]|TestR4|TestR3' -count=100
go test -race ./cmd/pulsar-win-probe -run 'TestR10|TestR8|TestR6F1[5-9]|TestR6F2[0-4]|TestR5F1[2-4]|TestR4|TestR3' -count=50
```

```text
PASS focused x100: probe 4.342s
PASS focused race x50: probe 5.777s
```

### Full host tests, race, and vet

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
```

```text
PASS test: root 3.159s; probe 1.188s; winprobe 2.797s; wire 0.790s
PASS race: root 4.226s; probe 1.888s; winprobe 3.696s; wire 2.624s
PASS vet: no output
```

The live shared module was coherent; no isolated copy or module metadata change was needed.

### Windows amd64 and arm64 cross-validation

For each `GOARCH=amd64` and `GOARCH=arm64`:

```bash
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go vet ./cmd/pulsar-win-probe ./internal/winprobe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-probe-<arch>.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-probe-<arch>.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-winprobe-<arch>.test.exe ./internal/winprobe
```

```text
PASS amd64: vet, all-package build, probe build, probe test compile, winprobe test compile
PASS arm64: vet, all-package build, probe build, probe test compile, winprobe test compile
PASS file inspection: PE32+ x86-64 and PE32+ Aarch64 for all six outputs
```

Development anomaly: the first Windows cross-check found `showMainWindow` (`func() bool`) passed to the untyped `func()` permit. It was corrected to the typed `runAbruptOperation` result seam; all focused, host, race, vet, and both Windows matrices above were rerun after that correction.

### Privacy, artifact, manifest, contract, formatting, and diagrams

```bash
go test ./internal/winprobe -run '^TestSanitizeLogEvent' -count=50
go test ./internal/winprobe -run 'TestArtifact|TestRecoverArtifacts|TestManifestValidation|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=10
xmllint --noout msix/AppxManifest.xml.in probe-msix/AppxManifest.xml.in
test -z "$(gofmt -l <task Go inventory>)"
rg trailing-whitespace check over task inventory
bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
diagram delimiter, byte-identity, and confirmed-branch checks
git diff --check
task-board validate
```

```text
PASS privacy x50: winprobe 0.384s
PASS artifact/recovery/manifest/sandbox/helper/privacy x10: winprobe 12.091s
PASS xmllint, gofmt, whitespace, diagram delimiter/identity, git diff --check: no output
PASS accepted Rev16 consistency: RESULT: PASS (0 anti-patterns in normative body)
PASS task-board validate: Board is valid. No issues found.
```

`plantuml` and a local PlantUML/Structurizr Docker image are unavailable, so raster rendering was not run. Both PlantUML files have valid single start/end delimiters, are byte-identical, and the confirmed branch was inspected directly.

## Acceptance checklist mapping

- Explicit quit: durable terminal intent, one-shot exact Stop, ordered artifact/native/resource/evidence teardown, hard deadline, and repeated-cycle tests remain green.
- Suspend: packaged `WM_POWERBROADCAST/PBT_APMSUSPEND` path requests exact nonblocking Stop; ordinary idle cleanup and rearm remain covered by prior suites.
- Session lock: WTS signal path requests exact nonblocking Stop; registration failure remains honest blocked evidence and a Windows hardware next action.
- Permission revoke: waiter-only permission query, fail-closed query failure/revoke Stop, artifact cleanup, and rearm gating remain covered.
- Confirmed shutdown: exact Stop precedes latch/wake; no not-yet-admitted ordinary callback or successor runs; one bounded no-sync partial append and waiter exit only.
- Repeated start/stop: exact generation/owner identity, hotkey ownership, retry, and 100-cycle/repeated race suites remain green.
- No capability, manifest, privacy, or AppContainer boundary was weakened.

## Residual platform gates

- `pwsh`, `cl.exe`, `makeappx.exe`, `signtool.exe`, and `plantuml` are unavailable on this macOS host.
- Native MSVC helper tests, signed MSIX package creation/install, WACK, and installed AppContainer execution were not run.
- A signed MSIX must still be run on Windows 10 and Windows 11 with real microphone hardware to observe actual `WM_QUERYENDSESSION`/`WM_ENDSESSION`, WTS lock/unlock, suspend/resume, permission revoke/restore, tray/hotkey teardown, repeated start/stop, and next-launch `.partial` recovery.
- Windows may terminate immediately after confirmed `WM_ENDSESSION`; terminal callback, native Release, durable evidence sync, hotkey/tray cleanup, helper destruction, and lifecycle PASS are intentionally not claimed for that branch.

## Dirty-tree boundary

`git status --short -- <task paths>` reports `M LOGBOOK.md` plus untracked board-resource, docs-diagram, probe, and README paths. The broader repository was already dirty with extensive concurrent coordinator, node-app, docs, workflow, module, onboarding, and other pulsar-win work. Those files were preserved. Because `pulsar-win/cmd/`, `pulsar-win/probe-msix/`, `docs/diagrams/`, and `.task-board/` are untracked as directories, reviewers must inspect the inventoried current files in full and use hashes rather than relying on `git diff`.
