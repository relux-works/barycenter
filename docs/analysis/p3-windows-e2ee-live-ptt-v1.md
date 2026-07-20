# Windows E2EE live PTT production-dark bridge

`TASK-260712-39vjzd` adds the bounded Windows client seam that protects an Opus
live frame before it becomes coordinator-visible and authenticates it before
the existing jitter, Opus FEC/PLC and PCM path. It does not enable E2EE, select
a provider or suite, wire a runtime, or advertise a capability.

## Accepted wire and cross-platform context

The bridge mirrors the reviewed `BE` opaque-live authority in
`coordinator/internal/e2eecontract/opaque_live.go`: 84 public routing bytes and
at most 512 opaque ciphertext bytes. The fixed Windows vector is byte-identical
to the accepted macOS vector. The legacy plaintext `BP` envelope stays a
different contract and is never accepted by the protected decoder.

The cross-device AAD is also intentionally identical to the accepted macOS
model. It binds the shared witnessed epoch and commit digest, not the
installation-local repository revision. Sender and receiver installations may
therefore have different local CAS revisions while still deriving the same
provider context. A two-repository fixture proves this round trip.

## Witnessed key and generation boundary

`WindowsE2EELiveSessionFactory` loads the existing DPAPI-backed device and group
leases, verifies exact target and local revision, reserves a crash-safe
`live_ptt` generation under the repository's cross-process share-none lock,
reloads the advanced record, and only then calls an injected deriver. The
deriver receives the exact identity lease, group-state lease, shared epoch,
commit, generation, target, sender, Air, codec and timing context.

The public factory and channel reject a provider that does not declare prior
production approval. The repository-only fixture constructor remains
unexported. There is no provider, KDF, AEAD, nonce algorithm, library, runtime,
UI or capability selection in this task.

## Sender and receiver placement

The existing microphone worker only reads and encodes into its bounded frame
queue. Transport retries happen on `transportWorker`; the protected sender
bridge is an injectable replacement for that worker's non-blocking
`trySendFrame` function. Sealing therefore never enters the WASAPI capture
callback or reader loop. A retry of the same frame returns the exact cached
ciphertext and nonce without resealing.

The receiver bridge decodes `BE`, verifies current in-memory authorization and
opens/authenticates the provider record before calling
`WindowsLiveJitterReceiver.Receive`. Tamper, replay, nonce reuse, stale epoch,
changed commit or target, and removed sender terminate the channel, destroy the
provider session once, revoke buffered playback and never reach Opus. The
existing eight-frame reorder window, 60 ms prebuffer, FEC, PLC, DND/policy
admission, backpressure and teardown owners are unchanged.

## Bounds, ownership and darkness

Every frame is constrained to 20 ms, 400 plaintext bytes, 512 ciphertext bytes,
15,000 sequences and the frozen capture-time progression. Provider inputs and
outputs, caller frames, cached retry frames and decoded plaintext use defensive
copies. Service-owned plaintext is zeroed after provider use; no key, plaintext,
nonce or live ciphertext is written to durable client state.

Automated fixtures prove exact wire parity, retry idempotence, ciphertext-only
coordinator visibility, authentication-before-jitter, tamper/replay/nonce
failure, provider-output bounds, epoch/membership teardown, AAD sensitivity,
witnessed generation reservation, cross-installation revision skew and bounded
incoming reorder. They use an explicit audit transform, not a production
cipher.

Signed MSIX behavior, native DPAPI/NTFS inspection, real provider and codec,
microphone/speaker quality, C1/C2 latency, coordinator packet capture, process
memory/crash/swap inspection and macOS-Windows hardware interop remain `not-run`
in manual epic `EPIC-260714-th54l3`. Runtime composition and production claims
remain forbidden while the production crypto and external security gates are
open.
