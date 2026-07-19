# P3 Windows E2EE key state foundation v1

Task: `TASK-260712-25dzp4`

Status: production-dark engineering foundation; no E2EE capability is advertised or runtime-wired

## Decision

The Windows client now has a narrow repository for opaque device identity material, group epoch state, history grants, and a bounded content-key cache. It does not choose a group-crypto library, cipher suite, protected-media container, or key serialization. Private blobs remain caller-supplied output of a future independently reviewed library. SHA-256 is used only for canonical-record consistency and non-secret filename tokens, not as encryption or production cryptographic evidence.

The repository is not referenced by `main` or any production composition and does not advertise `e2ee_media_v1`. The default constructor exists only behind the Windows build tag. Non-Windows production builds return unsupported rather than creating a plaintext fallback.

## DPAPI and file layout

The default Windows boundary reuses the reviewed onboarding primitives: `CryptProtectData` and `CryptUnprotectData` run in current-user scope with `CRYPTPROTECT_UI_FORBIDDEN`, without `CRYPTPROTECT_LOCAL_MACHINE`, optional entropy, or a secret description. Every plaintext envelope is bounded, DPAPI-protected before file I/O, and best-effort cleared after use. Native DPAPI allocations are copied, zeroed, and freed by the existing reviewed adapter.

Device metadata, signing private material, key-agreement private material, every group state, every grant, and the content-key cache occupy distinct `.dpapi` state files beneath `e2ee-key-state-v1`. Every slot has a separately DPAPI-protected witness file. No private material is written to `config.json`, sync preferences, ordinary logs, telemetry, or diagnostics.

Writes use a random ciphertext-only temporary file, write-through handle, complete write, `FlushFileBuffers`, checked close, and `MoveFileEx` replace plus write-through. The final destination is decrypted and compared byte-for-byte before the transition proceeds. Stale temporary ciphertext files are bounded by the exact filename grammar and removed on the next write.

Current-user DPAPI is not described as a device-only hardware key. Whether a managed/domain profile, system backup, roaming profile, or recovery credential can restore a DPAPI master key depends on the real Windows environment and policy. The engineering contract treats unavailable or mismatched protected state as fail-closed and requires re-verification/recovery; signed-package profile migration and backup/restore behavior remain manual evidence.

## Cross-process persist-before-ack

Every public operation first takes an in-process keyed lock and then holds one repository-wide Win32 file handle opened with share mode zero. A second process therefore fails busy before it can read and race the revision. The lock covers current-state validation, record/witness mutation, final readback, and return; a lock-close failure prevents acknowledgment.

Within that critical section each mutation performs:

1. load and validate the current DPAPI state/witness pair and expected revision;
2. atomically replace and read back the next canonical state record;
3. atomically replace and read back the witness containing the next revision and record digest;
4. reload and compare the complete pair;
5. only then return the revision or send-generation reservation.

A crash after the state replacement but before the witness replacement leaves a revision/digest mismatch and blocks use. If both replacements completed but a readback was lost, the higher revision/generation remains consumed and retrying the old revision conflicts. An ambiguous failure may skip a generation but cannot reuse it. Group commits advance exactly one epoch and must name the exact prior commit digest; stale epochs, gaps, forks, old revisions, malformed state, and generation overflow fail closed.

The witness detects ordinary torn writes, partial identity installation, and a group copied into a different installation. DPAPI also rejects ciphertext unavailable to the current protected user. This is not a claim that an attacker able to decrypt and coherently rewrite every file cannot roll back or clone the complete snapshot; that stronger protection remains outside this local consistency mechanism.

## Grants, cache, and secret exposure

History grants are monotonic in epoch coverage and expiry, explicitly revocable, and rejected after expiry. The content-key cache admits at most 32 unique entries and 64 KiB of key bytes, evicts expired/old entries, and supports full clearing. Individual keys are limited to 4 KiB, grants to 64 KiB, private key blobs to 4 KiB, and group state to 1 MiB.

Decoded secret values are exposed only through redacted closure-scoped leases. The closure receives a temporary copy that is overwritten after return; explicit destroy and a finalizer clear the retained lease buffer on a best-effort basis. Go copies, compiler/runtime behavior, DPAPI internals, paging, and crash dumps mean this is not a forensic-wipe guarantee.

## Target semantics and open gates

The shared cross-platform vector pins EPC-005 behavior: an active Air member with registered devices but no currently verified device is a `removed_endpoint`, not an `unsupported_target`. A verified device with no supported E2EE capability is an `unsupported_target`; a verified supported device is routable.

Automated evidence covers shared epoch/replay/fork/target vectors, both crash windows, partial device installation, cross-installation group copy, exclusive-lock denial, grant/cache bounds, deletion, redaction, DPAPI source posture, Windows amd64/arm64 compilation, and production-dark scans. It does not prove DPAPI, NTFS durability, installed-MSIX storage identity, roaming/backup/restore, crash-dump exclusion, memory forensics, or real crypto interoperability on Windows hardware. Those remain in manual epic `EPIC-260714-th54l3`.

Production enablement remains blocked by `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, external security closure `TASK-260712-1ulshp`, and downstream reviewed-library integration. The exact producer commit requires an independent delta review before acceptance.
