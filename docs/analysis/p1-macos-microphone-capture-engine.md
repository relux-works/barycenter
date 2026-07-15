# P1 macOS microphone capture engine

`TASK-260712-30abcm` adds the UI-independent macOS recording boundary. UI
wiring, upload, self-test playback and physical-device acceptance remain in
their later tasks.

## Contract

- `MacMicrophoneCaptureEngine.begin` accepts only an explicit Record action.
  It asks for TCC microphone access at that point, never during launch or
  passive device enumeration. Denied and restricted states are typed failures
  and leave all non-microphone application paths available.
- `MacAVAudioCaptureBackend` enumerates CoreAudio input-capable devices, marks
  the system default and can bind AVAudioEngine to a selected device. Capture
  is normalized to mono float samples at 48 kHz; the durable draft is PCM16 WAV.
- Samples arriving while the start cue is playing are discarded. The UI calls
  `startCueCompleted` to open the commit gate. On a successful stop the writer
  closes and finalizes exactly one user-recording draft before the stop-cue
  event is emitted, so neither cue can enter the microphone file.
- The local RMS meter is derived only from committed samples and clamped to
  `0...1`. No sample or file is handed to a network component. Upload remains a
  separate, later user action and service boundary.
- Recording is capped at 180 seconds and 50 MiB for the complete WAV, including
  its 44-byte header. Reaching either limit produces a successful draft with an
  explicit `duration_limit` or `byte_limit` terminal reason.
- Main-program output is ducked by 12 dB with a 100 ms attack and restored over
  160 ms. This is capture-time ducking, not acoustic echo cancellation. The
  implementation makes no AEC, noise-suppression or voice-processing claim.

## Terminal-state ownership

One serial lifecycle owner arbitrates stop, cancel, backend failure, input
device loss/configuration change, TCC revocation, system sleep, session resign
(the observable lock boundary) and application quit. A generation token also
prevents a late TCC response from starting capture after cancellation.

Normal stop or a hard limit closes the WAV and atomically promotes one draft.
Every unsafe terminal path stops the backend, restores main-program gain,
closes the writer and removes the partial. Permission is polled once per second
while active because macOS exposes no reliable microphone-revocation event.
The application lifecycle owner must call `shutdown()` on termination.

## Automated evidence and manual boundary

Deterministic Swift tests cover the explicit TCC boundary, denial, selected
device forwarding, cancellation during an open permission prompt, pre-cue
sample exclusion, exactly-one finalization, WAV bounds, local metering, hard
limits and all cleanup reasons. The release build and bundle check cover the
AppKit/AVFAudio/CoreAudio implementation plus the localized
`NSMicrophoneUsageDescription` resources.

No real microphone, audible cue/ducking, real TCC UI, sleep/lock transition,
device unplug, permission revocation, packaged-app or physical-hardware result
is claimed here. Those observations belong to manual epic
`EPIC-260714-th54l3`.
