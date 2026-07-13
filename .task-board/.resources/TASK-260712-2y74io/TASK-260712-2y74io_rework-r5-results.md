# TASK-260712-2y74io — R5 lifecycle rework outcome

Date: 2026-07-13  
Role: developer  
Disposition: ready for independent review; root line-by-line review and signed-Windows gates remain mandatory.

This outcome supersedes the R4 producer outcome only for root R4-F12 and the live R5-F13/F14 directives. The accepted Rev16 bridge, R1–R3 corrections, and R4 W1–W4 abrupt-shutdown/evidence constraints remain intact.

## Result

Confirmed `WM_ENDSESSION` no longer calls `lifecycleTracker.beginLifecycle` or acquires `lifecycleTracker.mu`. The current native capture generation and operation are an inseparable immutable object published through `atomic.Pointer`. The wndproc path performs only: close abrupt gate, load exact owner, claim its one-shot shutdown stop, set confirmation, wake, return.

Prepare and activation continue to use the accepted lifecycle owner, but publish/check the atomic native owner at the real helper boundary. If confirmation wins while either external callback is blocked, confirmation does not wait. A native operation returned by prepare after confirmation stops itself once, is not published active, and its production successor dispatcher rejects activation/evidence/UI/result/release/finalize work. An activation already in flight similarly has no successor.

The live F13 correction distinguishes `closing` from `confirmed`: ordinary waiter and wndproc callbacks are suppressed as soon as stop begins, but the waiter returns `false` and stays alive until confirmation is visible. Only then may it claim the exactly-once bounded no-sync abrupt drain and exit.

No capability, manifest, helper ABI, AppContainer boundary, native implementation, or evidence privacy rule changed. No production mock/fallback branch was added.

## Exact changed-file inventory and SHA-256

```text
497f159470e500d070276fbe4f54a272247a1945530faa48cf467f7f78ee62ac  LOGBOOK.md
ae6fcad9b20cba9990c41ad3330c36bf4fc32cba9ad5b691fd3fbb535c3f9520  pulsar-win/probe-msix/README.md
01e5732d1590918bdf6a974edd448b91fdcd10f8455d4d9c89f9cecbf870aacb  pulsar-win/cmd/pulsar-win-probe/coordinators.go
a38fef8130cb6d87007b4eaf40096bad818fdd2fd488775109fd0ccad8638254  pulsar-win/cmd/pulsar-win-probe/main_windows.go
97a65fab63958e2b84814312d8b06e6e1f81a91cc31a616c7d58ae27072580a1  pulsar-win/cmd/pulsar-win-probe/window_windows.go
1316439bb2cfc1a0667697517579d6b80f530da279c2dfc730cb814635e05f97  pulsar-win/cmd/pulsar-win-probe/lifecycle_r4_test.go
c965cb628ebabff503677ed9267ef62051fd9ff5201207c7c2401f4723173bb1  pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go
af14fd7cb00c1b21dbb7cd07c0d3b0286bea50201de0fa82e0c660a57c652d53  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go
```

`lifecycle_r5_test.go` is new. `lifecycle.go`, native code, manifest, artifact writer, and accepted Rev16 resource are unchanged. The probe tree remains untracked in the parent dirty worktree, so Git diff is not sufficient review evidence. `LOGBOOK.md` is shared with sibling work; this task added only the R5 F12 decision and F13/F14 correction bullets while preserving all concurrent content.

## Invariants and acceptance mapping

| Invariant / AC | Production result |
|---|---|
| Nonblocking confirmed wndproc | Adapter uses atomics plus the selected nonblocking `CaptureStop`; it does not reference the lifecycle tracker or any ordinary-work mutex. |
| Exact capture identity | Generation and operation ID are fields of one atomically published owner snapshot. Stale generation/ID clears fail; only a matching successful release clears the active pointer. |
| Stop order and idempotency | Abrupt gate closes before owner load; the exact current owner receives one stop claim before `confirmed.Store(true)` and wake. Repeated confirmation and late callback stop attempts add no effect. |
| Prepare race | Helper prepare admitted before confirmation may return. Publication checks the gate before and after CAS; a late owner stops itself and is not activated or handed to successor work. |
| Activation race | The exact published owner is stopped while activation may still hold `lifecycleTracker.mu`; confirmation returns before callback release. On return the production dispatcher rejects all successors. |
| Stop-to-latch interval | Waiter suppresses ordinary drains but remains alive while `closing=true, confirmed=false`. Both wndproc guards return protocol values without entering application callbacks. |
| Abrupt drain | It is keyed by the confirmation-captured exact owner and requires matching app generation/operation, existing writer, and valid format. Existing cap/no-sync/no-release behavior is unchanged. |
| Quit/suspend/lock/revoke | Existing generation-bound stop, artifact cleanup, hotkey unregister/rearm, permission fail-closed, evidence ordering, and repeated-cycle state machines are unchanged and covered by full/race regression suites. |
| Sandbox/privacy | Reviewed capabilities and manifest are unchanged; privacy/manifest/artifact tests pass. No Windows hardware evidence is inferred from host tests. |

## Deterministic test mapping

| Finding | Executable production-seam evidence |
|---|---|
| R5-F12 prepare mutex block | `TestR5F12ConfirmedShutdownDoesNotWaitForPrepareAndStopsLateOwner` holds the real tracker inside the prepare helper callback, confirms before release, then proves the late operation is stopped once, not published, and gets zero successor callbacks. |
| R5-F12 activation mutex block | `TestR5F12ConfirmedShutdownDoesNotWaitForActivationAndSuppressesSuccessor` holds real activation under the tracker, proves exact stop → wake before callback release, then proves no duplicate stop or successor. |
| R5-F12 post-confirm activation | `TestR5F12ActivationWaitingForLifecycleGateCannotStartAfterConfirmation` waits for the production monotonic activation-acquisition attempt while another callback holds `lifecycle.mu`; after confirmation and release, the helper activation callback count remains zero. |
| R5-F12 exact snapshot | `TestR5F12ExactOwnerSnapshotIgnoresStaleClearAndStopsOnce` rejects stale generation/operation clears and proves exact one-shot stop/wake. |
| R5-F13 lost abrupt drain | `TestR5F13WaiterStaysAliveBetweenStopAndConfirmedLatch` holds the real adapter stop seam, proves zero ordinary/abrupt work and no waiter exit before confirmation, protocol-correct wndproc suppression, then one abrupt callback and exit. |
| R5-F14 successor evidence | Late prepare and in-flight activation tests invoke their actual result dispatcher with separate activate/evidence/post/release/finalize hooks and prove all remain zero. `TestR5F14ProductionSuccessorDispatcherRunsOnlyWhileGateIsOpen` proves the same dispatcher invokes every hook when open and rejects every hook after gate closure. |
| R4 regression | Existing `TestR4F10*`, `TestR4W1*`, `TestR4W2*`, `TestR4W4*`, and source/AST checks remain green with the atomic adapter. |

## Exact verification after the latest repository edit

Host: Darwin 24.6.0 x86_64; `go version go1.26.0 darwin/amd64`.

From `pulsar-win`:

```bash
test -z "$(gofmt -l cmd/pulsar-win-probe/coordinators.go cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/window_windows.go cmd/pulsar-win-probe/lifecycle_r3_test.go cmd/pulsar-win-probe/lifecycle_r4_test.go cmd/pulsar-win-probe/lifecycle_r5_test.go cmd/pulsar-win-probe/lifecycle_source_test.go internal/winprobe/artifact.go internal/winprobe/artifact_test.go)"
go test ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR3|TestR4|TestR5|TestArtifactWriterConfirmedShutdown' -count=50
go test ./... -count=1
go test -race ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR3|TestR4|TestR5|TestArtifactWriterConfirmedShutdown' -count=20
go test -race ./... -count=1
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-r5-probe-amd64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-r5-probe-arm64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r5-probe-amd64.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r5-winprobe-amd64.test.exe ./internal/winprobe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r5-probe-arm64.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r5-winprobe-arm64.test.exe ./internal/winprobe
file /tmp/TASK-260712-2y74io-r5-*.exe
go test ./internal/winprobe -run 'TestSanitizeLogEvent' -count=50
go test -v ./internal/winprobe -run 'TestArtifactWriterConfirmedShutdownAppendNeverSyncsOrClosesPartial|TestArtifactWriterAbortRetriesOwnedCleanupPostcondition|TestManifest|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=1
xmllint --noout probe-msix/AppxManifest.xml.in
```

Unabridged result summary:

```text
PASS  gofmt inventory
ok    relux.works/duet/pulsar-win/cmd/pulsar-win-probe  2.015s  # focused R3/R4/R5 x50
ok    relux.works/duet/pulsar-win/internal/winprobe      0.744s  # focused artifact x50
ok    relux.works/duet/pulsar-win                        3.086s  # full uncached
ok    relux.works/duet/pulsar-win/cmd/pulsar-win-probe  0.799s
ok    relux.works/duet/pulsar-win/internal/winprobe      3.023s
ok    relux.works/duet/pulsar-win/wire                   1.097s
ok    relux.works/duet/pulsar-win/cmd/pulsar-win-probe  2.385s  # focused race x20
ok    relux.works/duet/pulsar-win/internal/winprobe      1.965s  # focused race x20
ok    relux.works/duet/pulsar-win                        4.097s  # full race
ok    relux.works/duet/pulsar-win/cmd/pulsar-win-probe  1.840s
ok    relux.works/duet/pulsar-win/internal/winprobe      4.041s
ok    relux.works/duet/pulsar-win/wire                   2.176s
PASS  host vet
PASS  Windows amd64 vet/build/probe-test-compile/winprobe-test-compile
PASS  Windows arm64 vet/build/probe-test-compile/winprobe-test-compile
PASS  file: all six artifacts are PE32+ for the requested x86-64/Aarch64 target
ok    relux.works/duet/pulsar-win/internal/winprobe      0.366s  # privacy x50
PASS  focused artifact/manifest/sandbox/payload/privacy suite
ok    relux.works/duet/pulsar-win/internal/winprobe      0.332s
PASS  xmllint
```

Additional focused development stress after F13/F14 correction:

```bash
go test -count=50 -run 'TestR5F1[234]|TestR5F12|TestR5F14' ./cmd/pulsar-win-probe
go test -race -count=30 -run 'TestR5F1[234]|TestR5F12|TestR5F14' ./cmd/pulsar-win-probe
```

Both passed (`0.414s`, `1.498s`). Earlier F12-only schedules also passed 100 ordinary repetitions and 50 race repetitions before the live F13/F14 correction.

From repository root:

```bash
bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
test "$(rg -c '^@startuml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(rg -c '^@enduml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
if rg -n '[[:blank:]]+$' LOGBOOK.md pulsar-win/probe-msix/README.md pulsar-win/cmd/pulsar-win-probe/coordinators.go pulsar-win/cmd/pulsar-win-probe/main_windows.go pulsar-win/cmd/pulsar-win-probe/window_windows.go pulsar-win/cmd/pulsar-win-probe/lifecycle_r4_test.go pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go; then exit 1; fi
git diff --check
task-board validate
```

Rev16 consistency reported `RESULT: PASS (0 anti-patterns in normative body)`. Diagram delimiter, whitespace, and `git diff --check` checks exited 0. Board validation reported `Board is valid. No issues found.`

## Development anomalies (all corrected)

1. The first formatting command used repository-root paths while already in `pulsar-win` and failed with `lstat pulsar-win/...: no such file or directory`; it was rerun with module-relative paths and passed.
2. An early `GOOS=windows ... go test -run '^$'` correctly cross-built but then tried to execute the Windows test binary on macOS and failed with `exec format error`. It was replaced by `go test -c`; amd64 and arm64 probe/winprobe test compilation passed.
3. Live root review identified F13 (premature waiter exit during stop-to-latch) and F14 (scheduler-permissive/tautological test evidence). Both were corrected before the full verification above. No test assertion or production build failed after those corrections.

## Dirty-worktree scope

The parent worktree contains extensive pre-existing/sibling coordinator, docs, planning, research, CI, and task-board changes. This task changed only the eight hashed files above. Because `pulsar-win/cmd`, `pulsar-win/internal`, `pulsar-win/native`, and `pulsar-win/probe-msix` are untracked as whole trees, `git diff --stat` does not enumerate this task's probe changes. `git diff --check` passes but is not review evidence for untracked files; reviewers must read every inventory file in full and compare the hashes. No unrelated edit was reverted. No commit or push was made.

## Remaining platform gates

- Native MSVC helper tests/build, MakeAppx packaging, signing, WACK, and actual MSIX/AppContainer execution were not available on this macOS host.
- Signed Windows 10 and Windows 11 hardware runs must still prove delivery/timing of `WM_QUERYENDSESSION`, confirmed/cancelled `WM_ENDSESSION`, `WM_POWERBROADCAST`, WTS lock/unlock notifications, and `AppCapability.AccessChanged`.
- Those runs must inspect the next-launch recovery state of the no-sync `.partial`, verify no hidden active recording or leaked hotkey across repeated cycles, and capture real ordered evidence. Host tests and cross-builds do not claim that evidence.

The implementation is ready for independent review, followed by mandatory root line-by-line/hash/test audit.
