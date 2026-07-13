# TASK-260712-2y74io root review round 2

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — R2 is not accepted**

Root read every R2 production/test/documentation file in full, recomputed every
producer hash, reran the host/race/Windows cross-build checks, and compared the
implementation against the accepted bridge, root R1-R10 contract, task guards,
and `docs/spec-self-contained-audio.md`. Green tests establish buildability but
do not exercise the failure schedules below.

The independent R2 outcome was read in full. Root confirms both of its HIGH
findings (lossy idle-cleanup posting and fail-open AccessChanged query failure)
and adds the independently reproduced settlement loss plus the exit/evidence,
privacy, rearm, and production-test failures below. This contract is cumulative
with `TASK-260712-2y74io_review-r2.md`.

## R2-F1 — observed capture settlement is still lost before stop publication (critical)

Locations: `lifecycle.go:562-586`; `main_windows.go:412-447, 1362-1468`.

The generation and lifecycle registration now share a mutex, but the settlement
ledger is not independent of a run's current cleanup stage. If terminal,
artifact, or release is observed after `beginLifecycle` creates a run at
`signal_observed` but before `requestLifecycleStop`/`drainTerminalIntent`
advances it to `stop_requested`, `advanceCaptureGeneration` rejects the event.
Because any rejection prevents the generation phase from advancing, the
already-observed fact is discarded. `CaptureRelease` may then succeed and the
app clears `captureOp`; after stop publication there is no callback left to
advance the run. The run and generation remain permanently active.

This window is especially large for graceful quit: `beginGracefulQuit` registers
the run on the UI/signal goroutine, while stop publication waits for the next
waiter poll. A similar loss occurs when a stale permission continuation calls
`settleOrCancelCaptureGeneration`: `captureRuns` ignores a bound run that has not
yet reached `stop_requested`, so `cancelCaptureGeneration` clears the generation
without satisfying the run that expects it.

Root reproduced the first schedule with an overlay test against the current
production tracker (no product file changed):

```text
--- FAIL: TestRootSettlementFactSurvivesBeforeStopPublication
    lifecycle_source_test.go:33: observed terminal fact was lost: phase=4, want at least 5
```

`replayCaptureSettlement` also replays only terminal and artifact, not release,
so it does not meet the explicit R1 terminal/artifact/release contract.

Required correction:

- persist monotonic per-generation terminal/artifact/release facts regardless of
  whether each bound lifecycle run is ready to consume them;
- atomically publish the stop intent with lifecycle registration, or replay the
  independent fact ledger after stop publication;
- never clear a generation while a bound run can still require its no-native or
  native settlement proof;
- add barrier tests for every terminal/artifact/release boundary before and
  after registration and before/after stop publication, including graceful quit,
  permission cancellation, overlapping edges, and release followed by N+1.

## R2-F2 — critical idle-cleanup transition is single-shot and lossy (critical)

Locations: `main_windows.go:1434-1436, 1463-1475`; callers at capture/artifact
release; `window_windows.go:356-359`.

`postIdleLifecycleCleanup` attempts one `PostMessageW` and discards the boolean
result. If the window queue cannot accept that message, no loop, timer, or durable
intent retries it. A suspend, lock, permission-revoke, or cancelled-shutdown run
then remains active forever and `UnregisterHotKey` is never attempted. The log
records a PostMessage failure but the process retains the exact resource the AC
says must not leak.

The direct permission-reallowed path at `main_windows.go:500-504` similarly
begins a rearm generation and ignores failure to post its UI continuation,
stranding hotkey rearm without a retry or escalation.

Required correction: make cleanup/rearm UI transitions durable coordinator
state that the waiter/UI loop retries until consumed; provide a defined timer
failure/graceful-exit fallback; test injected `PostMessageW==0` for no-capture,
capture-release, artifact-retry, cancelled-shutdown, and rearm paths.

## R2-F3 — permission-query failure is not fail-closed (high)

Location: `main_windows.go:469-474`.

When `AccessChanged` wakes the waiter and `CapPermissionCheck` fails, the code
only logs and returns. An active or preparing capture may continue recording
while current microphone authorization is unknown. The periodic defensive poll
has the same unsafe behavior. Artifact promotion later being conservative does
not undo recording after a revoke signal.

Required correction: on an AccessChanged query failure, and conservatively on a
runtime query failure while capture is owned, close the permission gate and run
the same generation-bound fail-closed stop/cancel/cleanup path. Test active,
prepare-in-flight, permission-pending, and no-capture states plus query recovery.

## R2-F4 — the hard quit deadline is armed after blocking I/O (critical)

Location: `main_windows.go:1562-1576`.

`requestGracefulQuit` synchronously calls `logLifecycleObservation` before
`exit.beginGraceful` and `time.AfterFunc`. `JSONLogger.Log` writes to the log file
and stderr. If that first write stalls, no watchdog exists, `quittingAt` remains
unset, and the Force Quit menu does not become available. This directly violates
R1-R5's requirement that no potentially unbounded log/sync stand in front of the
sole hard exit.

Required correction: commit exit arbitration and arm the hard process deadline
immediately after the in-memory gate transition, before every logger/filesystem
operation. Add a production-seam test with a blocking logger showing that the
hard-exit callback is already armed and can win.

## R2-F5 — evidence write failures are ignored and clean exit can be fabricated (high)

Locations: `main_windows.go:1329-1393, 1543-1559, 1603-1611`;
`window_windows.go:619-709`.

`probeApp.log` discards every `JSONLogger.Log` error. Lifecycle state advances
before its evidence row is known durable, and an already-advanced stage is not
relogged on retry. A failed signal, cleanup-stage, evidence-synced, or
process-exit write can therefore be absent while later `File.Sync` succeeds and
the state machine commits `WM_QUIT` as if a complete ordered record existed.

Required correction:

- make logger failure sticky and visible to the exit/evidence coordinator;
- never claim `evidence_synced` or a clean process-exit record when a required
  lifecycle row was not written;
- define bounded fail-closed behavior that still permits the hard watchdog to
  terminate the process;
- add injected short-write/error/stall tests at every lifecycle stage and prove
  no passing cleanup claim survives missing evidence.

## R2-F6 — structured logs contain prohibited local paths and original filenames (high/privacy)

Locations: `main_windows.go:194, 907-921, 997-1000` and raw filesystem errors
propagated from artifact cleanup into `FailureCause`/fields.

Startup recovery logs `outcome.Path`; capture terminal and lifecycle artifact
rows log `artifactPath`; picker evidence sets `DeviceName` to `picked.Name`.
Artifact errors include full paths as formatted error text. This violates spec
invariant 3.11 and the explicit observability rule at spec lines 959-960: ordinary
structured events may not contain original filenames or local filesystem paths.

Required correction: log generated/session IDs, sizes, hashes, reason/result,
and path-free error codes only; remove the picker filename; sanitize nested
fields and error text at the logging boundary; add recursive redaction tests for
Windows/POSIX paths, usernames, original filenames, tokens, and audio content.

## R2-F7 — rearm acceptance and start ownership are not one transition (medium)

Locations: `lifecycle.go:231-265, 283-302`; `main_windows.go:1526-1541`.

An active rearm does not block `beginCaptureGeneration`. `applyLifecycleRearm`
mutates global permission state before proving its generation current, clears the
rearm token before discovery/hotkey work, and registers the hotkey outside a
lifecycle-gated callback. A competing record can invalidate rearm and leave the
hotkey permanently unregistered after that capture; a stale permission result
can alter gate state even though its token is rejected.

Required correction: atomically validate generation/status, update permission
state, and claim gated rearm work; either block capture while rearm is in flight
or explicitly hand permission proof to that capture and retry hotkey rearm after
it ends. Add stale/current/reordered rearm, concurrent record, and terminal-edge
barrier tests.

## R2-F8 — R10 production-seam evidence is still missing (high)

Locations: `lifecycle_rework_test.go`, `lifecycle_source_test.go`.

The new tests primarily drive `lifecycleTracker` and isolated trivial helpers,
not the `probeApp` transition wiring. Examples:

- the saturated queue in `TestR4...` is an unrelated local channel;
- the stalled sync in `TestR5...` is an unrelated goroutine and cannot prove
  `requestGracefulQuit` arms the production watchdog before I/O;
- `TestR10...` cycles only the tracker and never drives helper, waiter, hotkey,
  PostMessage, logger, timer, permission, or app-state ownership;
- host race tests compile out both Windows production files.

This is why F1-F7 remain green. Extract narrow production coordinators/adapters
where needed and execute every failure schedule through the same functions used
by Windows code. Fakes may implement test interfaces; no mock/fallback branch may
enter the production package path. Keep native and signed-Windows gates honest.

## Verification reproduced by root

- all producer SHA-256 values matched;
- focused lifecycle suite repeated 50 times: pass;
- full host race suite: pass;
- vet, Windows amd64 cross-build and Windows vet: pass;
- the dedicated invariant overlay test failed exactly as shown in F1;
- native MSVC/MSIX execution and signed Windows 10/11 evidence remain unrun and
  continue as downstream gates.

## Required R3 handoff

Return to review only with exact changed files/hashes, F1-F8 invariant and test
mapping, unabridged verification, explicit remaining platform gaps, a new
independent review, and a new root line-by-line review. Update the LOGBOOK claim
that settlement replay is complete; it is currently false. Do not mark done.
