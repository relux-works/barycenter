# Independent delta review — TASK-260712-25dzp4 (Windows E2EE key state)

Reviewer: implementation-independent reviewer (Claude Fable 5, spawned reviewer role).
Date: 2026-07-20. Repository: `/Users/administrator/Developer/Ivan/barycenter`, branch `feat/task-260712-25dzp4`.

**Terminal verdict: APPROVE WITH NON-BLOCKING FOLLOW-UPS** (covers producer SHA `8f9ab2b` and the mid-review producer fix `c7c9b02`; see "Review-target movement" — the movement itself is the primary follow-up).

Statement of independence: I authored and modified **no** reviewed production code, tests, evidence, or planning files. My only writes were reviewer-created `/tmp` artifacts (all removed), this outcome resource, and board status/notes via `task-board`. Commit `c7c9b02` was authored by the producer session, not by this reviewer.

## Exact SHA / hash proof

- Review start: `git rev-parse HEAD` = `8f9ab2b4ec43f09c61607c6e3e3d2b3e286ccabb` (exact brief target); baseline `5f1756d57df16a476b2df353f60656d24b02f752`; `git diff HEAD -- pulsar-win scripts` empty (all reviewed files byte-identical to the commit).
- All **14** packet hashes in `acceptance/phase3/windows-e2ee-key-state-v1.json` independently reproduced with a separate Python sha256 script: 14/14 match at `8f9ab2b`.
- After tip movement (below), re-verified in detached read-only worktrees: `8f9ab2b` battery fully green; `c7c9b02` battery fully green with the repinned `repository` hash (`cf7114ab…`) reproduced, 14/14 match, and `git show c7c9b02:pulsar-win/windows_e2ee_key_state.go` byte-identical (sha256) to the delta I had already captured and read line-by-line.

## Review-target movement during review (process finding, Medium→resolved, non-blocking follow-up)

While this review was in flight, the production file `pulsar-win/windows_e2ee_key_state.go` was modified in the shared working tree at 02:47:58 by a process outside this review session (a concurrent full-access producer session was running; producer of record is `codex-root-inline`). At 02:53:51 the producer committed it as `c7c9b02 fix(pulsar-win): clear partial E2EE payloads` (28 source lines + 1 contract hash repin). Effects observed live: the standalone acceptance validator and contract-test batch failed on artifact hash drift until the commit landed.

Handling: per the brief I did not review any uncommitted delta and did not mutate the branch. I moved all evidence collection into detached worktrees pinned to exact SHAs. The delta is precisely a fix for the one code finding of this review (below), which I had identified independently before seeing it; because it is 28 lines I had already fully analyzed, I extended verification to `c7c9b02` rather than burning a review cycle. **Follow-up:** producer sessions must be quiesced while an exact-SHA independent review is in flight; a moving target invalidates review evidence and this only converged because the drift was detected.

## Findings (severity-ranked)

1. **[Low — fixed in `c7c9b02`] Decoded secrets not cleared on early decode-error paths (at `8f9ab2b`).** In `InstallDeviceIdentity` (re-install path), `LoadDeviceIdentity`, `requireInstallation`, `PersistGroupState`, `LoadGroupState`, `ReserveSendGeneration`, `StoreGrant`, `LoadGrant`, `CacheContentKey`, `LoadContentKey`, the `defer zeroBytes(...)` for decoded private keys / opaque state was registered only after all decodes succeeded; if a later decode failed (corrupt slot), an earlier-decoded secret stayed unzeroed in heap until GC. `loadRecord` similarly left `record.Payload` unzeroed when witness decode failed. Failure scenario: corrupt agreement slot + intact signing slot → signing private key bytes linger in process memory on the fail-closed path. Impact bounded: all such paths fail closed, and the ADR explicitly scopes memory clearing as best-effort (Go copies/paging/dumps excluded). `c7c9b02` registers closure defers before decoding and zeroes partial `record.Payload` — verified line-by-line and by full battery. No other zeroization gaps found.
2. **[Process, non-blocking] Mid-review branch mutation** — see section above.
3. **[Info] Duplication** — envelope/atomic-replace/witness machinery partially parallels `protected_repository.go` (different magic `BEKS` vs `BCDP`, bounds, witness semantics). Deliberate isolation of a dormant subsystem; acceptable now, consolidation candidate later. No hidden coupling to credential storage: only the already-reviewed primitives (`dataProtector`, `secureFileOps`, `keyedLockSet`, `zeroBytes`) are shared.

No Critical, High, or blocking-Medium code defects found.

## Answers to the required review questions

1. **DPAPI isolation — confirmed.** Six distinct slot kinds (`device_metadata`, `device_signing`, `device_agreement`, `group/<id>`, `grant/<id>`, `content_cache`), each with separate `state-…​.dpapi` and independently DPAPI-protected `witness-…​.dpapi` files under `e2ee-key-state-v1` (test asserts 6 protected files after identity install and no plaintext key bytes in any). Windows default wires `dpapiDataProtector{api: windowsDataProtectionAPI{}}`: `CryptProtectData`/`CryptUnprotectData` called with flags = `CRYPTPROTECT_UI_FORBIDDEN` (0x1) only — no `LOCAL_MACHINE` (0x4), entropy/description/prompt args all 0 → current-user scope. Non-Windows default returns `ErrWindowsE2EEUnavailable`; no plaintext fallback anywhere (tested). Native allocations: `Bytes()` copied, native buffer zeroed, then `LocalFree`; plaintext/ciphertext intermediates zeroed on all paths (post-`c7c9b02` including error paths). Ownership is clean: DPAPI output is copied before free; file layer only ever sees ciphertext.
2. **Cross-process serialization — confirmed, no escape found.** All 12 public state methods wrap `withExclusiveLock`; `DecideWindowsE2EETargetDevice` is pure. Order: in-process `keyedLockSet` keyed by lock path → `EnsureDir` → Win32 `CreateFile` share-none `AcquireLock` (`fileOpenAlways`) → body (read → revision validation → writes → final readback → returned reservation) → `Close`. Acquire failure → `ErrWindowsE2EEBusy` before any read; close failure after successful body → `ErrWindowsE2EEUnavailable` (no ack); body error takes precedence over close error. The OS lock spans the whole read-validate-write-readback window, so two processes cannot double-reserve a generation; in-process races are serialized by the keyed mutex (race detector ×20 clean). `repository.lock` is never deleted: `deleteSlot` touches only state/witness paths and `cleanupTemps` matches only the exact `<base>.tmp.<16 hex>` grammar.
3. **Durable persist-before-ack — confirmed.** `writeProtectedBytes`: stale-temp cleanup (exact grammar), bounded envelope (`BEKS` v1, length-checked ≤3 MiB plaintext / ≤4 MiB ciphertext both directions), random 8-byte-hex temp name, `CREATE_NEW` + `FILE_FLAG_WRITE_THROUGH` handle, DPAPI-protect **before** any file write (temp holds ciphertext only), short/overlong-write-guarded full write loop, `FlushFileBuffers`, checked close, `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)`, then destination decrypt + byte-exact readback. `persistSlot`: record write → witness (revision + record digest) write → full pair reload + field-exact comparison → only then return the revision. Crash windows tested with fault injection that genuinely hits the boundaries (indexed `failAt["move"]` between the two moves; `afterMove` hook killing the readback open after the witness move): state-written/witness-stale → digest mismatch → fails closed `rollback_or_clone`; both-written/readback-lost → generation/revision consumed, old revision later conflicts (no nonce reuse). Partial deletion of a pair → existence asymmetry → `rollback_or_clone`. Oversized/corrupt envelopes and short I/O fail closed.
4. **Epoch/replay/fork/clone — confirmed, no overclaim.** Advance requires `revision` match, exact `previousCommitDigest`, and `epoch == previous.Epoch+1`; `epoch <= previous` → `stale_epoch`; gap or wrong predecessor → `rollback_or_clone`; `previous.Epoch == MaxUint64` → `rollback_or_clone`; `SendGeneration == MaxUint64` → `replay`; stale revision → `conflict`. Copied group into a different installation fails on installation-ID binding (tested); partial three-slot identity fails closed (tested); non-canonical/unknown-field/trailing JSON fails via `DisallowUnknownFields` + re-marshal byte-equality (Go struct marshal is deterministic; no maps; `RawMessage` round-trips verbatim). Grants: same `FirstEpoch`, non-decreasing `LastEpoch`/expiry, replay rejected, expiry fail-closed, revoke → not-found (tested). Cache: unique object IDs, ≤32 entries, ≤64 KiB total, ≤4 KiB per key, expiry-evicting, oldest-first eviction, clearable (tested); eviction and skip paths zero evicted key slices; the `[:0]` filter aliasing is the safe standard idiom and the final deferred sweep zeroes all live key copies. EPC-005 pinned: active member, registered>0, verified=0 → `removed_endpoint`; verified>0, supported=0 → `unsupported_target`; verified+supported → `route` (shared vector, tested). The ADR explicitly does **not** claim protection against an attacker with DPAPI decryption authority coherently rewriting the full snapshot — correctly scoped, no overclaim.
5. **Secret and diagnostics boundary — confirmed (with finding 1 at `8f9ab2b`, fixed at tip).** Every payload copy has a zero path (success paths at `8f9ab2b`; all paths at `c7c9b02`); leases hand closures a temporary copy zeroed after return; `Destroy` + finalizer clear the retained buffer; repository and all lease `String`/`GoString` are redacted (tested); all errors are static sentinels carrying no key/ID material; file names embed only SHA-256 tokens of scope strings; forbidden diagnostic tokens (`config.json`, `log.Printf`, `slog.`, `fmt.Printf`) absent (test + validator). Realistic limits (Go runtime copies, paging, crash dumps, DPAPI internals) are stated in the ADR and deferred to manual forensics — honest.
6. **Production-dark and architecture — confirmed.** No runtime/composition callsite: `main.go` scan (test), plus validator scan of every non-E2EE production `.go` file, plus my independent repo-wide grep — the repository type appears only in its own files; `e2ee_media_v1` appears only in the pre-existing audit-model constant and is a forbidden token in key-state source. No crypto library/suite/container/key algorithm selected; all private blobs opaque caller-supplied; SHA-256 used only for consistency digests and filename tokens. Build tags correct (portable repo file; gated adapters; non-Windows default fails closed — tested); `windows/amd64` and `windows/arm64` vet + test-compile green; no `init()` side effects; all state bounded (envelope, cache, grant, key sizes), temp cleanup bounded by exact grammar — no resource-exhaustion vector found. Duplication noted as Info finding 3.
7. **Evidence integrity — confirmed.** 14/14 packet hashes reproduced independently at both SHAs. Validator assertions are fail-closed and non-circular in the ways that matter: hashes pin real files, ordering assertion checks state-write < witness-write < reload in source, and the 5-case unittest proves tampering (production enable, LOCAL_MACHINE, lock removal, manual-evidence inflation) raises; token-presence checks are syntactic but are backed by the semantic Go tests. Shared vectors are the byte-identical `protocol/e2ee-key-state-v1-vectors.json` consumed by `MacE2EEKeyStateTests.swift` with matching semantics (epoch 7→8, generation 1, crash-vector names/expectations, EPC-005). Native DPAPI, signed MSIX, NTFS, roaming/backup/restore, event log, crash dump, memory, and real crypto evidence remain `not-run`/`not-run-no-selected-stack` in the contract, and `EPIC-260714-th54l3` is in `backlog` — nothing invented (unittest enforces).

## Commands, counts, timings

At `8f9ab2b` (working tree pre-drift, then re-confirmed in detached worktree `/tmp/review-25dzp4-wt`):

- 14-hash independent reproduction: 14/14 OK.
- `gofmt -d` (5 files): clean (0.01s). `go vet ./...`: OK (~0.7–1.3s).
- `go test . -run '^TestWindowsE2EE' -count=1`: ok 0.46s / 0.58s (12 tests incl. 2 pre-existing audit tests; all 10 new tests PASS).
- `go test -race . -run '^TestWindowsE2EE' -count=20`: ok 6.6s / 8.5s.
- `go test ./...` and `go test -race ./...`: all packages ok.
- `GOOS=windows` amd64 vet OK; amd64/arm64 test binaries compiled (17.4 MB / 16.1 MB), removed after review.
- `python3 -m unittest scripts.acceptance.test_windows_e2ee_key_state`: 5 tests OK.
- `validate_windows_e2ee_key_state.py`: PASS (production disabled).
- `run_automated.py`: exit 0, 16/16 steps, contract batch "Ran 222 tests" OK (6m47s).

At `c7c9b02` (detached worktree `/tmp/review-25dzp4-wt2`): same battery — gofmt clean, vet OK, focused ok 0.50s, race ×20 ok 6.8s, full + full-race ok, amd64/arm64 cross OK, unittest 5/5, validator PASS, 14/14 hashes, `run_automated.py` exit 0 with "Ran 222 tests" (5m45s).

Note: my first working-tree runs of the Python validator/suite failed with "artifact drifted: repository" — that was the live detection of the mid-review mutation, not a defect at either SHA.

## Still-open gates and limitations

- Manual/hardware evidence (native DPAPI, signed MSIX identity, NTFS durability/locking, profile roaming/backup/restore, event log, crash dump, memory forensics, real crypto interop) remains `not-run` in `EPIC-260714-th54l3`; nothing here substitutes for it and none was accepted.
- Production enablement stays blocked by `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, external security closure `TASK-260712-1ulshp`, and future reviewed-library integration.
- Best-effort memory clearing is not a forensic-wipe guarantee (Go runtime copies, paging, dumps) — as documented.
- This local witness mechanism does not defeat a full-snapshot rollback/clone by an attacker holding DPAPI decryption authority — as documented.

## Verdict

**APPROVE WITH NON-BLOCKING FOLLOW-UPS.**

Follow-ups (non-blocking, with owners):
1. Orchestrator: enforce producer quiescence during exact-SHA independent reviews (this review's target moved mid-flight; evidence had to be re-established in detached worktrees).
2. Producer/future task: consider consolidating the duplicated envelope/atomic-replace/witness machinery with `protected_repository.go` once the E2EE stack is selected.
3. Manual evidence remains owned by `EPIC-260714-th54l3` with its existing DoD.
