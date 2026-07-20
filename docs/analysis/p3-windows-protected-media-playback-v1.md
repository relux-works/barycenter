# P3 Windows protected-media playback foundation v1

Date: 2026-07-20

Task: `TASK-260712-1u57qz`

## Decision

Pulsar Windows now has a production-dark protected-media playback
orchestration boundary. It authorizes the local policy and witnessed recipient
key state, validates the exact object, recipient, group revision, epoch,
generation and target snapshot, authenticates an encrypted manifest through an
injected provider, and exposes only independently authenticated records to the
existing bounded stream candidate decoder.

No provider, cryptographic library, suite, container, codec, decoder, HTTP
transport, runtime composition or capability advertisement is selected. The
public service constructor accepts only a provider that declares production
approval. The deterministic fixture constructor remains repository-internal
and is rejected by acceptance if referenced by the runtime tree.

## Incremental authenticated playback

Policy, DND and blocked-sender admission precede manifest access. Preparation
loads the recipient identity and group state from the accepted Windows DPAPI
repository, freezes revision, epoch, commit and target witnesses, validates the
bounded route, and requires a live bounded local history grant for an older
epoch. The injected opener authenticates the manifest, envelope, signature and
sender/group context and returns a zeroizing opaque lease.

The accepted Windows stream cache stores only ciphertext plus HMAC-obscured
public integrity metadata. Its authority key now includes the exact
`VariantURL` in addition to opaque identity and ETag. Equal ciphertext served
for two different objects therefore cannot share delete or revocation state.
Cache instances sharing one installation root serialize durable updates,
read-merge-write entries with tombstones as a monotonic union, and use unique
temporary files, so parallel objects cannot erase each other's cache or
revocation state.
Every cached or fetched chunk is SHA-256 verified before provider entry; every
provider result is bounded and revalidated before it reaches the decoder.
Cached ciphertext is authenticated again after restart. Plaintext is returned
only through the in-memory chunk-reader call and is never written by this
boundary. Route byte arrays and manifest slices are defensively cloned across
transport, provider and public prepared-playback ownership boundaries.

The existing `WindowsStreamCandidatePlayer` accepts an optional exact-manifest
chunk reader. Its clear-stream constructor and path are unchanged. Protected
EOF verifies the complete ciphertext object through the protected reader,
while generation, seek, ready/start/skew, bounded PCM ring, local volume,
pause/rebuffer and receipt behavior remains in the accepted player.

## Revocation and recovery

The reader reloads witnessed group state before fetch, after fetch, after
provider authentication and at whole-object completion. It requires the
frozen revision, epoch, commit and target. Historical playback reloads its
grant on every record and requires the exact group and epoch bounds.

Explicit revoke, remote revoke/block, expiry and delete create a monotonic
HMAC-scoped marker before cache tombstoning. The marker binds object, recipient,
group, epoch, generation, target, manifest, stream identity and ETag. Parallel
cache actors and restart therefore fail closed even if one actor held stale
index state. Wrong-target, membership-rotation, ciphertext-corruption and
record-authentication failures invalidate local ciphertext without creating a
permanent marker, allowing a later independently authorized history re-grant.

No error path falls back to plaintext, a legacy contract, coordinator
decryption or an unprotected player.

## Evidence and limitations

Deterministic Go scenarios cover shared macOS/Windows fixture parity,
production disablement, policy-before-network ordering, incremental restart
cache re-authentication, ciphertext and record tamper, downgrade, expiry,
wrong target, bounded history grants, grant revocation, membership rotation,
post-rotation re-grant, monotonic multi-actor/restart revocation, zeroizing
leases and authenticated-reader injection into the bounded player. Focused
stream player/cache regressions and race runs cover the generic path.

These tests are engineering evidence, not a signed-app or physical claim.
Signed MSIX, native DPAPI/NTFS/ACL behavior, real provider/container/codec
interop, memory/crash/swap/backup inspection, network capture, hardware
seek/interrupt/ducking, audible quality and macOS-Windows real-crypto playback
remain `not-run` in `EPIC-260714-th54l3`. Production enablement remains gated
by EPC-001, EPC-002, EPC-004, EPC-005 and `TASK-260712-1ulshp`. Any provider,
protocol, runtime or capability change requires delta review.
