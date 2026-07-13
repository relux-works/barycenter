# Independent review outcome — TASK-260712-2y74io

## Verdict

BACK TO `to-dev`. The implementation is not approvable. Four substantive findings remain. No production code was modified during this review.

## Review scope

Read in full: the task card and both guards; the lifecycle PlantUML input; `docs/spec-self-contained-audio.md` sections 3.13, 18, and 19 P1.0/P1.7; the complete accepted Rev16 bridge contract; root acceptance; root review amendments; producer outcome; every producer-listed changed file; and the relevant artifact/logger/helper production seams. Because the probe tree is untracked, findings are based on the complete current files rather than Git diff alone.

## Findings

### F1 — HIGH — lifecycle ownership is not atomic with capture ownership or quit gating

Locations:
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1154-1158`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1243-1255`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1338-1348`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:338-374`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:969-1027`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:833-848`
- `pulsar-win/cmd/pulsar-win-probe/lifecycle.go:302-305`

Failure scenario 1: `requestGracefulQuit` commits `exitGracefulPending`, observes no capture, and returns after queueing quit, but `a.quitting` is not set until the waiter later drains that command. A queued UI/hotkey action can therefore pass `requestRecord` and publish a new `captureOp` after quit is already pending. The quit run permanently has `CaptureExpected=false`; the waiter stops the real capture, but `captureRuns()` excludes the quit run, so the cleanup ID can skip capture settlement, artifact disposition, and capture release while later claiming an ordered graceful exit.

Failure scenario 2: suspend/lock/quit snapshots a nonzero `captureOp`, unlocks, and only then creates the lifecycle run. The waiter can terminalize, release, clear the operation, and call lifecycle advancement before the run exists. The later run has `CaptureExpected=true`, but no future terminal/release advancement remains; idle hotkey cleanup or graceful permission-unsubscribe ordering can stay stuck until forced exit.

Violated AC/invariant: deterministic idempotent shutdown; ordered evidence for the actual owned capture; no hidden capture/hung lifecycle; quit must prevent new work once committed.

Required correction: make lifecycle-run creation, capture generation/ownership capture, and the quit gate one synchronized transition. Mark quit synchronously before any UI action can start capture. Ensure the waiter cannot clear/release a captured generation before its lifecycle run is registered, or reconcile a release that already occurred. Add deterministic barrier-based tests for both interleavings through production state seams.

### F2 — HIGH — the only graceful-quit command can be dropped permanently

Locations:
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1338-1348`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1375-1386`

Failure scenario: the 32-entry command channel is full. The first quit call commits `exitGracefulPending`, but ignores `enqueue` returning false. The waiter never receives `kind:"quit"`, so it never sets `a.quitting`, never issues cooperative cancels, and never starts ordinary cleanup. Every later quit request fails the CAS and returns without retrying delivery. The only remaining path is the 30-second forced `os.Exit`, which is an internal queue failure, not a concrete platform limitation.

Violated AC/invariant: explicit quit must clean capture resources; repeated quit must be idempotent; ordinary in-process load must not convert clean shutdown into unclean process teardown.

Required correction: make quit delivery non-droppable (for example, shared terminal intent observed by every waiter iteration or a priority control path), handle wake failure independently, and add a saturated-queue/repeated-quit test.

### F3 — HIGH — retry and force paths do not actually guarantee a bounded exit

Locations:
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1270-1276`
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go:685-726`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1355-1372`

Failure scenario 1: after successful `CapDestroy`, the UI changes `exitState` to `exitGracefulComplete`, thereby defeating the 30-second watchdog. If evidence sync then fails, `scheduleDestroyRetry` calls `SetTimer`. When `SetTimer` returns zero, it only logs; there is no callback, immediate message, or force fallback. The hidden process can remain indefinitely with the watchdog unable to commit. The idle hotkey-unregister retry similarly ignores `SetTimer` failure entirely, leaving lifecycle cleanup and hotkey ownership stranded.

Failure scenario 2: the watchdog/Force Quit path calls blocking `logFile.Sync()` before `os.Exit(1)`. If that sync stalls, the sole hard-exit path stalls too; retry counts elsewhere do not bound a single syscall.

Violated AC/invariant: no hung hidden process; bounded evidence sync; hotkey cleanup must either complete or produce a concrete terminal limitation/next action. This also contradicts the README claim that storage faults cannot leave a hidden hung process.

Required correction: retain an independent hard deadline until `WM_QUIT`/process exit is committed; make timer scheduling failure take an explicit fallback; do not place an unbounded sync on the only hard-exit path. Add injected `SetTimer==0`, repeated sync-failure, and force-path tests.

### F4 — MEDIUM — `helperInitialized` races at the watchdog/CapDestroy boundary

Locations:
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:100`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1345-1347`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1359-1367`
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go:672-689`

Failure scenario: `tryDestroyOnce` reads and writes `helperInitialized` on the UI thread without `a.mu`; the timer goroutine can concurrently win the watchdog CAS and read that field while holding `a.mu`. The lock does not synchronize an unlocked writer, so this is a Go data race on the exact force/graceful boundary. The macOS race run cannot cover the Windows-tagged code.

Violated AC/invariant: data-race-free idempotent helper ownership and reliable cleanup evidence.

Required correction: protect every access with the same synchronization primitive (or atomics) and add a portable extracted state test or an executable Windows race test.

## Independent verification

PASS — formatting, complete host suite, and vet:

```bash
cd pulsar-win
test -z "$(gofmt -l cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/lifecycle_test.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/window_windows.go internal/winprobe/artifact_test.go)" && go test -count=1 ./... && go vet ./...
```

Output:
```text
ok  relux.works/duet/pulsar-win                         8.453s
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe   4.551s
ok  relux.works/duet/pulsar-win/internal/winprobe      6.541s
ok  relux.works/duet/pulsar-win/wire                   5.357s
```
Formatting and vet emitted no output; exit 0.

PASS — race:
```bash
cd pulsar-win
go test -race -count=1 ./...
```
Output:
```text
ok  relux.works/duet/pulsar-win                         10.668s
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe   7.071s
ok  relux.works/duet/pulsar-win/internal/winprobe      8.893s
ok  relux.works/duet/pulsar-win/wire                   6.781s
```
Exit 0. This host run excludes the Windows-tagged app code implicated by F4.

PASS — Windows cross-build, Windows vet, and Windows test compilation:
```bash
cd pulsar-win
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./... &&
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./... &&
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./cmd/pulsar-win-probe -o /tmp/TASK-260712-2y74io-review-probe.test.exe &&
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/winprobe -o /tmp/TASK-260712-2y74io-review-winprobe.test.exe
```
No stdout/stderr; exit 0.

PASS — focused lifecycle and real ArtifactWriter cleanup tests:
```bash
cd pulsar-win
go test -count=1 -v ./cmd/pulsar-win-probe -run 'TestLifecycle|TestAbruptShutdown|TestCancelledSystemShutdown'
go test -count=1 -v ./internal/winprobe -run TestArtifactWriterAbortRetriesOwnedCleanupPostcondition
```
Eight lifecycle tests, eight message-decision subtests, and the artifact cleanup postcondition test passed; package summaries:
```text
PASS
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe   4.463s
PASS
ok  relux.works/duet/pulsar-win/internal/winprobe      2.807s
```

PASS — XML, whitespace, Git diff check, diagram delimiters, forbidden-capability scan, accepted-contract consistency checker, and board validation. The consistency checker reported `RESULT: PASS (0 anti-patterns in normative body)`; board validation reported `Board is valid. No issues found.`

NOT RUN — `staticcheck` (not installed), rendered PlantUML (tool unavailable), native MSVC/C++ tests, MakeAppx/MSIX build, signing/WACK, and signed Windows 10/11 hardware lifecycle runs. `pwsh`, Windows SDK packaging tools, PlantUML, and CMake are unavailable on this macOS host. No Windows hardware pass is claimed.

## Architecture/safety observations

- No `runFullTrust` or `broadFileSystemAccess` declaration was introduced.
- No production mock path, secret logging, or audio sample leakage was found.
- Win32 message return semantics and UI-thread ownership for WTS/hotkey/`CapDestroy` are otherwise consistent with the selected contract.
- The documented WTS/power/AppContainer and confirmed-`WM_ENDSESSION` limitations remain honest downstream signed-hardware gates.

## Review change scope

Reviewer changed no source, tests, docs, config, or `LOGBOOK.md`. Board-only mutations are this outcome resource, review notes/checklist, and the terminal `to-dev` status. The producer-listed task files remain dirty/untracked exactly as found.
