# Independent cleanup delta review v2 — TASK-260712-25dzp4 (Windows E2EE key state)

Reviewer: implementation-independent completion reviewer (Claude Fable 5, spawned reviewer role, run RUN-260719-c26436).
Date: 2026-07-20. Repository: `/Users/administrator/Developer/Ivan/barycenter`, branch `feat/task-260712-25dzp4`.

**Terminal verdict: APPROVE WITH NON-BLOCKING FOLLOW-UPS** (carried forward from v1; no new findings).

Statement of independence: I authored and modified **no** reviewed production code, tests, evidence, or planning files. My only writes were reviewer-created `/tmp` artifacts (all removed), this outcome resource, and board status/notes via `task-board`. All evidence below was collected in a fresh detached worktree at the exact final SHA — not the producer's working tree.

## Exact SHA / hash proof

- Detached worktree `git worktree add --detach /tmp/review-25dzp4 c7c9b020…`; `git rev-parse HEAD` = `c7c9b0206f61aa98920e8a21db55265fc9543b96`; `git status --porcelain` empty (0 lines) — reviewed bytes are exactly the committed final SHA.
- `sha256sum pulsar-win/windows_e2ee_key_state.go` = `cf7114abc4f86dc6de005e1e4679400c499b80ee94fd69792fe2f67b3aae15e5` — matches the brief's final repository artifact SHA-256 and the repinned `repository` hash in `acceptance/phase3/windows-e2ee-key-state-v1.json`.
- `git diff --check 8f9ab2b4..c7c9b020` clean (exit 0).
- All **14** packet hashes independently recomputed with a standalone Python sha256 walker: **14/14 match** at `c7c9b02`.

## Challenge answers

1. **No hidden scope — confirmed.** `git diff --stat 8f9ab2b4..c7c9b020` touches exactly `pulsar-win/windows_e2ee_key_state.go` (28 diff lines; 16 insertions / 14 deletions across the two files, of which the packet accounts for 1/1) and the one-line `repository` hash repin in the packet. Read line-by-line: every hunk is either (a) converting a post-decode `defer zeroBytes(x)` into a closure-based `defer func() { zeroBytes(x) }()` registered **before** the first `decodeWindowsE2EEPayload` call, or (b) adding `zeroBytes(record.Payload)` on the two malformed-canonical-JSON error returns in `loadRecord`. No behavioral, protocol, bounds, locking, or evidence change beyond the cleanup.
2. **Closure defers before decode at every secret-allocating target — confirmed.** All ten sites verified: `InstallDeviceIdentity` re-install path (sp/ap, lines 332–333), `LoadDeviceIdentity` (403–404), `PersistGroupState` previous opaque state (442), `LoadGroupState` (491), `ReserveSendGeneration` (524), `StoreGrant` previous opaque grant (576), `LoadGrant` (617), `CacheContentKey` entries (658), `LoadContentKey` entries (716), `requireInstallation` (791–792). Each is a closure over the variable (evaluated at run time, not defer-registration time), so the pre-decode-nil-slice-by-value hazard named in the brief is avoided at every site; `windowsE2EEDevicePayload` (metadata) holds no secret slices, so its lack of a defer is correct.
3. **Partial-payload zeroing and single-clear ownership — confirmed.** Malformed record JSON zeroes the partially decoded `record.Payload` at `windows_e2ee_key_state.go:902`; malformed witness JSON zeroes the already-decoded record payload at line 906; the existing validation-failure path (line 910) still zeroes. On success paths, `decodeWindowsE2EEPayload` (line 1128) clears `record.Payload` exactly once via its own defer; the decoded-target closures clear the *decoded* slices, which are distinct allocations (base64 → fresh slice), so there is no double-clear of a still-needed buffer. The new closures run only when the `withExclusiveLock` body returns — after `newWindowsE2EESecretLease`/`WindowsE2EESecretLease` has copied the bytes into the redacted lease — so leases never observe cleared values (verified by the passing lease round-trip tests).
4. **Cache aliasing/eviction — no defect.** `kept := payload.Entries[:0]` is the standard in-place filter; dropped (expired/replaced) entries have their `Key` bytes zeroed inline before their backing-array positions can be overwritten; front-evicted entries are zeroed before `payload.Entries = payload.Entries[1:]`; the single deferred `zeroWindowsE2EEContentEntries(payload.Entries)` closure observes the final slice and sweeps all surviving key copies (including the freshly appended copy of the caller's key — the caller retains ownership of its own slice). `LoadContentKey` copies the hit entry's key into the lease before the deferred sweep. No missed clear, no harmful double-clear, no by-value nil capture.
5. **Hashes, tests, validator, full harness — all reproduced.** In the detached worktree at `c7c9b02`:
   - 14/14 packet hashes match (independent recomputation).
   - `gofmt -d` (5 files): clean, 0.01s. `GOTOOLCHAIN=go1.25.12 go vet ./...`: OK, ~1.0s.
   - `go test . -run '^TestWindowsE2EE' -count=1`: ok 0.47s — 12/12 tests pass (10 new key-state tests + 2 pre-existing audit-model tests).
   - `go test -race . -run '^TestWindowsE2EE' -count=20`: ok 7.4s, race-clean.
   - `go test ./...` and `go test -race ./...`: all packages ok (15.2s / 19.1s), `go version go1.25.12 darwin/amd64`.
   - `GOOS=windows CGO_ENABLED=0` amd64 vet OK (1.1s); amd64 and arm64 test binaries compiled (5.3s / 5.6s), removed after review.
   - `python3 -m unittest scripts.acceptance.test_windows_e2ee_key_state`: 5/5 OK (0.02s). `validate_windows_e2ee_key_state.py`: "Windows E2EE key state: PASS (production disabled)".
   - **Full harness `run_automated.py`: exit 0, 16/16 stages, 5m43s**, manifest records `head: c7c9b0206f…`, `startDirty: false`, `endDirty: false`, `status: pass`, contract batch "Ran 222 tests in 94.8s … OK", toolchains `go1.25.12`, Xcode `26.2` (build 17C52), Swift `6.2.3`, `manualEvidence: not-run` — the producer's clean-worktree full-harness claim is independently confirmed, not merely trusted.
6. **Carried-forward findings from v1** (`TASK-260712-25dzp4_independent-delta-review-v1.md`):
   - v1 finding 1 (Low, decoded secrets not cleared on early decode-error paths at `8f9ab2b`) — **this delta is the fix; verified resolved at `c7c9b02`.**
   - v1 finding 2 (process, non-blocking): producer sessions must be quiesced while an exact-SHA independent review is in flight — remains a standing orchestration follow-up (owner: orchestrator).
   - v1 finding 3 (Info): envelope/atomic-replace/witness machinery partially parallels `protected_repository.go` — deliberate isolation of a dormant subsystem; consolidation candidate once the E2EE stack is selected (owner: future producer task).
   - No Critical, High, or blocking-Medium findings at either SHA; nothing in this delta introduces new findings.

## Still-open gates and limitations (unchanged, none accepted here)

- Real DPAPI, signed MSIX identity, NTFS durability/locking, profile roaming/backup/restore, event log, crash dump, memory forensics, and real crypto interop remain manual, `not-run`, owned by `EPIC-260714-th54l3` (verified still in `backlog`); the packet's `manualEvidence` and validator enforce this fail-closed.
- Production enablement remains blocked by `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, and external security closure `TASK-260712-1ulshp`.
- Best-effort memory clearing is not a forensic-wipe guarantee (Go runtime copies, paging, crash dumps), and the local witness mechanism does not defeat a coherent full-snapshot rollback/clone by an attacker holding DPAPI decryption authority — both correctly documented in the ADR.

## Verdict

**APPROVE WITH NON-BLOCKING FOLLOW-UPS** — the cleanup delta is exactly its claimed scope, fixes the one code finding of the v1 review, and the full evidence battery reproduces independently at the exact final SHA. Follow-ups (all carried, none blocking): producer quiescence during exact-SHA reviews (orchestrator); eventual envelope-machinery consolidation (future task); manual/hardware evidence per `EPIC-260714-th54l3`'s existing DoD.
