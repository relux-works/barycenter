# P1 Windows local capture and self-test integration

`TASK-260712-1p8ykc` binds the previously accepted Windows capture, local
self-test, short-file intake, main-window, tray and hotkey seams into one
production composition. The paired and accountless launches construct the same
local workflow; coordinator state changes presentation only and is not required
to record, review or play local audio.

## Single workflow owner

`WindowsCaptureWorkflowController` is the state owner used by the window, tray,
foreground Escape accelerator and `RegisterHotKey`. It serializes normal
recording and the five-second self-test before native permission/capture work is
entered. Both use the selected AppCapability input, expose the same bounded
meter projection and share the cue/capture service, so a second permission
prompt or WASAPI stream cannot race the first.

Normal stop produces an owner-only durable unsent draft. Self-test stop remains
exactly five seconds and produces a disposable `self_test` draft that is played
through `WindowsOverlayMediaClipMixer`, then removed on replace, delete, Escape,
lock, suspend or quit. Start/stop cue playback uses the reviewed packaged WAV
through that same production mixer. No capture callback performs upload,
filesystem import or UI work.

## Device, output and file boundaries

Capture inputs come from the signed helper's
`DeviceInformation.FindAllAsync(AudioCapture)` ABI and are selected by stable
ID. Render outputs come from active `IMMDevice` endpoints; selecting the next
output drains the current event-driven WASAPI loop before opening the new
endpoint, preventing two render owners. Both names and the live input level are
projected textually in EN/RU.

The standard `FileOpenPicker` adapter transfers an
`IStorageItemHandleAccess` handle into `WindowsBrokeredAudioFile`; the domain
layer never receives a path. Explorer drop is a fallback adapter: it consumes
the one exact shell-granted item and exposes only its basename, size and bounded
open function to the same intake. Content probing, duration/size limits,
canonicalization and private draft storage remain identical for picker and
drop. Hotkey conflict or unavailability leaves window/tray buttons enabled and
truthfully labelled.

## Shutdown and evidence boundary

Shutdown first prevents new workflow operations, cancels picker/permission
contexts, stops capture/self-test, waits for recording and file operations,
unsubscribes the permission event, closes the native handle, then stops output.
Lock, suspend and permission revocation use typed stop reasons and cannot leave
a reusable temporary self-test artifact.

Portable unit and race tests cover workflow exclusion, selected-device request
projection, brokered intake, shutdown cancellation/drain, draft cleanup and
source-level Win32 wiring. Windows cross-build validates the production files.
No physical permission prompt, microphone, speaker, audible result, packaged
AppContainer, Explorer interaction, shortcut conflict, lock or suspend
observation is claimed here; all such manual evidence remains exclusively in
`EPIC-260714-th54l3`.
