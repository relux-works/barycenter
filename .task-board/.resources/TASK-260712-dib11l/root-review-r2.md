# Root review round 2 — TASK-260712-dib11l

Verdict: **rejected for rework**. Return to `to-dev`; do not mark `done`.

The root reviewer read the complete task-scoped Go, Win32, C++/WinRT, native-test,
MSIX and CI implementation again after the Sol Max rework, compared it with
`windows-capture-bridge-r16.md` and `root-review-r1.md`, and independently reran
every locally available check. The rework fixes many round-one defects, but the
following remaining gaps still allow missing or false evidence.

## Blocking findings

1. **The required production callback/concurrency coverage is still replaced by
   model/seam tests on the hardest paths.**
   `test_activation_cancel_orderings` only checks the return value of
   `PlanActivationCancellation(0/1)`; it never drives the real
   `ActivationHandler`, operation registry, duplicated notification handles or
   either callback/thread cancellation ordering. Likewise,
   `test_unsubscribe_with_inflight_production_handler` constructs a bare
   `PermissionNotificationState` and calls `SignalPermissionNotification`
   directly; it never exercises `CapPermissionSubscribe`, the registered
   `AccessChanged` delegate/token, or `CapPermissionUnsubscribe`. The real export
   barrier test covers PREPARED cancellation, but not an in-flight activation
   callback. This does not satisfy round-one finding 5 or the Rev16 deterministic
   Diagram-A/Diagram-B and unsubscribe-fence tests. Add injectable seams around
   the actual production callback/registration paths and prove registry,
   signal/close, release-after-notify and quiescence behavior there.

2. **`AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR` is detected but never logged.**
   `DrainCapturePackets` sets `PacketDrainResult.timestampErrorObserved`, but the
   capture loop consumes only `cleanupReleaseHResult`; the timestamp flag has no
   production consumer or log sink. The native test merely asserts the boolean.
   Rev16 explicitly requires timestamp errors to be logged and accepted. Record
   this in retrievable scenario/native evidence without changing the terminal
   cause.

3. **Cleanup failures are not retrievable task evidence.**
   `ReleaseBuffer`/`IAudioClient::Stop` diagnostics are retained in
   `CaptureCleanupDiagnostics`, but production emits them only through
   `OutputDebugStringA`. They are absent from `CaptureGetResult` and from the
   application's JSONL scenario log, so a normal packaged run without an
   attached debugger loses the evidence. The outcome therefore overstates that
   these failures are recorded. Preserve the original terminal cause, but route
   the secondary cleanup HRESULTs to a deterministic, retrievable evidence sink
   and test the production route (not only `ExecuteCaptureCleanup` in isolation).

4. **`hiddenDuringCapture` can still be a false positive.**
   `hideMainWindow` sets it whenever `captureOp != 0`, which includes PREPARING
   and ACTIVATING, and showing the window again never clears or qualifies it.
   A user can hide during preparation, restore before `CAP_STATE_CAPTURING`, then
   complete a valid ten-second capture and receive `hiddenDuringCapture=true`
   even though no captured frame overlapped the hidden interval. Gate this
   evidence on positive CAPTURING observation and actual hidden/capturing
   overlap; add deterministic state-transition tests for hide-before-start,
   restore-before-start, hide-during-capture and picker restore.

5. **The tray-only picker window state is not restored on synchronous open
   failure.**
   `requestPicker` records `hideAfterPicker`, restores the main window, then only
   clears/re-hides through the asynchronous terminal path. If `PickerOpen`
   returns a failure (or owner restoration fails before an operation exists),
   no terminal callback runs: the original hidden state remains latched and the
   main window can remain unexpectedly visible. Handle every initiation failure
   in the same state machine and add shell-level tests for the full restore
   truth table.

6. **Most helper result-query failures remain silent and can strand registry
   entries.**
   Permission, enumeration, default-device and picker drains test `state == 0`
   before handling a failed call HRESULT, so a normal failed export (whose output
   structs remain zero-initialized) returns silently. `CaptureGetResult` failures
   are also silently returned. The enumeration `callHR.Failed()` branch is thus
   unreachable for the common zero-state failure. Persistent failure leaves the
   operation unreleased, prevents graceful cleanup, and emits no failure cause,
   contrary to the task AC. Check failed call HRESULTs before interpreting output
   state, log fail/blocked evidence, and define a fail-closed release/abort policy
   with tests.

## Corrections accepted from round 1

- `capture_started=pass` now requires a positively observed CAPTURING state.
- Evidence is clipped at the exact ten-second whole-frame boundary while later
  buffered frames are drained/discarded.
- Missing `AccessChanged` monitoring blocks promotion before a promotable
  sidecar can be created.
- `CaptureRequestStop` no longer takes the activation handoff mutex; the actual
  export barrier covers the PREPARED case.
- Normal/recovered WAVs receive strict final-file verification.
- Startup rollback, required child-control checks, conditional hotkey evidence,
  explicit native exit-code checks, exact manifest capabilities, device-info
  reporting, validation-only identity wording and generated-cache cleanup are
  materially improved.

## Independent root checks

Passed on macOS:

- `git diff --check` and clean `gofmt -d ./cmd ./internal`
- `go test -count=1 ./...`
- `go test -race -count=1 ./internal/winprobe ./cmd/pulsar-win-probe`
- `go vet ./...`
- Windows/amd64 cross-build and `go vet`
- Windows/amd64 test-binary compilation for the helper wrapper and shell
- independent decoder gate for mono, stereo, four- and eight-channel float WAV
- manifest `xmllint` and CI YAML parse

Still unexecuted and unclaimed: MSVC compilation/CTest, PowerShell regression,
MakeAppx/staging, signing, WACK, installed MSIX, and Windows 10/11 hardware.

## Rework handoff

Fix the six findings without weakening the frozen ABI/AppContainer posture.
Return only to `to-review`, attach exact test/evidence limitations, and do not
commit. Root review remains mandatory before acceptance.
