# P3 E2EE coordinator routing and rotation foundation v1

Task: `TASK-260712-20j5tm`

Status: dormant engineering foundation; production E2EE remains disabled

## Decision

The coordinator now has an additive, keyless state machine for client-produced
public E2EE proposals and commits. It is deliberately not exposed through the
production HTTP/WebSocket protocol, does not advertise `e2ee_media_v1`, and
uses no production cipher suite, MLS implementation, media container, or
signature verifier. `ProductionConfig` therefore rejects every routed event.

The implemented boundary is useful before a crypto/provider decision because
it makes identity, Air membership, target snapshots, serialization, delivery,
acknowledgement, and failure behavior concrete without giving the coordinator
any group or content secret.

## Exact membership and rotation

Each registered public device is bound to one coordinator actor and one stable
protocol actor identifier. A canonical target snapshot hashes the sorted set
of current verified devices, their actor and Orbit bindings, public-package
digest, independent-verification digest, actor-membership role/join lineage,
and Air-membership ID/role/revision. Consequently a leave/rejoin of the same
device or a role change is still a new snapshot and requires rotation. Active installation
actors with an unverified or absent device registration make the target
unsupported; the protected path never silently excludes them or falls back to
plaintext. Fully revoked devices are removed endpoints and require rotation.

An initialized E2EE group persists its exact device snapshot. Reconciliation
compares that snapshot with current actor, membership, Air, and device rows. A
join, leave, device revoke, actor disable, or unsupported installation creates
one durable `rotation.require` state and revokes undelivered events for removed
devices. Protected-object staging checks the same state twice: it records the
requirement before the write and rechecks the exact membership inside the
write transaction. No post-change protected object can be sealed until a valid
client commit advances exactly one epoch to the required snapshot.

## Proposal and commit routing

Both entry points first use the strict coordinator envelope decoders. Unknown
or secret-bearing fields are rejected. The injected verifier, suite allowlist,
group, Air, epoch, previous commit, target snapshot, actor/device binding, and
current membership must all agree.

Proposals are serialized as immutable public group events and delivered only
to surviving devices from the prior epoch. A newly joined device does not
receive proposal history. An accepted client-produced commit advances the
epoch with a compare-and-swap, replaces the exact member snapshot, satisfies
the matching rotation requirement, revokes pending delivery for removed
devices, and creates durable delivery rows only for the new exact snapshot.
Competing commits have one winner; stale, replayed, malformed, unauthorized,
removed-device, future-gap, and fork traffic fails closed. An exact-current
competing predecessor marks the group forked, while malformed predecessor
bytes cannot poison that state.

The coordinator stores the signed public envelope bytes, digests, delivery
metadata, and acknowledgement state. It does not create a proposal or commit,
does not parse or produce MLS secrets, and cannot create, unwrap, escrow, log,
or recover epoch/session/content keys.

## Delivery-loss recovery

Every proposal/commit recipient has an immutable `(event, device)` delivery
binding. Pending rows can be queried after process and database restart; the
original strict public envelope is returned byte-for-byte. Acknowledgement is
an exact digest and revision compare-and-swap. Duplicate acknowledgement,
removed-device polling, and revoked pending delivery fail closed. This is
metadata recovery only: it reconstructs no client secret and makes no
availability promise against a malicious delivery service.

## Compatibility and honest limits

The schema is additive. Legacy media, transmission, ACL, retention, deletion,
inbox, and history tables are untouched. Groups created only for the previous
dormant schema fixture remain inert until their routing snapshot is explicitly
initialized. Production capability advertisement and all shipped plaintext
paths are unchanged.

This task does not implement the opaque ciphertext upload/fetch API assigned
to `TASK-260712-1yz5ca`, client key state, client cryptography, media
encryption/decryption, report evidence export, recovery grants, or a production
crypto/container selection. Real signed-app, device, interoperability, and
hardware evidence remains `not-run` in `EPIC-260714-th54l3`. Production gates
`EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, and the independent external
security gate remain open. Because this adds protocol-state behavior after the
accepted design review, an independent delta review of the exact producer
hashes is mandatory before task acceptance.
