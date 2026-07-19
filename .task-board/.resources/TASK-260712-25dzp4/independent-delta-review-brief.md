# Independent delta review: TASK-260712-25dzp4

Role: implementation-independent reviewer. Do not edit production code, tests, evidence, planning, or board status manually. Review the exact producer commit and attach a written outcome resource with an explicit verdict.

## Exact review target

- Repository: `/Users/administrator/Developer/Ivan/barycenter`
- Branch: `feat/task-260712-25dzp4`
- Exact producer SHA: `8f9ab2b4ec43f09c61607c6e3e3d2b3e286ccabb`
- Baseline merge: `5f1756d57df16a476b2df353f60656d24b02f752`
- Scope: production-dark Windows E2EE key-state foundation only.

Prove `git rev-parse HEAD` is the exact producer SHA and every reviewed file is byte-identical to it. If the workspace moved, use read-only `git show` or a detached temporary worktree. Do not review an uncommitted production delta and do not mutate the producer branch.

## Required review questions

1. DPAPI isolation: confirm distinct protected device metadata, signing, agreement, group, grant and bounded cache files plus independent protected witnesses. Verify the default actually calls the reviewed DPAPI adapter in current-user scope with `CRYPTPROTECT_UI_FORBIDDEN`, never `LOCAL_MACHINE`, optional entropy, description or plaintext fallback. Challenge native allocation zero/free and ciphertext/plaintext ownership.
2. Cross-process serialization: trace the in-process keyed lock and Win32 `CreateFile` share-none lock across read, revision validation, writes, final readback, returned reservation and lock close. Attempt to find any method that escapes the lock or can double-reserve generation across processes. Verify busy/close failures cannot acknowledge.
3. Durable persist-before-ack: inspect random ciphertext-only temp, complete write, write-through flag, flush, checked close, `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)`, exact destination decrypt/readback, record then witness then full pair reload. Challenge state-write/witness-write and both-written/readback-lost crash windows, stale temp cleanup, partial deletion, short I/O and oversized/corrupt envelopes.
4. Epoch/replay/fork/clone: challenge exact predecessor, one-epoch advance, overflow, stale revision, gap, copied group under another installation, partial three-slot identity, malformed canonical JSON, grant monotonicity/expiry/revoke, cache uniqueness/byte bounds/expiry/clear and EPC-005 target semantics. Separate what DPAPI+installation+witness detects from a coherent full snapshot rollback/clone by an attacker with decryption authority; reject overclaims.
5. Secret and diagnostics boundary: inspect every payload copy and zero path, DPAPI plaintext/ciphertext cleanup, closure lease behavior/finalizer, errors/descriptions, config/log/telemetry/crash source scans and file naming. Note realistic Go/runtime/page/crash-dump limits.
6. Production-dark and architecture: confirm no runtime/composition callsite, capability, crypto library/suite/container/key algorithm or plaintext fallback. Check the new 1,235-line repository for avoidable duplication, hidden coupling to credential storage, Windows/non-Windows build-tag correctness, package initialization effects and resource exhaustion.
7. Evidence integrity: independently reproduce all 14 packet hashes; audit validator assertions for circular/superficial checks; verify shared vectors match macOS semantics; ensure native DPAPI, signed MSIX, NTFS, profile roaming/backup/restore, event log, crash dump, memory and real crypto evidence remain `not-run` in `EPIC-260714-th54l3`.

Pay special attention to lock acquisition/release ordering, process-lock versus OS-lock scope, lock-file persistence, failure precedence when mutation and lock close both fail, record payload zeroing, cache slice aliasing/eviction zeroing, DPAPI master-key roaming limits, canonical JSON stability, and whether test fault injection truly hits the claimed crash boundaries.

## Independent commands

At minimum run and record exact results/timings for:

```sh
cd pulsar-win
gofmt -d windows_e2ee_key_state.go windows_e2ee_key_state_default_windows.go windows_e2ee_key_state_default_other.go windows_e2ee_key_state_test.go protected_platform_nonwindows_test.go
GOTOOLCHAIN=go1.25.12 go vet ./...
GOTOOLCHAIN=go1.25.12 go test . -run '^TestWindowsE2EE' -count=1
GOTOOLCHAIN=go1.25.12 go test -race . -run '^TestWindowsE2EE' -count=20
GOTOOLCHAIN=go1.25.12 go test ./...
GOTOOLCHAIN=go1.25.12 go test -race ./...
GOTOOLCHAIN=go1.25.12 GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOTOOLCHAIN=go1.25.12 GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/review-windows-e2ee-amd64.test .
GOTOOLCHAIN=go1.25.12 GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/review-windows-e2ee-arm64.test .
cd ..
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts.acceptance.test_windows_e2ee_key_state
PYTHONDONTWRITEBYTECODE=1 python3 scripts/acceptance/validate_windows_e2ee_key_state.py
```

Run the exact contract acceptance list from `scripts/acceptance/run_automated.py` if time permits; producer reports 222/222. Remove only reviewer-created `/tmp` binaries. Independently source-search composition roots and diagnostic sinks.

## Verdict format

Attach `TASK-260712-25dzp4_independent-delta-review-v1.md` containing exact SHA/hash proof, commands/counts/timings, severity-ranked findings with file/line evidence, explicit answers to all questions, a statement that no reviewed code was authored or modified, still-open gates/limitations, and one terminal verdict: `APPROVE`, `APPROVE WITH NON-BLOCKING FOLLOW-UPS`, or `REJECT`.

Any Critical/High rejects. Medium normally blocks unless demonstrably outside dormant engineering scope with an explicit downstream owner/DoD. Manual real-app/hardware evidence cannot be accepted here.
