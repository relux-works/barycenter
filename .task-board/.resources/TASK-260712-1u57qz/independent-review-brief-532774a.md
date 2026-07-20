# Independent review brief — TASK-260712-1u57qz

Review exact producer commit
`532774a1c37778a744acba53e897c6308435ebc0` on branch
`feat/task-260712-1u57qz`. Current HEAD may contain only later task-board and
planning tracking; first prove that `git diff 532774a..HEAD` changes no
production, tests, protocols, acceptance packets, validators or ADR bytes.

Act only as an independent reviewer. Do not modify production code. You may
create temporary reviewer tests, but remove them before the terminal verdict.
Run all required evidence synchronously in this Claude turn; do not launch a
background monitor that can outlive the turn. Attach one terminal verdict
resource and route the task strictly:

- ACCEPTED and `done` only with zero open Critical/High/Medium finding;
- REJECTED and `development` when any Critical/High/Medium finding remains;
- keep signed-MSIX/native/hardware/real-provider/manual checklist item open and
  do not reinterpret deterministic fixtures or cross-builds as manual evidence.

Audit the complete producer delta from main `c5eede96a18e19703c503ca32256e87a2b932838`,
not only the happy-path tests.

## Required security and lifecycle review

1. Prove policy/DND/block and exact request admission precede manifest/range,
   key lease or provider access as applicable.
2. Prove contract/capability, object, recipient, group, revision, epoch,
   generation, target, manifest digest, route URL, ranges and ETag are exact and
   bounded; malformed, downgrade, future/forked and wrong-target input fails
   closed without fallback.
3. Prove identity/group/history leases come only from the accepted Windows key
   repository; revision, epoch, commit, target, expiry and live bounded history
   grant are rechecked before fetch, after fetch, after record authentication
   and at whole-object completion.
4. Prove transport/provider/caller cannot mutate the reader's frozen route
   slices. Probe aliasing provider output with ciphertext and verify only
   service-owned authenticated plaintext reaches the decoder.
5. Prove ciphertext hash is checked before provider entry, provider record
   authentication precedes decoder bytes, provider failure purges cache, and no
   plaintext/key enters disk, upload, log, HTTP, capability or runtime shapes.
6. Audit explicit revoke, remote revoke/block/delete, expiry, wrong target,
   corruption, membership rotation and grant revoke. Verify permanent versus
   retryable classification does not brick a legitimate bounded post-rotation
   re-grant and never resurrects a revoked route.
7. Adversarially review shared-root cache concurrency. Reproduce parallel
   distinct-object hits, same-object flights, tombstone versus cache hit,
   restart, index merge, orphan/temp cleanup and final-file collision. Confirm
   process-wide serialization, monotonic tombstone union, unique temp files and
   exact VariantURL authority prevent last-writer-wins loss or cross-object
   poisoning. Call out the explicit cross-process/manual boundary honestly.
8. Review player injection and lifetime: exact manifest equality, protected
   reader selected at decoder boundary, protected whole-object verification,
   revoke/close idempotence, decoder join, clear-stream behavior unchanged,
   generation/seek/clock/receipt/volume/PCM bounds preserved.
9. Confirm public production constructor rejects the audit provider; fixture
   constructor and service have no non-test runtime callsite; no provider,
   library, suite, container, codec, decoder, HTTP transport or capability is
   selected or advertised.
10. Recompute every artifact SHA in
    `acceptance/phase3/windows-protected-media-playback-v1.json`, verify exact
    macOS fixture parity, and review the delta-aware Phase 2/key-state guards so
    they cannot authorize an unrelated source change.

## Required fresh evidence

- focused protected playback tests and focused `-race`;
- `TestWindowsStream` regressions and `-race`;
- full `go test ./...`, full `go test -race ./...`, and `go vet ./...` in
  `pulsar-win`;
- Windows amd64 and arm64 blind test compile;
- `python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'` (210 tests
  expected at this commit);
- `python3 scripts/acceptance/run_automated.py --suite all` with a fresh 16/16
  manifest, or an exact synchronous equivalent covering all 16 commands.

Producer evidence to independently reproduce: final automated manifest
`.temp/acceptance/20260720T060457Z/manifest.json`, status pass, 16/16, manual
evidence `not-run`. The signed MSIX/native DPAPI/NTFS/ACL, real crypto/provider/
decoder, traffic, disk/log/memory/crash/swap/backup, physical audio/hardware and
real macOS-Windows interop scope stays in `EPIC-260714-th54l3` and must not be
claimed here.
