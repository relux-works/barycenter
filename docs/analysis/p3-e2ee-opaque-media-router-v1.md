# P3 E2EE opaque media router foundation v1

Task: `TASK-260712-1yz5ca`

Status: production-dark engineering foundation; `e2ee_media_v1` remains disabled

## Decision

The coordinator now has a bounded internal router for client-produced encrypted manifests, opaque key envelopes, ciphertext chunks, authenticated range boundaries, and opaque live frames. The coordinator never selects a cipher suite or container, never encrypts or decrypts content, and never creates, unwraps, escrows, derives, or logs a group, session, or content key.

These entry points are deliberately not registered as production HTTP or WebSocket routes. A production route cannot safely bind a request to a verified E2EE device until the downstream macOS and Windows key-state tasks land and the external crypto implementation gate accepts a concrete stack. The internal methods are the exact authorization and persistence boundary those routes must call; legacy plaintext upload, download, transmission, inbox, history, DND, receipt, and live-PTT handlers remain unchanged and cannot silently enter this path.

## Stored object flow

`StageE2EEProtectedObject` keeps the existing immutable object header and additionally freezes the exact current recipient-device lineage when group routing is initialized. The encrypted manifest must hash to the declared manifest digest. Staging, every chunk append, finalization, manifest fetch, and range fetch recheck the exact actor/device/Air snapshot or fail with a rotation requirement. A removed, revoked, rejoined-under-new-lineage, disabled, unsupported, or non-target device cannot fetch the object.

Ciphertext chunks are immutable and contiguous. Each chunk is at most 1 MiB, an object is at most 64 MiB and 1024 chunks, no actor may hold more than four staged objects, and a rolling 24-hour actor reservation is capped at 512 MiB. Appends are offset/index compare-and-swap operations; an exact duplicate is idempotent and conflicting bytes fail. Finalization streams every chunk through SHA-256 and accepts only the declared count, total length, per-chunk hashes, and whole-object ciphertext hash.

Range reads return at most 4 MiB and only complete authenticated chunk boundaries. Exact object epoch, generation, target snapshot, manifest digest, optional If-Range manifest digest, frozen recipient lineage, current membership, device verification, and fork state must agree. Egress uses the canonical 1 MiB tiny-range admission floor and a 512 MiB rolling device budget; failed reads roll their reservation back.

Deletion changes the object to `deleted`, removes server-held ciphertext chunks, and denies all later manifest/range access. Immutable audit and header metadata remain. This is server-access revocation only: it makes no claim to erase ciphertext, keys, or plaintext already copied by an authorized device.

## Opaque live flow

Protected live traffic has a separate `BE` wire envelope and cannot be parsed by the legacy plaintext `BP` frame decoder. Its public header binds session ID, group epoch, session generation, frame sequence, capture monotonic time, exact target snapshot digest, and bounded ciphertext length. This does not authenticate ciphertext without a future reviewed suite; it prevents accidental downgrade/cross-routing while the capability is dark.

Starting a live session requires the current exact group snapshot and a generation greater than every prior session for that sender/group. Only one session may be active per group. The coordinator persists public replay/rate metadata and exact recipient lineage but never persists a frame payload. Relay accepts at most 512 opaque bytes per frame, a maximum eight-frame burst and 50 frames/second, monotonic sequence/timing with an eight-frame gap bound, and at most the reviewed live duration. It returns one bounded frame copy plus a sorted recipient-device list rather than per-recipient queues.

A slow, blocked, DND, policy-rejected, revoked, or unsupported recipient is terminated independently; other recipients continue. Receipts are monotonic, idempotent public metadata. Membership change records the rotation requirement and terminates the live session before another frame. Coordinator restart marks every active session and recipient `coordinator_restart`; the sender must use a higher generation. Terminal rows have bounded batch pruning while immutable audit metadata remains.

## Service compatibility

The opaque repository creates no `media_items`, stream variants, transmissions, inbox rows, history rows, saved cues, or legacy live sessions. Canonical legacy ACL, report, delete, DND, block, receipt, retention, quota, and cache behavior therefore remains byte-for-byte unchanged and is covered by the full coordinator regression suite. The new router provides explicit deletion, policy-recipient termination, receipt, upload quota, range quota, restart, and prune seams without inventing a parallel plaintext service.

Downstream runtime tasks must bind authenticated sockets/requests to the verified device ID, invoke existing DND/block/transmission/inbox/history services before calling these seams, and never accept a caller-supplied device ID as authentication. Report plaintext export remains owned by `TASK-260712-2i0w6x`; client encryption/decryption and secure key state remain owned by the platform tasks.

## Honest limits and gates

Random fixture bytes exercise opaque transport but are not evidence of an accepted cryptographic container. Server-state capture demonstrates only that the coordinator has no decoder, key, plaintext column, or legacy protected-media link; it is not a cryptographic confidentiality proof. Production enablement, product claims, signed packages, real-device interoperability, hardware playback, and manual evidence remain blocked.

Open gates remain `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, and `TASK-260712-1ulshp`. Because this task adds new protocol and persistence behavior after the accepted design review, the exact producer commit requires another independent delta review before acceptance.
