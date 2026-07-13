# TASK-260712-2y74io independent lifecycle review R9

Date: 2026-07-13  
Role: independent reviewer  
Verdict: **BACK TO DEVELOPMENT**

The frozen R8 implementation matches every guard hash and its focused suites are green, but it still permits ordinary artifact finalization and `CaptureRelease` after confirmed `WM_ENDSESSION`. The project lifecycle sequence diagram also contradicts the accepted abrupt-shutdown contract. No product, test, documentation, manifest, or existing board resource was modified by this review.

## Scope and frozen-input verification

I read the task card, R8 producer outcome, all frozen R8 production/test/documentation files in full, the task and project lifecycle diagrams, the producer task block in `LOGBOOK.md`, the accepted Rev16 bridge contract and root acceptance, root amendments, prior lifecycle guards/outcomes, and `docs/spec-self-contained-audio.md` sections 3.13, 18, and 19 P1.0/P1.7. The probe tree is untracked, so the review used full-file inspection rather than `git diff` as implementation evidence.

All R9 frozen hashes matched:

| Frozen input | Observed SHA-256 |
|---|---|
| `pulsar-win/cmd/pulsar-win-probe/coordinators.go` | `1114a0a692b981eb46c2bd5508c3ccab050addf0bd4b2fc72fa2fd7832443a5a` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle.go` | `8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68` |
| `pulsar-win/cmd/pulsar-win-probe/main_windows.go` | `36f971e8c7877c0ab80d289d59dba09356faf5bc7551be239365f460bb2e3455` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go` | `e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go` | `7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go` | `99a78a3fa1e120c645930fe67ed68d0db376f34c3855e4751afb0a172bcee749` |
| `pulsar-win/probe-msix/README.md` | `1919528ea96be20d948b9aced81093bde152a7d1c16f23656650d3f8b2181327` |
| R8 producer outcome | `692c4d2b1823dba9b6445e969bbeeafba9109f637aa3cb63516b851069319691` |

`LOGBOOK.md` is shared and changed concurrently; its observed review-end hash was `fb813c6c3f0823abb1b1d46df6628d539fefc640e481b5f7e841a68541ea934b`. Only the task block was used as task evidence.

## Findings

### HIGH — R9-F28: a pre-confirmation drain can start Finalize and Release after the confirmation latch

Locations:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:174-181` — `runOrdinary` checks `closing` only once at the outer callback entry.
- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:657-687` — `runCaptureQueryFailureCleanup` performs Stop, artifact finalization, and exact-owner Release as one callback with no abrupt-gate admission between those operations.
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:921-943` — the production query-failure path checks `isClosing` only after `runCaptureResultQueryFailure` has returned, so the check is too late to suppress Finalize/Release.
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1125-1195` — the ordinary terminal path likewise observes terminal, finalizes, posts/logs, and calls `requestRelease` before its first post-terminal `isClosing` check.
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go:417-423` — confirmed `WM_ENDSESSION` concurrently closes/latches/wakes without waiting, as required.

Deterministic production-seam schedule:

1. The waiter enters the same `shutdown.runOrdinary` admission used by a production drain while `closing=false`.
2. `runCaptureQueryFailureCleanup` claims the exact owner Stop and blocks inside the real Stop callback seam.
3. The wndproc adapter confirms shutdown. It does not wait, cannot claim a duplicate Stop, stores `confirmed=true`, and wakes once.
4. The Stop callback returns exact `S_OK`.
5. The already-entered cleanup continues and calls artifact finalization and exact-owner `CaptureRelease` while `confirmed=true`.

I reproduced this with a review-only test in a hash-matching `/tmp` copy. The test asserts the defective effects, so its PASS means the boundary was crossed:

```text
go test ./cmd/pulsar-win-probe -run '^TestR9PreConfirmationDrainFinalizesAndReleasesAfterConfirmation$' -count=100
PASS: 0.373s

go test -race ./cmd/pulsar-win-probe -run '^TestR9PreConfirmationDrainFinalizesAndReleasesAfterConfirmation$' -count=100
PASS: 1.429s
```

Observed on every run: confirmation returned before the blocked Stop was released; afterward `FinalizeAttempted=true`, `ReleaseAttempted=true`, `Released=true`, with one finalization and one native Release after the confirmation latch.

This violates the cumulative R3-F10/W1-W4 abrupt boundary and the R9 guard: a release admitted before confirmation may finish, but a release/finalization attempt not yet admitted at confirmation must be rejected. It also contradicts the README claim that confirmed shutdown performs no post-latch finalization or release. The outer drain admission is too coarse to serve as a per-operation permit.

Required correction:

- Add a nonblocking abrupt admission boundary at every ordinary query/take/finalize/abort/Release operation, including query-failure cleanup, terminal cleanup, finalized-release retry, and artifact retry. Confirmation must never wait for these permits.
- Preserve the distinction that an actual `CaptureRelease` invocation admitted before confirmation may return, while Finalize/Release not yet admitted at confirmation execute zero callbacks.
- Add deterministic production-seam tests for confirmation while Stop is blocked, after Stop result publication but before Finalize, before exact-owner Release admission, and during an already-admitted Release. Assert no post-latch finalization, release, owner clear, lifecycle settlement, UI post, or passing evidence.
- Keep the confirmed wndproc limited to the exact nonblocking Stop plus latch/wake; do not move cleanup into the wndproc.

Existing R4/R8 tests do not cover this schedule. `TestR4F10OrdinaryPermitLinearizesAcrossConfirmationWithoutBlockingWndProc` treats an entire arbitrary callback as one permit, while `TestR8F25ConfirmedShutdownDoesNotWaitOrAdmitReleaseSuccessors` either starts Release before confirmation or tries a new outer callback after confirmation. Neither confirms between a Stop callback and later Finalize/Release inside the same production cleanup call.

### MEDIUM — R9-F29: the architecture sequence still specifies forbidden confirmed-shutdown work

Locations:

- `docs/diagrams/p1-windows-store-spike-lifecycle.puml:64-67` says confirmed Windows shutdown unregisters the hotkey and records abrupt-handoff evidence.
- `.task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml:41-45` collapses quit/lock/suspend/revoke into one unregister-and-finalize path and does not represent the accepted abrupt OS-owned branch.

Both statements conflict with the accepted Rev16/R3-F10 boundary and the current implementation/README: confirmed `WM_ENDSESSION` must perform no hotkey unregister, lifecycle evidence, sync, finalization, release, or helper destruction. The R8 handoff checked only PlantUML delimiters on the older task resource, not semantic consistency of the project diagram.

Required correction: update the sequence to separate graceful quit and return-to-idle edges from confirmed `WM_ENDSESSION`. The confirmed branch should show exact-owner nonblocking Stop before latch/wake, one bounded no-sync partial append, waiter exit, and Windows/startup-recovery ownership, with no Log or Tray cleanup message.

## Positive review results

- F25 Stop/result/Release ordering is materially improved for ordinary execution: immutable `(generation, operationID, owner pointer)` identity is preserved; pending Stop has an immediate or activation-deferred producer; Release cannot overtake the claimed Stop result; exact `S_OK` alone transfers ownership; failed Release remains retryable; terminal-first Stop is stable not-requested; operation-ID reuse does not match an old pointer.
- Publication losers have exact orphan ownership, a stored one-shot Stop producer before waiter visibility, terminal query, gated Release, and pointer-exact removal. Orphan query-to-release has a second abrupt admission and did not exhibit F28.
- Zero-ID `S_OK`, invalid-handle query, failed/unexpected Stop, finalized retry authority, evidence-before-escalation, and sticky evidence suppression are covered by production coordinators and deterministic tests.
- Waiter/UI ownership, permission fail-closed paths, lifecycle settlement replay, hotkey/tray retry ownership, privacy redaction, AppContainer capability boundaries, and honest signed-Windows limitations remain intact in the inspected code.

These positives do not waive R9-F28 or the diagram contradiction.

## Independent commands and results

Focused lifecycle schedules:

```text
go test ./cmd/pulsar-win-probe -run 'TestR8|TestR6F1[5-9]|TestR6F2[0-4]|TestR5F1[2-4]|TestR4|TestR3' -count=100
PASS: probe 3.766s

go test -race ./cmd/pulsar-win-probe -run 'TestR8|TestR6F1[5-9]|TestR6F2[0-4]|TestR5F1[2-4]|TestR4|TestR3' -count=50
PASS: probe 4.031s
```

Relevant host test/race/vet:

```text
go test ./cmd/pulsar-win-probe ./internal/winprobe -count=1
PASS: probe 0.379s; winprobe 1.974s

go test -race ./cmd/pulsar-win-probe ./internal/winprobe -count=1
PASS: probe 1.466s; winprobe 3.192s

go vet ./cmd/pulsar-win-probe ./internal/winprobe
PASS: no output
```

The live module-wide commands are currently blocked by unrelated concurrent root-package work:

```text
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
FAIL: credential_model.go:349:19: undefined: utf8
```

The probe, winprobe, and wire packages still passed in those `go test ./...` runs before the unrelated root-package failure. No sibling-owned source was changed to hide this live-tree result.

Windows cross-validation for both `amd64` and `arm64`:

```text
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go vet ./cmd/pulsar-win-probe ./internal/winprobe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go build ./...
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go build -o <probe.exe> ./cmd/pulsar-win-probe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go test -c ./cmd/pulsar-win-probe
GOOS=windows GOARCH=<arch> CGO_ENABLED=0 go test -c ./internal/winprobe
PASS: relevant vet, all-production build, probe build, and both test compilations for both architectures; outputs are PE32+ x86-64/AArch64.
```

Full Windows `go vet ./...` reports two pre-existing non-task warnings in `recovery_clipboard_windows.go:179,202` (`possible misuse of unsafe.Pointer`); relevant task-package vet is clean.

Privacy/artifact/manifest/static checks:

```text
go test ./internal/winprobe -run '^TestSanitizeLogEvent' -count=50
PASS: 0.451s

go test ./internal/winprobe -run 'TestArtifact|TestRecoverArtifacts|TestManifestValidation|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=10
PASS: 12.333s

xmllint --noout msix/AppxManifest.xml.in probe-msix/AppxManifest.xml.in
PASS

gofmt inventory, changed-scope whitespace, task diagram delimiters, and git diff --check
PASS

bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
PASS: RESULT: PASS (0 anti-patterns in normative body)

task-board validate
PASS: Board is valid. No issues found.
```

`plantuml` is unavailable on this host. Delimiter syntax passed, but semantic inspection produced R9-F29.

## Residual signed-Windows gates

Not run and not claimed on this macOS host:

- native MSVC helper compilation/CTest and registry/operation injection tests;
- PowerShell packaging, MakeAppx, signing, WACK, install, and real packaged AppContainer execution;
- actual Windows 10/11 delivery and timing for `WM_QUERYENDSESSION`/confirmed/cancelled `WM_ENDSESSION`, suspend/resume, WTS lock/unlock, and `AppCapability.AccessChanged`;
- real microphone permission revoke, hardware capture, repeated hotkey/capture cycles, process termination, and next-launch partial recovery.

These remain downstream gates. They cannot compensate for the host-reproduced post-latch Finalize/Release defect.
