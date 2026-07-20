# P3 macOS protected-media send foundation v1

Date: 2026-07-20

Task: `TASK-260712-2kcduo`

## Decision

NodeCore now contains a production-dark macOS protected-media sending
pipeline. It owns the ordering and persistence boundary around local
preparation, exact target confirmation, key-state generation reservation,
ciphertext staging, resumable upload, finalization and terminal cleanup. It is
not connected to the application composition root and cannot advertise
`e2ee_media_v1`.

The pipeline intentionally does not select or implement a codec, container,
cipher suite, group-crypto library or signature implementation. Those
operations remain behind `MacProtectedMediaSealing`. The public constructor
accepts only a provider that declares itself production-approved; the sole
path for an unapproved deterministic provider is an internal repository-test
constructor. This preserves the existing EPC-001/EPC-002 no-go decision while
making the orchestration, failure and recovery behavior reviewable.

## Send order and immutable bindings

For a new draft the actor performs the following order:

1. require an explicit rights acknowledgement and exact target confirmation;
2. reject an unverified, removed or unsupported recipient without a plaintext
   fallback;
3. fingerprint the bounded source and validate app-private ownership policy;
4. load the witnessed group state and compare its exact revision and target
   snapshot;
5. reserve one `media` send generation in `MacE2EEKeyStateRepository` before
   asking the provider for nonces or ciphertext;
6. reload the group state at the reservation revision and pass short-lived
   identity/group leases to the provider;
7. validate the provider's exact context, unique nonces, bounds and provider
   authentication result;
8. atomically persist only encrypted manifest, opaque envelopes,
   authenticated public metadata, signature and ciphertext chunks;
9. stage, upload contiguous immutable chunks, and finalize using stable
   per-operation idempotency keys; and
10. remove the ciphertext staging directory and, for an app-owned draft, its
    plaintext only after confirmed publication.

The generation reservation is consumed even when sealing later fails. A
retry after an interrupted upload reloads the exact persisted ciphertext,
checks the source fingerprint, verifies the sealed artifact again and starts
at the first unacknowledged chunk. It does not invoke the sealer or reserve a
new generation. Repeating a chunk after an ambiguous response is safe only
under the uploader protocol's exact-byte idempotency contract.

## Plaintext and cleanup policy

Input is a regular local file no larger than 64 MiB. NodeCore reads it only to
calculate a streaming SHA-256 consistency fingerprint; it never writes a
plaintext copy. The future provider must stream/prepare locally within its own
independent review boundary.

Two explicit policies exist:

- `user_owned_retain` never deletes a user-selected external file;
- `app_private_delete_on_terminal` is allowed only for a path resolving below
  the configured private plaintext-draft root and deletes it after confirmed
  publication, explicit cancellation, or bounded expiry recovery.

Ciphertext draft directories use mode 0700 and files use mode 0600. Draft
retention is at most 24 hours. One recovery call inspects at most 100 expired
drafts. Explicit cancellation first requests idempotent remote deletion when
a staged object exists, then performs local terminal cleanup. Interrupted
network operations intentionally retain the bounded draft for resume and are
not treated as explicit user cancellation.

`MacE2EEKeyStateRepository` now grants a non-releasable protected-send owner
claim. A runtime composition must create one repository and one send actor;
attempting to construct a second sender over the same repository fails before
any work. This prevents two in-process send owners from double-preparing one
generation. The pipeline is not runtime-wired, so cross-process serialization
is still a production integration gate if future packaging introduces more
than one process with Keychain access.

## Evidence and limitations

The shared repository vector file freezes the deterministic fixture manifest,
chunk offsets, digests, nonce uniqueness, resume expectations and fail-closed
outcomes. Twelve macOS unit scenarios cover clip/track/saved-cue routing,
production-disablement, unsupported targets, duplicate nonces, invalid
provider authentication, source/ciphertext tamper, interruption/resume,
explicit cancel, expiry recovery, user-owned retention and single ownership.
The fixture transforms bytes only to exercise orchestration; it is not a
cipher and is inaccessible to the app target.

No coordinator HTTP endpoint is registered by this task. The existing dormant
opaque router receives only the modeled ciphertext fields and has no decoder
or ffmpeg path. There is no signed/notarized application, real Keychain
accessibility result, real codec/container, real cryptographic
interoperability, hardware evidence or product E2EE claim. Those remain in
`EPIC-260714-th54l3` and the open EPC/TASK gates. Any provider selection,
runtime wiring, protocol-field change or capability advertisement requires a
delta design review and the independent implementation review
`TASK-260712-1ulshp`.
