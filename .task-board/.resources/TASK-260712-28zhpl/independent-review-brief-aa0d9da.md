# Independent review brief — TASK-260712-28zhpl

Review exact producer commit
`aa0d9daac964101135eea50fa94103c0152c43c3` only. Use a detached worktree or
otherwise prove that the reviewed production bytes equal this SHA. Do not
modify production code. Return a severity-ranked verdict resource and route
the task according to the explicit reviewer role: ACCEPTED only with zero open
Critical/High/Medium; otherwise REJECTED/to-dev with reproducible findings.

## Scope and authority

Task: Windows protected clip/track/saved-cue sending. The accepted design
authority is candidate-neutral `e2ee-media-audit.v1`, the independently
approved E2EE design verdict, the accepted coordinator opaque router, the
accepted Windows key-state repository and the accepted macOS send foundation.
This task must stay production-dark because EPC-001/002/004/005 and external
security acceptance remain open.

Primary artifacts:

- `pulsar-win/windows_protected_media_send.go`
- `pulsar-win/windows_protected_media_send_test.go`
- `protocol/windows-protected-media-send-v1-vectors.json`
- `acceptance/phase3/windows-protected-media-send-v1.json`
- `docs/analysis/p3-windows-protected-media-send-v1.md`
- `scripts/acceptance/validate_windows_protected_media_send.py`
- the narrow dormant-integration delta in
  `scripts/acceptance/validate_windows_e2ee_key_state.py`

## Required review questions

1. Verify unsupported/removed/unverified recipients, rights and exact target
   confirmation fail before generation reservation with no fallback.
2. Verify `media` generation reservation uses the accepted current-user DPAPI
   key state and its process + native share-none cross-process lock, then
   re-witnesses exact revision/epoch/commit/target before provider entry.
3. Verify the service-owned source is bounded to 64 MiB, never durably copied,
   zeroed immediately after provider return and absent from every uploader
   shape/log/runtime path. Look for symlink/path ownership and TOCTOU issues.
4. Verify provider output binds the exact group/epoch/generation/target/sorted
   recipients, has unique bounded nonces/chunks, authenticates before
   persistence, and cannot enable a production provider through existing app
   composition or capability advertisement.
5. Verify state JSON is strict and ciphertext-only; all source/chunk/whole
   digests and author/epoch/commit/target are checked on resume; an interrupted
   send reuses exact ciphertext and stable idempotency keys without reseal,
   nonce reuse or generation reuse.
6. Audit ambiguous Stage/PutChunk/Finalize results, the durable published
   revision checkpoint, explicit cancel and expired recovery. Ensure remote
   ciphertext deletion precedes local terminal cleanup and active drafts are
   not concurrently recovered.
7. Verify app-owned plaintext deletion is canonical-root confined and
   user-owned files are always retained. Distinguish code evidence from native
   NTFS/ACL/MSIX/manual evidence.
8. Independently recompute all ten packet hashes and Windows/macOS audit
   fixture parity. Confirm the older Windows key-state validator was narrowed
   only for this exact dormant boundary rather than weakening runtime-darkness.
9. Search all non-test production files for runtime/composition/capability
   wiring, provider/library/suite/container selection, plaintext fallback,
   HTTP/WS/ffmpeg integration, logging and invented manual evidence.
10. Decide whether any client-local contract delta reopens the approved crypto
    design gate. No real codec/container/crypto or signed-app claim is allowed.

## Producer evidence to reproduce independently

- `cd pulsar-win && go test -run '^TestWindowsProtectedMedia' -count=1`
  (25 test cases)
- `cd pulsar-win && go test -race -run '^TestWindowsProtectedMedia' -count=1`
- `cd pulsar-win && go test -race -run '^TestWindowsE2EE' -count=1`
- `cd pulsar-win && go vet ./... && go test ./... && go test -race ./...`
- Windows amd64 and arm64 `go test -c` blind compiles
- `python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'`
  (205/205)
- `python3 scripts/acceptance/run_automated.py` (16/16; producer manifest
  `.temp/acceptance/20260720T043704Z/manifest.json`)

Manual signed-MSIX, native DPAPI/NTFS, hardware, real cryptographic/container
interop, coordinator traffic capture, crash/swap/backup/memory inspection and
audible playback remain `not-run` in `EPIC-260714-th54l3`. Do not infer them
from deterministic fixtures or cross-compilation.
