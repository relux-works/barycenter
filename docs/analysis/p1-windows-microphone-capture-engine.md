# P1 Windows microphone capture engine

- Task: `TASK-260712-2w4gyw`
- Engineering boundary: AppCapability/WASAPI native bridge plus a UI- and
  network-independent Go lifecycle
- Manual evidence: `EPIC-260714-th54l3`

## Contract

`WindowsMicrophoneCaptureService.Start` rejects every request that is not
marked as an explicit user Record action. Only an accepted explicit action may
call `CapPermissionCheck` and, when required, `AppCapability.RequestAccessAsync`.
Denial, a missing declaration and an unavailable capability are typed failures
and create no media path.

The production backend reuses the signed-probe `pulsar-capture.dll` ABI selected
by the foundation spike. It resolves the default input through
`MediaDevice.GetDefaultAudioCaptureId` or forwards a caller-selected endpoint,
then owns `CapturePrepare`, `ActivateAudioInterfaceAsync`, event-driven WASAPI
reads, terminal observation and `CaptureRelease`. The production MSIX now
declares `microphone` and stages this DLL next to the CGO-free Go executable.

Native float frames are downmixed and normalized to 48 kHz mono PCM16. The
complete WAV is capped at 180 seconds and 50 MiB including its 44-byte header.
The duration cap currently dominates the byte cap at this canonical format;
both remain independently enforced so a later format change cannot silently
remove the storage boundary. A hard stop returns `duration_limit` or
`size_limit`, not an ambiguous success.

## Privacy and lifecycle ownership

The start cue completes before the native stream opens. This is a stronger
commit gate than filtering already-open frames and makes cue contamination
impossible through this service. On normal stop, the service closes and syncs
the WAV, atomically promotes exactly one `durable_unsent` private draft, and
only then plays the stop cue. Samples feed only a bounded local RMS callback;
the backend and service expose no network/export seam.

Capture ducks the local main program to 0.25 (approximately -12 dB) and restores
it after every terminal path. This is output ducking only: no acoustic echo
cancellation, noise suppression or voice-processing claim is made.

One service reservation spans permission, device resolution, native open and
active capture. Cancel, quit, session lock, suspend, device loss and permission
revocation cancel a pending operation or stop the active native owner. These
unsafe paths close the stream and remove the private partial; they never emit a
stop cue or create a draft. Overflow, format/discontinuity and WASAPI failures
are typed backend failures and follow the same fail-closed path.

## Automated and manual boundary

Portable Go tests cover the explicit permission boundary, denial, selected
input forwarding, permission/cue/open order, stereo downmix, sample-rate
conversion, local metering, ducking, complete WAV finalization, exactly-one
draft semantics, all unsafe lifecycle reasons and the 180-second hard limit.
The Windows amd64 build and vet pass against the real dynamic-loader adapter;
the existing signed-probe CI compiles and tests the same native helper.

No real microphone, Windows permission dialog, audible cue/ducking, hidden
window capture, endpoint unplug, permission revocation, session lock/suspend or
signed Windows 10/11 result is claimed here. Those observations remain in the
manual-only test epic `EPIC-260714-th54l3`.
