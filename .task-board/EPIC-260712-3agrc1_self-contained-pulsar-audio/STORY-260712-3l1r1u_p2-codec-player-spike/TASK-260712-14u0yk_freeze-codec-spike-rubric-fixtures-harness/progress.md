## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T16:58:54Z

## Blocked By
- TASK-260712-1xik11

## Blocks
- TASK-260712-dqdoqj
- TASK-260712-1vdlkw
- TASK-260712-3vkcki
- TASK-260712-298tyq
- TASK-260712-1canzv
- TASK-260712-350u8d

## Checklist
- [x] Map every section 20.2 proof item and relevant section 20.5 gate to one artifact-producing check
- [x] Assemble the shared MP3 AAC Opus fixture corpus including long, seek-heavy, and corrupt cases
- [x] Define the shared HTTP range harness and bounded-memory measurement method used by every candidate
- [x] Freeze the scheduled-start, pause, seek, resume, and RSS evidence format
- [x] Record the concrete candidate shortlist for pure-Go, Media Foundation, and bundled paths
- [x] Freeze exact p95 sample counts and 5 s start, 3 s seek, 100 ms skew, 200 MiB RSS hard gates
- [x] Include all three platform pairings, VBR seek and hostile decoder fixtures

## Notes
Strict inline execution started from synchronized main a6f3963 after accepted Air story tracking merge. Freezing a candidate-neutral section 20.2/B1/20.5 fixture corpus, faulting range harness, sample counts, hard gates and artifact schema before any decoder probe. Real hardware/platform-pairing execution remains manual-test evidence; this task owns reproducible fixtures, deterministic harnesses and repository validation.
Candidate-neutral codec spike foundation complete locally. Frozen contract maps every section 20.2/B1/20.5 proof to artifacts; recipes cover MP3, AAC-LC and Opus in MP3, M4A fast-start, ADTS and Ogg at 12 s, 1 h and 2 h with CBR/VBR, seek markers and eight hostile mutations. Exact FFmpeg 8.1.2 generation emits one content-addressed fixture lock; no executable download is allowed. Authenticated target-bound RFC range harness covers normal/no-range/slow/reset/truncate/corrupt/ETag/revoke using bounded streaming. Evaluator enforces all three pairings, exact 3+30 timing samples, 5 s/3 s/100 ms/200 MiB plus duration-independence gates, complete artifacts and zero failures; synthetic results cannot become final claims. Codec tests passed 10/10 repetitions and repository acceptance passed 12/12. No decoder, license, physical performance or hardware result is claimed.
Accepted after hosted CI run 29434417154 passed all four jobs (node-core 1m18s, pulsar-win 1m45s, coordinator 2m32s, packaged probe 2m50s). Engineering commit aba592e landed through PR #102 at merge 8f91187d3ab9bb62fe31a00407a1a7058df27d9b.

## Precondition Resources
(none)

## Outcome Resources
- [p2-codec-spike-rubric-fixtures-harness.md](file://TASK-260712-14u0yk/p2-codec-spike-rubric-fixtures-harness.md) — Frozen corpus, range/fault, metric, candidate and evidence handoff
- [codec-spike-rubric-v1.json](file://TASK-260712-14u0yk/codec-spike-rubric-v1.json) — Machine-readable section 20.2, B1 and 20.5 proof contract
- [repository-acceptance-manifest.json](file://TASK-260712-14u0yk/repository-acceptance-manifest.json) — Passing 12-command repository acceptance manifest
