# Root review round 1 — TASK-260712-dib11l

Verdict: **rejected for rework**. Return to `to-dev`; do not mark `done`.

The root reviewer read every changed implementation/test/build file, compared
the native helper and shell with `windows-capture-bridge-r16.md` plus
`implementation-handoff.md`, and independently reran the locally available
checks. The implementation is a substantial real helper, not a stub, but the
following issues prevent acceptance.

## Blocking correctness and evidence issues

1. **A terminal session can be mislabeled `capture_started=pass`.**
   `drainCapture` creates the artifact and logs `capture_started` when
   `result.State >= CaptureStateCapturing`. That includes stopped/failed/
   cancelled terminal states. A failure after format negotiation but before
   successful capture can therefore create a writer and emit a false pass.
   Separate “observed CAPTURING” from terminal-only drain. A coalesced
   CAPTURING→terminal notification may still require draining buffered frames,
   but must not claim capture-start success without positive evidence. Add
   deterministic tests for (a) fail-before-Start with valid format and (b)
   terminal-first observation with buffered frames.

2. **The ten-second bound is not exact.** The shell writes the full 4096-frame
   read and requests stop only after the artifact is already at/over the bound;
   later drains can add still more frames. Rev16 requires clipping at the last
   whole-frame boundary. Compute remaining frames before each write, write only
   that many, request stop once, and continue draining/discarding as needed
   without extending the artifact. Test the crossing packet and post-stop
   buffered packets.

3. **Permission-monitor failure can still produce a promoted/pass artifact.**
   `PermissionSubscribe` failure is logged as blocked, but capture continues and
   `ArtifactWriter.Finalize` receives only the final `PermissionCheck` value. A
   user-stop with current `Allowed` can therefore promote even though the
   reviewed revoke monitor was unavailable. Until the separately gated fallback
   is hardware-proven, a missing `AppCapability.AccessChanged` monitor must
   block promotion/fail closed. Track the monitor gate explicitly and test it.
   If polling is retained as diagnostic defense, label it as polling rather
   than falsely attributing every 100 ms check to an `AccessChanged` event.

4. **`CaptureRequestStop` is not strictly non-blocking.** After the packed CAS,
   `install_reason` takes `handoff_mutex`; `CaptureActivate` holds that mutex
   across `ActivateAudioInterfaceAsync`. Thus a lifecycle/waiter stop can block
   behind activation, contrary to the frozen non-blocking contract. External
   reasons 0..6 have deterministic HRESULTs and must not need this mutex.
   Preserve reason/HRESULT publication and priority without adding a blocking
   lock to the stop path; add a barrier test proving stop returns while the
   activation handoff lock/call is stalled.

5. **Required production-path lifecycle/concurrency tests are missing.** The
   current shared-pointer “callback fence” test is a toy and does not exercise
   the real operation registry, callbacks, notification publication, or
   teardown. Add injectable production seams and deterministic tests required
   by the handoff/Rev16, including: activation cancel orderings, release
   immediately after terminal notification, callback/thread quiescence,
   unsubscribe with an in-flight handler, PREPARING/PREPARED cancellation,
   duplicate-handle/launch failures, priority/seal races, wrong-thread/init
   rollback/re-init, and the required acquired-packet cleanup branches. Expand
   the picker truth-table tests to all mandatory null/negative/failed/repeat/
   release rows, not only a subset.

6. **Cleanup failures are silently dropped.** `cleanup_capture` discards the
   `IAudioClient::Stop` HRESULT, and `DrainCapturePackets.cleanupReleaseHResult`
   is never consumed. Rev16 requires preserving the original terminal cause
   while recording cleanup `ReleaseBuffer`/`Stop` failures and proving later
   releases still execute. Add a deterministic diagnostic/test hook that does
   not change the frozen public ABI, and cover the release order and failure
   evidence.

7. **WAV promotion lacks the required local post-write verification.** Normal
   finalization and startup recovery return `pass` after header rewrite/rename
   without running a strict local WAV parser over the final file. The independent
   decoder unit test is useful but is not runtime verification of each recovered
   artifact. Add strict final-file verification before pass; on failure remove
   the invalid final artifact and report fail/discard per the accepted matrix.
   Keep the external decoder/hardware gate explicitly unclaimed.

8. **Startup failure paths leak initialized helper state and handles.** After
   `CapInit` succeeds, a later `CreateEvent` failure closes only the log file,
   leaks already-created events, and never calls same-thread `CapDestroy`.
   `createWindows` failure similarly returns without destroying the initialized
   helper. Implement ordered rollback for every partially initialized resource;
   normal cleanup must still destroy the helper before closing Go-owned events.
   Add failure-injection tests for each initialization stage.

9. **Several UI evidence paths can claim pass without proving the action.**
   Child control creation errors are ignored while `controls_ready` checks only
   main-window visibility. `WM_HOTKEY` logs `hotkey_toggle=pass` even when
   recording is denied, unavailable, quitting, or no operation is started/
   stopped. Track all required control handles and return failure on creation
   errors; log hotkey receipt as an attempt and pass only the concrete accepted
   action/result. Record hidden-during-capture for button, close, tray, and
   picker-restore hide paths consistently.

10. **The Windows build gate can be falsely green.** In PowerShell,
    `$ErrorActionPreference = "Stop"` is not a portable guarantee that nonzero
    native process exit codes throw. The script checks only MakeAppx explicitly.
    Check `$LASTEXITCODE` after CMake configure/build, CTest, `go vet`, `go test`,
    and `go build` (or use a helper that throws). Add a script-level regression
    or CI assertion so a deliberately failing native command fails the job.

## Additional required hardening

- Detect and log `WaitForMultipleObjects == WAIT_FAILED`, `GetMessageW == -1`,
  and critical `PostMessageW`/timer failures; do not silently spin or lose the
  only transition message.
- Strengthen manifest assertions to the exact reviewed capability set (the
  existing three network capabilities plus microphone), so future unexpected
  capabilities cannot pass merely because they are not in a short forbidden
  list.
- Do not silently drop individual `CapGetDeviceInfo` errors and then report the
  enumeration result as an unqualified pass.
- Preserve the separate probe identity as a validation target only; state
  clearly that it is not yet the Store product package and does not by itself
  satisfy Partner Center certification.
- Remove generated `.gocache`/`.build` artifacts before handoff.

## Independent root checks completed

Passed on macOS:

- `git diff --check`
- clean `gofmt -d ./cmd ./internal`
- `go test -count=1 ./...`
- `go test -race -count=1 ./internal/winprobe`
- `go vet ./...`
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`
- `GOOS=windows GOARCH=amd64 go vet ./...`
- Windows test-binary cross-compilation for `internal/winprobe` and the probe shell
- `xmllint` manifest parsing and Ruby YAML parsing

Still unexecuted and unclaimed: MSVC native compilation/CTest, PowerShell
staging, MakeAppx, signing, WACK, and Windows 10/11 packaged hardware runs.

## Rework handoff

Fix the issues above against Rev16 without weakening the ABI/AppContainer
posture. Read the full current diff before editing, preserve unrelated user
changes, add regression tests for every fix, rerun all available checks, record
exact limitations, and return the task to `to-review` only. No commit.
