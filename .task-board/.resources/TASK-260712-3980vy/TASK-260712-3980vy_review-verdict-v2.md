# Independent re-review verdict — TASK-260712-3980vy (rework commit c9faa7e)

Reviewer: independent security/protocol/lifecycle/concurrency/realtime reviewer
(run RUN-260720-8f681f). Scope: exact rework commit
`c9faa7ef4a5cc089ebfb83bdce11fadfcfe669b8` against rejected producer commit
`d8a429c` and outcome `TASK-260712-3980vy_review-verdict-v1.md`, per
`independent-re-review-brief-c9faa7e.md`. Full diff, full current files,
updated ADR/vectors/packet, hash pins and accepted design authority inspected.
No production code modified by this review.

## Independently reproduced evidence (at c9faa7e)

- `swift-format lint --strict` on the three task files: clean.
- `swift test --filter MacE2EELivePTTTests`: 10/10 passed (includes both new
  fixtures).
- `swift test` full package: 350 tests in 56 suites passed.
- `python3 -m unittest discover -s scripts/acceptance`: 200 tests OK.
- `python3 scripts/acceptance/run_automated.py`: 16/16 steps, manifest emitted.
- All 9 pinned artifact hashes in `acceptance/phase3/macos-e2ee-live-ptt-v1.json`
  (including the design-review resource pin) match `shasum -a 256` of the tree.
  `MacE2EEKeyState.swift` pin is unchanged from d8a429c — the file was not
  touched, so no sibling-packet hash cascade is required; grep confirms no
  other packet pins the changed files.
- Working tree contains no uncommitted code changes (board files and
  `__pycache__` only).

## HIGH-1 — CLOSED

Device-local Keychain record revision is fully removed from the cross-device
surface:

- `MacE2EELiveSessionContext` now carries `commitDigest` (validated 64-hex),
  not `groupRevision` (`MacE2EELivePTT.swift:145-197`). AAD binds
  `commit_digest` + `epoch` and contains no revision field
  (`MacE2EELivePTT.swift:678-679`); grep confirms zero remaining
  `groupRevision`/`group_revision` references in source, vectors, ADR, packet
  and validator.
- Sender binds `current.metadata.commitDigest` from the witnessed re-read
  after generation reservation (`MacE2EELivePTT.swift:401-404`); receiver
  binds `group.metadata.commitDigest` from its own witnessed record
  (`MacE2EELivePTT.swift:436-439`). `reserveSendGeneration` bumps only
  `sendGeneration`/`updatedAtMS` and can never change `commitDigest`
  (`MacE2EEKeyState.swift:536-541`); the digest is installed only through
  `persistGroupState`'s epoch-transition validation (previous-commit chain,
  epoch+1), so both peers holding the same witnessed epoch/commit compute
  byte-identical AAD.
- Local revision survives only as setup/CAS witnesses:
  `expectedGroupRevision` in the outgoing request and the renamed
  `expectedLocalGroupRevision` in the incoming request
  (`MacE2EELivePTT.swift:377,393,433`) — correct local crash/race checks, per
  the v1 required fix.
- `MacE2EELiveAuthorizationSnapshot` now binds `commitDigest`; per-frame
  authorization compares `current.commitDigest == context.commitDigest`
  (`MacE2EELivePTT.swift:638`). The v1-noted factory-test hack
  (`group.revision + 1`) is gone; the snapshot is now faithful to a verified
  control-plane transition.
- New fixture `crossInstallationRoundTrip`
  (`MacE2EELivePTTTests.swift:494-558`): sender and receiver use separate
  repositories/keychains/identities; after sender generation reservation the
  test asserts `receiverGroup.revision != outgoing.reservation.revision`
  (2 vs 1), yet protect→open round-trips the exact plaintext. The fixture
  cipher's tag is `digest(key + AAD + nonce + ciphertext)` with a shared fixed
  key and `open` recomputes it from the receiver's independently constructed
  AAD, so the test genuinely proves cross-installation AAD byte-equality — the
  d8a429c code fails this test (receiver revision guard and AAD mismatch).

## Contract delta — properly recorded

The AAD binding change is treated as a reviewed cross-client contract delta:
ADR (`docs/analysis/p3-macos-e2ee-live-ptt-v1.md`) documents local revision as
setup/CAS-only and commit-digest binding; vectors `aadFields` list
`commit_digest` (no `group_revision`) and `failClosed` lists
`changed_commit_digest`; the packet invariant is renamed to
`exact-air-target-sender-session-epoch-commit-generation-sequence-codec-timing-aad`;
the validator enforces the new source strings and both new fixture names;
`deltaReviewRequired` remains true. The public `BE` wire is byte-identical:
`MacE2EEOpaqueLiveFrame` is untouched by the diff and the fixed vector
`encodedHex` is unchanged, so Go parity from v1 stands.

## LOW-1 — CLOSED

`malformedProviderOutput` is a distinct failure case. Seal side: empty or
oversized nonce token / empty or oversized wire ciphertext →
`malformedProviderOutput` (`MacE2EELivePTT.swift:527-530`), separated from the
nonce-reuse set-insert guard (`:531-533`). Open side: malformed nonce token
and malformed plaintext are likewise split from `nonceReuse`
(`:592-600`). All still terminate fail-closed (protect terminates on every
non-`invalidFrame` failure, open terminates on every failure); the new fixture
proves `malformedProviderOutput` + terminal.

## LOW-2 — CLOSED

`frame.sequence <= 15_000` is enforced in the entry guard of `protect()`
(`MacE2EELivePTT.swift:508`), before AAD construction and `crypto.seal`. The
15,000-bound fixture drives a full session: `sealCount == 15_000`, attempt
15,001 throws non-terminal `invalidFrame`, `sealCount` remains 15,000 (no
seal, no nonce consumed, no state growth) and the channel stays usable-closed
rather than terminating on an encoder-side bound.

## Regression sweep — no new findings

- Retry idempotence (cached opaque on identical frame), auth-before-jitter
  barrier, tamper/replay/stale-epoch/foreign-target/removed-sender fail-closed
  paths, single `destroy()` via `cryptoDestroyed`, lock scope, teardown
  purge and reorder window are unchanged and re-verified by the green battery.
- Production darkness intact: decision flags all false/dark, every
  manual-evidence field `not-run`, NodeApp grep clean for any
  `MacE2EELive`/`MacE2EEKeyState` reference, audit-fixture constructors remain
  internal, composition gate (`crossProcessGenerationSerializationApproved`)
  unchanged, no capability/UI claim, no plaintext fallback, no invented
  hardware/packet-capture evidence.
- v1 informational items INFO-1 (attestation-not-enforcement cross-process
  gate), INFO-2 (`trySend` collapses terminal into backpressure) and INFO-3
  (duplicate revokes receiver) remain as recorded — non-blocking, owned by the
  open composition gates and future runtime wiring.

## Checklist assessment

1. Session key/AAD binding — now coherent across devices; met.
2. Encrypt off capture callbacks, verify before jitter decode — met.
3. Reject nonce reuse/replay/tamper/stale epoch/removed sender — met.
4. C1-C2/FEC/PLC/backpressure/DND/teardown bounds — met in code; real
   hardware remains manual in EPIC-260714-th54l3, as scoped.
5. Coordinator capture cannot reproduce speech — met at fixture level,
   honestly declared not-run for production.
6. Single-instance ownership / cross-process serialization — in-process
   enforced; cross-process remains the explicit dormant attestation gate.

## Verdict

**ACCEPTED**

HIGH-1, LOW-1 and LOW-2 are all closed at exact commit c9faa7e with no new
Critical, High or Medium finding. Zero open Critical/High/Medium. Evidence
battery independently reproduced green (lint clean, 10/10, 350/350, 200/200,
16/16, all pins match). Manual hardware/real-provider work remains owned by
EPIC-260714-th54l3. No production E2EE readiness is claimed; runtime
composition, provider selection, capability and UI claims remain dark behind
the open EPC gates.
