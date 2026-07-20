# Independent re-review brief — TASK-260712-28zhpl

Review exact rework commit
`b2a4af69530545ede4b82f31a451c556ef7c536f` against producer
`aa0d9daac964101135eea50fa94103c0152c43c3` and the two reviewer repros in
`TASK-260712-28zhpl_review-repro-test.go`.

This run must produce and attach a terminal severity-ranked verdict and route
the task before ending. Run long tests synchronously with an adequate timeout;
do not arm a background monitor and end the Claude turn while it is running.
Do not modify production code. ACCEPT only with zero open
Critical/High/Medium; otherwise REJECT/to-dev with exact repro evidence.

## Mandatory closure checks

1. Re-run both original repro scenarios against `b2a4af6` and prove they now
   converge:
   - app-owned plaintext already absent at terminal cleanup is idempotent;
     cancel/recovery remove ciphertext and later drafts are not wedged;
   - a state-less final draft collision is rejected before generation
     reservation, bounded recovery removes it, and the same draft ID can then
     publish with generation 1.
2. Audit the structural prevention: initial chunks and strict `state.json` are
   built inside a private `.prepare-<draft>-*` directory and the complete
   directory is atomically renamed to the final draft ID. Verify every failure
   path removes its temp directory and cannot overwrite an existing final
   draft.
3. Audit missing-source cleanup carefully: it may converge only when the stored
   canonical path is lexically below the configured private root; an existing
   symlink/directory/foreign path must still fail closed and must never be
   deleted.
4. Re-review the entire exact aa0d9da scope, not only the delta: target
   admission before reservation; Windows share-none generation serialization;
   plaintext lifetime/zeroing; provider context, nonce and authentication;
   strict ciphertext-only crash state; author/epoch/commit/target/source resume
   binding; ambiguous idempotent upload/finalize; published revision checkpoint;
   cancel/expiry ordering; production-dark runtime/capability/provider state.
5. Recompute all ten current packet hashes and Windows/macOS audit-fixture
   parity. Confirm no accepted authority packet was silently weakened.
6. Keep signed-MSIX, native DPAPI/NTFS, real provider/container/crypto,
   hardware, traffic capture, memory/crash and cross-platform playback as
   `not-run` manual scope in `EPIC-260714-th54l3`.

## Exact rework evidence to reproduce

- `cd pulsar-win && go test -run '^TestWindowsProtectedMedia' -count=1`
  (27 test cases)
- `cd pulsar-win && go test -race -run '^TestWindowsProtectedMedia' -count=1`
- `cd pulsar-win && go test -race -run '^TestWindowsE2EE' -count=1`
- `cd pulsar-win && go vet ./... && go test ./... && go test -race ./...`
- Windows amd64/arm64 `go test -c` blind compiles
- `python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'`
  (205/205)
- `python3 scripts/acceptance/run_automated.py` (16/16; producer manifest
  `.temp/acceptance/20260720T045743Z/manifest.json`)

The initial run `RUN-260720-6ead84` did not create a verdict because its
background acceptance monitor outlived the Claude turn. Its two attached
repros are nevertheless treated as blocking review findings and are the
primary closure authority for this re-review.
