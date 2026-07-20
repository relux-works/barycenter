# P3 macOS protected-media playback foundation v1

Date: 2026-07-20

Task: `TASK-260712-tcwn44`

## Decision

NodeCore now has a production-dark protected-media playback orchestration
boundary. It authenticates the requested object, recipient, group revision,
epoch, generation and exact target snapshot before exposing a range reader to
the existing bounded macOS streamed-track player. No production cryptographic
provider, codec, container or decoder is selected, and the service is not
wired into `NodeApp` or capability advertisement.

The public constructor accepts only a provider whose independent implementation
declares itself production-approved. The deterministic byte-transform fixture
is reachable solely through an internal test constructor and is explicitly not
cryptography. This preserves EPC-001/EPC-002 and the external security review
gate while making lifecycle and failure behavior executable.

## Authenticated incremental path

Preparation loads witnessed device and group state, freezes the group revision,
fetches a bounded encrypted manifest route and checks its exact request binding.
For a historical epoch it requires a live local grant whose group and epoch
range cover the route. The injected provider authenticates the manifest,
envelope, sender/group context and signature and returns a short-lived,
zeroizing opaque lease.

`MacStreamChunkCache` remains the sole durable range cache. It stores only the
route's ciphertext chunks and public integrity metadata, with the existing
1 MiB request/chunk, 64 MiB variant, 512 MiB global and 128 MiB pinned bounds.
Every cache hit is passed back through `authenticateAndDecrypt`; only its
successful result is returned to a decoder. Hash failure happens before the
provider, and record-authentication failure invalidates cached ciphertext.
Decrypted bytes are never written to disk. Concurrent cache actors sharing an
installation root serialize index mutations, refresh and merge durable entries,
and treat tombstones as a monotonic union. Unique temporary names avoid
same-root write collisions; one active track therefore cannot erase another
clip's revocation before restart.

`MacStreamCandidatePlayer` gained an optional injected chunk reader. Its normal
clear streamed-track path is unchanged. A protected player refuses a manifest
different from the injected reader and retains existing generation, seek,
ready/start/skew, bounded PCM ring, volume, pause/rebuffer and receipt behavior.
The decoder still has no credential, transport or cache-directory access.

## Revocation and state changes

Before and after each fetch and decrypt, the reader reloads witnessed group
state and requires the frozen revision, epoch and target snapshot. Historical
reads also reload the grant, require it to remain unexpired and cover the exact
group/epoch range, and fail closed after local revocation. Membership rotation
therefore fails closed even after preparation. Explicit revocation, remote
revoked responses remove cached chunks and leave a tombstone that survives
restart. Expiry, membership rotation, missing history grants and provider
authentication failures invalidate local bytes without permanently blocking a
later authorized retry. A candidate player retains the prepared playback owner
for the decoder lifetime, so dropping the caller's wrapper cannot revoke a live
stream.

Policy/DND/blocked-sender checks happen before manifest or range access. The
route refuses legacy contract/capability downgrade, future epochs, wrong
recipient/target/generation, malformed ranges and over-limit manifests. There
is no plaintext fallback or mixed-version capability downgrade.

## Evidence and limitations

Nine serialized Swift scenarios cover production disablement, Mac/Windows
shared fixture chunks without full download, restart cache re-authentication,
ciphertext and record tamper, downgrade/expiry/target/policy failures, bounded
history grants, membership change, explicit revocation persistence and
injection into the bounded player. The shared vector freezes ciphertext and
authenticated-output digests and labels both platform producers as fixtures,
not real interoperability evidence.

Real cryptographic Mac/Windows vectors, a selected codec/container, signed and
notarized Keychain/crash evidence, physical hardware playback/seek/interrupt
quality and audible verification remain manual work in `EPIC-260714-th54l3`.
Production enablement also remains gated by EPC-001, EPC-002, EPC-004, EPC-005
and `TASK-260712-1ulshp`. Any provider selection, runtime wiring, protocol field
change or capability advertisement requires delta review.
