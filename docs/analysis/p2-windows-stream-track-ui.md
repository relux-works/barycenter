# P2 Windows streamed-track UI

- Task: `TASK-260712-3lximx`
- Shared model: `pulsar.stream-track-ui-model.v1`
- Native surface: `pulsar-win/main_window_windows.go`
- Composition and durable intake: `pulsar-win/windows_stream_track.go`

## Outcome

The Windows History section now exposes a native long-track surface alongside
the existing clip and Spotify/history controls. A user can pick or drop an
eligible audio file, retain an app-private draft, review the current content
policy, perform an authenticated resumable `audio_track` upload, refresh the
projection, select a capable explicit target or current Air, choose queue or
replace intent, and see distinct upload, processing and audible-playback
progress. Delete is explicitly confirmed. Retry reuses the same local draft ID
and therefore the same coordinator idempotency key.

Queue, replace, pause, seek, resume and report are concrete native buttons with
stable EN/RU names and tab traversal. They are enabled only by exact actions in
the shared projection. The accepted codec/player ADR remains a production
no-go, so the current adapter publishes upload/policy/delete/retry only. The UI
therefore shows `variant_unavailable` after a server-confirmed upload and keeps
all playback controls disabled. It does not manufacture a variant, advertise
`stream_track_v1`, or fall back to a whole-file clip.

## Bounded and crash-resumable file handling

`FileOpenPicker` and `WM_DROPFILES` both enter through
`WindowsBrokeredAudioFile`. The long-track store consumes that capability once
with a 64 KiB copy buffer, writes an app-private `0600` data file and atomically
switches a bounded metadata record. The old draft remains authoritative until
the replacement metadata is durable. No original broad path enters the shell
snapshot or UI.

The upload adapter creates an `audio_track` session and streams at most 4 MiB
per PUT. Each process restart repeats the POST with `track:<local-id>`; the
coordinator returns its durable upload offset and the client continues from
that exact byte. Intermediate responses intentionally contain no upload token,
so the client retains the session credential in memory for the request series.
Draft bytes and the last confirmed offset survive network failure and restart.

Server validation remains authoritative for type, duration, rights, quota and
processing. Filename extension and the 500 MiB limit are only early intake
guards. A local MIME guess, completed upload or filename never marks a track
ready; readiness still requires server metadata and a production variant
manifest.

## Accessibility and automated evidence

- `Ctrl+Shift+L` opens long-track intake from any section and moves the user to
  the same command surface used by drag/drop.
- Every operation is a standard Win32 button with visible EN/RU action text;
  the existing `IsDialogMessageW` loop supplies keyboard tab traversal.
- Layout is computed in DIPs and tested without overlap at 96, 144 and 192 DPI.
- Tests use a reader that rejects requests above 64 KiB and a scripted upload
  transport that verifies exact 4 MiB request bodies, resume offsets and final
  byte progress.
- Projection tests prove that local, media and target capabilities are not
  rendered into user-facing text.

Automated coverage is best-effort engineering evidence only. Packaged MSIX
interaction, Narrator announcements, real high-DPI monitors, a one-hour file,
audible seek/rebuffer behavior and real Windows hardware remain in the separate
manual-testing epic and are not claimed here.

## Verification

```sh
cd pulsar-win
go test ./...
go test -race ./...
go vet ./...
GOOS=windows GOARCH=amd64 go test -c -o /tmp/pulsar-win.test.exe .
```
