# P1 macOS capture, self-test and shortcut integration

`TASK-260712-1s6h6t` closes the application-composition seam left by the
accepted macOS shell, capture engine, local self-test/file intake and shortcut
tasks. `MacCaptureAppComposition` is the single app-owned lifecycle boundary;
`MacCaptureWorkflowController` is the single microphone and local-output
operation gate.

## Runtime composition

- Before pairing, a local `AudioEngine` supplies the same production overlay
  mixer/output path without starting librespot or a coordinator client.
- After pairing, the composition is rebuilt on the live `CoreRuntime` audio
  engine. The accountless engine is stopped first, so only one FIFO reader and
  one output graph remain active.
- One TCC-gated `MacMicrophoneCaptureEngine` is shared by normal recording and
  the exact five-second self-test. Events are routed only to the current owner;
  the other workflow cannot start concurrently.
- Normal recording serializes the reviewed start cue, microphone commit,
  finalization and stop cue. A completed owner-only draft becomes visible only
  after the stop cue succeeds. No upload starts in this task.
- The self-test continues to play only a finalized five-second recording and
  deletes its owned draft on close. File review and acceptance retain the
  previously reviewed content/limit/security-scope behavior.

## Shell and lifecycle behavior

The main window, status menu and global hotkey call one action object. The shell
projects requesting/finalizing as `processing`, active recording as
`recording`, fixed failures as `failed`, and clamps both recording and self-test
meters. Input selection is bounded to enumerated CoreAudio devices and persisted
without a path or device label. Output selection remains the existing system
output picker.

The bounded Carbon shortcut presets remain independent of direct buttons. A
conflict or unavailable registration is textual and does not disable the button.
`Esc` remains foreground-only; a hidden active recording has the explicit menu
Cancel action. Sleep/session loss cancels through the accepted shortcut and
capture lifecycle owners. Runtime replacement and quit synchronously stop the
shortcut lifecycle, self-test timer/output, capture engine and local audio graph.

## Local-only and manual boundary

The integration composition contains no coordinator, HTTP, media upload or
transmission client. Automated tests cover cue/capture ordering, device choice,
single-owner exclusion, durable-draft publication after the stop cue, deletion,
idempotent shutdown, localized model projection and forbidden dependency source
guards.

No real TCC prompt, physical microphone, audible cue/playback, CoreAudio route,
global shortcut conflict, sleep/session lock or packaged sandbox result is
claimed. Those observations remain exclusively in manual epic
`EPIC-260714-th54l3`.
