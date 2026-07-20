# Independent review verdict — TASK-260712-3980vy (commit d8a429c)

Reviewer: independent security/protocol/lifecycle/concurrency/realtime reviewer
Scope: exact commit `d8a429c44a39971d89a739c7a0ab610fce16d9b5`, full diff from parent
`0fb16e2`, task README, all six checklist items, accepted design authority
(`TASK-260712-aniuyy_independent-design-review-v1.md`), Go `BE` opaque-live
contract (`coordinator/internal/e2eecontract/opaque_live.go`), acceptance packet
`acceptance/phase3/macos-e2ee-live-ptt-v1.json` and every pinned artifact
(sha256 re-verified against the working tree at the exact commit).

## Independently reproduced evidence

- `swift-format lint --strict` on the three task files: clean.
- `swift test --filter MacE2EELivePTTTests`: 8/8 passed.
- `swift test` full package: 348 tests in 56 suites passed.
- `python3 -m unittest discover -s scripts/acceptance`: 200 tests OK.
- `python3 scripts/acceptance/run_automated.py`: 16/16 steps, manifest emitted.
- All 9 pinned artifact hashes in `macos-e2ee-live-ptt-v1.json` and the 5
  cascade re-pins in sibling packets match `shasum -a 256` of the tree.

## What checks out

- **Byte-for-byte `BE` wire parity.** `MacE2EEOpaqueLiveFrame`
  (`MacE2EELivePTT.swift:21-142`) matches `opaque_live.go` exactly: magic
  `0x42 0x45`, version 1, big-endian field layout at identical offsets,
  reserved bytes 82-83 zero-checked, flags masked to start|end, non-zero
  16-byte session, seq ∈ [1, 15000], ciphertext ∈ [1, 512], 64-lowercase-hex
  digest, `(seq==1) == startFlag`. The fixed vector
  (`protocol/macos-e2ee-live-ptt-v1-vectors.json`) decodes by hand to the Go
  layout. No protocol delta, no field added, no legacy `BP` downgrade path
  (distinct magic, no fallback branch anywhere).
- **Witnessed state and generation reservation.** `prepareOutgoing` loads
  identity + group, checks revision/target, reserves a crash-safe `live_ptt`
  generation, re-reads the advanced record and re-checks
  revision/epoch/target before invoking the derivation seam
  (`MacE2EELivePTT.swift:355-409`). `reserveSendGeneration` keeps the
  skip-but-never-reuse crash property (`MacE2EEKeyState.swift:507-543`).
- **AAD ambiguity resistance.** Length-prefixed strings, fixed-width
  big-endian integers, fixed order, domain string
  `barycenter.e2ee.live.aad.v1`; binds group, sender device/actor/orbit/node,
  Air domain+ID, target, session, epoch, generation, sequence, flags, capture
  time, codec, frame/payload/jitter/duration bounds
  (`MacE2EELivePTT.swift:650-679`). No concatenation ambiguity.
- **Nonce and retry policy.** Unique-nonce sets in both directions; transport
  retry returns the exact cached ciphertext+nonce without resealing (verified
  by `sealCount == 1` across two send attempts); nonce reuse terminates.
- **Authentication barrier.** `receiveOpaque` decodes the public envelope and
  fully authenticates before `MacLiveJitterReceiving.receive`; tamper, stale
  epoch, foreign target, removed sender, replay and membership change all
  fail closed, terminate the channel, destroy the provider session exactly
  once (`cryptoDestroyed` guard, throwing-init destroy paths covered) and
  revoke buffered PCM.
- **Realtime bounds.** The microphone callback remains a bounded mailbox
  offer (source-inspected and enforced by
  `validate_macos_e2ee_live_ptt.py:88-92`); seal/open run on the sender
  worker/jitter path; per-frame authorization is an in-memory snapshot read;
  Keychain I/O happens only at session setup. Synthetic capture timestamps
  (`captureBaseUs + (seq-1)*20_000`, `MacLiveCaptureSender.swift:404`) match
  the exact-pacing checks. Incoming reorder is limited to the existing
  8-frame window so Opus FEC remains possible. Memory is bounded
  (≤15000 sequences; nonce sets ≤ ~3.7 MB worst case per direction, freed on
  terminate). DND, backpressure, jitter, FEC/PLC, PCM and teardown owners are
  untouched.
- **One-owner semantics.** `claimE2EELiveSendOwnership` enforces a single,
  never-released in-process reservation owner
  (`MacE2EEKeyState.swift:297-306`). The cross-process gate is an explicit
  composition attestation parameter — a dormant-code honor gate, not a
  bypass: no `productionApproved` provider exists in the repository, the
  fixture constructors are internal to NodeCore, and NodeApp contains zero
  references (grep clean; validator enforces `main.swift` non-wiring).
- **Production-dark honesty.** Decision flags all dark, every manual-evidence
  field `not-run`, no capability string, no plaintext fallback, no invented
  hardware/packet-capture evidence. Hash cascade in the four sibling packets
  is a pure re-pin of the modified `MacE2EEKeyState.swift`; genuinely
  non-protocol-affecting.

## Findings

### HIGH-1 — AAD binds a device-local Keychain record revision; two distinct devices can never authenticate each other's frames

- `MacE2EEKeyStateRepository.persist` assigns `revision = expectedRevision + 1`
  per **local** write (`MacE2EEKeyState.swift:909`); the key-state ADR calls
  this "coherent local lineage". Every installation starts at 1 and advances
  independently.
- `reserveSendGeneration` persists the group record and therefore bumps the
  **sender's** local revision by one per session
  (`MacE2EEKeyState.swift:531-542`); receivers never observe this write. The
  sibling protected-media domain shares the same group record, adding further
  sender-only bumps.
- The sender's session context binds the post-reservation local revision
  (`groupRevision: reservation.revision`, `MacE2EELivePTT.swift:395-397`) and
  it is appended into AAD (`MacE2EELivePTT.swift:666`).
- The receiver is forced to bind its **own** local revision:
  `prepareIncoming` guards `group.metadata.revision == request.groupRevision`
  (`MacE2EELivePTT.swift:425-428`), so the caller cannot pass the sender's
  value unless it coincidentally equals the receiver's local counter.
- AEAD open requires byte-identical AAD. Concrete failure: device A installs
  group at record revision 1, reserves generation → context revision 2 →
  every frame's AAD binds 2. Device B holds the same epoch/commit at its own
  record revision 1. B either fails `prepareIncoming` (revision guard) or
  computes AAD with 1 ≠ 2 → `crypto.open` fails → `authenticationFailed` →
  channel terminated and PCM revoked on the very first frame. Skew grows with
  every sender session and differs per join time, so this is not a first-run
  edge: cross-device sessions can essentially never authenticate.
- The test battery masks it: loopback tests share one context object between
  sender and receiver channels (`MacE2EELivePTTTests.swift:279-286`), and the
  factory test hand-tunes the authorization snapshot to `group.revision + 1`
  (`MacE2EELivePTTTests.swift:434`) — itself evidence of the incoherence: a
  faithful snapshot source (updated "only from a verified membership/epoch
  control-plane transition", `MacE2EELivePTT.swift:213-215`) would hold the
  pre-reservation revision and make even the sender's first `protect()` throw
  `membershipChanged`.
- Impact: fail-closed, so no confidentiality/integrity exposure — but the
  task's core deliverable ("bridge macOS live sender and receiver frames to
  the reviewed group state") and its AC ("All macOS source and target
  pairings preserve C1-C2 under encryption") are unachievable outside
  loopback. The accepted design authority requires an "epoch-bound session
  key" and never mandates binding a local lineage counter into AAD, so fixing
  this does not conflict with the accepted design.
- Required fix: bind only values identical on every member for the same
  witnessed group state — epoch plus the shared `commitDigest` (already
  64-hex, witnessed) are natural candidates — and remove the device-local
  record revision from `MacE2EELiveSessionContext`/AAD/authorization
  equality, or introduce a genuinely synchronized coordinator-witnessed group
  revision that all devices store. Keep the local-revision checks inside
  `prepareOutgoing`'s witnessed re-read (those are correct as local
  crash/race witnesses). Add a two-installation fixture test (separate
  repositories with deliberately skewed local revisions) proving
  protect→open round-trips; that test would have caught this. Since AAD is a
  cross-client contract (the future Windows peer must compute identical
  bytes), record the AAD change as a delta for the audit gate even though the
  `BE` wire is untouched.

### LOW-1 — Malformed provider output is reported as `nonceReuse`

`protect()` folds empty/oversized `wireCiphertext` and oversized nonce tokens
into the `nonceReuse` failure (`MacE2EELivePTT.swift:519-523`). Fail-closed
behavior is preserved (terminates), but diagnostics misattribute a malformed
provider as nonce reuse. Split the failure code.

### LOW-2 — Duration-boundary seal happens before the frame is known encodable

At sequence 15001 `protect()` builds AAD and calls `crypto.seal` (line 517),
inserts the nonce token (line 521), and only then fails at
`_ = try opaque.encoded()` (line 530) with non-terminal `invalidFrame`. A
retrying caller re-seals each attempt: with a random-nonce provider,
`outgoingNonces` grows per retry; with a deterministic provider the channel
terminates on `nonceReuse`. Bounded in practice by session teardown at
`maxDurationMs`; still, validate `sequence <= 15000` (and construct/validate
the opaque header) before invoking the provider.

### INFO-1 — Cross-process serialization is an attestation, not an enforcement

`crossProcessGenerationSerializationApproved: Bool`
(`MacE2EELivePTT.swift:330-334`) is an honor-system composition gate.
Consistent with the brief's dormant-code allowance and checklist item 6
(in-process one-owner is enforced and never released); real serialization or
single-instance enforcement remains owned by the open composition gate and
must land before any runtime wiring.

### INFO-2 — `trySend` collapses terminal state into backpressure `false`

`MacE2EELiveSenderBridge.trySend` (`MacE2EELivePTT.swift:720-725`) returns
`false` for both transport backpressure and a terminated channel; a future
composition must consult `isTerminal()` or the capture sender will retry a
dead session until its own drain/backpressure teardown. Bounded today.

### INFO-3 — Duplicate frame revokes the whole receiver

The E2EE path terminates and revokes on a duplicate (`e2ee_replay`), stricter
than the plaintext guard's benign `.duplicate`. Defensible over the ordered
coordinator transport (a duplicate implies relay misbehavior) and consistent
with the stated fail-closed policy; noted for awareness.

## Checklist assessment

1. Session key/AAD binding — implemented, but see HIGH-1 (revision binding
   incoherent across devices).
2. Encrypt off capture callbacks, verify before jitter decode — met.
3. Reject nonce reuse/replay/tamper/stale epoch/removed sender — met.
4. Preserve C1-C2/FEC/PLC/backpressure/DND/teardown bounds — met in code;
   real-hardware C1-C2 remains manual (EPIC-260714-th54l3), as scoped.
5. Coordinator capture cannot reproduce speech — met at fixture level
   (ciphertext-only wire; no key material coordinator-side); production proof
   gated on the real provider, honestly declared not-run.
6. Single-instance ownership / cross-process serialization — in-process
   ownership enforced; cross-process remains an explicit attestation gate
   (INFO-1), acceptable for production-dark.

## Verdict

**REJECTED**

One open High finding (HIGH-1). Per the review brief, acceptance requires
zero open Critical/High/Medium findings. Everything else — wire parity,
fail-closed behavior, realtime bounds, production-dark honesty, evidence
battery (lint clean, 8/8, 348/348, 200/200, 16/16, hashes) — independently
reproduced and sound. Rework scope is contained: fix the cross-device
group-state binding (HIGH-1), optionally fold in LOW-1/LOW-2, add the
two-installation round-trip fixture, update the pinned hashes/cascade, and
return for another reviewer cycle. No production E2EE readiness is claimed.
