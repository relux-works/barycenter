# Independent review verdict — TASK-260712-2kcduo

Verdict: **ACCEPTED**

- Reviewed ref: exact commit `30d23def4350aab22a19824c1e0cbcfad1a5f8da` in a detached
  worktree (`git worktree add --detach /tmp/review-2kcduo 30d23de…`). No moving branch
  reviewed; no production artifact modified by the reviewer.
- Scope: production-dark macOS protected-media send foundation
  (`MacProtectedMediaSendService`), key-state single-send-owner claim, golden/tamper/resume
  vectors, ADR, acceptance packet and cascade updates.
- No Critical, High, or Medium findings. Low/Info findings below are non-blocking
  follow-ups for the runtime-integration tasks.

## Reproduced evidence (fresh, this review)

All commands run inside the detached worktree at the exact SHA; all exit 0.

```text
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacProtectedMediaSendTests
  → 12/12 passed (Suite MacProtectedMediaSendTests passed)
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacE2EEKeyStateTests
  → 11/11 passed
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app
  → 331 tests in 54 suites passed (full regression)
python3 -m unittest scripts.acceptance.test_macos_protected_media_send
  → 5/5 OK (including fail-closed mutation tests: productionEnabled, runtimeHTTPWired,
    bounds drift, invented manual evidence)
python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'
  → 190/190 OK (full acceptance regression incl. cascade packets)
```

Hash recomputation: all 8 `artifacts[].sha256` entries in
`acceptance/phase3/macos-protected-media-send-v1.json` recomputed independently with
Python `hashlib` — all match (send-pipeline, send-tests, send-vectors, key-state,
protocol-authority, opaque-router, adr, threat-model). Cascade verified:
`macos-e2ee-key-state-v1.json` and `e2ee-recovery-device-transfer-v1.json` updated to the
new `MacE2EEKeyState.swift` hash `c10d1923…`, and `windows-e2ee-key-state-v1.json` updated
to the new macOS key-state packet hash `11994bf5…`; the 190-test discovery run proves the
cascade validators accept exactly this state.

Golden vectors independently recomputed from the fixture algorithm (not by re-running the
producer's tests): source SHA-256 `41640d55…`, manifest `fa8d7ea7…`, both chunk digests,
sizes, offsets, nonces (`fixture-nonce-1-0/1`) and whole-ciphertext digest `3d63152a…` all
match `protocol/macos-protected-media-send-v1-vectors.json`. Board resource copies under
`.task-board/.resources/TASK-260712-2kcduo/` are byte-identical to the repo artifacts.

## Checklist and AC verification

1. **Local preparation for clip/track/saved-cue** — `MacProtectedMediaKind` covers
   clip/saved_cue/track; `clipTrackAndSavedCueShareTheBoundedProtectedPipeline` exercises
   all three through the same bounded pipeline. Per the binding no-selected-stack
   constraint, no codec/container/crypto toolchain is selected: real preparation stays
   behind `MacProtectedMediaSealing` (`MacProtectedMediaSend.swift:161-168`) and the
   deterministic audit fixture proves orchestration only. Satisfied at foundation scope.
2. **Unique keys/nonces, authenticated manifests, target envelopes** — artifact
   validation enforces per-artifact nonce uniqueness
   (`MacProtectedMediaSend.swift:597`), non-empty bounded encrypted manifest, opaque key
   envelopes, authenticated manifest and signature (:592-595), exact context binding to
   group/epoch/generation/target-digest/sorted recipient set (:589, :464-470), and
   provider authentication before persistence (:605, test
   `invalidProviderSignatureFailsClosedBeforeCiphertextPersistence`). Duplicate nonces
   fail closed and consume the reserved generation (test + vector `duplicate-nonce`).
3. **Idempotent resume without reuse** — resume loads the exact persisted ciphertext,
   revalidates source fingerprint, chunk digests/offsets and whole-ciphertext digest
   (`validateResume`/`validateStoredCiphertext`), re-verifies the artifact, does not
   re-invoke the sealer and does not reserve a new generation
   (`interruptedUploadResumesExactCiphertextWithoutGenerationReuse`: sealCount=1,
   stageCount=1, sendGeneration stays 1). Stage/chunk/finalize/delete use stable
   per-draft idempotency keys (`mac-protected-{stage,chunk,finalize,delete}-…`) with an
   exact-byte idempotency contract documented on `MacProtectedMediaUploading`
   (:200-215); the fixture uploader rejects same-key/different-bytes replays.
4. **Plaintext policy** — two explicit policies: `user_owned_retain` never deletes
   (test `userOwnedSelectedFileIsRetainedAfterPublication`);
   `app_private_delete_on_terminal` is admitted only for paths resolving strictly below
   the private draft root (symlinks resolved, `isOwned` :831-835, checked at admission
   :578-582 and again at cleanup :754), deleted only after confirmed publication,
   explicit cancel, or bounded expiry recovery (≤100 drafts/run, 24h lifetime).
   Ciphertext dirs 0700, files 0600. NodeCore never writes a plaintext copy; it only
   streams a SHA-256 fingerprint (64 KiB reads, 64 MiB cap, regular-file check).
5. **No server plaintext, no silent downgrade** — `MacProtectedMediaStageRequest`
   carries only ciphertext, encrypted/authenticated manifests, opaque envelopes,
   signature and digests; no plaintext field exists in the upload protocol. Validator
   forbids `AES.GCM`/`ChaChaPoly`/`AVAssetExportSession`/`ffmpeg`/`URLSession(` in the
   pipeline source. Unsupported recipient → `unsupported_target` before generation
   reservation (test asserts sendGeneration stays 0); unverified/removed recipient →
   `target_changed`; stale revision/target digest → `target_changed`/`stale_key_state`;
   no code path falls back to the legacy media pipeline. Recipient admission re-runs on
   resume (`validate(request)` precedes the resume branch in `send`), so a downgrade
   cannot be smuggled through a retry.
6. **Single send owner before runtime wiring** — `claimProtectedMediaSendOwnership()`
   (`MacE2EEKeyState.swift:287-293`) is a non-releasable per-repository claim taken in
   both service initializers; a second sender over the same repository fails with
   `conflict` before any work (test `keyStateRepositoryAllowsOnlyOneProtectedSendOwner`).
   See finding L3/I2 for the exact resolution scope versus the earlier M1 concern.
7. **Tests green** — see reproduced evidence; focused, key-state regression, full Swift,
   focused acceptance and full acceptance discovery all pass.
8. **Fits project architecture** — actor-based NodeCore service behind protocol seams,
   production-dark, consistent with the prior key-state/recovery/opaque-router pattern
   (packet + validator + fail-closed mutation tests + cascade pins).

## Production-dark boundary (verified)

- `MacProtectedMediaSendService` does not appear anywhere in `node-app/Sources/NodeApp/`;
  the composition root (`main.swift`) has no reference and the validator enforces this.
- The audit-fixture initializer is `internal` to NodeCore
  (`MacProtectedMediaSend.swift:345`), so the app target cannot construct a service with
  an unapproved provider; the public initializer builds, but `send` fails
  `production_disabled` unless `sealer.productionApproved` (test
  `productionInitializerCannotEnableAuditFixtureProvider` proves no seal/stage occurs).
- `e2ee_media_v1` exists only as the dormant `E2EEAuditContract.capability` constant.
  Runtime capability advertisement flows exclusively from
  `ProtocolCapabilities` via `PlayerCore.advertisedCapabilities` /
  `MediaClipClient.advertisedCapabilities` / mixer `deliveryCapabilities`
  (overlay/interrupt/media-clip families); no E2EE capability is reachable.
- No logging of plaintext, secrets, keys or paths: the pipeline contains no log/print
  statement at all; persisted state (`state.json`, mode 0600 inside a 0700 dir) stores
  the source path and digests as declared, never key material or plaintext.

## Single-send-owner claim vs the earlier key-state review (brief item 5)

The prior independent key-state review (TASK-260712-1x9ruo, finding M1) flagged that
`processLock` serialization is process-local and required integration tasks to enforce
single-instance ownership or add cross-process serialization before runtime wiring.

For this dormant one-process integration the claim **resolves the process-local
duplicate-owner concern**: one repository can host at most one send actor for its entire
lifetime, and the claim is intentionally non-releasable (replacing the owner requires a
new repository that re-witnesses Keychain state). Additionally,
`reserveSendGeneration` performs its read-increment-persist under the process-wide static
lock with revision compare-and-swap, so even two repository instances over the same store
cannot double-reserve a generation *within one process* (they would serialize and receive
distinct generations, or conflict on a stale revision).

**This is explicitly not cross-process serialization.** The Keychain layer has no
cross-process CAS; two processes sharing the access group could still double-reserve. That
remains a binding production gate if packaging ever adds a second process with Keychain
access — correctly disclosed in the ADR ("cross-process serialization is still a
production integration gate…") and in the packet
(`deferredScope: cross-process-key-state-serialization-if-packaging-adds-another-process`).

## Design-delta decision (brief item 7)

Compatible with the approved candidate-neutral design; **no Critical/High design gate is
reopened**. No cipher suite, container, codec, group-crypto library or signature scheme is
selected or implied — the fixture suite/container strings are branded
`AUDIT_FIXTURE_*_NOT_FOR_PRODUCTION` and the vectors file carries
`status: audit-fixture-only-production-disabled`. The contract remains
`e2ee-media-audit.v1`; no coordinator endpoint, capability advertisement, or protocol
field change ships. The new upload orchestration (stage/chunk/finalize shapes, idempotency
keys, chunking bounds) is client-local behind protocols, and the packet pins
`deltaReviewRequired: true` with open gates `EPC-001, EPC-002, EPC-004, EPC-005,
TASK-260712-1ulshp` — nothing is silently blessed.

## Findings

No Critical. No High. No Medium.

- **L1 (resume binding gaps, `MacProtectedMediaSend.swift:659-675`)** — `validateResume`
  does not compare `draft.authorDeviceID` to `request.authorDeviceID`, and no
  group-revision recheck occurs on resume (the stored draft records epoch/generation but
  not the reservation revision). Stored values always win and recipient admission re-runs,
  so there is no plaintext, downgrade, or nonce-reuse path; a mismatched resume caller
  merely publishes under the originally bound author/epoch/generation. Also, a
  target-digest change on resume surfaces as `invalid_request` rather than
  `target_changed`. Suggested tightening for the runtime-integration task.
- **L2 (expiry recovery orphans staged remote objects,
  `MacProtectedMediaSend.swift:402-422`)** — `recoverExpiredDrafts` performs local
  terminal cleanup but, unlike `cancel`, never issues the idempotent remote `delete` for a
  draft with a staged `remote` object. An expired interrupted draft leaves a ciphertext
  object server-side (ciphertext-only, no confidentiality impact); server-side GC or a
  delete-on-expiry should be settled in the coordinator API task.
- **L3 (single-owner claim is per-repository-instance,
  `MacE2EEKeyState.swift:287-293`)** — nothing prevents constructing two repositories
  over the same Keychain store, each claiming one sender. In-process generation
  uniqueness still holds via the shared static lock + revision CAS (see above), so this
  is composition discipline, not a reservation hazard; the ADR states the one-repository
  requirement. Keep as an explicit acceptance criterion of the runtime wiring task.
- **L4 (vector without matching unit fixture,
  `protocol/macos-protected-media-send-v1-vectors.json:28`)** — the
  `target-membership-changed → target_changed` fail-closed vector has no dedicated Swift
  test (the code path at `MacProtectedMediaSend.swift:572-574` and the revision/digest
  guard at :437-439 are exercised only indirectly). The packet's `fixtures` map does not
  overclaim it. Add a fixture in the next iteration.
- **I1 (in-flight draft vs expiry recovery)** — `recoverExpiredDrafts` does not consult
  `activeDrafts`; a send suspended at an upload await whose draft crosses expiry could
  have its chunk files removed mid-flight, failing closed with `persistence_failed`. No
  reuse or plaintext risk; harmless under the documented 24h bound.
- **I2 (transient default-mode window)** — chunk/state files are written atomically and
  then chmod'd to 0600; the brief pre-chmod window inherits the process umask inside an
  already-0700 directory, so it is not exposable. Setting POSIX permissions at creation
  would remove the window.
- **I3 (cancel error naming)** — `cancel(draftID:)` reports `busy` for a malformed
  draft ID (`MacProtectedMediaSend.swift:386-388`); fail-closed but the code is
  misleading. Cosmetic.

## Non-claims (explicitly not verified by this review)

Signed/notarized app behavior, physical Keychain accessibility, real cryptographic
interoperability, real codec/container preparation, hardware upload/resume/quota
evidence, memory/crash/swap/backup plaintext-hygiene evidence, and audible-media checks
are all `not-run` and owned by `EPIC-260714-th54l3` and the open production gates
(`EPC-001, EPC-002, EPC-004, EPC-005, TASK-260712-1ulshp`). No real-crypto or
real-container claim is inferred from the deterministic audit fixture. The reviewer
authored and modified none of the reviewed code.
