# P2 macOS streamed-track UI

- Task: `TASK-260712-2psvhu`
- Shared model: `pulsar.stream-track-ui-model.v1`
- Native surface: `node-app/Sources/NodeAppUI/PulsarStreamTrackView.swift`
- Composition and durable intake: `node-app/Sources/NodeApp/MacStreamTrackAppComposition.swift`

## Outcome

The macOS History section now includes a native long-track surface next to the
existing delivery history. A user can choose or drop eligible audio, inspect
its filename, size and state, retain one app-private draft, accept the current
content policy, separately confirm rights for each upload or retry, and perform
an authenticated resumable `audio_track` upload. The surface renders distinct
upload, server-processing and audible-playback progress, current Air and exact
target choices, queue or replace intent, retry, confirmed delete, transport
controls and report input from the shared model.

Queue, replace, pause, seek, resume and report remain visible but fail closed
unless their exact capability commands exist. The accepted codec/player ADR is
still a production no-go. This adapter therefore publishes policy, upload,
retry and delete only, never advertises playback capability, never manufactures
a ready variant and never falls back to the short-clip or Spotify path. A
server-confirmed generic upload remains in processing with
`variant_unavailable` until a future accepted production decoder supplies an
authoritative variant manifest.

## Durable bounded intake and upload

Security-scoped picker and drop URLs are consumed once by
`MacStreamTrackDraftStore`. It copies through a 64 KiB `FileHandle` buffer into
an app-private `0600` file. Bounded JSON metadata is switched atomically, and a
previous draft stays authoritative until its replacement is durable. The
draft ID, confirmed offset and media ID survive process restart.

`PhaseOneAppClient.uploadTrack` repeats the POST with the stable
`track:<local-id>` idempotency key and resumes at the coordinator-owned offset.
It seeks directly in the private file and sends at most 4 MiB in each PUT. The
initial scoped upload token is retained for the request series because
intermediate offset responses intentionally omit it. No intake or upload path
maps, slices or reads the entire track into `Data`.

Filename extension and the 500 MiB local bound are early guards only. The
coordinator remains authoritative for media validation, rights, quota,
processing and readiness. Content-policy acceptance does not imply ownership;
the native confirmation dialog supplies a separate per-attempt rights signal,
and the composition and HTTP client both reject uploads without it.

## Accessibility and automated evidence

- `Command-Shift-L` opens the long-audio picker; file drop enters the same
  durable intake path.
- Standard SwiftUI controls expose visible EN/RU labels, target toggles,
  confirmations and progress accessibility values.
- Shared-model generation fences keep stale pause, seek and resume commands
  from reaching the adapter.
- Deterministic tests cover restart/replacement/delete durability, invalid and
  oversized intake, explicit rights propagation, exact resume offsets,
  intermediate token omission, 4 MiB request bounds and source-level absence
  of whole-file reads in production long-track paths.

Automated coverage is best-effort engineering evidence only. Packaged app
interaction, real VoiceOver announcements, a one-hour audible playback flow,
rebuffer behavior and physical macOS hardware remain in the separate manual
testing epic and are not claimed here.

## Verification

```sh
swift build --package-path node-app --target NodeApp
swift test --package-path node-app
```

The local CommandLineTools installation can build `NodeApp` but lacks the
Swift `Testing` module used by this repository. The complete test command is
therefore executed by hosted macOS CI.
