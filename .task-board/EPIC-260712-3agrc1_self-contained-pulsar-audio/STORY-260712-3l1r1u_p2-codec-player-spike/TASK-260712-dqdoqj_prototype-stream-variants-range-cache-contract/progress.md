## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T17:32:57Z

## Blocked By
- TASK-260712-14u0yk

## Blocks
- TASK-260712-3vkcki
- TASK-260712-298tyq
- TASK-260712-1canzv
- TASK-260712-ibuaxj
- TASK-260712-2eympi
- TASK-260712-350u8d

## Checklist
- [x] Compare original-upload versus canonical-variant storage for MP3 AAC and Opus fixtures
- [x] Prototype stream_variants rows plus variant metadata and integrity fields
- [x] Define the byte-range HTTP contract, auth behavior, and restart-safe cache semantics
- [x] Name cache ceilings and eviction rules that stay bounded on two-hour media
- [x] Publish fixture and contract details that every decoder prototype can consume unchanged
- [x] Specify RFC range, conditional, target ACL and non-disclosing failure behavior
- [x] Specify chunk integrity, VBR seek mapping and delete or disable cache invalidation

## Notes
Strict inline execution started from synchronized main de13463 after TASK-260712-14u0yk tracking PR #103 passed hosted run 29434751854 and merged. Implementing candidate-neutral stream variants, authenticated RFC range semantics, bounded restart-safe private cache, integrity/VBR mapping and revoke/delete invalidation; no real hardware claim.
Accepted on exact engineering code head 733b5c6. Frozen p2-stream-variants-range-cache.v1 materializes constrained SQLite stream_variants rows, content-addressed whole/chunk SHA-256 manifests, chunk-aligned <=10 s seek maps, authenticated single-range/ETag/conditional semantics with uniform ACL denial, and an HMAC-namespaced 512 MiB/64 MiB bounded cache with atomic LRU, 128 MiB pin budget, restart reconciliation, path/symlink defense and durable no-refill invalidation. Codec suite 8/8 repeatedly; exact local repository gate 12/12 at .temp/acceptance/task-260712-dqdoqj-exact/manifest.json. Hosted run 29436698927 passed node-core 1m11s, pulsar-win 1m58s, coordinator 2m10s and packaged probe 2m38s. PR #104 merged as f6dd5c2. No decoder, license, audible, Store, production ingest or real-hardware claim.

## Precondition Resources
(none)

## Outcome Resources
- [p2-codec-player-spike-components.puml](file://TASK-260712-dqdoqj/p2-codec-player-spike-components.puml) — Component seam reference for stream variants and range contract work
- [stream-contract-v1.json](file://TASK-260712-dqdoqj/stream-contract-v1.json) — Frozen candidate-neutral stream variants, range and cache contract
- [p2-stream-variants-range-cache-contract.md](file://TASK-260712-dqdoqj/p2-stream-variants-range-cache-contract.md) — Reviewed implementation and downstream decoder handoff
- [repository-acceptance-manifest.json](file://TASK-260712-dqdoqj/repository-acceptance-manifest.json) — Exact local repository acceptance manifest, 12 of 12 automated commands passed
