# Root line-by-line review — TASK-260712-2y74io — round 1

## Verdict

**REWORK REQUIRED.** I read every producer-changed file in full, the relevant
helper/artifact seams, the complete accepted Rev16 ownership/lifecycle contract,
the root amendments, producer evidence, and the independent R1 review. I also
reran the host/race/cross-build/static checks. No lifecycle code is accepted in
this round.

The independent reviewer found four valid blockers. The root review confirms
them and adds failures that the independent report did not cover. Corrections
must be made as one coherent ownership/state-machine change; local logging or
timeout patches are insufficient.

## Blocking findings

### R1 — CRITICAL — lifecycle runs are not bound atomically to a capture generation

Locations: `main_windows.go:833-848,1154-1158,1212-1255,1338-1348`;
`lifecycle.go:160-205,302-305`.

`requestLifecycleStop` snapshots `captureOp`, releases `a.mu`, and only then
creates a tracker run. The waiter may terminalize, dispose, release, and clear
that operation before the run exists. The late run permanently expects capture
stages that will never recur. Conversely, `advanceCaptureLifecycles` advances
every capture-expecting run without checking which `captureOperationId`
produced the event, so a later capture can falsely complete an older run.

`requestGracefulQuit` has the same split transition and does not synchronously
close the start gate. A new operation can appear after the quit run recorded
`CaptureExpected=false`, or the old operation can settle before registration.

Required correction: introduce an explicit monotonic capture generation/ID in
the lifecycle run. Under one synchronization boundary, commit terminal intent,
close the relevant start gate, snapshot/register the exact operation generation,
and publish the stop intent. Settlement must target that generation only and
must replay already-observed terminal/artifact/release state if registration
loses an interleaving. Never let generation N+1 advance a run for N.

Required tests: deterministic barriers for (a) release between snapshot and run
registration, (b) quit with no capture followed by a competing start, (c) old
run plus new generation terminal/release, and (d) overlapping lifecycle edges
against one generation.

### R2 — CRITICAL — stale UI continuations can start or activate capture after lifecycle stop

Locations: `main_windows.go:432-474,969-1055`; `window_windows.go:300-335,378-420`.

`drainPermissionRequest` posts `wmAppPermissionReady` after releasing the
permission operation without proving the initiating request is still current.
The window handler unconditionally calls `prepareCapture`; `prepareCapture`
does not recheck quit/suspend/lock/shutdown state or a request generation.
Likewise `activateCapture` validates only `captureOp == id`, not whether that
generation has been stopped or a lifecycle barrier is pending.

Concrete quit schedule: permission result wins cancellation and posts READY;
the waiter processes quit, releases all operations, posts cleanup-ready and
exits; READY is then dispatched first and creates a new capture after the only
waiter has gone. Cleanup cannot drain/release it and reaches forced teardown or
a hidden active session. Equivalent schedules exist for suspend and lock.

Required correction: every asynchronous continuation (permission-ready,
capture-ready/activate, picker/discovery continuation where relevant) must
carry and validate a request generation plus the current lifecycle gate before
calling a helper start/activate API. Stop/quit/lock/suspend/shutdown invalidates
pending generations. A stale message is a logged no-op, never a restart.

Required tests: barrier-driven production-seam tests for permission completion
and capture-ready arriving before/after quit, suspend, lock, and shutdown; prove
no helper prepare/activate call and no owned operation survives.

### R3 — CRITICAL — `WM_QUERYENDSESSION` does not establish a shutdown-pending barrier

Locations: `main_windows.go:969-1055`; `window_windows.go:378-420`.

Suspend and lock set explicit flags, but `WM_QUERYENDSESSION` records no pending
shutdown state. If the query observes no capture, a user action or stale async
continuation can start one before confirmed/cancelled `WM_ENDSESSION`. The run
keeps `CaptureExpected=false` and can claim idle cleanup while capture is live.

Required correction: synchronously set `shutdownPending` on query, block all
new work/continuations, keep the direct nonblocking stop semantics required by
Rev16, and clear the gate only on the documented cancelled-shutdown path. A
confirmed shutdown must keep it terminal. Test query→start race, query→cancel,
and query→confirm with and without an existing capture.

### R4 — HIGH — graceful quit can be permanently dropped by the bounded command queue

Locations: `main_windows.go:1338-1348,1375-1386`.

After the first CAS commits `exitGracefulPending`, a full 32-entry channel makes
`enqueue(quit)` fail. The result is ignored; later quit calls lose the CAS and
cannot redeliver. The waiter never sees the terminal intent and ordinary load
degrades to a 30-second unclean exit.

Required correction: terminal intent must be non-droppable and independently
observable on every waiter iteration (or use an equivalent priority path).
Event wake failure must not lose state. Test a saturated queue, wake failure,
and repeated quit; cooperative cleanup must still begin exactly once.

### R5 — HIGH — retry/force paths do not provide a real hard bound

Locations: `main_windows.go:1270-1276,1319-1335,1355-1372`;
`window_windows.go:637-726`.

Successful `CapDestroy` changes `exitState` to complete before evidence sync and
`WM_QUIT`, disabling the watchdog. If either retry timer returns zero, code only
logs and no callback/fallback remains. The idle hotkey retry ignores the timer
result entirely. The force path and exhausted evidence path perform blocking
`Sync` calls on their only exit path; a stalled filesystem can stall forever.

Required correction: retain an independent hard process deadline until
`WM_QUIT`/exit is irrevocably committed; timer scheduling failure must execute a
defined immediate/fallback transition; no potentially unbounded logging/sync
may stand in front of the sole hard exit. Add injectable `SetTimer==0`, repeated
sync failure/stall, hotkey retry, and watchdog-vs-destroy tests.

### R6 — HIGH — accepted waiter-only `CapPermissionCheck` ownership is violated

Locations: `main_windows.go:183,396-430,777,969-1018,1299-1317`.

Rev16 makes all query/read/take/release exports, explicitly including
`CapPermissionCheck`, exclusive to the waiter after runtime ownership begins.
The waiter correctly calls it during drain/promotion, but UI paths call it from
`requestRecord` and the newly added `rearmAfterLifecycle`, potentially in
parallel. The pre-waiter startup check is a separate initialization case.

Required correction: route runtime permission queries through the waiter and
return generation-bound results to the UI; do not weaken the accepted owner
model. Test simultaneous AccessChanged/rearm/record and assert one calling
thread/serialized query ownership.

### R7 — HIGH — `helperInitialized` is a real cross-thread data race

Locations: `main_windows.go:100,1355-1367`; `window_windows.go:672-689`.

The UI thread reads/writes the plain bool without `a.mu`; the watchdog reads it
under `a.mu`. A lock held by only one side is not synchronization. Host race
tests exclude this Windows-tagged path.

Required correction: one lock or atomic for every access, with a portable
extracted-state test and/or executable Windows race coverage.

### R8 — MEDIUM — repeated OS signal evidence is returned but not persisted

Location: `lifecycle.go:185-205`.

For an existing run, `observe` modifies only a copy. The stored signal and
`RepeatedSignal` remain unchanged. For example, cleanup after confirmed
`WM_ENDSESSION` can continue to report the earlier `WM_QUERYENDSESSION`, making
the evidence row contradict the event that committed the disposition.

Required correction: persist an exact latest signal plus ordered/history data
(or another unambiguous representation) and test QUERY→confirmed/cancelled and
repeated suspend/lock sequences.

### R9 — MEDIUM — tray-icon ownership is cleared before deletion succeeds

Location: `window_windows.go:496-515`.

`trayIconAdded=false` is committed before `Shell_NotifyIconW(NIM_DELETE)`. A
failure therefore cannot be retried, yet later exit evidence can imply complete
resource release.

Required correction: clear ownership only after successful deletion, propagate
failure into the retry/terminal decision, and inject one failure followed by a
successful retry.

### R10 — HIGH — current tests cannot substantiate the AC

`lifecycle_test.go` exercises the portable tracker, not the production
cross-thread state transitions above. The claimed 100 cycles do not drive the
actual hotkey/permission/capture/waiter/process ownership seams. Green host
race tests compile out the Windows state that races.

Required correction: extract testable production coordinators/interfaces as
needed and add deterministic schedule tests for every finding above. Preserve
the existing native/signed-hardware gates; do not replace those gates with
mocks or claim hardware evidence from host-only tests.

## Verification reproduced by root

- `go test -race -count=1 ./...`: pass.
- Windows amd64 cross-build, vet, and test compilation: pass.
- XML/manifest/static consistency checks: pass.
- One parallel ordinary-suite run failed in
  `TestSupervisorSpawnRestartStop`; the test passed 10 isolated repetitions and
  a subsequent full sequential run. Treat this as a contention-associated
  anomaly to retain in evidence, not as proof of a lifecycle fix.

These checks establish buildability only. They do not exercise or discharge
R1–R10.

## Acceptance boundary for the next round

Return to review only with: exact changed-file inventory; an invariant table
covering start gates, generations, ownership, and exit states; deterministic
test names mapped to R1–R10; fresh host/race/Windows cross checks; and honest
remaining signed-Windows hardware gates. Do not mark the task done.
