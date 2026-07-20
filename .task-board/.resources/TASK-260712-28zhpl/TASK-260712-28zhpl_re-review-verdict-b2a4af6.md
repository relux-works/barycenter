# Independent re-review verdict — TASK-260712-28zhpl @ b2a4af6

**Verdict: ACCEPTED.** Zero open Critical/High/Medium findings. Both blocking
RUN-260720-6ead84 repros are structurally closed and proven by test. Reviewed
bytes: HEAD `b2a4af69530545ede4b82f31a451c556ef7c536f` with a clean production
worktree (only `.task-board` files dirty), verified before review. No
production code was modified; reviewer test scaffolding was run and removed
(worktree confirmed clean afterwards).

## Closure of the two blocking repros

**F1 — already-missing app-owned plaintext wedged terminal cleanup and
recovery.** Original repro re-run against b2a4af6 fails exactly at the
convergence point: `Cancel` now returns nil where it previously returned
`ErrWindowsProtectedMediaLocalCleanup` forever. Reviewer-authored closure test
(explicit-Cancel path, complementing the producer's recovery-path test
`TestWindowsProtectedMediaAlreadyMissingOwnedPlaintextCleanupConverges`)
verified: cancel with plaintext already absent converges and removes remote
ciphertext then the local draft; a second cancel is idempotent; a later-sorted
expired draft B is recovered (removed=1) instead of being wedged. The
missing-path branch converges only when the stored canonical path is lexically
below the private plaintext root (`validWindowsProtectedStoredSourcePath` +
`isWindowsProtectedOwned` on the stored path); reviewer tests confirmed a
directory at the source path and a root-escaping symlink still fail closed
with `ErrWindowsProtectedMediaLocalCleanup` and nothing — neither the foreign
node, the symlink itself, nor its outside target — is deleted.

**F2 — state-less final draft directory permanently blocked its draft ID.**
Original repro re-run fails exactly at the convergence point: recovery now
removes the orphan (removed=1 where it was 0). Producer test
`TestWindowsProtectedMediaStateLessFinalOrphanIsRecoverableAndDoesNotConsumeGeneration`
verified: the collision is rejected with `ErrWindowsProtectedMediaPersistence`
*before* generation reservation (SendGeneration stays 0), bounded recovery
removes the orphan, and the same draft ID then publishes with generation 1.

## Structural prevention audit

- `persistPrepared` now assembles chunks and strict `state.json` inside
  `os.MkdirTemp(ciphertextRoot, ".prepare-<draft>-")` (chmod 0700) and
  publishes with a single `os.Rename` to the final draft ID. Every failure
  path removes the temp directory: chmod failure removes explicitly; all later
  failures are covered by the `failed` deferred `RemoveAll`; a failed rename
  leaves `failed=true` so the temp is also removed.
- No silent overwrite of an existing final draft: Send rejects when the final
  path exists without `state.json` (before reservation) under the in-process
  per-draft lock; recovery skips active drafts; cross-process, rename onto a
  non-empty directory fails on POSIX and onto any existing directory on
  Windows, and final draft dirs are never empty (state.json + chunks).
- Recovery cannot sweep an in-flight `.prepare-*` directory: the leading dot
  fails `validWindowsProtectedToken`, so those entries are skipped
  (reviewer-test verified: removed=0, directory retained).

## Full aa0d9da-scope re-review (unchanged areas)

Re-verified in the b2a4af6 tree: unverified/removed/unsupported recipients and
rights/target confirmation fail in `validateRequest` before any key-state
access, with unsupported recipients mapped to a distinct non-fallback error;
`media` generation reservation happens only after witnessing exact
revision/target and is re-witnessed (revision/epoch/target/commit) before
provider entry, over the accepted DPAPI key state and its share-none lock;
plaintext is bounded to 64 MiB via LimitReader, canonicalized through
`canonicalRegularPath`, zeroed immediately after `Seal` and never enters any
uploader shape; artifact validation binds the exact context (group, epoch,
generation, target digest, sorted unique recipients), enforces unique bounded
nonces/chunks and requires `sealer.Verify` before persistence; stored state is
strict (`DisallowUnknownFields`, full digest/offset/nonce/phase revalidation,
ciphertext-only fields); resume rebinds source fingerprint, author, epoch,
commit and target and re-verifies the artifact without resealing or reserving
a new generation; Stage/PutChunk/Finalize use stable draft-scoped idempotency
keys with exact ciphertext reuse; the finalized revision is checkpointed
durably before terminal cleanup; cancel and expiry issue the remote delete
before local cleanup and skip active drafts. Production-dark holds: the
service is referenced by no non-test file other than its own, the only
constructors are the gated public one (requires `ProductionApproved()`) and
the audit-only fixture constructor, and imports contain no HTTP/WS/logging.

## Authority and evidence reproduction

- All ten acceptance packet hashes in
  `acceptance/phase3/windows-protected-media-send-v1.json` recomputed and
  match; only the three rework artifacts changed hashes; windows-key-state,
  protocol-authority, opaque-router, threat-model, macos-parity-vectors and
  design-review packets are byte-identical. `validate_windows_e2ee_key_state.py`
  has a zero-line diff in the rework — no runtime-darkness weakening.
- Windows/macOS fixture parity independently recomputed: suite, container,
  source/manifest/ciphertext SHA256, chunks and resume all match; fail-closed
  inventory is 11 including the two new lifecycle vectors; invariants grew to
  22 with the two new cleanup invariants, and the validator pins the new
  atomic-rename and Lstat code paths plus both new regression tests.
- Evidence reproduced synchronously on this run: focused 27/27 cases pass and
  under `-race`; `TestWindowsE2EE` under `-race` passes; `go vet ./...`, full
  `go test ./...` and full `-race` pass; Windows amd64 and arm64 blind
  `go test -c` compile; `python3 -m unittest discover` 205/205; automated
  harness 16/16 with fresh manifest
  `.temp/acceptance/20260720T050800Z/manifest.json`.

## Non-blocking observations (Low / informational)

- **L1 (Low, hygiene):** a crash during prepare leaves a `.prepare-<draft>-*`
  temp directory that no recovery path ever garbage-collects (recovery
  correctly skips dotted names to protect in-flight prepares). Content is
  ciphertext-only and the draft ID is not blocked, so there is no security or
  convergence impact — only unbounded-in-principle disk accumulation across
  repeated crashes. Suggested follow-up: age-based sweep of `.prepare-*`
  entries older than the maximum draft lifetime.
- **L2 (informational, pre-existing):** a present-but-corrupt `state.json`
  still halts `RecoverExpiredDrafts` at that entry with an error. This is
  deliberate fail-closed behavior (the draft may hold a live remote reference
  that should not be silently dropped) and surfaces the error to the caller;
  noted for operational awareness.
- **L3 (informational, pre-existing):** in the source-exists branch, a symlink
  planted at the stored source path that resolves to *another* owned regular
  file under the private plaintext root would delete that resolved target.
  Deletion remains strictly confined to app-owned regular files under the
  canonical private root (escaping symlinks, directories and foreign paths
  fail closed, test-verified), and planting the symlink already requires write
  access inside the app-private root, so this stays within the documented
  policy boundary.

## Manual scope

Signed-MSIX, native DPAPI/NTFS/ACL, real provider/container/crypto interop,
hardware, coordinator traffic capture, memory/crash/swap/backup inspection and
cross-platform audible playback remain `not-run` manual scope in
`EPIC-260714-th54l3` and are not inferred from the deterministic fixtures or
cross-compiles reproduced here.

## Routing

All Definition-of-Done review items hold: implementation matches the AC under
the deterministic-fixture boundary, the solution fits the accepted
production-dark architecture, and all reproduced tests are green. Task routed
to `done`.
