# P1 Windows UI and data integration

## Accepted engineering boundary

The Windows shell now binds accountless Create and Join to the accepted
self-service identity API and the DPAPI credential repository. Create keeps one
installation attempt across retry and does not activate the installation until
the one-time recovery JSON is written through an explicit Save dialog and its
exact actor/recovery metadata is acknowledged. Join activates only after the
protected bundle save succeeds. An already paired installation presents its
identity as active and does not issue a destructive replacement request.

The paired shell binds finalized microphone recordings and explicitly selected
short-audio files to the Phase 1 media, transmission, presence and history HTTP
contracts. File selection has a dedicated outgoing action; it does not reuse or
promote the disposable local self-test draft. This client:

- accepts only an active, origin-bound control capability;
- rejects cross-origin redirects and credential query parameters;
- uses the upload capability only for the exact resumable PUT;
- requires a completed upload response before local byte cleanup;
- exposes requested and effective delivery separately, including downgrade;
- keeps interrupt fallback tokens memory-only and requires an explicit
  “Confirm: after current” action before resubmitting the frozen request;
- renders canonical route names rather than opaque server identifiers;
- executes only history actions advertised for the selected item.

## Durable draft rule

`phase-one/draft-outbox-v1.json` contains no path or credential. It freezes the
first route and requested delivery and derives stable Windows upload and
transmission idempotency keys from the opaque local draft ID. Metadata is
written owner-only through a synced temporary file, atomic rename and parent
directory sync before network work. It also freezes `microphone` versus `file`
provenance so a picked-file retry remains a picked-file transmission after an
outage or process restart.

Short-lived interrupt confirmation tokens are deliberately excluded from that
file. After restart, retry obtains a fresh challenge; it never silently assumes
consent from an expired or lost token.

An upload failure retains the finalized WAV. A completed server upload is
persisted before local deletion. If the process stops or cleanup fails at that
boundary, restart retries cleanup without another upload and does not transmit
until the retained confirmed bytes are handled honestly. A transmission retry
uses the same media ID and transmission key. Explicit delete removes remote
media first when it exists, then only the exact app-owned local draft.

Self-test media is excluded twice: capture recovery publishes only
`user_recording/durable_unsent` handles, and the outbox rejects every other
class/state. Both microphone and picked-file outgoing drafts enter through that
same durable handle boundary.

## UI projection

The History surface provides:

- This Pulsar, My Barycenter and Current air routing;
- Overlay, Interrupt and After current delivery choices;
- outgoing draft selection, send/retry and explicit delete;
- presence, history and receipt status with degraded/error codes;
- delete, replay and block only when advertised by the selected history item.

EN/RU copy uses titles, sender display names and canonical labels. Public IDs
remain internal composition keys and are never used as user-facing labels.

## Automated evidence

Portable tests cover authenticated request shape, redirect/configuration
failure, durable restart/retry/delete, confirmed-upload cleanup after restart,
self-test exclusion, identity attempt/recovery activation, canonical shell
projection and UI action availability. The repository verification also runs
Go race tests plus Windows amd64 vet/build. These are deterministic engineering
checks; no real Windows UI, DPAPI prompt, microphone, speaker, network outage,
AppContainer or physical-hardware result is claimed here. Those observations
remain in `EPIC-260714-th54l3`.

The accepted local matrix is:

- `pulsar-win`: `go test ./...`, `go test -race ./...`, `go vet ./...`, plus
  Windows amd64 cross-`vet` and cross-`build`;
- `coordinator`: `go test ./...` and `go vet ./...`;
- `node-app`: Xcode-toolchain `swift test`, 211 tests in 35 suites.

PlantUML is not installed in the execution environment, so the two diagram
sources were reviewed as text but no rendered-diagram claim is made.
