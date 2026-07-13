# TASK-260712-2y74io — lifecycle cleanup implementation results

Date: 2026-07-13 (Asia/Tbilisi)
Role: developer
Handoff target: independent review and root line-by-line review

## Outcome

The packaged Windows probe now has a production lifecycle decision table and
ordered evidence tracker for explicit quit, Windows shutdown/sign-out,
suspend/resume, session lock/unlock, and microphone permission revoke/restore.
The tracker is the ordering authority used by the Windows code; it rejects
claims that advance beyond an unobserved capture, artifact, registration,
helper, or durability postcondition.

Graceful quit drains capture and all other async operations, verifies temporary
artifact disposition, releases the capture, unsubscribes permission callbacks,
unregisters the hotkey and WTS notifications, destroys the helper after
quiescence, removes the tray icon, syncs the evidence log, writes an exit-ready
record, syncs that record, then posts `WM_QUIT`. Evidence sync has a bounded
retry so a storage failure cannot leave a hidden hung process; exhaustion is
failing evidence, not a clean-shutdown claim.

Suspend, session lock, and permission revoke stop capture with their native
reason, verify artifact cleanup/release, unregister the hotkey, and return to an
explicit no-hidden-capture idle state. Resume/unlock or restored permission may
re-register the hotkey, but never restarts capture automatically. Recording is
blocked while paused, quitting, or retrying an owned temporary-artifact cleanup.

`WM_QUERYENDSESSION` requests nonblocking stop. Confirmed `WM_ENDSESSION` wakes
one best-effort waiter drain, unregisters the hotkey in the window-procedure
budget, and records an honest abrupt-OS-handoff limitation. It does not
fabricate terminal, artifact, helper-destroy, or fsync evidence that Windows may
preempt. Cancelled shutdown converts the same run into ordered idle cleanup.

## Exact changed files

- `pulsar-win/cmd/pulsar-win-probe/lifecycle.go` — production Win32 message
  decision table, lifecycle edges/modes/stages, thread-safe monotonic cleanup
  tracker, prerequisite validation, and idempotent run selection.
- `pulsar-win/cmd/pulsar-win-probe/lifecycle_test.go` — focused production-seam
  tests for signal mapping, active/no-capture graceful order, honest query-fail
  settlement, rejected false cleanup, 100 repeated lock cycles, abrupt shutdown,
  and cancelled shutdown.
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go` — lifecycle state ownership,
  permission revoke detection, capture/artifact/release evidence, retryable real
  `ArtifactWriter.Abort` cleanup, idle rearm gates, graceful quit, bounded forced
  exit evidence, and abnormal local cleanup.
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go` — power/shutdown/WTS
  handlers, WTS registration, stateful hotkey/tray ownership, idle/rearm messages,
  and ordered UI-thread graceful destruction/evidence sync.
- `pulsar-win/internal/winprobe/artifact_test.go` — proves a failed owned-path
  removal remains retryable and a second production `Abort` establishes absence.
- `pulsar-win/probe-msix/README.md` — exact selected OS signals, cleanup order,
  evidence fields, bounded failure behavior, and signed-MSIX platform limits.
- `docs/diagrams/p1-windows-store-spike-lifecycle.puml` — implemented lifecycle
  branches and abrupt Windows shutdown boundary.
- `LOGBOOK.md` — durable decisions and Windows/AppContainer limitations.

No manifest capability was added or weakened. No mock was introduced into a
production path. No Windows device or signed-MSIX evidence was fabricated. No
commit or push was created.

## Acceptance criteria and checklist mapping

1. Quit / explicit shutdown without dangling capture resources:
   - tray Quit, hidden `WM_CLOSE`, Ctrl-C, and SIGTERM share the graceful path;
   - capture/picker/permission/enumeration operations drain before quiescence;
   - temporary artifact, capture, callback, hotkey, WTS, helper, tray, and log
     ownership are released in asserted order;
   - Force Quit/watchdog paths log remaining ownership and failed evidence.
2. Session lock / suspend and observed signal:
   - `WM_POWERBROADCAST/PBT_APMSUSPEND`, both resume variants,
     `WM_WTSSESSION_CHANGE/WTS_SESSION_LOCK`, and `WTS_SESSION_UNLOCK` are
     recorded verbatim as `observedOSSignal`;
   - new capture and hotkey activation remain unavailable until rearm;
   - failed WTS registration records `GetLastError` and the signed-hardware next
     action because the session-lock signal cannot then be directly observed.
3. Permission revoke cleanup before exit or idle:
   - `AppCapability.AccessChanged+CheckAccess`, with bounded CheckAccess polling
     as deterministic defense, requests `CAP_REASON_PERMISSION_REVOKE`;
   - revoked evidence is fail-closed, temporary artifacts and capture are
     released before hotkey-unregistered idle, and rearm requires allowed access.
4. Concrete platform limits:
   - signed AppContainer power/WTS delivery is deferred to the Windows 10/11
     hardware matrix with explicit blocked evidence on registration failure;
   - confirmed `WM_ENDSESSION` records that Windows may preempt terminal callback,
     helper destruction, or file/log durability, plus the startup-recovery next
     action.
5. Repeated cycles / no hung process or leaked hotkey:
   - production register/unregister state is idempotent;
   - lifecycle runs are monotonic and removed only at idle/handoff/exit;
   - 100 repeated lock cycles pass; capture is never auto-restarted;
   - artifact and sync retries block false success, while bounded sync/force
     exits prevent an indefinitely hidden process.

## Verification — exact commands and unabridged pass/fail summary

PASS — formatting, complete Go suite, and host vet:

```bash
cd pulsar-win
test -z "$(gofmt -l cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/lifecycle_test.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/window_windows.go internal/winprobe/artifact_test.go)" && go test -count=1 ./... && go vet ./...
```

Output:

```text
ok   relux.works/duet/pulsar-win                         4.717s
ok   relux.works/duet/pulsar-win/cmd/pulsar-win-probe   1.367s
ok   relux.works/duet/pulsar-win/internal/winprobe      3.288s
ok   relux.works/duet/pulsar-win/wire                   1.055s
```

`gofmt -l` and `go vet ./...` produced no output; combined exit was 0.

PASS — race suite:

```bash
cd pulsar-win
go test -race -count=1 ./...
```

Output:

```text
ok   relux.works/duet/pulsar-win                         6.615s
ok   relux.works/duet/pulsar-win/cmd/pulsar-win-probe   3.644s
ok   relux.works/duet/pulsar-win/internal/winprobe      5.446s
ok   relux.works/duet/pulsar-win/wire                   3.270s
```

PASS — Windows cross-build, Windows vet, and Windows test-binary compilation:

```bash
cd pulsar-win
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./... && \
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./... && \
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./cmd/pulsar-win-probe -o /tmp/TASK-260712-2y74io-probe.test.exe && \
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/winprobe -o /tmp/TASK-260712-2y74io-winprobe.test.exe
```

No stdout/stderr; exit 0. The Windows binaries were compiled, not executed on
macOS.

PASS — focused lifecycle and production artifact retry tests:

```bash
cd pulsar-win
go test -count=1 -v ./cmd/pulsar-win-probe -run 'TestLifecycle|TestAbruptShutdown|TestCancelledSystemShutdown'
go test -count=1 -v ./internal/winprobe -run TestArtifactWriterAbortRetriesOwnedCleanupPostcondition
```

All eight lifecycle tests and all eight message subtests passed; the production
artifact retry test passed. Package summaries were:

```text
PASS
ok   relux.works/duet/pulsar-win/cmd/pulsar-win-probe   0.352s
PASS
ok   relux.works/duet/pulsar-win/internal/winprobe      0.516s
```

PASS — manifest, whitespace, and diff validation:

```bash
xmllint --noout pulsar-win/probe-msix/AppxManifest.xml.in && \
git diff --check && \
! rg -n '[[:blank:]]+$' LOGBOOK.md docs/diagrams/p1-windows-store-spike-lifecycle.puml pulsar-win/probe-msix/README.md pulsar-win/cmd/pulsar-win-probe/lifecycle.go pulsar-win/cmd/pulsar-win-probe/lifecycle_test.go pulsar-win/cmd/pulsar-win-probe/main_windows.go pulsar-win/cmd/pulsar-win-probe/window_windows.go pulsar-win/internal/winprobe/artifact_test.go
```

No stdout/stderr; exit 0.

PASS — accepted bridge consistency and board validation:

```bash
bash .research/root-checks/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
task-board validate
```

The consistency checker reported every named R11–R15 guard PASS and
`RESULT: PASS (0 anti-patterns in normative body)`. Board validation reported
`Board is valid. No issues found.`

PASS (structural only) — diagram delimiter check:

```bash
test "$(grep -c '^@startuml' docs/diagrams/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(grep -c '^@enduml' docs/diagrams/p1-windows-store-spike-lifecycle.puml)" -eq 1
```

No stdout/stderr; exit 0.

## Required checks not run and why

- `probe-msix/build-probe.ps1` (native MSVC build/tests, package staging, and
  MakeAppx): NOT RUN. This host is macOS and has no `pwsh`, CMake, Visual Studio
  Windows toolchain, Windows SDK, or MakeAppx. The Go Windows targets were
  cross-built as the available compile validation.
- Native `pulsar-capture` C++ tests: NOT RUN for the same missing Windows
  C++/WinRT/MSVC/CMake environment. Existing Go/native-contract tests passed.
- Rendered PlantUML validation: NOT RUN because no `plantuml` executable or jar
  is installed. Start/end structure and repository whitespace were checked.
- Signed-MSIX Windows 10/11 lifecycle execution: NOT RUN because this host
  cannot install/run an MSIX and has no Windows microphone/session/power
  hardware. This remains the explicit downstream hardware evidence gate; no
  runtime pass is claimed here.

## Residual platform gaps / next action

1. Build and sign the probe on Windows, then validate WTS registration and
   actual lock/unlock delivery in AppContainer on Windows 10 and Windows 11.
2. Validate power notification delivery and stop latency across real suspend and
   resume; confirm no capture indicator/session survives.
3. Revoke microphone permission during capture; verify ordered JSONL cleanup,
   absence of temporary artifacts, hotkey unregistration, and rearm only after
   allowed access.
4. Exercise query/cancelled/confirmed shutdown. For confirmed `WM_ENDSESSION`,
   measure which evidence reaches disk and verify any partial is discarded by
   fail-closed startup recovery.
5. Run at least repeated 10x start/stop and lifecycle cycles while checking the
   process, Windows capture indicator, hotkey collision, evidence directory,
   and logs.

## Git diff/status scope

The worktree was already dirty and the Windows probe/docs trees were already
untracked when this task began. Unrelated coordinator, CI, planning, research,
and spec changes were preserved and not edited by this task. Task-scoped status:

```text
 M LOGBOOK.md
?? docs/diagrams/p1-windows-store-spike-lifecycle.puml
?? pulsar-win/cmd/pulsar-win-probe/lifecycle.go
?? pulsar-win/cmd/pulsar-win-probe/lifecycle_test.go
?? pulsar-win/cmd/pulsar-win-probe/main_windows.go
?? pulsar-win/cmd/pulsar-win-probe/window_windows.go
?? pulsar-win/internal/winprobe/artifact_test.go
?? pulsar-win/probe-msix/README.md
```

Because the probe tree is untracked as a whole, Git has no pre-task index blob
from which to produce a task-only diff for those files. The exact files above
are the files touched by this task. `git diff --check` passes, and no unrelated
dirty file was reverted or overwritten.
