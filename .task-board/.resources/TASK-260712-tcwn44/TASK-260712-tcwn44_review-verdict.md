# Independent review verdict — TASK-260712-tcwn44

Commit reviewed: `dac4286f9459359ad477f89e43944d8635905467`
(`feat(e2ee): add macOS protected media playback foundation`), diffed against
first parent `9f813a9`. Reviewer: independent security/correctness/lifecycle/
realtime-audio review per `independent-review-brief.md`. Worktree not modified.

## Verdict

**REJECTED**

One Medium finding (M1) is open in this producer commit. Everything else in the
commit verified clean; the rework scope is narrow and well-bounded (durable
cache-index coordination across concurrently live `MacStreamChunkCache`
instances plus one regression test).

## Test evidence (re-run by reviewer, 2026-07-20)

- `swift test --filter MacProtectedMediaPlaybackTests` — 8/8 pass (0.17 s).
- `swift test --filter MacStreamTrackPlayerTests` — 6/6 pass (0.58 s).
- Full `swift test --package-path node-app` — 339 tests / 55 suites, all pass
  (includes `MacOverlayMediaClipMixerTests` mixer regressions).
- `python3 -m unittest scripts.acceptance.test_macos_protected_media_playback
  scripts.acceptance.test_stream_performance_review` — 11/11 pass.
- All 8 artifact SHA-256 pins in
  `acceptance/phase3/macos-protected-media-playback-v1.json` recomputed against
  the commit's blobs — exact match. Phase 2 `stream-performance-review-v1.json`
  re-pin of `MacStreamTrackPlayer.swift`
  (`0cbe7a19…f849a7`) matches the commit blob; regression cascade is honest.
- `protocol/macos-protected-media-playback-v1-vectors.json` and the board
  resource copy are byte-identical; chunk digests match the fixture data
  (asserted in `sharedMacWindowsFixtureFreezesExactAuthenticatedRanges`).

## Findings

### M1 (Medium, correctness/security-hardening): durable tombstones are lost and index writes race when multiple `MacStreamChunkCache` instances share `cacheRoot`

Evidence:
- `MacProtectedMediaPlayback.swift:505-513` — `prepare()` constructs a **new**
  `MacStreamChunkCache` actor per call on the shared `cacheRoot`;
  `purge()` (`:620-636`) constructs yet another. Concurrently live prepared
  playbacks (track + overlay clips — the product's normal mixer state, and this
  task's AC explicitly preserves mixer semantics) therefore each hold an
  independent in-memory copy of the shared `index-v1.json`.
- `MacStreamTrackCache.swift:695-700` — `persist()` rewrites the **entire**
  index (entries + tombstones) from that instance's in-memory state,
  last-writer-wins, with no cross-instance lock or read-merge.
- `MacStreamTrackCache.swift:537-543` — the cache-hit path calls `persist()`
  on **every** chunk read.

Concrete failure scenario (deterministic, no adversary):
1. Track A is playing (prepared playback P_A, cache instance C_A).
2. User deletes clip B → `MacProtectedMediaPreparedPlayback.revoke()`
   (`MacProtectedMediaPlayback.swift:363-367`) or readChunk revocation path
   (`:319-324`) tombstones B's variant in C_B and persists — index now
   durably records tombstone(B).
3. Track A reads its next chunk → C_A hit path persists C_A's stale in-memory
   index, which predates B's tombstone → **tombstone(B) is erased from disk**.
   Because persist-on-hit fires on every chunk, loss is near-certain whenever
   anything else is playing.
4. After restart, the "revoked-cache-restart → revoked" durable guarantee
   (packet invariant `revocation-and-membership-change-purge-and-tombstone`,
   AC "delete and restart fail safely", fail-closed vector
   `revoked-cache-restart`) no longer holds locally; a stale or misbehaving
   transport that still serves object B is not stopped by the local tombstone.
   The existing test passes only because it exercises the sequence with no
   concurrent second playback.

Same-root-cause secondary effects:
- Two live instances persisting concurrently can collide on the fixed
  `index-v1.json.part` temp name (`atomicWrite`,
  `MacStreamTrackCache.swift:702-724`, `.withoutOverwriting`) → spurious
  `cache_unavailable` failure thrown from a cache **hit** path → unnecessary
  fail-closed playback freeze of a healthy stream.
- Two concurrent playbacks of the same object can collide on a chunk
  `.part` file the same way.
- A stale instance's persist can resurrect index entries whose files were
  deleted by another instance (benign — repaired at next init — but noisy).

Impact bound (why Medium, not High): ciphertext files themselves are deleted
by the revoking instance and are not resurrected; the live revoked instance
stays revoked in memory (`MacProtectedMediaAuthorization`); replay after
restart still requires the transport to serve the revoked object and
`prepare()` to re-pass manifest/group/grant validation. The loss is of an
explicitly claimed durable defense-in-depth property plus a real spurious-
freeze race — not a plaintext/key exposure or unauthenticated decode.

Required rework (producer, `to-dev`): make durable index state safe under the
multi-instance usage `prepare()`/`purge()` create — e.g. one shared cache
actor per `cacheRoot` owned by the service, or file-locked read-merge-write
in `persist()` treating tombstones as a monotonic union, or per-variant index
shards. Add a regression test: two live prepared playbacks, revoke one, drive
a chunk read on the other, restart, assert the tombstone still blocks.

### L1 (Low): permanent tombstone over-blocks later legitimately re-authorized history playback

`readChunk` maps mid-playback `expired`/`targetChanged` (membership rotation)
to a **permanent** tombstone keyed on HMAC(identity, etag)
(`MacProtectedMediaPlayback.swift:319-324`; `MacStreamTrackCache.swift:632-639`;
no tombstone-clear API exists). After a rotation or expiry, a later playback of
the same object under a valid new history grant / re-issued route reuses the
same manifest identity+etag → first chunk read freezes with a misleading
`revoked` code despite passing `prepare()`. This contradicts the ADR's own
claim that authorized retries are not permanently blocked (that claim is made
only for missing-grant/auth failures; rotation/expiry landing in the permanent
bucket looks over-broad). Fail-closed direction, so Low — but it will brick
legitimate re-granted history playback per install until the cache root is
wiped. Recommend: tombstone only for explicit revocation/delete; use
`invalidate` for rotation/expiry, and surface a typed failure distinct from
`revoked`. Can be fixed together with M1 or explicitly accepted as product
behavior.

### L2 (Low): undocumented lifetime coupling between `MacProtectedMediaPreparedPlayback` and the player

`MacStreamCandidatePlayer` retains only the reader; `deinit { reader.close() }`
(`MacProtectedMediaPlayback.swift:369`) revokes authorization and zeroizes the
open lease if the caller drops the prepared object mid-playback → healthy
stream freezes `revoked`. Fail-closed, but the retention contract should be
documented (or the player should retain the prepared object) before runtime
wiring.

### I1 (Info): ADR miscount

`docs/analysis/p3-macos-protected-media-playback-v1.md` says "Seven serialized
Swift scenarios"; the suite has 8 tests.

### I2 (Info): plaintext chunk handling

Decrypted chunks travel as ordinary `Data` (bounded ≤ 1 MiB, never written to
disk, never logged; disk-scan assertions prove cache/key hygiene) but are not
zeroized after decoder hand-off and not in locked memory. Consistent with the
AC's "where practical" caveat while no real decoder exists; revisit at
provider/decoder selection.

## Verified clean (exact-commit evidence)

- **Binding**: `validate(route:request:group:)`
  (`MacProtectedMediaPlayback.swift:569-618`) pins contract/capability
  (downgrade forbidden), object/recipient/group/epoch/generation, exact target
  snapshot digest, `SHA256(encryptedManifest) == manifestDigest`, size bounds
  (1 MiB manifest/envelope, 64 KiB signature), identifier/digest hygiene,
  `svm1.protected.` namespace, per-variant/global cache bounds. Future epoch →
  `forkedEpoch`; same-epoch digest mismatch → `targetChanged`; historical epoch
  without grant → `missingGrant`.
- **Authentication ordering**: cache verifies chunk SHA-256 for both cached
  and freshly fetched bytes before any provider call
  (`MacStreamTrackCache.swift:537-538,566-573`, bounded 2-attempt retry);
  provider AEAD (`authenticateAndDecrypt`) must succeed before bytes reach a
  decoder; tamper test proves `decryptCount == 0` on hash mismatch and cache
  purge on record-auth failure.
- **Dynamic re-checks**: revalidate closure runs before and after fetch and
  decrypt per record — live route expiry vs clock, live group
  revision/epoch/target reload, live grant reload with expiry/range/group
  check (`MacProtectedMediaPlayback.swift:517-550`); grant metadata survives
  lease `destroy()` (immutable structs; only secrets zeroize —
  `MacE2EEKeyState.swift:103-213`); membership rotation and grant revocation
  fail closed post-prepare (tests prove).
- **Canonical ranges**: adapter permits only exact manifest chunk boundaries
  with pinned path+etag (`MacProtectedMediaPlayback.swift:227-236`); etag
  drift invalidates; pre-existing HTTP fetcher enforces 206 + exact
  Content-Range + ETag echo + no redirects + 1 MiB network bound.
- **Ciphertext-only durable cache**: restart test replays from cache without
  a second range fetch and re-authenticates (`decryptCount` increments); disk
  scans assert no plaintext and no group-key/open-lease canaries on disk;
  0o700/0o600 permissions, fsync'd atomic writes.
- **Player injection**: 10-line diff; injected reader must match the loaded
  manifest exactly or load fails; generation/seek/deadline/PCM-ring/receipt
  paths untouched; decoder receives only the authenticated reader (test);
  6 player regression tests + full 339-test suite green.
- **Production-dark honesty**: public constructor requires a
  production-approved provider; fixture path is internal-only; validator greps
  forbid crypto/codec/URLSession primitives in the playback file; packet
  decision flags all false; manual gates explicitly `not-run`; no logging in
  new source.

## Production-blocking deferred external/manual gates (NOT defects of this commit)

- Provider/suite/container/decoder selection and external security review
  (EPC-001, EPC-002, EPC-004, EPC-005; `TASK-260712-1ulshp`).
- Runtime/NodeApp composition and capability advertisement (intentionally dark).
- Signed+notarized package, physical Keychain/crash/restart, memory-artifact
  and crash-dump leakage scans (`manualEvidence`: not-run; the AC's
  "scan macOS disk logs memory artifacts and crashes" is satisfied at
  fixture level only — disk scans automated, memory/crash scans deferred to
  EPIC-260714-th54l3).
- Real cryptographic Mac/Windows interop vectors and real codec/container,
  hardware playback/seek/interrupt/audible checks.
- Cross-process single-instance ownership of `MacE2EEKeyStateRepository`
  (checklist item is scoped "before runtime wiring"; an in-process static
  lock exists; the cross-process story remains open and now also applies to
  the ciphertext cache root per M1).

## Routing

`to-dev` for M1 rework (with L1/L2 disposition at producer's discretion),
then another reviewer cycle per the verdict branches.
