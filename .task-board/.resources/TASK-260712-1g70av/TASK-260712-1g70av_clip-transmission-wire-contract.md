# P1 clip transmission wire contract

Task: `TASK-260712-1g70av` — clip-transmission-wire-contract  
Frozen input: `docs/analysis/p1-transmission-contract-v1.md`

## Accepted implementation boundary

The canonical coordinator codec and both node mirrors now encode and decode the
same additive phase-one messages:

- coordinator to node: `prepare_media`, `play_media_at`, `cancel_media`, and
  `presence_update`;
- node to coordinator: `media_ready`, `media_started`, `media_ended`,
  `media_failed`, `media_cancelled`, and `set_dnd`.

Each type has one versioned envelope fixture in `protocol/golden`. Strict Go
and Windows decoding rejects fields that are not represented by the typed
payload; the Swift semantic round trip removes any unrepresented field and
therefore fails its golden comparison. The interrupt form of `play_media_at`
and omission of delivery-conditional and DND-conditional fields have explicit
compatibility tests in all three mirrors.

`play_voice` and `solo_voice` remain known, encodable and decodable. No new
message replaces or mutates their payload. A mixed-version downgrade can keep
using the legacy Session path for the whole transmission.

Root review also corrected a latent fixture defect: the pre-existing golden
`msg_` values were one or two characters short, two used non-Crockford letters,
and the legacy `el_` examples were not 26-character ULIDs. All 39 envelope IDs
and every element fixture now use the documented prefix plus a valid
26-character Crockford ULID. Recursive Go and Windows guards, plus the Swift
envelope guard, prevent malformed identifiers from returning.

## Capability negotiation

The shared flag vocabulary now includes:

- `interrupt_resume_v1`;
- `media_clip_v1`;
- `overlay_mix_v1`; and
- the existing `seamless_adoption_v1`.

Registration accepts only unique, non-empty printable-ASCII names in strict
byte order. Non-string JSON entries fail typed decoding. Unknown canonical
names are retained for diagnostics but do not satisfy known feature checks.
The authenticated hub passes an immutable capability set to the serialized
coordinator loop, where each reconnect replaces the prior snapshot rather than
unioning it. Approach state copies the exact home snapshots, so later scheduler
work can decide each target's media, overlay and interrupt support without
guessing from app versions.

The current macOS and Windows playback builds still advertise only behavior
they already implement (`seamless_adoption_v1`). Their dedicated client-hook
tasks must add the new flags only together with the corresponding playback
behavior. Receiving a clip command before that point is logged and ignored;
it is never approximated locally through `play_voice`.

## Automated evidence

- Coordinator: `go vet ./...`, `go test ./...`, and race-enabled protocol,
  hub and coordinator-loop tests pass.
- Windows: `go vet ./...`, `go test ./...`, race-enabled wire/player tests and
  `GOOS=windows GOARCH=amd64 go build ./...` pass.
- macOS source: `swift build` passes. Local `swift test` reaches the already
  known workstation toolchain gap `no such module 'Testing'`; the repository's
  authoritative macOS hosted job remains the acceptance source for Swift
  contract tests.
- All 39 golden envelopes round-trip in the Go and Windows mirrors locally;
  the Swift suite enumerates the same 39 files and is exercised in hosted CI.
- Golden identifier guards validate every `msg_`, `el_`, `m_` and `tr_` value
  against its exact prefix and 26-character Crockford ULID shape.
- The Windows mirror remains gofmt-normalized byte-equivalent to the canonical
  coordinator protocol sources.

No real-app, speaker, packaged-install or physical-hardware result is claimed.
Those checks remain in `EPIC-260714-th54l3`.

## Downstream handoff

- `TASK-260712-2qpp6w` may use the persisted target capability booleans and
  strict HTTP resolution without changing this wire vocabulary.
- `TASK-260712-26ip33` and `TASK-260712-2bbz13` own authenticated media fetch,
  hash/decode preparation, generation idempotency, scheduled playback,
  cancellation acknowledgement and advertising the new flags.
- `TASK-260712-31vvjt` owns the barrier, RTT scheduling and lifecycle routing;
  it must use the exact current connection capability snapshot.
- Later DND/presence work owns authorization and projection population; it must
  preserve sorted nodes and capability arrays and the frozen privacy boundary.
