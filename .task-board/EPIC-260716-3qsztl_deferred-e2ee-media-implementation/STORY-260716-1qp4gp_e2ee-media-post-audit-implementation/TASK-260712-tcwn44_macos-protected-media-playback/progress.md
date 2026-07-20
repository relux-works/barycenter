## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T02:36:29Z

## Blocked By
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6

## Checklist
- [ ] Verify manifest envelope and each chunk before decode
- [ ] Implement authenticated ranges seeks and ciphertext-only durable cache
- [ ] Purge revoked deleted expired corrupt and wrong-target state
- [ ] Meet Phase 2 player gates and existing mixer semantics
- [ ] Scan macOS disk logs memory artifacts and crashes for leakage
- [ ] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.
- [ ] Implementation matches AC
- [ ] Solution fits project architecture
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

## Precondition Resources
- [independent-review-brief.md](file://TASK-260712-tcwn44/independent-review-brief.md) — Exact-SHA independent security and realtime review instructions

## Outcome Resources
- [macos-protected-media-playback-v1.json](file://TASK-260712-tcwn44/macos-protected-media-playback-v1.json) — Fail-closed automated acceptance packet; production remains dark
- [p3-macos-protected-media-playback-v1.md](file://TASK-260712-tcwn44/p3-macos-protected-media-playback-v1.md) — Architecture decision and deferred manual evidence
- [macos-protected-media-playback-v1-vectors.json](file://TASK-260712-tcwn44/macos-protected-media-playback-v1-vectors.json) — Shared Mac/Windows audit-fixture range vectors
- [TASK-260712-tcwn44_spawn-log_-reviewer--reviewer--claude-.log](file://TASK-260712-tcwn44/TASK-260712-tcwn44_spawn-log_-reviewer--reviewer--claude-.log) — System spawn log captured by task-board
- [TASK-260712-tcwn44_review-verdict.md](file://TASK-260712-tcwn44/TASK-260712-tcwn44_review-verdict.md) — Independent review verdict for commit dac4286: REJECTED (1 Medium: durable tombstone loss / index races across concurrent cache instances; 2 Low; 2 Info). All required tests green; packet hashes verified.
