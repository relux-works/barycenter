# P3 Windows protected-media send foundation v1

Date: 2026-07-20

Task: `TASK-260712-28zhpl`

## Decision

`pulsar-win` now contains a production-dark protected-media send boundary for
recorded clips, selected tracks and saved-cue media. It owns exact-target
admission, witnessed Windows key-state reservation, bounded local preparation,
ciphertext-only crash state, resumable upload, finalization and terminal
cleanup. It is absent from `main.go`, the Windows shell/composition files,
capability advertisement and every existing plaintext upload path.

The boundary does not select a codec, container, cipher suite, group-crypto
library, key-wrap or signature implementation. Those operations remain behind
`WindowsProtectedMediaSealer`. The exported constructor can send only when an
injected provider declares independent production approval; the deterministic
provider used by unit tests is reachable only through an unexported audit
constructor. No approved production provider exists while EPC-001, EPC-002,
EPC-004 and EPC-005 remain open.

## Ordered send and immutable retry

For a new draft the service:

1. requires the rights reminder and exact target confirmation;
2. rejects removed, unverified or unsupported recipients before key-state
   mutation and never falls back to plaintext;
3. resolves a regular source file, reads at most 64 MiB into bounded local
   memory, fingerprints it and never writes a plaintext copy;
4. loads the current-user-DPAPI identity/group witnesses and checks the exact
   local revision and immutable target digest;
5. reserves one `media` generation through
   `WindowsE2EEKeyStateRepository`, whose process lock plus native share-none
   repository lock serializes the read/increment/write across processes;
6. reloads the exact reservation revision and passes short-lived identity and
   group leases plus plaintext bytes to the injected provider;
7. zeroes the service-owned plaintext buffer after the provider returns,
   validates the exact group/epoch/generation/target/recipient context, unique
   nonces and byte bounds, and requires provider authentication before any
   ciphertext persistence;
8. writes only encrypted/authenticated manifest material, opaque envelopes,
   signature, digests and ciphertext chunks into a private draft directory;
9. stages, uploads and finalizes through stable exact-byte idempotency keys;
   and
10. after confirmed publication removes local ciphertext and removes plaintext
    only when it is an app-owned path below the configured private root.

A generation remains consumed after provider cancellation, malformed output or
failed authentication. An interrupted upload reloads the exact saved chunks,
checks every offset/chunk/whole digest and strict state schema, re-fingerprints
the source, and re-verifies the artifact without resealing or reserving another
generation. The finalized object revision is durably checkpointed before
terminal cleanup, so a cleanup retry never finalizes twice and cancellation
uses the current remote revision. Resume also rebinds author identity and the current witnessed
epoch, commit digest and target snapshot. A later epoch/commit or target change
therefore fails closed instead of publishing stale ciphertext.

## Cleanup and concurrency

`user_owned_retain` never deletes a selected external file.
`app_private_delete_on_terminal` is admitted only for a canonical path strictly
below the private plaintext-draft root. That owned file is removed after
publication, explicit cancellation, or expired-draft recovery. Ciphertext
directories/files are created with private Unix-mode equivalents for portable
tests; native ACL and signed-MSIX behavior remains manual evidence.

Explicit cancellation and expiry recovery both issue an idempotent remote
delete for a staged object before local cleanup. Recovery processes at most 100
drafts per invocation and skips an in-process active draft. A per-draft active
set rejects duplicate local sends; cross-process duplicate callers cannot
double-reserve a generation because Windows key state uses a share-none lock.
Colliding draft-directory creation remains fail-closed.

## Cross-platform fixture and evidence boundary

The Windows audit provider deliberately emits the same manifest, two chunks,
nonces, offsets and digests as the accepted macOS audit fixture. This proves
orchestration and canonical context parity only. It is not encryption and does
not establish real macOS/Windows interoperability.

Unit scenarios cover the production gate, three media kinds, golden parity,
generation-safe resume, unsupported/member/verification admission, duplicate
nonce and signature rejection, source/chunk/author/epoch drift, cancel and
expiry remote deletion, user-owned retention, active-draft concurrency and
strict unknown-field rejection. Focused race tests, full portable regression,
Windows compile gates and acceptance validation are the best-effort coding
evidence for this task.

No real DPAPI/NTFS/ACL, signed MSIX, real codec/container/crypto, coordinator
traffic capture, crash dump/swap/backup inspection, hardware upload or playback
has been claimed. Those checks remain `not-run` in `EPIC-260714-th54l3`.
Provider selection, runtime wiring, protocol changes or capability enablement
require independent delta review and the later external security acceptance.
