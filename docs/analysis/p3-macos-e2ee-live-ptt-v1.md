# macOS E2EE live PTT production-dark bridge

`TASK-260712-3980vy` adds the bounded macOS client seam required to encrypt an
Opus live frame before it becomes coordinator-visible and to authenticate it
before the jitter/decoder path. It does not enable E2EE, select a cryptographic
provider, or advertise an E2EE capability.

## Accepted wire, no protocol delta

The bridge mirrors the already reviewed `BE` opaque-live frame in
`coordinator/internal/e2eecontract/opaque_live.go`: 84 public routing bytes and
at most 512 opaque ciphertext bytes. It is intentionally distinct from the
legacy plaintext `BP` frame. The coordinator can validate session, epoch,
generation, target, sequence, timing, size, rate, and recipient lineage, but it
does not receive a key or plaintext and does not persist live frame payloads.

The Swift encoder/decoder is covered by a fixed byte vector. No field, size, or
flag was added to the reviewed wire authority, so this task does not reopen the
design review. A future change to that authority still requires delta review.

## Key and context boundary

`MacE2EELiveSessionFactory` loads independently witnessed device and group
records from `MacE2EEKeyStateRepository`, checks the immutable target and group
revision, reserves a crash-safe generation in the `live_ptt` domain, reloads
the advanced state, and only then asks an injected reviewed provider to derive
the session. The provider receives the exact identity lease, group-state lease,
epoch, generation, target, sender, Air, codec, and timing context. NodeCore does
not implement a candidate KDF, AEAD, MLS library, or nonce encoding.

The app cannot construct the audit-fixture path. A production factory requires
both a provider that declares independent approval and an explicit composition
attestation that generation reservation is serialized across processes. The
repository also allows only one live reservation owner per loaded instance and
never releases that claim. There is no NodeApp composition while EPC-001/002
and the implementation-specific cross-process review remain open.

## Frame path and real-time bounds

The existing microphone callback still performs only a bounded mailbox offer.
Opus encoding, frame protection, and transport already run on the sender's
serial worker. `MacE2EELiveSenderBridge` therefore encrypts off the capture
callback. If transport applies backpressure, the channel returns the exact
cached ciphertext and nonce for the same source frame instead of resealing it.

On receive, `MacE2EELiveReceiverBridge` decodes the public `BE` envelope and
opens/authenticates it before calling `MacLiveJitterReceiving.receive`. A
tampered, replayed, stale, foreign-target, removed-sender, or nonce-reused frame
never reaches Opus, FEC, PLC, or the PCM ring. The legacy receiver continues to
own jitter, 60 ms prebuffer, FEC/PLC, DND decisions, PCM bounds, and teardown.

Every frame rechecks a bounded in-memory authorization snapshot produced by a
verified control-plane transition. An epoch, group revision, target, or sender
membership change terminates the channel, destroys its provider session once,
and revokes buffered playback. Rekey is deliberately not attempted mid-session;
the next session must reserve a new generation against the new epoch.

## Authenticated data and nonce policy

Canonical length-prefixed/fixed-width AAD binds the contract, group, sender
device/actor/orbit/node, Air domain and ID, immutable target, session, epoch,
group revision, generation, sequence, flags, monotonic capture time, codec,
frame size, jitter size, and maximum duration. The provider contract requires
its opaque ciphertext to carry and authenticate its nonce and returns a stable
nonce token so NodeCore can independently reject reuse in either direction.

Outgoing sequence is contiguous. Incoming sequence may reorder only inside the
existing eight-frame window so Opus FEC remains possible. Retry is idempotent;
replay terminates and revokes. Nothing in this path writes plaintext, keys,
nonces, or live ciphertext to a durable cache.

## Evidence and remaining gates

Automated fixtures prove byte-exact `BE` encoding, witnessed epoch derivation,
unique live generation reservation, AAD sensitivity, retry idempotence,
ciphertext-only coordinator visibility, authentication-before-jitter, and
fail-closed tamper/replay/nonce/membership behavior. The fixture transform is
explicitly not a production cipher.

Real speech capture, C1/C2 latency and quality, memory/crash inspection,
cross-process contention, signed/notarized package behavior, macOS/Windows
interop, and packet capture on hardware remain in manual epic
`EPIC-260714-th54l3`. EPC-001, EPC-002, EPC-004, EPC-005 and external security
closure remain open; consequently runtime wiring, UI claims, and capability
advertisement remain forbidden.
