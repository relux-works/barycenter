# P3 E2EE recovery, device transfer, and history grants v1

Task: `TASK-260712-1rziyo`

Status: production-dark engineering foundation; no runtime route, production cryptographic suite, or recovery capability is enabled

## Decision

A replacement device can bootstrap only the current clean group epoch. A `device_transfer` or `recovery` package is accepted only between two distinct verified devices in the same Orbit; a `welcome` package may target another current Air member. Every package is opaque to the coordinator and is immutably bound to the exact group, epoch, target snapshot, issuer and recipient actor/Orbit, member revisions, device revisions, and verification digests. Its TTL is at most 15 minutes and one successful consume makes it terminal. Foreign recipients, replays, stale epochs or targets, changed lineage, revoked devices, and cloned revisions fail closed.

Transfer does not create a history grant. The recipient imports the current epoch through the existing initial group-state persistence path (`expectedRevision == 0`); retrying the same bootstrap encounters an existing epoch and is rejected. If no surviving authorized device or separately reviewed user-held recovery capability can produce the opaque package, protected history is unrecoverable. The coordinator has no key, plaintext, unwrap API, or escrow fallback.

## Explicit historical access

Historical access is a separate encrypted grant for one named ready protected object and an explicit epoch interval containing that object. Creation requires the exact current clean group snapshot, verified current issuer and recipient lineage, the recipient device revision, approval time, and either the object's author device or the current Air owner. A grant is either:

- `one_time`, with exactly one read; or
- `time_bound`, with at most 32 reads and a maximum 30-day lifetime.

Authorization rechecks both issuer and recipient actor, Orbit, member revision, device revision, verification digest, current group target, expiry, status, and read budget in the same transaction that increments the counter. Creation, read, expiry, and revocation generate content-free audit events. Concurrent one-time reads have one winner.

## Lost devices, reset, and cleanup

Public-device revocation now revokes every pending transfer and active history grant involving that device in the same transaction and records a required group rotation. The old device therefore cannot consume an outstanding package or use an active grant, and new protected sends remain blocked until a client-produced commit excludes it.

The macOS and Windows repositories expose an explicit local identity reset for re-enrollment. It removes only the fixed metadata/signing/agreement slots, record before witness. Partial deletion remains detectable and retryable. Existing group state, grants, and content-cache records are not relabeled: they remain bound to the old random installation ID and cannot be opened by the replacement identity. This is reset, not credential recovery.

Expired local history grants can be deleted only from a caller-provided list capped at 100 unique IDs. Missing IDs are harmless, active grants are retained, corrupt/torn records stop cleanup, and no filesystem or Keychain enumeration is treated as an authorization index. Shared policy values are frozen in `protocol/e2ee-recovery-v1-vectors.json` and exercised by coordinator, macOS, and Windows tests.

## Operational and evidence boundary

The coordinator stores only opaque package/grant bytes and their digests. No table or audit field stores plaintext, media keys, recovery secrets, or unwrapped group state. Companion binding rows are immutable except for package consumption time and grant access counters/timestamps. Expiry sweeps are bounded to 1,000 coordinator artifacts per transaction and 100 caller-enumerated local grants per call.

This task provides unit-tested, production-dark seams only. It does not prove real Keychain or DPAPI behavior, device-to-device interoperability, signed-package behavior, secure transport, production crypto, recovery UX, or successful recovery of any real media. Those real-app and real-hardware checks remain in the manual testing epic `EPIC-260714-th54l3`; cryptographic and release gates remain open until their independent evidence is accepted.
