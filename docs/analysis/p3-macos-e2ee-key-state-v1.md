# P3 macOS E2EE key state foundation v1

Task: `TASK-260712-1x9ruo`

Status: production-dark engineering foundation; no E2EE capability is advertised or runtime-wired

## Decision

NodeCore now exposes a narrow macOS repository for opaque device identity material, group epoch state, history grants, and a bounded content-key cache. It does not choose a group-crypto library, cipher suite, protected-media container, or key serialization. The private blobs passed to this boundary must eventually come from the independently reviewed library selected by the open E2EE implementation gate. CryptoKit is used here only to hash local canonical records and account scopes; those hashes are consistency witnesses, not encryption, authentication, or a production cryptographic claim.

The repository is deliberately unreferenced by the app composition root and does not advertise `e2ee_media_v1`. Send, playback, live PTT, and UX integrations receive only metadata, closure-scoped secret leases, and explicit state transitions. Legacy plaintext behavior is unchanged.

## Keychain layout and lifecycle

The dedicated `works.relux.pulsar.e2ee` service uses the data-protection Keychain with `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` and synchronizable explicitly disabled. Device metadata, signing private material, key-agreement private material, every group state, every grant, and the content-key cache occupy distinct state slots. Each slot has a separate canonical witness item. Signing and agreement keys therefore never share one Keychain value.

Device installation creates a random 256-bit installation identifier with `SecRandomCopyBytes`. Every dependent record binds to it. A partial three-slot device installation, a copied group state presented under another installation, a missing witness, a mismatched revision/digest, or malformed bounded payload fails closed as `rollbackOrClone`. There is intentionally no silent repair path: device-state loss or a partial identity installation requires the future recovery/re-verification flow.

`ThisDeviceOnly` items are excluded from synchronizable Keychain and normal device migration/backup restoration. That matches the threat-model decision that losing the device-local identity is irrecoverable locally and requires re-verification or device transfer rather than restoring private state from coordinator data. Physical backup/restore and Keychain accessibility behavior still require manual evidence on a signed build.

## Persist-before-ack protocol

Every mutation is serialized inside the owning process and follows the same sequence:

1. load and validate the current state/witness pair and expected revision;
2. write the next canonical state record;
3. read back the exact state bytes;
4. write the independent witness containing the next revision and state digest;
5. read back the exact witness and re-load the complete pair;
6. only then return the new revision or send-generation reservation.

A crash after the state write but before the witness write leaves a detectable mismatch and blocks use. If both writes completed but the final readback was lost, the mutation remains consumed: restart observes the higher revision/generation and a retry with the old revision conflicts. Thus an ambiguous failure can skip a send generation but cannot reuse it. The witness protects ordinary crash consistency and coherent local lineage; it is not claimed to defeat a malicious process that can arbitrarily rewrite every item in the app's Keychain access group.

Group commits advance exactly one epoch and must name the exact previous commit digest. Stale epochs, gaps, forks, old revisions, and generation overflow fail closed. Deleting obsolete group state removes the state item first, so a deletion crash cannot resurrect a usable secret.

## Grants, cache, and secret exposure

History grants are monotonic in epoch coverage and expiry, explicitly revocable, and rejected after expiry. The content-key cache admits at most 32 unique entries and 64 KiB of key bytes, evicts expired/old entries, and supports complete clearing. Decoded secrets are exposed only through closure-scoped leases whose descriptions are redacted. Lease destruction overwrites the current `Data` buffer before release on a best-effort basis; Swift/Foundation copies and allocator behavior mean this is not a guaranteed forensic wipe.

The repository contains no preferences, ordinary log, telemetry, or crash-report writes. Callers must preserve that boundary and must not stringify or retain lease bytes.

## Target semantics and open gates

The shared vector pins EPC-005 behavior: an active Air member with registered devices but no currently verified device is a `removed_endpoint`, not an `unsupported_target`. A verified device with no supported E2EE capability is an `unsupported_target`; a verified supported device is routable.

Automated evidence covers canonical state-machine vectors, predecessor/fork/replay checks, both crash windows, cross-installation clone rejection, partial device installation, grant and cache bounds, deletion, redaction, and production-dark source scans. It does not prove Keychain behavior on physical hardware, signed/notarized package entitlements, backup/restore behavior, real crypto interoperability, or memory forensics. Those remain in manual epic `EPIC-260714-th54l3`.

Production enablement remains blocked by `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, the external security closure `TASK-260712-1ulshp`, and the downstream reviewed-library implementation tasks. The exact producer commit requires an independent delta review before this task is accepted.
