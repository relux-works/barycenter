# P1 macOS local self-test and short-file intake

`TASK-260712-3lg0ht` implements the offline domain flow and SwiftUI surface for
the macOS **Try locally** destination. Final application lifecycle binding to
the already accepted shell, capture engine and later hotkey controller remains
owned by `TASK-260712-1s6h6t`; no network integration is required for this
feature.

## Offline self-test

- `MacProductionLocalClipOutput` is a local-only facade over the exact
  `MacOverlayMediaClipMixer` instance also used by received clips. It creates no
  transmission payload on the wire, fetch, coordinator callback, upload
  session or telemetry dependency.
- The builtin action plays the reviewed bundled cue directly through that
  output. The microphone action asks the accepted capture engine to begin only
  after the visible Record action, finishes the start cue, schedules stop
  exactly five seconds later, serializes the stop cue, and then plays the
  completed draft through the same output.
- Captured samples are never monitored live. The only playback input is the
  finalized local WAV after capture has stopped.
- Close, explicit delete and cancellation stop the timer/capture/output and
  delete the owned draft. Self-test events contain phases, a bounded meter,
  review metadata, opaque draft handles and fixed failure codes only.

## Picker and drag/drop

The macOS view uses the system audio picker and a URL drop destination. Both
paths first produce a review; neither accepts a file merely because it was
selected or dropped. The review contains the source filename for the current
UI session, detected format, probed duration, byte size, audience choices,
eligible delivery modes, a rights reminder and the explicit requirement that
the server probe remains authoritative.

Phase-one limits are 180 seconds and 50 MiB. Overlay is offered only through 60
seconds. Unsupported, empty, unreadable, over-size and over-duration inputs are
fail-closed and receive honest Phase 2 streaming guidance. Acceptance reopens
security-scoped access, probes again to prevent review/accept drift, and streams
the decoded input through `ExtAudioFile` into a fixed 44.1 kHz stereo PCM16 WAV.
Only the canonical output is finalized as an owner-only durable unsent draft;
the source path, security token and filename are not stored in draft metadata.

## Automated evidence and manual boundary

Swift tests cover complete review projection, every local rejection class,
private canonicalization, the exact default five-second duration, cue/capture/
cue/playback ordering, local-only dependency boundaries and close cleanup. UI
model tests cover meter clamping, EN/RU copy completeness, review state and all
action seams.

No audible-output, real microphone/TCC, Finder picker/drop, selected hardware
route or packaged-app result is claimed by this coding task. Those observations
remain in manual epic `EPIC-260714-th54l3`.
