# Independent re-review verdict — TASK-260712-tcwn44

Commit reviewed: `8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af`
(`fix(e2ee): preserve concurrent playback revocations`), diffed against
`b509f85034a91fcbcc756d236fe05979085825d9`. Reviewer: independent
security/correctness/lifecycle/realtime-audio re-review per
`independent-review-brief.md`. Worktree not modified; code files at branch
HEAD (`4bb3849`) are byte-identical to the reviewed commit (only board files
differ).

## Verdict

**ACCEPTED**

No open Critical, High or Medium finding. M1 and both Low findings from the
prior REJECTED verdict (commit `dac4286`) are closed by this commit without
introducing a new Critical/High/Medium issue. One narrow residual Low and four
informational notes are recorded below for future work; none blocks acceptance
under the review contract.

## Prior findings — closure verification

### M1 (Medium) — CLOSED: durable tombstone loss / index races across concurrent cache instances

- `MacStreamChunkCache` now owns a process-wide lock
  (`MacStreamTrackCache.swift:429`, `withProcessLock` `:718-724`). Every index
  read/mutation path — init (`:458-459`), cache hit (`:537-559`), post-fetch
  insert (`:594-613`), `setPinned` (`:618-637`), `invalidate` (`:641-648`),
  `tombstone` (`:652-660`), `stats` (`:664`) — runs read-merge-write under it.
- `synchronizeLocked()` (`:729-778`) re-reads the durable index and merges:
  tombstones are a **monotonic union** (`tombstones.formUnion`, `:747`),
  entries survive only if the chunk file still exists with exact size, valid
  key/variant hex, no symlink, and no tombstone — this also prevents a stale
  instance resurrecting entries whose files another instance deleted.
  `persistLocked()` (`:780-789`) synchronizes before writing, so a hit-path
  persist can no longer erase another instance's tombstone.
- The exact prior failure scenario is regression-tested:
  `concurrentCacheHitCannotEraseAnotherVariantsDurableTombstone`
  (`MacProtectedMediaPlaybackTests.swift:520-547`) — two live prepared
  playbacks on one root, revoke B, drive a cache hit on A, restart B, assert
  readChunk still fails with **zero** new range fetches. I confirmed the test
  targets the real bug shape: under the old code A's persist-on-hit would have
  dropped tombstone(B) and B's restart would have refetched successfully.
- Temp-name collisions (spurious `cache_unavailable` freeze on a healthy hit)
  are fixed by unique per-write names
  (`destination + ".\(UUID().uuidString).part"`, `:791-794`); leftover `.part`
  files are still swept at init (`:503-505`).
- TOCTOU between fetch and insert is handled: the tombstone check is re-run
  under the lock after the awaited fetch (`:596-598`).

### L1 (Low) — CLOSED (one narrow residual, see R1): rotation/expiry no longer permanently tombstone

- `readChunk` failure mapping (`MacProtectedMediaPlayback.swift:316-325`):
  `targetChanged`/`expired` now `invalidate` (purge, retryable);
  only `revoked`/`blocked` revoke authorization and write a durable tombstone.
- `prepare()` purge permanence likewise restricted to `revoked`/`blocked`
  (`:465-467`).
- Legitimate history re-grant after membership rotation is now proven:
  `membershipChangeAndExplicitRevocationPersistAsTombstones`
  (`MacProtectedMediaPlaybackTests.swift:487-499`) rotates the group, then
  re-prepares with a fresh bounded history grant at the new revision and
  successfully reads plaintext. Explicit revocation + restart still fails
  closed with no refetch (`:501-517`).

### L2 (Low) — CLOSED: player now retains the prepared playback

- `makeCandidatePlayer` calls `player.retainProtectedLifetime(self)`
  (`MacProtectedMediaPlayback.swift:354-363`;
  `MacStreamTrackPlayer.swift:150,206-208`), so dropping the caller's wrapper
  can no longer revoke a live stream. No retain cycle: the prepared object
  never references the player, and all player timers capture `self` weakly
  (`MacStreamTrackPlayer.swift:193,261,265`), so player release still tears
  down the reader/lease via `deinit`. Tested by dropping the wrapper before
  `load` (`MacProtectedMediaPlaybackTests.swift:554-560`) with decode still
  succeeding. ADR documents the lifetime contract.

### I1 (Info) — CLOSED: ADR now says "Nine serialized Swift scenarios"; the suite has exactly 9 tests.

### I2 (Info) — unchanged by design: plaintext chunks remain ordinary bounded `Data` (never on disk, never logged), not zeroized post-decoder; consistent with the AC's "where practical" caveat until provider/decoder selection.

## New findings

### R1 (Low, residual of L1): transport-asserted `expired` mid-fetch still writes a permanent tombstone

`MacProtectedMediaRangeAdapter.fetchRange` maps `.expired` (alongside
`.revoked`/`.blocked`) to `authorization.revoke()` + `frozen(code: "revoked")`
(`MacProtectedMediaPlayback.swift:248-253`), and the cache fetch path
tombstones on that code (`MacStreamTrackCache.swift:586`). Failure scenario:
local clock says the route is live (revalidate passes `route.expiresAtMS`)
but the server reports expiry mid-range-fetch → durable permanent tombstone
on HMAC(identity, etag) → a later legitimately re-granted history playback of
the same variant freezes `revoked` at first chunk read until the cache root
is wiped. This is the same over-blocking shape L1 described, through a
narrower, server-triggered path, and it contradicts the reworked ADR sentence
"Expiry … invalidate local bytes without permanently blocking a later
authorized retry." Fail-closed direction, requires a server-side expiry signal
racing local clocks — Low. Recommend mapping transport `.expired` to
`etag_changed`-style invalidation (or a distinct typed code) in the next
producer pass; may ride along with runtime wiring.

### N1 (Info): the coordination lock is one global static across all cache roots and is held across file I/O

Hit path holds it through a full chunk read (≤1 MiB) + SHA-256 + full index
rewrite (`MacStreamTrackCache.swift:537-559`), blocking a cooperative-pool
thread and serializing all concurrent playbacks (track + N overlay clips)
in-process. Correct, but revisit granularity (per-root lock, or one shared
actor per root) before runtime wiring with real concurrent overlay load.

### N2 (Info): coordination is in-process only

`NSRecursiveLock` does not serialize two app processes sharing a cacheRoot.
This matches the already-deferred "cross-process single-instance ownership"
gate (tracked for `MacE2EEKeyStateRepository`); the ciphertext cache root
remains explicitly included in that gate, as the prior verdict noted.

### N3 (Info): `pinned` state is per-instance and not durable

`CodingKeys` exclude `pinned` (`MacStreamTrackCache.swift:419`), and the merge
preserves only local pins (`:773`), so one instance's eviction pass cannot see
another instance's pins and may evict its pinned next-chunk. Self-healing
(refetch on miss, hash-verified); memory/disk bounds unaffected.

### N4 (Info): corrupt-index asymmetry — mid-run `synchronizeLocked` fails closed (`cache_unavailable`) while restart init repairs by resetting the index **including tombstones** (`:467-480`)

So external corruption of `index-v1.json` can drop durable tombstones across
restart. Writes are atomic + fsync'd, so this needs external interference;
the durable tombstone is defense-in-depth behind live authorization
revalidation. Pre-existing init behavior, unchanged by this commit.

## Test evidence (re-run by reviewer, 2026-07-20)

- `swift test --filter MacProtectedMediaPlaybackTests` — 9/9 pass (0.27 s).
- `swift test --filter MacStreamTrackPlayerTests` — 6/6 pass (0.52 s).
- Full `swift test --package-path node-app` — 340 tests / 55 suites, all pass
  (+1 vs prior review: the new concurrent-tombstone regression; includes
  `MacOverlayMediaClipMixerTests` mixer regressions).
- `python3 -m unittest scripts.acceptance.test_macos_protected_media_playback
  scripts.acceptance.test_stream_performance_review` — 11/11 pass.
- All 9 artifact pins in `acceptance/phase3/macos-protected-media-playback-v1.json`
  (now including the new `ciphertext-cache` artifact) and all pins in
  `acceptance/phase2/stream-performance-review-v1.json` recomputed against the
  commit's blobs — exact match; the phase2 re-pin of the two touched files is
  honest and the regression cascade holds.
- Board resource copies of the packet, ADR and vectors are byte-identical to
  the repo files at the commit.
- Validator hardening verified: `validate_macos_protected_media_playback.py`
  now requires the coordination tokens (`processLock`, `synchronizeLocked()`,
  `tombstones.formUnion`, `UUID().uuidString`), the new test name, and the new
  16th invariant
  `concurrent-cache-index-writes-preserve-monotonic-revocation-tombstones`.

## Acceptance criteria / checklist status

- Fixture-level AC met: incremental playback of shared Mac/Windows fixtures
  without full download; tamper/replay/wrong-target/revoked-grant/cache-
  substitution/delete/restart fail safely; no unauthenticated bytes ever reach
  a decoder (`decryptCount == 0` on hash mismatch; decoder only sees
  post-AEAD output); downgrade forbidden; disk scans clean of plaintext and
  key canaries; Phase 2 player/mixer gates green.
- Production-dark posture intact: public constructor still requires a
  production-approved provider; fixture opener is internal-only; validator
  greps forbid crypto/codec/URLSession primitives in the playback file; no
  logging added.

## Production-blocking deferred external/manual gates (NOT defects of this commit)

Unchanged from the prior verdict: provider/suite/container/decoder selection
and external security review (EPC-001/002/004/005, `TASK-260712-1ulshp`);
runtime/NodeApp composition and capability advertisement (intentionally dark);
signed+notarized package with physical Keychain/crash/restart, memory-artifact
and crash-dump leakage scans (checklist "Scan macOS disk logs memory artifacts
and crashes" is satisfied at fixture level for disk only; memory/crash deferred
to EPIC-260714-th54l3); real cryptographic Mac/Windows interop vectors and real
codec/container hardware playback; cross-process single-instance ownership of
`MacE2EEKeyStateRepository` and of the ciphertext cache root (N2) before
runtime wiring.

## Routing

`done`. R1 (Low) and N1–N4 are recorded for the runtime-wiring/provider-selection
follow-ups; none requires another producer cycle on this task.
