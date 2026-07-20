## Status
done

## Assigned To
[reviewer] reviewer (claude)

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T02:44:46Z

## Blocked By
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6

## Checklist
- [x] Verify manifest envelope and each chunk before decode
- [x] Implement authenticated ranges seeks and ciphertext-only durable cache
- [x] Purge revoked deleted expired corrupt and wrong-target state
- [x] Meet Phase 2 player gates and existing mixer semantics
- [ ] Scan macOS disk logs memory artifacts and crashes for leakage
- [ ] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
spawn queued: [reviewer] reviewer (claude) (run=RUN-260720-2f341a, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260720-2f341a)
Review of exact commit dac4286f9459359ad477f89e43944d8635905467: REJECTED — see outcome resource TASK-260712-tcwn44_review-verdict.md. Tests all green (focused 8/8, player 6/6, full suite 339/339, python acceptance 11/11); all 8 packet hash pins match the commit; production-dark posture, binding, auth ordering, dynamic revocation and player injection verified clean. One Medium defect blocks acceptance: prepare()/purge() create a fresh MacStreamChunkCache per call on the shared cacheRoot while persist() rewrites the whole index last-writer-wins on every cache hit — with two live playbacks (normal mixer state) a revocation tombstone written by one instance is erased by the other, breaking the durable revoked-cache-restart guarantee (packet invariant + AC "delete and restart fail safely"); same root cause also yields spurious cache_unavailable freezes from fixed .part temp-name collisions. Rework: serialize/merge durable index state across instances (single shared cache actor per root, or file-locked read-merge-write with tombstones as monotonic union) + concurrent-playback tombstone regression test. Low findings L1 (rotation/expiry permanently tombstone the variant, bricking later legitimately re-granted history playback with a misleading revoked code) and L2 (undocumented PreparedPlayback lifetime coupling — deinit freezes an active player) may be fixed alongside or explicitly dispositioned. Deferred external/manual gates (provider selection EPC-001/002/004/005, runtime wiring, signed/notarized memory+crash scans, real interop vectors, cross-process key-state ownership) are NOT defects of this commit.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260720-2f341a, pid=90779, exit=0)
Reviewer M1 rework: cache actors now serialize and read-merge-write one durable index with monotonic tombstones and unique temp files; concurrent A-hit/B-revoke/restart regression added. L1 changed rotation/expiry to invalidate, explicit revoke only tombstones. L2 player retains prepared lifetime owner. Re-review required on new exact SHA.
spawn queued: [reviewer] reviewer (claude) (run=RUN-260720-cf2797, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260720-cf2797)
Re-review of exact rework commit 8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af: ACCEPTED — see outcome resource TASK-260712-tcwn44_review-verdict-8c26762.md. M1 closed: process-wide lock + read-merge-write synchronizeLocked/persistLocked with tombstones as monotonic union, entry resurrection blocked by file-existence validation, UUID temp names end .part collisions; exact prior failure shape covered by new concurrentCacheHitCannotEraseAnotherVariantsDurableTombstone regression. L1 closed: targetChanged/expired now invalidate (retryable), only revoked/blocked tombstone; legitimate history re-grant after rotation proven by test. L2 closed: player retains prepared owner, no retain cycle (weak timer captures), ADR documents contract. I1 fixed (Nine scenarios = 9 tests). Tests re-run green: playback 9/9, player 6/6, full suite 340/55, python acceptance 11/11; all phase3 (9 pins incl. new ciphertext-cache artifact) and phase2 hash pins match commit blobs; board resource copies byte-identical; validator hardened with coordination tokens + 16th invariant. New findings, none blocking: R1 Low residual — transport-asserted expired mid-fetch still maps to code revoked and durably tombstones (MacProtectedMediaPlayback.swift:248-253 -> MacStreamTrackCache.swift:586), bricking later re-granted history under server/local clock skew, contradicts ADR expiry sentence; fix with runtime wiring. Info: global single lock held across file I/O serializes all playbacks in-process (revisit granularity before wiring); coordination is in-process only (cross-process cache-root ownership stays in the deferred gate); pinned state per-instance so cross-instance eviction may drop another playback pins (self-healing); corrupt-index restart repair resets tombstones (external-interference only, defense-in-depth). Deferred external/manual gates unchanged (provider selection EPC-001/002/004/005, runtime wiring, signed/notarized memory+crash scans EPIC-260714-th54l3, real interop vectors, cross-process ownership). Checklist items 5 and 6 remain open by design as deferred gates. Routed done.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260720-cf2797, pid=8383, exit=0)

## Precondition Resources
- [independent-review-brief.md](file://TASK-260712-tcwn44/independent-review-brief.md) — Exact-SHA independent re-review of cache race and lifecycle fixes

## Outcome Resources
- [macos-protected-media-playback-v1.json](file://TASK-260712-tcwn44/macos-protected-media-playback-v1.json) — Fail-closed automated acceptance packet; production remains dark
- [p3-macos-protected-media-playback-v1.md](file://TASK-260712-tcwn44/p3-macos-protected-media-playback-v1.md) — Architecture decision and deferred manual evidence
- [macos-protected-media-playback-v1-vectors.json](file://TASK-260712-tcwn44/macos-protected-media-playback-v1-vectors.json) — Shared Mac/Windows audit-fixture range vectors
- [TASK-260712-tcwn44_spawn-log_-reviewer--reviewer--claude-.log](file://TASK-260712-tcwn44/TASK-260712-tcwn44_spawn-log_-reviewer--reviewer--claude-.log) — System spawn log captured by task-board
- [TASK-260712-tcwn44_review-verdict.md](file://TASK-260712-tcwn44/TASK-260712-tcwn44_review-verdict.md) — Independent review verdict for commit dac4286: REJECTED (1 Medium: durable tombstone loss / index races across concurrent cache instances; 2 Low; 2 Info). All required tests green; packet hashes verified.
- [TASK-260712-tcwn44_review-verdict-8c26762.md](file://TASK-260712-tcwn44/TASK-260712-tcwn44_review-verdict-8c26762.md) — Independent re-review verdict for rework commit 8c26762: ACCEPTED (M1/L1/L2 closed; 1 residual Low R1 + 4 info notes recorded for runtime wiring; all tests green, all hash pins match)
