# TASK-260712-2y74io — R4/W1–W4 producer outcome

Date: 2026-07-13. Role: developer. Target: `to-review`; fresh independent and root line-by-line/hash/test reviews remain mandatory.

## Scope and result

This resource supersedes the earlier producer outcomes for root R3 F10/F11 and live root directives W1–W4. The accepted Rev16 bridge, AppContainer capability boundary, lifecycle generation ledger, permission ownership, privacy boundary, and earlier R1/R2/R3 corrections remain unchanged.

- F10/W1 confirmed shutdown: `confirmedShutdownAdapter` is the production wndproc seam. The first confirmed `WM_ENDSESSION` closes the start gate and binds the exact active capture generation, issues only that generation's nonblocking `CaptureStop(shutdown)`, atomically latches confirmation, signals the waiter, and returns. It does not reuse ordinary lifecycle cleanup. Repeated confirmation adds no effect.
- W2 queued-window boundary: both production wndprocs consult `confirmedShutdownMessageGate` before their message switch. Once confirmed, `WM_QUERYENDSESSION` and `WM_POWERBROADCAST` receive their required true result and every other queued application message receives zero without dispatching application, lifecycle, UI, logging, native-helper, release, destroy, or cleanup work.
- F10/W3 waiter/evidence boundary: every ordinary waiter drain and each physical evidence log/sync callback shares the monotonic pre-confirmation admission gate. Confirmation never waits for an admitted callback. Pre-confirmation queued successors and every post-confirmation enqueue are suppressed with one stable nonsecret error; synchronous callers settle and async rows are discarded.
- W1 abrupt buffered handoff: the only post-latch data path is `drainConfirmedShutdownBuffer`, capped at eight 4096-frame reads from the exact already-owned capture into the existing safe `.partial` writer through `WriteBufferedFramesWithoutSync`. It cannot log, sync, close, finalize, abort, release, delete, post UI cleanup, unregister hotkeys, or destroy the helper. Startup recovery owns the remaining artifact.
- F11 evidence prerequisite boundary: the first write/sync error, short write, enqueue/ack timeout, saturation, unknown operation, or abrupt suppression is sticky. The worker may finish the one callback already admitted, then drains queued successors as control messages only; passing cleanup/evidence/process-exit claims cannot physically survive a missing prerequisite.
- W4 non-window callback boundary: post-pump error UI/logging, late signal/evidence quit requests, watchdog callbacks, timer continuations, and deferred resource close all use the same ordinary admission or an immediate confirmed-latch guard. No new callback begins after confirmation.

No production mock, capability/manifest weakening, native-helper contract change, commit, push, or fabricated Windows evidence was introduced.

## Exact changed-file inventory and SHA-256

```text
02d34ec9af04fb31a6dfb18d0158fffd4e46c051a486dfc60b73144b8aeaab4a  LOGBOOK.md
eb501c7a1484b18ca9c6b8bfa25e3157b188864807798932d0b1c9e4a6fc6207  pulsar-win/cmd/pulsar-win-probe/coordinators.go
15fd1febd634767d02da392d67eaac9ca7e10a98afcf03ad607c9adf458f53b0  pulsar-win/cmd/pulsar-win-probe/main_windows.go
8258140bb0784da952bed1ca7e7e4679db7f9a84195c91794c0b7ea78ef46364  pulsar-win/cmd/pulsar-win-probe/window_windows.go
7a1625d2a24a193ee389c82e26496f9e8a232d6c97ac20e73b78322c6a188486  pulsar-win/cmd/pulsar-win-probe/lifecycle_r3_test.go
8eb1b770ebff44b08e792c47353f5ea58f183e6d12f9f971c23181b9e2c58da1  pulsar-win/cmd/pulsar-win-probe/lifecycle_r4_test.go
8b72c73c0cbbdffa61dc7458528ff4acc6fe988e94b1af84cef0a93bd1b2fe00  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go
01b932959bc2518c77c3f6ee25afae7557321744bd675761c78cf9ab0b6efc04  pulsar-win/internal/winprobe/artifact.go
67f2d3bfe2386e2f1f4da8b6142df209ed3fd23f2619f3f68ad0969f188cbc0f  pulsar-win/internal/winprobe/artifact_test.go
ff208b54cc1139ae62895318e48aaf3dd5e18a39d6452710b4b930a9b5e41a40  pulsar-win/probe-msix/README.md
```

`lifecycle_r4_test.go` is new. The probe tree is untracked in the parent worktree, so Git diff alone is insufficient. Pre-edit hashes were `4175d127...` (coordinators), `487d5201...` (main), `92387361...` (window), `20b4ab84...` (R3 tests), `23c38a40...` (source tests), `2deb590f...` (artifact), `b367e791...` (artifact tests), and `3d67c2fd...` (README). `lifecycle.go` remains byte-for-byte at its accepted baseline and is not in the inventory.

`LOGBOOK.md` was `de4077d9...` immediately before this task's first patch. Sibling task additions, including `TASK-260712-m5264f`, remain preserved in the shared-file hash above. No coordinator application, CI, spec, diagram, planning, research, or sibling implementation was modified.

## Invariants and acceptance mapping

| Area | Result |
|---|---|
| Confirmed wndproc | Exact-generation gate/stop → monotonic latch → waiter wake → return. Nothing else is admitted; repeat confirmation is idempotent. |
| Queued messages | Both wndprocs guard before the switch. All post-latch command/hotkey/timer/WM_APP/close/cancel/resume/repeat-shutdown callbacks are inert with protocol-correct returns. |
| Ordinary ownership | A permit admitted before confirmation may finish and cannot block the wndproc. No later ordinary waiter, evidence, lifecycle, UI, resource, timer, signal, watchdog, or pump-error callback starts. |
| Abrupt buffered handoff | Exact capture ID, existing writer, and valid format are mandatory. Reads/appends are hard-capped; no ordinary cleanup operation occurs; `.partial` remains startup-recovery-owned. |
| Evidence order | Admission is checked before enqueue and immediately before physical I/O. First prerequisite failure/suppression is stable and sticky; queued passing rows never reach logger/sync callbacks. |
| Quit/suspend/lock/revoke | Earlier idempotent generation-bound stop, artifact cleanup, hotkey unregister/rearm, permission fail-closed handling, ordered evidence, and repeated-cycle behavior remain covered by the full and race suites. |
| Platform honesty | Host tests prove production coordinators/adapters/file seams and Windows buildability only. Signed MSIX/AppContainer notification delivery and real Windows timing remain hardware gates. |

## Deterministic test map

F10/W1:

- `TestR4F10ConfirmedShutdownStopLatchAndWakeOrder`
- `TestR4F10ConfirmedShutdownWinsEveryCoalescedWaitIndex`
- `TestR4F10OrdinaryPermitLinearizesAcrossConfirmationWithoutBlockingWndProc`
- `TestR4W1ProductionAdapterAllowsOnlyBoundedBufferedAppendAfterConfirmation`
- `TestR4W1ConfirmedShutdownBufferIsHardCapped`
- `TestR4ConfirmedShutdownDrainCannotEnterOrdinaryCleanupAPIs`
- `TestArtifactWriterConfirmedShutdownAppendNeverSyncsOrClosesPartial`

F11/W3:

- `TestR4F11FailedPrerequisiteSuppressesQueuedPassingRows`
- `TestR4F11SyncTimeoutSuppressesAlreadyQueuedPassingRows`
- `TestR4F11UnknownEvidenceOperationFailsClosed`
- `TestR4F11SuppressedSynchronousRepliesReceiveStableFailure`
- updated `TestR3F5EvidenceQueueSaturationIsStickyAndSuppressesCleanClaims`

W2/W4:

- `TestR4W2ProductionMessageGateSuppressesEveryQueuedApplicationClass`
- `TestR4W4LateNonWindowCallbacksAreSuppressedAfterConfirmation`
- source invariants verify both guards precede each wndproc switch, confirmed shutdown forbids post-latch work, evidence admission precedes queue and I/O, graceful quit/watchdog use ordinary admission, main post-pump UI/logging is gated, and deferred close returns at the latch.

These are the portable production coordinators/adapters invoked by Windows code. Test fakes enter only through callback seams; no fake branch enters the packaged production path.

## Exact verification after the latest edit

Host: Darwin 24.6.0 x86_64; `go version go1.26.0 darwin/amd64`.

From `pulsar-win`:

```bash
test -z "$(gofmt -l cmd/pulsar-win-probe/coordinators.go cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/window_windows.go cmd/pulsar-win-probe/lifecycle_r3_test.go cmd/pulsar-win-probe/lifecycle_r4_test.go cmd/pulsar-win-probe/lifecycle_source_test.go internal/winprobe/artifact.go internal/winprobe/artifact_test.go)"
go test ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR4|TestR3F5EvidenceQueueSaturation|TestArtifactWriterConfirmedShutdown' -count=100
go test ./... -count=1
go test -race ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR4|TestR3F5EvidenceQueueSaturation|TestArtifactWriterConfirmedShutdown' -count=20
go test -race ./... -count=1
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-r4-probe-amd64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-r4-probe-arm64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r4-probe-amd64.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r4-winprobe-amd64.test.exe ./internal/winprobe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r4-probe-arm64.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r4-winprobe-arm64.test.exe ./internal/winprobe
file /tmp/TASK-260712-2y74io-r4-*.exe
go test ./internal/winprobe -run 'TestSanitizeLogEvent' -count=50
go test -v ./internal/winprobe -run 'TestArtifactWriterConfirmedShutdownAppendNeverSyncsOrClosesPartial|TestArtifactWriterAbortRetriesOwnedCleanupPostcondition|TestManifest|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=1
xmllint --noout probe-msix/AppxManifest.xml.in
```

```text
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe  3.942s  # focused x100
ok  relux.works/duet/pulsar-win/internal/winprobe      0.516s  # focused x100
ok  relux.works/duet/pulsar-win                        4.034s  # full uncached
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe  0.366s
ok  relux.works/duet/pulsar-win/internal/winprobe      2.241s
ok  relux.works/duet/pulsar-win/wire                   1.429s
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe  2.088s  # focused race x20
ok  relux.works/duet/pulsar-win/internal/winprobe      1.853s  # focused race x20
ok  relux.works/duet/pulsar-win                        4.862s  # full race
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe  1.812s
ok  relux.works/duet/pulsar-win/internal/winprobe      3.000s
ok  relux.works/duet/pulsar-win/wire                   2.682s
ok  relux.works/duet/pulsar-win/internal/winprobe      0.336s  # privacy x50
PASS  # focused manifest/artifact/privacy
ok  relux.works/duet/pulsar-win/internal/winprobe      0.421s
```

Formatting, host/Windows vet, Windows builds/test compilation, and `xmllint` exited 0. `file` identified both application binaries and all four test executables as PE32+ x86-64/Aarch64 for Windows.

From the repository root:

```bash
bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
test "$(rg -c '^@startuml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(rg -c '^@enduml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
rg -n '[[:blank:]]+$' <the ten-file R4 inventory>
git diff --check
task-board validate
```

Rev16 consistency passed every guard and reported `RESULT: PASS (0 anti-patterns in normative body)`. Diagram delimiter, whitespace, and `git diff --check` checks exited 0 silently. Board validation reported `Board is valid. No issues found.`

## Corrected development anomalies

1. An early `GOOS=windows ... go test` attempted to execute a Windows binary on macOS and produced the expected `exec format error`; it was replaced by `go test -c`, which passed for both architectures.
2. Root W1 caught post-latch hotkey unregister/lifecycle evidence in the initial R4 version. Those actions were removed and exact production-adapter/evidence counter tests added.
3. The first W1 source check retained a stale call-form substring after method-value wiring. The assertion was corrected and all subsequent focused/full/race/cross/static runs passed.
4. One privacy command was invoked from the repository root and reported no main module; the exact command was rerun from `pulsar-win` and passed 50 repetitions.
5. Root W2–W4 audits identified post-latch message-pump, evidence-queue, and non-window callback paths before handoff. Both wndprocs, physical evidence calls, enqueue paths, quit/watchdog entry points, main post-pump error handling, and deferred close are now gated; their deterministic schedules passed 100 focused and 20 race repetitions.

No anomaly resulted in a product workaround.

## Residual platform gates

`pwsh`, CMake/MSVC (`cl`/`msbuild`), MakeAppx, WACK/appcert, and PlantUML are unavailable on this host. Native MSVC helper tests/package creation and signed Windows 10/11 AppContainer execution were not run. Downstream hardware review must exercise explicit quit, suspend/resume, WTS lock/unlock, revoke/regrant, `WM_QUERYENDSESSION` cancel/confirm, hotkey/tray ownership, repeated cycles, real microphone capture, abrupt partial state, and next-launch recovery. These remain evidence gates, not host passes.

The R4/W1–W4 producer work is ready for review; no commit or push was made.
