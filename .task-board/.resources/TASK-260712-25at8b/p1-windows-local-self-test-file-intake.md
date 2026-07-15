# P1 Windows local self-test and short-file intake

`TASK-260712-25at8b` supplies the UI-independent Windows domain flow that the
later `TASK-260712-1p8ykc` integration binds to the main window, tray,
AppCapability capture bridge, `FileOpenPicker` and drop data object. It does not
claim that those physical surfaces have already been exercised.

## Offline production-output path

`WindowsProductionLocalClipOutput` prepares and arms the same
`WindowsOverlayMediaClipMixer` used by received media, but calls the mixer
directly instead of entering `MediaClipClient`. Its synthetic in-process
schedule has `ReportStarted` and `ReportEnded` disabled and owns no fetcher,
WebSocket, coordinator, upload or receipt callback. The reviewed builtin cue is
staged into the production MSIX under `Assets/Audio` and validated by exact
format, byte count and digest before the service can be constructed.

The self-test capture request is explicitly classed as `self_test`, so a normal
stop finalizes into restart-disposable `self-tests/*.selftest.wav`, not a
durable user-upload draft. The capture service plays the start cue before it
opens the native stream and the stop cue after it closes and finalizes the
stream. The controller stops the session after exactly five seconds, then plays
only the completed self-test file through the production mixer. Close, delete,
cancel and stale-generation paths remove every owned self-test artifact.

## Brokered file boundary

`WindowsBrokeredAudioFile` carries only a display name, a bounded size hint and
a function that opens a fresh broker-authorized stream. It deliberately carries
no ambient filesystem path. Both picker and drag/drop adapters can construct the
same value after the OS grants access; the later integration task owns those
WinRT/OLE bindings.

Review reopens and content-probes the stream under the 50 MiB source limit. File
extensions never establish type. The current production Windows decoder can
prepare strict RIFF/WAVE PCM16 and float32 input. Accepted WAV is decoded and
canonicalized to bounded 44.1 kHz stereo PCM16 before it enters the opaque,
owner-only durable-draft store. MP3, M4A/AAC, OGG/Opus and FLAC signatures are
recognized but honestly rejected until a reviewed local decoder path exists;
they are never presented as accepted merely because the server supports those
formats.

Phase-one duration is at most 180 seconds and source size is at most 50 MiB;
overlay is offered only through 60 seconds. The review always includes the
broker display name, detected format, actual size, decoded duration, target
audiences, eligible delivery modes, rights reminder and the fact that the
server will probe accepted bytes again. Limit failures point to Phase 2 streamed
tracks; corrupt, truncated, extension-spoofed and locally unsupported files
stay fail-closed.

## Automated evidence and manual boundary

Deterministic tests cover direct mixer scheduling with telemetry disabled,
single-owner cancel/dispose, the exact five-second timer contract, cue ordering,
self-test capture classification, completed-recording playback, stale close,
replacement/delete/close cleanup, content-signature review, strict RIFF bounds,
canonical private intake and release-MSIX cue staging. Full Go vet, race tests
and Windows cross-build are the engineering gate.

No real microphone, speaker, Windows permission prompt, Explorer picker,
drag/drop, packaged AppContainer, clean install or audible result is claimed.
Those observations remain exclusively in `EPIC-260714-th54l3`.
