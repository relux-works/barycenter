# Independent review verdict — TASK-260712-1u57qz

**Verdict: ACCEPTED → done. Zero open Critical/High/Medium findings.**

Reviewer: independent Claude (Fable 5, max), read-only on production code.
Reviewed commit: exact producer commit `532774a1c37778a744acba53e897c6308435ebc0`
on `feat/task-260712-1u57qz`, full delta audited from accepted main
`c5eede96a18e19703c503ca32256e87a2b932838`.

## Delta integrity

`git diff 532774a..HEAD` touches only `.planning/` and `.task-board/` tracking
files (review brief resource + progress files). Zero production, test,
protocol, acceptance-packet, validator or ADR bytes changed after the producer
commit. The producer delta from main is exactly the 11 production/test/
protocol/acceptance/validator files plus board/planning tracking.

## Required security and lifecycle review (brief items 1–10)

1. **Admission ordering** — `validateWindowsProtectedMediaPlaybackRequest`
   (identifier/digest/epoch/generation bounds, then policy/DND/blocked) runs
   before any key-state load, manifest fetch or range fetch.
   `TestWindowsProtectedMediaPlaybackProductionAndPolicyRemainDark` proves 0
   manifest and 0 range calls on policy denial. VERIFIED.
2. **Exact bounded binding, fail-closed** — `validateWindowsProtectedMediaPlaybackRoute`
   requires exact contract `e2ee-media-audit.v1` / capability `e2ee_media_v1`
   (downgrade typed as `downgrade_forbidden`), exact object/recipient/group/
   epoch/generation/target match against the request, manifest digest recomputed
   over the encrypted manifest, bounded suite/container/manifest/envelope/
   signature sizes, `svm1.protected.` identity prefix, `/v1/media/<object>/`
   route prefix, per-variant size ceiling, canonical contiguous chunk ranges via
   `validateWindowsStreamManifestShape`. Future/forked epoch → `forked_epoch`;
   same-epoch target mismatch → `target_changed`; historical epoch without
   grant → `missing_grant`. No fallback path exists. VERIFIED.
3. **Accepted key repository + recheck cadence** — identity/group/grant leases
   come only from `WindowsE2EEKeyStateRepository` (`windows_e2ee_key_state.go`
   SHA unchanged from accepted main and pinned in the packet). Revision, epoch,
   commit digest, target digest, and live bounded grant (group + epoch bounds +
   expiry at fresh `checkedAt`) are rechecked by `authorizeAndRevalidate` before
   fetch, after fetch, after record authentication, and on both sides of
   whole-object verification. VERIFIED.
4. **Frozen route slices / aliasing** — route is deep-cloned at Prepare, per
   provider call, and for the public `Route` field. Reviewer wrote a temporary
   adversarial probe (removed before this verdict): a provider returning
   plaintext aliasing the ciphertext buffer and mutating it post-return, a
   transport retaining and mutating returned bodies, and caller mutation of
   `prepared.Route` — decoder-visible bytes stayed the exact authenticated
   plaintext, cached ciphertext stayed sound; passed under `-race`. VERIFIED.
5. **Hash before provider, auth before decoder, no leakage** — cache verifies
   SHA-256 of every fetched and every cached chunk before bytes reach the
   provider; provider record authentication (AEAD seam) gates decoder bytes;
   failed record auth purges cache (`stats.Bytes == 0` asserted); plaintext is
   copied into service-owned memory and source buffers zeroized; disk scan test
   proves no plaintext/lease canary on disk; `String()/GoString()` redact route
   and lease; no log/HTTP/capability writes in the boundary. VERIFIED.
6. **Lifecycle classification** — permanent (revocation marker + tombstone):
   blocked, revoked, expired. Retryable/local-only (invalidate, no marker):
   wrong target, membership rotation, corrupt ciphertext, invalid record auth.
   `MembershipRotationPurgesButAllowsBoundedRegrant` proves rotation purges the
   frozen reader and a bounded history re-grant reopens under current-state
   authorization; `RevocationMarkerIsMonotonicAcrossActors` proves a revoked
   route never resurrects across parallel actors and restart (0 extra range
   fetches after revoked restart). VERIFIED.
7. **Shared-root cache concurrency** — process-wide serialization via
   `globalWindowsStreamProcessLocks` keyed by absolute index path; read-merge-
   write with monotonic tombstone union in `mergePersistedIndexProcessLocked`;
   unique temp files via `os.CreateTemp`; final-file rename collision accepted
   only on byte-identical content; variant authority includes exact
   `VariantURL` (+identity+ETag) so equal ciphertext across objects cannot
   share delete/revocation state; tombstone re-checked after index merge on
   the cache-hit fast path (covers tombstone-vs-hit race); restart repair
   removes orphans/temp/mismatched files; lock ordering is consistently
   `cache.mu → processMu` (no deadlock path). Concurrent distinct-object,
   same-object flight dedupe, restart merge and cross-actor tombstone tests
   pass under `-race`. **Honest boundary:** the index lock registry is
   process-global only — two separate OS processes sharing a root are not
   serialized on the index file. This does not break security fail-closed:
   revocation markers are `O_EXCL`-created files `Lstat`-checked on every
   record read (cross-process safe), every chunk is re-hashed on every read,
   and record AEAD is the final gate; the residual cross-process exposure is
   cache-efficiency, not authorization. True multi-process/forensic evidence
   stays manual in EPIC-260714-th54l3. VERIFIED with boundary called out.
8. **Player injection and lifetime** — `Load` requires
   `reflect.DeepEqual(player.chunks.Manifest(), manifest)`; protected reader is
   selected at the decoder boundary only (`startDecoderLocked`); protected EOF
   verification routes through `VerifyWhole` on the protected reader; `Revoke`
   routes to the reader (marker + tombstone + lease destroy) and `Close` is
   `closeOnce`-idempotent with decoder join preserved; clear-stream constructor
   and path byte-identical in behavior (nil reader → existing cache reader);
   generation/seek/clock/receipt/volume/PCM ring bounds unchanged — full
   `TestWindowsStream` regressions plus `-race` green. VERIFIED.
9. **Production darkness** — `Prepare` rejects non-`ProductionApproved`
   providers outside fixture mode (`production_disabled` test); fixture
   constructor `newWindowsProtectedMediaPlaybackServiceForAudit` has zero
   non-test callsites (grep + validator runtime-tree scan asserting
   `WindowsProtectedMediaPlaybackService` absent from all other non-test
   sources); validator forbids crypto/codec/exec/HTTP tokens in the boundary
   file; no capability advertised. VERIFIED.
10. **Packet and guards** — all 11 artifact SHA-256 in
    `acceptance/phase3/windows-protected-media-playback-v1.json` recomputed by
    the reviewer and exact; macOS fixture parity (suite, container, producers,
    ciphertext and per-chunk ciphertext/plaintext digests, bounds) verified by
    validator, Go parity test and file-level SHA pin; 9 fail-closed vectors
    present. Delta-aware Phase 2 guard in
    `validate_stream_performance_review.py` is bounded to exactly
    `stream_player.go`/`stream_cache.go` and only accepts a digest that the
    production-dark playback packet pins exactly while
    `productionEnabled`/`runtimeHTTPWired` are false — it cannot authorize an
    unrelated source change. Key-state guard extension pins the exact dormant
    witnessed boundary tokens. VERIFIED.

## Fresh evidence (all reproduced synchronously this review)

- `go vet ./...` (pulsar-win): PASS
- focused `-run 'TestWindowsProtectedMedia'` and same with `-race`: PASS
- `TestWindowsStream` regressions and `-race`: PASS
- full `go test -count=1 ./...` and full `go test -race -count=1 ./...`: PASS
  (all 4 packages)
- Windows blind test compile amd64 + arm64 (`-exec /usr/bin/true`): PASS,
  0 FAIL
- `python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'`:
  **210 tests, OK** (matches expected count)
- `python3 scripts/acceptance/run_automated.py --suite all`: fresh manifest
  `.temp/acceptance/20260720T061455Z/manifest.json`, status **pass, 16/16**
- Producer manifest `.temp/acceptance/20260720T060457Z/manifest.json`
  independently confirmed: status pass, 16 commands
- Temporary reviewer aliasing probe (provider/transport/caller mutation):
  PASS under `-race`; probe file removed; `git status` clean of reviewer
  artifacts before verdict

## Open manual scope (intentionally NOT claimed)

Checklist item "Scan signed Windows disk logs memory artifacts and crashes for
leakage" remains open. Signed MSIX, native DPAPI/NTFS/ACL, real crypto/
provider/decoder, traffic, disk/log/memory/crash/swap/backup forensics,
physical audio/hardware and real macOS–Windows interop remain `not-run` in
EPIC-260714-th54l3. Deterministic fixtures and cross-builds were not
reinterpreted as manual evidence.

## Informational notes (no severity, no rework required)

- Cross-process index serialization is in-process only (see item 7); security
  fail-closed is preserved by cross-process-safe revocation markers and
  per-read hashing/AEAD. Should be kept in mind when the app ever gains a
  second process sharing the cache root.
- Grant expiry or explicit grant revoke observed mid-playback permanently
  bricks that exact route (marker binds object/epoch/generation/manifest/ETag),
  by design per the "never resurrects a revoked route" invariant; re-shares
  produce new routes and rotation re-grants are proven to reopen.
