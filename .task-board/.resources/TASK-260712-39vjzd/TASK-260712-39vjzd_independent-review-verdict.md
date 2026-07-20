# Independent review verdict — TASK-260712-39vjzd

- **Task**: TASK-260712-39vjzd — Encrypt and authenticate Windows live PTT end to end
- **Producer commit reviewed**: `aee07339bcfe014b39edac10734f713d11333792` (exact)
- **Baseline**: accepted main merge `e47eb6b583fa0319beee460b87397bdb75dbcf39`
- **Reviewer**: independent security/protocol/lifecycle/concurrency/realtime reviewer (Claude Fable 5), terminal-completion run after incomplete `RUN-260720-c87e23`
- **Date**: 2026-07-20

## Terminal verdict

**ACCEPTED** — zero open Critical/High/Medium findings.

The prior reviewer run left no recorded audit artifact (its spawn log is empty),
so this run independently re-audited the producer diff and full current files
rather than trusting the summarized prior result, and completed the mandatory
synchronous acceptance harness.

## Diff and cleanliness gates

- `git diff aee0733..HEAD --name-only` touches only `.planning/` and
  `.task-board/` tracking files — tracking-only, no production or test delta
  hidden after the reviewed commit.
- Working tree contains only board tracking changes; no temporary reviewer
  production or test file exists. This review modified no production code and
  added no temporary test files.

## Audit findings by required focus item

1. **BE wire parity** — `WindowsE2EEOpaqueLiveFrame` Encode/Decode in
   `pulsar-win/windows_e2ee_live_ptt.go` is byte-exact with
   `coordinator/internal/e2eecontract/opaque_live.go`: 84-byte header
   (`"BE"`, version 1, flags, 16-byte session, u64 epoch, u64 generation,
   u32 sequence, u64 captureMonotonicUS, 32-byte target digest, u16 length,
   u16 reserved=0), 512-byte ciphertext cap, identical validation including the
   `sequence==1 ⇔ Start` and 15000 max-sequence rules. The wire vector in
   `protocol/windows-e2ee-live-ptt-v1-vectors.json` matches the encoder and the
   accepted macOS vector layout.
   `TestWindowsE2EELiveOpaqueFrameMatchesAcceptedBEWire` also proves a legacy
   plaintext `BP` frame is rejected (`legacy_bp_downgrade` fail-closed). No
   hidden wire delta.
2. **Cross-device AAD** — `windowsE2EELiveAuthenticatedData` matches the
   accepted macOS ordering in `node-app/Sources/NodeCore/MacE2EELivePTT.swift`
   field-for-field (contract label; length-prefixed groupID, authorDeviceID,
   senderNodeID, targetSnapshotDigest, playbackDomain, codecProfile; raw
   sessionID; commitDigest; then u64 epoch, generation, senderActorID,
   senderOrbitID, playbackDomainID, frameMS, maximumPlaintextBytes,
   jitterBufferMS, maximumDurationMS, flags, sequence, captureMonotonicUS —
   all big-endian, signed values as two's-complement bit patterns). AAD binds
   shared epoch + commitDigest and never the device-local repository revision.
   `TestWindowsE2EELiveCrossInstallationRoundTripWithSkewedLocalRevisions`
   reproduces the two-installation fixture with deliberately different local
   revisions and exact protect→open success.
3. **Outgoing factory order** — `PrepareOutgoing` performs witnessed identity
   load → group-state load → exact revision/target CAS → cross-process
   `live_ptt` `ReserveSendGeneration` → advanced-state reload → unchanged
   epoch/revision/commit/target check → start-payload consistency check →
   derivation. Reservation results are consumed, never retried on ambiguity;
   `TestWindowsE2EELiveFactoryReservesWitnessedGeneration` verifies the
   witnessed reservation (domain `live_ptt`, generation 1, revision advance).
4. **Ownership** — `Protect` copies plaintext before `Seal`, copies provider
   nonce/ciphertext outputs before zeroing the service-owned plaintext copy
   (copy-before-zero order), and caches deep clones for retry; `Open` copies
   plaintext into the returned frame before zeroing the provider buffer.
   Returned frames are clones; caller/provider mutation cannot alter retry or
   cached state (`TestWindowsE2EELiveProviderAndCallerAliasingCannotMutateRetryState`
   asserts the alias fixture would fail on an unsafe copy order).
5. **Retry and bounds** — exact byte-identical retry with a single seal and no
   plaintext on the wire
   (`TestWindowsE2EELiveRetryReusesCiphertextAndAuthenticatesBeforeJitter`);
   sequence 15001 rejected before seal with seal-count 0
   (`TestWindowsE2EELiveProviderOutputAndDurationBounds`); malformed provider
   output (`ErrWindowsE2EELiveMalformedProviderOutput`) is distinct from nonce
   reuse (`ErrWindowsE2EELiveNonceReuse`); outgoing/incoming nonce reuse,
   tamper, replay and out-of-window input all terminate fail-closed
   (`TestWindowsE2EELiveTamperReplayAndNonceReuseFailClosed`).
6. **Authorization and teardown** — `validateWindowsE2EELiveAuthorization`
   runs before every seal and open; changed epoch/commit/target/sender
   membership terminates the channel, destroys the provider session exactly
   once (guarded by `terminal` + `cryptoDestroyed` under the channel mutex),
   revokes the receiver via the bridge, and never admits bytes to the jitter
   receiver. Finalizer is set at construction and cleared in `Terminate`;
   explicit close, error teardown and finalizer paths cannot double-destroy
   (`TestWindowsE2EELiveMembershipAndCommitChangeTerminate` asserts destroy
   count 1 after both error teardown and explicit `Terminate`).
7. **Seam placement** — `WindowsE2EELiveSenderBridge.TrySend` matches the
   existing injected `trySendFrame func(protocol.LivePTTBinaryFrame) bool`
   invoked on the transport worker after the bounded capture queue
   (`windows_live_capture_sender.go:589`); sealing is off capture callbacks.
   `WindowsE2EELiveReceiverBridge.ReceiveOpaque` decodes and authenticates
   before `windowsLiveJitterReceiving.Receive`, mapping replay→Duplicate and
   stale/membership→Stale for existing decision handling. No change to
   DND/policy admission, backpressure, 8-frame reorder window, prebuffer,
   FEC/PLC, PCM bounds or teardown semantics
   (`TestWindowsE2EELiveIncomingReorderRemainsWithinExistingWindow`; full live
   capture/receiver/node regressions green including race).
8. **Production darkness** — `NewWindowsE2EELiveSessionFactory` and
   `NewWindowsE2EELiveFrameChannel` refuse any deriver/session without
   `ProductionApproved()`; the audit constructors
   (`newWindowsE2EELiveSessionFactoryForAudit`,
   `newWindowsE2EELiveFrameChannelForAudit`) are unexported. No non-test
   Windows source references the factory, bridges, deriver or any
   KDF/AEAD/nonce/suite/library/runtime/UI/capability; no crypto primitive is
   imported. `TestWindowsE2EELiveUnreviewedProviderCannotCrossProductionFactory`
   proves both production gates fail closed and destroy the rejected session.
9. **Hash pins** — all 11 artifact hashes in
   `acceptance/phase3/windows-e2ee-live-ptt-v1.json` recomputed exact
   (implementation, key state, tests, both vector files, analysis doc, audit
   contract, coordinator wire authority, opaque-router packet, macOS live
   packet, design-review resource). The
   `validate_windows_e2ee_key_state.py` exception is scoped to exactly
   `windows_e2ee_live_ptt.go` and requires the dormant witnessed-boundary
   markers (no default repository constructor, production-dark marker,
   audit-only factory, `!derivation.ProductionApproved()` gate, `live_ptt`
   reservation, commit CAS). Existing macOS/key-state/opaque-router packets
   remain frozen and untouched by the producer commit.
10. **Checklist item 5 stays open** — real coordinator traffic capture, signed
    MSIX, native DPAPI/NTFS, real provider/crypto/codec, microphone/speaker,
    latency, memory/crash/swap/backup and macOS–Windows hardware interop
    remain `not-run` manual scope in `EPIC-260714-th54l3`. No production E2EE
    or manual evidence is claimed; all evidence below is deterministic audit
    fixture, blind cross-build and local test evidence only.

## Reproduced evidence (this run, all green)

From `pulsar-win/`:

- `go test -count=1 -run 'TestWindowsE2EELive' ./...` — ok
- `go test -race -count=1 -run 'TestWindowsE2EELive' ./...` — ok
- `go test -count=1 -run 'TestWindowsLive(Capture|Receiver|PTTNode)' ./...` — ok
- `go test -race -count=1 -run 'TestWindowsLive(Capture|Receiver|PTTNode)' ./...` — ok
- `go test -count=1 ./...` and `go test -race -count=1 ./...` — ok
- `go vet ./...` — clean
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64|arm64 go test -exec /usr/bin/true ./...` — both blind compiles ok

Acceptance:

- `python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'` —
  **215/215 OK**
- `python3 scripts/acceptance/run_automated.py --suite all` — run
  **synchronously to completion** this session; fresh manifest
  `.temp/acceptance/20260720T071009Z/manifest.json` with status **`pass`** and
  **16 commands**, every command exit code 0 (including coordinator vet/tests,
  moderation contract, previous-HEAD rollback, Windows vet/tests/race,
  amd64/arm64 cross-builds and Swift tests).

## Open findings

None at Critical, High or Medium severity. No Low findings warranting rework:
the fail-closed termination of a channel on any duplicate/out-of-window frame
is the accepted model required by the review brief, matching macOS, and is
classified correctly for the existing jitter decision handling.

## Routing

Verdict `ACCEPTED` → reviewer DoD items checked and task set to `done`.
Checklist item 5 (real coordinator traffic capture) remains deliberately open
as deferred manual scope in `EPIC-260714-th54l3`.
