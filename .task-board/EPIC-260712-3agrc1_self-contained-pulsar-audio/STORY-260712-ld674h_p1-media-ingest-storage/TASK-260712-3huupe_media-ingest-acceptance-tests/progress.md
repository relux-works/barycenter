## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T06:21:51Z

## Blocked By
- TASK-260712-2af2dp
- TASK-260712-1bnos4
- TASK-260712-gj0cko
- TASK-260712-12ojcb

## Blocks
- TASK-260712-jolzhh
- TASK-260712-wy05n6
- TASK-260712-1xkn75

## Checklist
- [ ] Add accepted-format and corrupt-input fixtures for the common ingest path
- [ ] Cover idempotency, quota, delete, expiry and tenant ACL scenarios end to end
- [ ] Map each story acceptance criterion to automated evidence or an explicit fixture
- [ ] Exercise polyglot, decompression-bomb, network-protocol and worker-crash fixtures
- [ ] Exercise concurrent resume, stale worker, delete and cleanup-restart races

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge 9f2aea8e5b9200d1e4077a5576dde18f8051bba5. This task is limited to deterministic automated fixtures and unit/integration acceptance evidence; manual real-app and real-hardware verification remains deferred to EPIC-260714-th54l3.
Automated acceptance delta implemented inline: common-service live matrix now covers WAV/PCM, MP3, M4A/AAC, M4A/ALAC, ADTS AAC, OGG/Opus, OGG/Vorbis and FLAC; adversarial common-service failures cover corrupt, truncated, polyglot, protocol-shaped, declared-length, compressed-duration, probe/worker timeout-crash and canonical-output classes; a synthetic HTTP test joins resumable upload, target ACL, non-disclosure, delete and cleanup. Cleanup crash retry now reopens SQLite and the lifecycle service. Local coordinator vet/test/race, focused race stress x20 and exact pinned predecessor suite are green. Local ffmpeg is absent, so live codec and 181-second AAC fixtures are pending authoritative hosted coordinator CI. No real-app or hardware result is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-acceptance-evidence.md](file://TASK-260712-3huupe/p1-media-ingest-acceptance-evidence.md) — Story acceptance criteria mapped to deterministic tests and explicit manual deferrals
